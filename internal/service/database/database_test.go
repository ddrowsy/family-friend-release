package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenCreatesSchemaAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "family-friend.db")

	db := openTestDB(t, ctx, path)
	assertMigrationVersion(t, db, migrationVersion)
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	db = openTestDB(t, ctx, path)
	assertMigrationVersion(t, db, migrationVersion)
}

func TestSchemaSupportsCustomerProfileAndDeviceRelationships(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx, filepath.Join(t.TempDir(), "relationships.db"))

	exec(t, db, "INSERT INTO customers(id, created_at) VALUES ('customer-1', CURRENT_TIMESTAMP), ('customer-2', CURRENT_TIMESTAMP)")
	exec(t, db, `INSERT INTO profiles(customer_id, id, name, created_at, updated_at) VALUES
		('customer-1', 'harry', 'Harry', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('customer-1', 'james', 'James', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('customer-2', 'harry', 'Other Harry', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	exec(t, db, `INSERT INTO devices(customer_id, id, created_at) VALUES
		('customer-1', 'laptop', CURRENT_TIMESTAMP),
		('customer-1', 'desktop', CURRENT_TIMESTAMP)`)
	exec(t, db, `INSERT INTO device_profiles(customer_id, device_id, profile_id, created_at) VALUES
		('customer-1', 'laptop', 'harry', CURRENT_TIMESTAMP),
		('customer-1', 'desktop', 'harry', CURRENT_TIMESTAMP),
		('customer-1', 'laptop', 'james', CURRENT_TIMESTAMP)`)

	assertCount(t, db, "SELECT COUNT(*) FROM device_profiles WHERE customer_id = 'customer-1' AND profile_id = 'harry'", 2)
	assertCount(t, db, "SELECT COUNT(*) FROM device_profiles WHERE customer_id = 'customer-1' AND device_id = 'laptop'", 2)
	assertCount(t, db, "SELECT COUNT(*) FROM profiles WHERE id = 'harry'", 2)
}

func TestSchemaEnforcesDeviceProfileForeignKeysAndCustomerBoundary(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx, filepath.Join(t.TempDir(), "foreign-keys.db"))

	exec(t, db, "INSERT INTO customers(id, created_at) VALUES ('customer-1', CURRENT_TIMESTAMP), ('customer-2', CURRENT_TIMESTAMP)")
	exec(t, db, "INSERT INTO profiles(customer_id, id, name, created_at, updated_at) VALUES ('customer-1', 'harry', 'Harry', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)")
	exec(t, db, "INSERT INTO devices(customer_id, id, created_at) VALUES ('customer-2', 'laptop', CURRENT_TIMESTAMP)")

	assertExecFails(t, db, "INSERT INTO device_profiles(customer_id, device_id, profile_id, created_at) VALUES ('customer-1', 'missing', 'harry', CURRENT_TIMESTAMP)")
	assertExecFails(t, db, "INSERT INTO device_profiles(customer_id, device_id, profile_id, created_at) VALUES ('customer-1', 'laptop', 'harry', CURRENT_TIMESTAMP)")
	assertExecFails(t, db, "INSERT INTO device_profiles(customer_id, device_id, profile_id, created_at) VALUES ('customer-2', 'laptop', 'harry', CURRENT_TIMESTAMP)")
}

func TestMigrationRemovesTemporaryBrowserApprovalTable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "migration.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	if err := applyMigration(ctx, db, 1, migrations[0]); err != nil {
		t.Fatalf("apply migration 1: %v", err)
	}
	assertCount(
		t,
		db,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'temporary_browser_approvals'",
		1,
	)
	if err := db.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	db = openTestDB(t, ctx, path)
	assertMigrationVersion(t, db, migrationVersion)
	assertCount(
		t,
		db,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'temporary_browser_approvals'",
		0,
	)
}

func openTestDB(t *testing.T, ctx context.Context, path string) *sql.DB {
	t.Helper()
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func exec(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	if _, err := db.Exec(statement); err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
}

func assertExecFails(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	if _, err := db.Exec(statement); err == nil {
		t.Fatalf("Exec succeeded, want constraint error: %s", statement)
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("QueryRow returned error: %v", err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}

func assertMigrationVersion(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&got); err != nil {
		t.Fatalf("reading migration version: %v", err)
	}
	if got != want {
		t.Fatalf("migration version = %d, want %d", got, want)
	}
}

func TestSchemaEnforcesPairingSessionCustomerAndCodeUniqueness(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx, filepath.Join(t.TempDir(), "pairing.db"))

	exec(
		t,
		db,
		"INSERT INTO customers(id, created_at) VALUES "+
			"('customer-1', CURRENT_TIMESTAMP), "+
			"('customer-2', CURRENT_TIMESTAMP)",
	)
	exec(
		t,
		db,
		"INSERT INTO pairing_sessions("+
			"id, customer_id, state, code, expires_at, created_at, updated_at"+
			") VALUES ("+
			"'session-1', 'customer-1', 'waiting', 'code-1', "+
			"'2026-09-22T14:00:35.000000000Z', "+
			"'2026-09-22T14:00:00.000000000Z', "+
			"'2026-09-22T14:00:00.000000000Z')",
	)

	assertExecFails(
		t,
		db,
		"INSERT INTO pairing_sessions("+
			"id, customer_id, state, code, expires_at, created_at, updated_at"+
			") VALUES ("+
			"'session-2', 'customer-1', 'waiting', 'code-2', "+
			"'2026-09-22T14:00:35.000000000Z', "+
			"'2026-09-22T14:00:00.000000000Z', "+
			"'2026-09-22T14:00:00.000000000Z')",
	)
	assertExecFails(
		t,
		db,
		"INSERT INTO pairing_sessions("+
			"id, customer_id, state, code, expires_at, created_at, updated_at"+
			") VALUES ("+
			"'session-3', 'customer-2', 'waiting', 'code-1', "+
			"'2026-09-22T14:00:35.000000000Z', "+
			"'2026-09-22T14:00:00.000000000Z', "+
			"'2026-09-22T14:00:00.000000000Z')",
	)
}
