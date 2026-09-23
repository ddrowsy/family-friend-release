package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
	"github.com/ddrowsy/family-friend-release/internal/service/database"
)

const testApprovalCodeIterations = 1_000

func TestApprovalCodeSetVerifyReplaceAndCustomerIsolation(t *testing.T) {
	ctx := context.Background()
	store, _ := newApprovalCodeTestStore(t)

	for _, customerID := range []CustomerID{"customer-1", "customer-2"} {
		if err := store.CreateProfile(ctx, customerID, "child"); err != nil {
			t.Fatalf("CreateProfile %s: %v", customerID, err)
		}
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode customer-1: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-2", "9876"); err != nil {
		t.Fatalf("SetApprovalCode customer-2: %v", err)
	}

	if err := store.VerifyApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("VerifyApprovalCode customer-1: %v", err)
	}
	err := store.VerifyApprovalCode(ctx, "customer-1", "9876")
	if !errors.Is(err, ErrInvalidApprovalCode) {
		t.Fatalf("cross-customer code error = %v, want ErrInvalidApprovalCode", err)
	}
	if err := store.VerifyApprovalCode(ctx, "customer-2", "9876"); err != nil {
		t.Fatalf("VerifyApprovalCode customer-2: %v", err)
	}

	if err := store.SetApprovalCode(ctx, "customer-1", "5678"); err != nil {
		t.Fatalf("replace approval code: %v", err)
	}
	err = store.VerifyApprovalCode(ctx, "customer-1", "1234")
	if !errors.Is(err, ErrInvalidApprovalCode) {
		t.Fatalf("old code error = %v, want ErrInvalidApprovalCode", err)
	}
	if err := store.VerifyApprovalCode(ctx, "customer-1", "5678"); err != nil {
		t.Fatalf("replacement code verification: %v", err)
	}
}

func TestApprovalCodeVerifierPersistsWithoutPlaintext(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "service.db")
	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	store := NewStore(db)
	store.approvalCodeIterations = testApprovalCodeIterations

	if err := store.CreateProfile(ctx, "customer-1", "child"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	code := "secret-4827"
	if err := store.SetApprovalCode(ctx, "customer-1", code); err != nil {
		t.Fatalf("SetApprovalCode: %v", err)
	}

	var persisted []byte
	if err := db.QueryRow(
		`SELECT approval_code_hash
		FROM parent_credentials
		WHERE customer_id = ?`,
		"customer-1",
	).Scan(&persisted); err != nil {
		t.Fatalf("read persisted verifier: %v", err)
	}
	if bytes.Contains(persisted, []byte(code)) {
		t.Fatalf("persisted verifier contains plaintext approval code")
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	db, err = database.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer db.Close()
	store = NewStore(db)
	store.approvalCodeIterations = testApprovalCodeIterations

	if err := store.VerifyApprovalCode(ctx, "customer-1", code); err != nil {
		t.Fatalf("VerifyApprovalCode after reopen: %v", err)
	}
}

func TestApprovalCodeThrottlesFailedAttempts(t *testing.T) {
	ctx := context.Background()
	store, _ := newApprovalCodeTestStore(t)
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	store.now = func() time.Time {
		return now
	}

	if err := store.CreateProfile(ctx, "customer-1", "child"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode: %v", err)
	}

	for attempt := 0; attempt < maxApprovalCodeFailures; attempt++ {
		err := store.VerifyApprovalCode(ctx, "customer-1", "wrong")
		if !errors.Is(err, ErrInvalidApprovalCode) {
			t.Fatalf(
				"attempt %d error = %v, want ErrInvalidApprovalCode",
				attempt+1,
				err,
			)
		}
	}
	err := store.VerifyApprovalCode(ctx, "customer-1", "1234")
	if !errors.Is(err, ErrApprovalCodeThrottled) {
		t.Fatalf("locked verification error = %v, want ErrApprovalCodeThrottled", err)
	}

	now = now.Add(approvalCodeLockDuration + time.Second)
	if err := store.VerifyApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("verification after throttle expiry: %v", err)
	}
}

func TestSetApprovalCodeResetsThrottle(t *testing.T) {
	ctx := context.Background()
	store, _ := newApprovalCodeTestStore(t)

	if err := store.CreateProfile(ctx, "customer-1", "child"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode: %v", err)
	}
	for attempt := 0; attempt < maxApprovalCodeFailures; attempt++ {
		_ = store.VerifyApprovalCode(ctx, "customer-1", "wrong")
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "5678"); err != nil {
		t.Fatalf("replace approval code: %v", err)
	}
	if err := store.VerifyApprovalCode(ctx, "customer-1", "5678"); err != nil {
		t.Fatalf("replacement code should clear throttle: %v", err)
	}
}

func TestValidateDeviceProfileAccess(t *testing.T) {
	ctx := context.Background()
	store, _ := newApprovalCodeTestStore(t)

	if err := store.CreateProfile(ctx, "customer-1", "harry"); err != nil {
		t.Fatalf("CreateProfile harry: %v", err)
	}
	if err := store.CreateProfile(ctx, "customer-1", "james"); err != nil {
		t.Fatalf("CreateProfile james: %v", err)
	}
	if err := store.CreateProfile(ctx, "customer-2", "harry"); err != nil {
		t.Fatalf("CreateProfile other customer: %v", err)
	}
	if err := store.RegisterDevice(
		ctx,
		DeviceRegistration{
			CustomerID: "customer-1",
			DeviceID:   "laptop",
		},
	); err != nil {
		t.Fatalf("RegisterDevice customer-1: %v", err)
	}
	if err := store.RegisterDevice(
		ctx,
		DeviceRegistration{
			CustomerID: "customer-2",
			DeviceID:   "other-laptop",
		},
	); err != nil {
		t.Fatalf("RegisterDevice customer-2: %v", err)
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("LinkDeviceProfile: %v", err)
	}

	if err := store.ValidateDeviceProfileAccess(
		ctx,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("linked device validation: %v", err)
	}
	err := store.ValidateDeviceProfileAccess(
		ctx,
		"customer-1",
		"laptop",
		"james",
	)
	if !errors.Is(err, ErrDeviceProfileNotLinked) {
		t.Fatalf("unlinked device error = %v, want ErrDeviceProfileNotLinked", err)
	}
	err = store.ValidateDeviceProfileAccess(
		ctx,
		"customer-2",
		"laptop",
		"harry",
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-customer device error = %v, want ErrNotFound", err)
	}
}

func TestApprovalVerifierIsNotInPublicModels(t *testing.T) {
	publicModels := []any{
		ProfileConfiguration{
			CustomerID: "customer-1",
			ProfileID:  "harry",
			Policy: policy.ProfilePolicy{
				SchemaVersion: 1,
				Version:       "1",
				Modules:       map[string]policy.ModulePolicy{},
			},
		},
		DeviceRegistration{
			CustomerID: "customer-1",
			DeviceID:   "laptop",
		},
		DeviceProfileAssignment{
			CustomerID: "customer-1",
			DeviceID:   "laptop",
			ProfileIDs: []ProfileID{"harry"},
		},
		BrowserApprovalResult{
			CustomerID: "customer-1",
			ProfileID:  "harry",
			DeviceID:   "laptop",
		},
	}
	data, err := json.Marshal(publicModels)
	if err != nil {
		t.Fatalf("Marshal public models: %v", err)
	}
	for _, forbidden := range []string{
		"approval_code_hash",
		"failed_attempts",
		"locked_until",
	} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("public models expose private field %q", forbidden)
		}
	}
}

func TestSetApprovalCodeRejectsMissingCustomerAndEmptyCode(t *testing.T) {
	ctx := context.Background()
	store, _ := newApprovalCodeTestStore(t)

	err := store.SetApprovalCode(ctx, "missing", "1234")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing customer error = %v, want ErrNotFound", err)
	}

	if err := store.CreateProfile(ctx, "customer-1", "child"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-1", " "); err == nil {
		t.Fatal("empty approval code returned nil error")
	}
}

func newApprovalCodeTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "service.db")
	db, err := database.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	store := NewStore(db)
	store.approvalCodeIterations = testApprovalCodeIterations
	return store, path
}
