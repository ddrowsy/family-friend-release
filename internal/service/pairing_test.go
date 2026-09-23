package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/service/database"
)

func TestStorePairingSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newPairingStore(t, ctx)
	now := time.Date(
		2026,
		time.September,
		22,
		14,
		0,
		0,
		0,
		time.UTC,
	)
	store.now = func() time.Time { return now }

	first, err := store.CreatePairingSession(
		ctx,
		"customer-1",
		"111111",
		now.Add(35*time.Second),
	)
	if err != nil {
		t.Fatalf("CreatePairingSession first: %v", err)
	}
	if first.ID == "" || first.State != PairingStateWaiting {
		t.Fatalf("first session = %#v", first)
	}

	replacement, err := store.CreatePairingSession(
		ctx,
		"customer-1",
		"222222",
		now.Add(35*time.Second),
	)
	if err != nil {
		t.Fatalf("CreatePairingSession replacement: %v", err)
	}
	if replacement.ID == first.ID {
		t.Fatal("replacement reused pairing session ID")
	}
	if _, err := store.PairingSession(
		ctx,
		"customer-1",
		first.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replaced session error = %v, want ErrNotFound", err)
	}

	byCode, err := store.PairingSessionByCode(ctx, "222222")
	if err != nil {
		t.Fatalf("PairingSessionByCode: %v", err)
	}
	if byCode.ID != replacement.ID {
		t.Fatalf("code lookup ID = %q, want %q", byCode.ID, replacement.ID)
	}

	rotated, err := store.RotatePairingSessionCode(
		ctx,
		"customer-1",
		replacement.ID,
		PairingCodeRotation{
			Code:      "333333",
			ExpiresAt: now.Add(35 * time.Second),
		},
	)
	if err != nil {
		t.Fatalf("RotatePairingSessionCode: %v", err)
	}
	if rotated.Code != "333333" {
		t.Fatalf("rotated code = %q", rotated.Code)
	}
	if _, err := store.PairingSessionByCode(
		ctx,
		"222222",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("previous code lookup error = %v, want ErrNotFound", err)
	}

	claimed, err := store.ClaimPairingSession(
		ctx,
		"333333",
		PairingDeviceClaim{
			DeviceID:   "device-1",
			DeviceName: "Harry Laptop",
			Platform:   "linux",
		},
		[]byte{},
	)
	if err != nil {
		t.Fatalf("ClaimPairingSession: %v", err)
	}
	if claimed.State != PairingStatePendingConfirmation {
		t.Fatalf("claimed state = %q", claimed.State)
	}
	if claimed.Code != "" {
		t.Fatalf("claimed code = %q, want empty", claimed.Code)
	}
	if claimed.DeviceID != "device-1" {
		t.Fatalf("claimed device = %q", claimed.DeviceID)
	}
	if !claimed.ExpiresAt.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("claimed expiry = %v", claimed.ExpiresAt)
	}
	if _, err := store.PairingSessionByCode(
		ctx,
		"333333",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claimed code lookup error = %v, want ErrNotFound", err)
	}

	persisted, err := store.PairingSession(
		ctx,
		"customer-1",
		replacement.ID,
	)
	if err != nil {
		t.Fatalf("PairingSession claimed: %v", err)
	}
	if persisted.State != PairingStatePendingConfirmation || persisted.Code != "" {
		t.Fatalf("persisted claimed session = %#v", persisted)
	}

	if err := store.DeletePairingSession(
		ctx,
		"customer-1",
		replacement.ID,
	); err != nil {
		t.Fatalf("DeletePairingSession: %v", err)
	}
	if _, err := store.PairingSession(
		ctx,
		"customer-1",
		replacement.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session error = %v, want ErrNotFound", err)
	}
}

func TestStorePairingSessionRejectsExpiredCodeAndSession(t *testing.T) {
	ctx := context.Background()
	store := newPairingStore(t, ctx)
	now := time.Date(
		2026,
		time.September,
		22,
		14,
		0,
		0,
		0,
		time.UTC,
	)
	store.now = func() time.Time { return now }

	session, err := store.CreatePairingSession(
		ctx,
		"customer-1",
		"111111",
		now.Add(35*time.Second),
	)
	if err != nil {
		t.Fatalf("CreatePairingSession: %v", err)
	}

	now = now.Add(36 * time.Second)
	if _, err := store.PairingSessionByCode(
		ctx,
		"111111",
	); !errors.Is(err, ErrPairingSessionExpired) {
		t.Fatalf("expired code error = %v, want ErrPairingSessionExpired", err)
	}
	if _, err := store.PairingSession(
		ctx,
		"customer-1",
		session.ID,
	); !errors.Is(err, ErrPairingSessionExpired) {
		t.Fatalf("expired session error = %v, want ErrPairingSessionExpired", err)
	}
	if _, err := store.ClaimPairingSession(
		ctx,
		"111111",
		PairingDeviceClaim{DeviceID: "device-1"},
		[]byte{},
	); !errors.Is(err, ErrPairingSessionExpired) {
		t.Fatalf("expired claim error = %v, want ErrPairingSessionExpired", err)
	}
}

func TestStorePairingSessionCodeIsUniqueAndReplacementIsTransactional(t *testing.T) {
	ctx := context.Background()
	store := newPairingStoreWithCustomers(
		t,
		ctx,
		"customer-1",
		"customer-2",
	)
	now := time.Date(
		2026,
		time.September,
		22,
		14,
		0,
		0,
		0,
		time.UTC,
	)
	store.now = func() time.Time { return now }

	first, err := store.CreatePairingSession(
		ctx,
		"customer-1",
		"same-code",
		now.Add(35*time.Second),
	)
	if err != nil {
		t.Fatalf("CreatePairingSession first: %v", err)
	}
	if _, err := store.CreatePairingSession(
		ctx,
		"customer-2",
		"same-code",
		now.Add(35*time.Second),
	); err == nil {
		t.Fatal("duplicate pairing code returned nil error")
	}

	persisted, err := store.PairingSession(
		ctx,
		"customer-1",
		first.ID,
	)
	if err != nil {
		t.Fatalf("PairingSession after duplicate code: %v", err)
	}
	if persisted.Code != "same-code" {
		t.Fatalf("persisted code = %q", persisted.Code)
	}
}

func TestStorePairingSessionRequiresExistingCustomer(t *testing.T) {
	ctx := context.Background()
	store := newPairingStore(t, ctx)
	now := time.Date(
		2026,
		time.September,
		22,
		14,
		0,
		0,
		0,
		time.UTC,
	)
	store.now = func() time.Time { return now }

	_, err := store.CreatePairingSession(
		ctx,
		"missing",
		"111111",
		now.Add(35*time.Second),
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing customer error = %v, want ErrNotFound", err)
	}
}

func newPairingStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	return newPairingStoreWithCustomers(t, ctx, "customer-1")
}

func newPairingStoreWithCustomers(
	t *testing.T,
	ctx context.Context,
	customerIDs ...CustomerID,
) *Store {
	t.Helper()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStore(db)
	for _, customerID := range customerIDs {
		if err := store.CreateProfile(ctx, customerID, "bootstrap"); err != nil {
			t.Fatalf("CreateProfile %s: %v", customerID, err)
		}
	}
	return store
}
