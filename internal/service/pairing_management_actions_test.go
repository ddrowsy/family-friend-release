package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type pendingPairingTestInput struct {
	CustomerID CustomerID
	Code       string
	DeviceID   DeviceID
	DeviceName string
	Proof      string
}

func TestStoreConfirmPairingSessionRegistersDeviceAndRetainsCompletion(t *testing.T) {
	ctx := context.Background()
	store := newPairingStoreWithCustomers(
		t,
		ctx,
		"customer-1",
		"customer-2",
	)
	now := pairingManagementTestTime()
	store.now = func() time.Time { return now }

	pending := createPendingPairingForTest(
		t,
		ctx,
		store,
		pendingPairingTestInput{
			CustomerID: "customer-1",
			Code:       "111111",
			DeviceID:   "device-1",
			DeviceName: "New Laptop Name",
			Proof:      "claim-proof",
		},
	)

	if err := store.RegisterDevice(
		ctx,
		DeviceRegistration{
			CustomerID: "customer-1",
			DeviceID:   "device-1",
		},
	); err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}
	if _, err := store.db.ExecContext(
		ctx,
		"UPDATE devices SET name = ? WHERE customer_id = ? AND id = ?",
		"Old Laptop Name",
		"customer-1",
		"device-1",
	); err != nil {
		t.Fatalf("seed device name: %v", err)
	}

	confirmed, err := store.ConfirmPairingSession(
		ctx,
		"customer-1",
		pending.ID,
	)
	if err != nil {
		t.Fatalf("ConfirmPairingSession: %v", err)
	}
	if confirmed.State != PairingStateConfirmed {
		t.Fatalf("confirmed state = %q", confirmed.State)
	}
	if !confirmed.ExpiresAt.Equal(now.Add(pairingCompletionTimeout)) {
		t.Fatalf("confirmed expiry = %v", confirmed.ExpiresAt)
	}

	resolved, err := store.PairingSessionByBootstrapProof(
		ctx,
		pending.ID,
		"claim-proof",
	)
	if err != nil {
		t.Fatalf("PairingSessionByBootstrapProof: %v", err)
	}
	if resolved.State != PairingStateConfirmed {
		t.Fatalf("resolved state = %q", resolved.State)
	}

	var deviceName string
	if err := store.db.QueryRowContext(
		ctx,
		"SELECT name FROM devices WHERE customer_id = ? AND id = ?",
		"customer-1",
		"device-1",
	).Scan(&deviceName); err != nil {
		t.Fatalf("read paired device: %v", err)
	}
	if deviceName != "New Laptop Name" {
		t.Fatalf("device name = %q", deviceName)
	}
	profileIDs, err := store.DeviceProfiles(ctx, "customer-1", "device-1")
	if err != nil {
		t.Fatalf("DeviceProfiles: %v", err)
	}
	if len(profileIDs) != 0 {
		t.Fatalf("paired device profiles = %v, want none", profileIDs)
	}

	if _, err := store.ConfirmPairingSession(
		ctx,
		"customer-1",
		pending.ID,
	); !errors.Is(err, ErrPairingSessionState) {
		t.Fatalf("second confirm error = %v, want ErrPairingSessionState", err)
	}
}

func TestStoreCancelPairingSessionRetainsCompletionWithoutDevice(t *testing.T) {
	ctx := context.Background()
	store := newPairingStore(t, ctx)
	now := pairingManagementTestTime()
	store.now = func() time.Time { return now }

	pending := createPendingPairingForTest(
		t,
		ctx,
		store,
		pendingPairingTestInput{
			CustomerID: "customer-1",
			Code:       "222222",
			DeviceID:   "device-2",
			DeviceName: "Cancelled Laptop",
			Proof:      "cancel-proof",
		},
	)

	cancelled, err := store.CancelPairingSession(
		ctx,
		"customer-1",
		pending.ID,
	)
	if err != nil {
		t.Fatalf("CancelPairingSession: %v", err)
	}
	if cancelled.State != PairingStateCancelled {
		t.Fatalf("cancelled state = %q", cancelled.State)
	}
	if !cancelled.ExpiresAt.Equal(now.Add(pairingCompletionTimeout)) {
		t.Fatalf("cancelled expiry = %v", cancelled.ExpiresAt)
	}

	resolved, err := store.PairingSessionByBootstrapProof(
		ctx,
		pending.ID,
		"cancel-proof",
	)
	if err != nil {
		t.Fatalf("PairingSessionByBootstrapProof: %v", err)
	}
	if resolved.State != PairingStateCancelled {
		t.Fatalf("resolved state = %q", resolved.State)
	}
	if _, err := store.DeviceProfiles(
		ctx,
		"customer-1",
		"device-2",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelled device registration error = %v, want ErrNotFound", err)
	}

	service := newManagementPairingService(store)
	service.generateCode = func() (string, error) { return "333333", nil }
	fresh, err := service.create(ctx, "customer-1")
	if err != nil {
		t.Fatalf("create after cancel: %v", err)
	}
	if fresh.ID == pending.ID || fresh.Code != "333333" {
		t.Fatalf("fresh session = %#v", fresh)
	}
	if _, err := store.PairingSession(
		ctx,
		"customer-1",
		pending.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replaced terminal session error = %v, want ErrNotFound", err)
	}
}

func TestStorePairingTerminalTransitionsRejectInvalidSessions(t *testing.T) {
	t.Run("wrong customer", func(t *testing.T) {
		ctx := context.Background()
		store := newPairingStoreWithCustomers(
			t,
			ctx,
			"customer-1",
			"customer-2",
		)
		now := pairingManagementTestTime()
		store.now = func() time.Time { return now }
		pending := createPendingPairingForTest(
			t,
			ctx,
			store,
			pendingPairingTestInput{
				CustomerID: "customer-1",
				Code:       "444444",
				DeviceID:   "device-4",
				DeviceName: "Laptop",
				Proof:      "proof-4",
			},
		)

		if _, err := store.ConfirmPairingSession(
			ctx,
			"customer-2",
			pending.ID,
		); !errors.Is(err, ErrNotFound) {
			t.Fatalf("wrong customer error = %v, want ErrNotFound", err)
		}
	})

	t.Run("waiting", func(t *testing.T) {
		ctx := context.Background()
		store := newPairingStore(t, ctx)
		now := pairingManagementTestTime()
		store.now = func() time.Time { return now }
		waiting, err := store.CreatePairingSession(
			ctx,
			"customer-1",
			"555555",
			now.Add(pairingWaitingLifetime),
		)
		if err != nil {
			t.Fatalf("CreatePairingSession: %v", err)
		}

		if _, err := store.CancelPairingSession(
			ctx,
			"customer-1",
			waiting.ID,
		); !errors.Is(err, ErrPairingSessionState) {
			t.Fatalf("waiting cancel error = %v, want ErrPairingSessionState", err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		ctx := context.Background()
		store := newPairingStore(t, ctx)
		now := pairingManagementTestTime()
		store.now = func() time.Time { return now }
		pending := createPendingPairingForTest(
			t,
			ctx,
			store,
			pendingPairingTestInput{
				CustomerID: "customer-1",
				Code:       "666666",
				DeviceID:   "device-6",
				DeviceName: "Laptop",
				Proof:      "proof-6",
			},
		)
		now = now.Add(pairingPendingConfirmationTimeout + time.Second)

		if _, err := store.ConfirmPairingSession(
			ctx,
			"customer-1",
			pending.ID,
		); !errors.Is(err, ErrPairingSessionExpired) {
			t.Fatalf("expired confirm error = %v, want ErrPairingSessionExpired", err)
		}
	})
}

func TestManagementPairingConfirmAndCancelHTTP(t *testing.T) {
	ctx := context.Background()
	store := newPairingStoreWithCustomers(
		t,
		ctx,
		"customer-1",
		"customer-2",
	)
	now := pairingManagementTestTime()
	store.now = func() time.Time { return now }
	handler := NewHTTPHandler(store)

	confirmSession := createPendingPairingForTest(
		t,
		ctx,
		store,
		pendingPairingTestInput{
			CustomerID: "customer-1",
			Code:       "777777",
			DeviceID:   "device-7",
			DeviceName: "Confirm Laptop",
			Proof:      "proof-7",
		},
	)
	confirmPath := managementPairingRoute + "/" + confirmSession.ID + "/confirm"

	response := serveManagementPairingRequest(
		t,
		handler,
		managementPairingHTTPRequest{
			method: http.MethodPost,
			path:   confirmPath,
		},
	)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing caller confirm status = %d", response.Code)
	}

	response = serveManagementPairingRequest(
		t,
		handler,
		managementPairingHTTPRequest{
			method:     http.MethodPost,
			path:       confirmPath,
			customerID: "customer-2",
		},
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("wrong customer confirm status = %d", response.Code)
	}

	response = serveManagementPairingRequest(
		t,
		handler,
		managementPairingHTTPRequest{
			method:     http.MethodPost,
			path:       confirmPath,
			customerID: "customer-1",
		},
	)
	if response.Code != http.StatusNoContent {
		t.Fatalf("confirm status = %d; body = %s", response.Code, response.Body.String())
	}

	cancelSession := createPendingPairingForTest(
		t,
		ctx,
		store,
		pendingPairingTestInput{
			CustomerID: "customer-1",
			Code:       "888888",
			DeviceID:   "device-8",
			DeviceName: "Cancel Laptop",
			Proof:      "proof-8",
		},
	)
	cancelPath := managementPairingRoute + "/" + cancelSession.ID + "/cancel"
	response = serveManagementPairingRequest(
		t,
		handler,
		managementPairingHTTPRequest{
			method:     http.MethodPost,
			path:       cancelPath,
			customerID: "customer-1",
		},
	)
	if response.Code != http.StatusNoContent {
		t.Fatalf("cancel status = %d; body = %s", response.Code, response.Body.String())
	}
}

func pairingManagementTestTime() time.Time {
	return time.Date(
		2026,
		time.September,
		22,
		14,
		0,
		0,
		0,
		time.UTC,
	)
}

func createPendingPairingForTest(
	t *testing.T,
	ctx context.Context,
	store *Store,
	input pendingPairingTestInput,
) PairingSession {
	t.Helper()

	waiting, err := store.CreatePairingSession(
		ctx,
		input.CustomerID,
		input.Code,
		store.now().UTC().Add(pairingWaitingLifetime),
	)
	if err != nil {
		t.Fatalf("CreatePairingSession: %v", err)
	}
	claimed, err := store.ClaimPairingSession(
		ctx,
		input.Code,
		PairingDeviceClaim{
			DeviceID:   input.DeviceID,
			DeviceName: input.DeviceName,
			Platform:   "linux",
		},
		hashPairingBootstrapProof(input.Proof),
	)
	if err != nil {
		t.Fatalf("ClaimPairingSession: %v", err)
	}
	if claimed.ID != waiting.ID {
		t.Fatalf("claimed ID = %q, want %q", claimed.ID, waiting.ID)
	}
	return claimed
}
