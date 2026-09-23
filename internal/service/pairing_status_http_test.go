package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type pairingStatusSessionInput struct {
	CustomerID CustomerID
	Code       string
	DeviceID   DeviceID
	Proof      string
}

func TestDevicePairingStatusHTTPReturnsPairingStates(t *testing.T) {
	t.Run("pending confirmation", func(t *testing.T) {
		ctx := context.Background()
		store := newPairingStore(t, ctx)
		now := pairingStatusTestTime()
		store.now = func() time.Time { return now }

		session := createPairingStatusSession(
			t,
			ctx,
			store,
			pairingStatusSessionInput{
				CustomerID: "customer-1",
				Code:       "111111",
				DeviceID:   "device-1",
				Proof:      "proof-1",
			},
		)
		response := serveDevicePairingStatusRequest(
			t,
			NewHTTPHandler(store),
			session.ID,
			"proof-1",
		)

		assertPairingStatusResponseHasNoIdentity(t, response.Body.Bytes())
		assertPairingStatusResponse(
			t,
			response,
			PairingCompletionPendingConfirmation,
		)
	})

	t.Run("confirmed", func(t *testing.T) {
		ctx := context.Background()
		store := newPairingStore(t, ctx)
		now := pairingStatusTestTime()
		store.now = func() time.Time { return now }

		session := createPairingStatusSession(
			t,
			ctx,
			store,
			pairingStatusSessionInput{
				CustomerID: "customer-1",
				Code:       "222222",
				DeviceID:   "device-2",
				Proof:      "proof-2",
			},
		)
		if _, err := store.ConfirmPairingSession(
			ctx,
			"customer-1",
			session.ID,
		); err != nil {
			t.Fatalf("ConfirmPairingSession: %v", err)
		}

		response := serveDevicePairingStatusRequest(
			t,
			NewHTTPHandler(store),
			session.ID,
			"proof-2",
		)
		assertPairingStatusResponse(
			t,
			response,
			PairingCompletionConfirmed,
		)
	})

	t.Run("cancelled", func(t *testing.T) {
		ctx := context.Background()
		store := newPairingStore(t, ctx)
		now := pairingStatusTestTime()
		store.now = func() time.Time { return now }

		session := createPairingStatusSession(
			t,
			ctx,
			store,
			pairingStatusSessionInput{
				CustomerID: "customer-1",
				Code:       "333333",
				DeviceID:   "device-3",
				Proof:      "proof-3",
			},
		)
		if _, err := store.CancelPairingSession(
			ctx,
			"customer-1",
			session.ID,
		); err != nil {
			t.Fatalf("CancelPairingSession: %v", err)
		}

		response := serveDevicePairingStatusRequest(
			t,
			NewHTTPHandler(store),
			session.ID,
			"proof-3",
		)
		assertPairingStatusResponse(
			t,
			response,
			PairingCompletionCancelled,
		)
	})

	t.Run("expired terminal result", func(t *testing.T) {
		ctx := context.Background()
		store := newPairingStore(t, ctx)
		now := pairingStatusTestTime()
		store.now = func() time.Time { return now }

		session := createPairingStatusSession(
			t,
			ctx,
			store,
			pairingStatusSessionInput{
				CustomerID: "customer-1",
				Code:       "444444",
				DeviceID:   "device-4",
				Proof:      "proof-4",
			},
		)
		confirmed, err := store.ConfirmPairingSession(
			ctx,
			"customer-1",
			session.ID,
		)
		if err != nil {
			t.Fatalf("ConfirmPairingSession: %v", err)
		}
		now = confirmed.ExpiresAt.Add(time.Second)

		response := serveDevicePairingStatusRequest(
			t,
			NewHTTPHandler(store),
			session.ID,
			"proof-4",
		)
		assertPairingStatusResponse(
			t,
			response,
			PairingCompletionExpired,
		)
	})
}

func TestDevicePairingStatusHTTPRejectsMissingWrongAndCrossSessionProof(t *testing.T) {
	ctx := context.Background()
	store := newPairingStoreWithCustomers(
		t,
		ctx,
		"customer-1",
		"customer-2",
	)
	now := pairingStatusTestTime()
	store.now = func() time.Time { return now }

	first := createPairingStatusSession(
		t,
		ctx,
		store,
		pairingStatusSessionInput{
			CustomerID: "customer-1",
			Code:       "555555",
			DeviceID:   "device-5",
			Proof:      "proof-5",
		},
	)
	second := createPairingStatusSession(
		t,
		ctx,
		store,
		pairingStatusSessionInput{
			CustomerID: "customer-2",
			Code:       "666666",
			DeviceID:   "device-6",
			Proof:      "proof-6",
		},
	)
	handler := NewHTTPHandler(store)

	tests := []struct {
		name      string
		sessionID string
		proof     string
	}{
		{
			name:      "missing proof",
			sessionID: first.ID,
		},
		{
			name:      "wrong proof",
			sessionID: first.ID,
			proof:     "wrong-proof",
		},
		{
			name:      "cross-session proof",
			sessionID: second.ID,
			proof:     "proof-5",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveDevicePairingStatusRequest(
				t,
				handler,
				test.sessionID,
				test.proof,
			)
			assertStatus(t, response, http.StatusNotFound)
		})
	}
}

func createPairingStatusSession(
	t *testing.T,
	ctx context.Context,
	store *Store,
	input pairingStatusSessionInput,
) PairingSession {
	t.Helper()

	if _, err := store.CreatePairingSession(
		ctx,
		input.CustomerID,
		input.Code,
		store.now().UTC().Add(pairingWaitingLifetime),
	); err != nil {
		t.Fatalf("CreatePairingSession: %v", err)
	}

	session, err := store.ClaimPairingSession(
		ctx,
		input.Code,
		PairingDeviceClaim{
			DeviceID:   input.DeviceID,
			DeviceName: "Test Device",
			Platform:   "windows",
		},
		hashPairingBootstrapProof(input.Proof),
	)
	if err != nil {
		t.Fatalf("ClaimPairingSession: %v", err)
	}
	return session
}

func serveDevicePairingStatusRequest(
	t *testing.T,
	handler http.Handler,
	sessionID string,
	proof string,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(
		http.MethodGet,
		APIV1Path+"/pairing/"+sessionID+"/status",
		nil,
	)
	if proof != "" {
		request.Header.Set(
			"Authorization",
			pairingBootstrapAuthorizationScheme+" "+proof,
		)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertPairingStatusResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	want PairingCompletionState,
) {
	t.Helper()

	assertStatus(t, response, http.StatusOK)
	var result DevicePairingStatusResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode pairing status response: %v", err)
	}
	if result.State != want {
		t.Fatalf("pairing status = %q, want %q", result.State, want)
	}
}

func assertPairingStatusResponseHasNoIdentity(t *testing.T, body []byte) {
	t.Helper()

	fields := map[string]any{}
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("decode pairing status fields: %v", err)
	}
	if _, ok := fields["customer_id"]; ok {
		t.Fatal("pairing status response exposed customer_id")
	}
	if _, ok := fields["device_id"]; ok {
		t.Fatal("pairing status response exposed device_id")
	}
}

func pairingStatusTestTime() time.Time {
	return time.Date(
		2026,
		time.September,
		22,
		15,
		0,
		0,
		0,
		time.UTC,
	)
}
