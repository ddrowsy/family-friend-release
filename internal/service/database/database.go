package database

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const migrationVersion = 5

var migrations = []string{
	`CREATE TABLE customers (
		id TEXT PRIMARY KEY,
		created_at TEXT NOT NULL
	);
	CREATE TABLE profiles (
		customer_id TEXT NOT NULL,
		id TEXT NOT NULL,
		name TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (customer_id, id),
		FOREIGN KEY (customer_id) REFERENCES customers(id)
	);
	CREATE TABLE profile_configs (
		customer_id TEXT NOT NULL,
		profile_id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		policy_json BLOB NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (customer_id, profile_id),
		FOREIGN KEY (customer_id, profile_id) REFERENCES profiles(customer_id, id)
	);
	CREATE TABLE devices (
		customer_id TEXT NOT NULL,
		id TEXT NOT NULL,
		name TEXT,
		agent_version TEXT,
		created_at TEXT NOT NULL,
		last_seen_at TEXT,
		PRIMARY KEY (customer_id, id),
		FOREIGN KEY (customer_id) REFERENCES customers(id)
	);
	CREATE TABLE device_profiles (
		customer_id TEXT NOT NULL,
		device_id TEXT NOT NULL,
		profile_id TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY (customer_id, device_id, profile_id),
		FOREIGN KEY (customer_id, device_id) REFERENCES devices(customer_id, id),
		FOREIGN KEY (customer_id, profile_id) REFERENCES profiles(customer_id, id)
	);
	CREATE TABLE parent_credentials (
		customer_id TEXT PRIMARY KEY,
		approval_code_hash BLOB NOT NULL,
		failed_attempts INTEGER NOT NULL DEFAULT 0,
		locked_until TEXT,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (customer_id) REFERENCES customers(id)
	);
	CREATE TABLE temporary_browser_approvals (
		customer_id TEXT NOT NULL,
		profile_id TEXT NOT NULL,
		browser_profile TEXT NOT NULL,
		url_rule TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		source_device_id TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY (customer_id, profile_id, browser_profile, url_rule),
		FOREIGN KEY (customer_id, profile_id) REFERENCES profiles(customer_id, id),
		FOREIGN KEY (customer_id, source_device_id) REFERENCES devices(customer_id, id)
	);`,
	`DROP TABLE IF EXISTS temporary_browser_approvals;`,
	`CREATE TABLE pairing_sessions (
		id TEXT PRIMARY KEY,
		customer_id TEXT NOT NULL UNIQUE,
		state TEXT NOT NULL,
		code TEXT,
		device_id TEXT,
		device_name TEXT,
		platform TEXT,
		expires_at TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (customer_id) REFERENCES customers(id)
	);
	CREATE UNIQUE INDEX pairing_sessions_code_idx
	ON pairing_sessions(code)
	WHERE code IS NOT NULL;`,
	`ALTER TABLE pairing_sessions ADD COLUMN claim_proof_hash BLOB;`,
	`CREATE TABLE browser_access_requests (
		id TEXT PRIMARY KEY,
		customer_id TEXT NOT NULL,
		profile_id TEXT NOT NULL,
		source_device_id TEXT NOT NULL,
		url TEXT NOT NULL,
		site TEXT NOT NULL,
		browser_profile TEXT NOT NULL,
		state TEXT NOT NULL,
		decision_action TEXT,
		duration_seconds INTEGER,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		decided_at TEXT,
		FOREIGN KEY (customer_id, profile_id) REFERENCES profiles(customer_id, id),
		FOREIGN KEY (customer_id, source_device_id) REFERENCES devices(customer_id, id)
	);
	CREATE INDEX browser_access_requests_pending_idx
	ON browser_access_requests(customer_id, profile_id, source_device_id, site, browser_profile, state);`,
}

// Open opens the service SQLite database and applies pending migrations.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening SQLite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	if err := initialize(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func initialize(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enabling SQLite foreign keys: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("setting SQLite busy timeout: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("creating migration table: %w", err)
	}

	return migrate(ctx, db)
}

func migrate(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("reading migration version: %w", err)
	}

	for index := version; index < len(migrations); index++ {
		if err := applyMigration(ctx, db, index+1, migrations[index]); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, version int, statements string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting migration %d: %w", version, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, statements); err != nil {
		return fmt.Errorf("applying migration %d: %w", version, err)
	}
	if _, err := tx.ExecContext(
		ctx,
		"INSERT INTO schema_migrations(version, applied_at) VALUES (?, CURRENT_TIMESTAMP)",
		version,
	); err != nil {
		return fmt.Errorf("recording migration %d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration %d: %w", version, err)
	}
	return nil
}
