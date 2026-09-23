package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestDevicePairingClaimHTTPTransitionsWaitingSession(t *testing.T) {
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
		"123456",
		now.Add(35*time.Second),
	)
	if err != nil {
		t.Fatalf("CreatePairingSession: %v", err)
	}

	handler := NewHTTPHandler(store)
	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   devicePairingClaimRoute,
			body: DevicePairingClaimRequest{
				Code:       "123456",
				DeviceID:   "device-1",
				DeviceName: "Harry Laptop",
				Platform:   "linux",
			},
		},
	)
	assertStatus(t, response, http.StatusOK)

	body := response.Body.Bytes()
	var result DevicePairingClaimResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode claim response: %v", err)
	}
	if result.SessionID != session.ID {
		t.Fatalf("session ID = %q, want %q", result.SessionID, session.ID)
	}
	if result.State != PairingStatePendingConfirmation {
		t.Fatalf("state = %q", result.State)
	}
	if !result.ExpiresAt.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("expiry = %v", result.ExpiresAt)
	}

	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("decode claim fields: %v", err)
	}
	if _, ok := fields["customer_id"]; ok {
		t.Fatal("claim response exposed customer_id")
	}

	persisted, err := store.PairingSession(ctx, "customer-1", session.ID)
	if err != nil {
		t.Fatalf("PairingSession: %v", err)
	}
	if persisted.Code != "" || persisted.DeviceID != "device-1" {
		t.Fatalf("persisted session = %#v", persisted)
	}
	if persisted.DeviceName != "Harry Laptop" || persisted.Platform != "linux" {
		t.Fatalf("persisted device details = %#v", persisted)
	}
	if _, err := store.DeviceProfiles(
		ctx,
		"customer-1",
		"device-1",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("device registration error = %v, want ErrNotFound", err)
	}

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   devicePairingClaimRoute,
			body: DevicePairingClaimRequest{
				Code:       "123456",
				DeviceID:   "device-1",
				DeviceName: "Harry Laptop",
				Platform:   "linux",
			},
		},
	)
	assertStatus(t, response, http.StatusNotFound)
}

func TestDevicePairingClaimHTTPRejectsUnknownExpiredAndInvalidClaims(t *testing.T) {
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
	handler := NewHTTPHandler(store)

	validRequest := DevicePairingClaimRequest{
		Code:       "654321",
		DeviceID:   "device-1",
		DeviceName: "Harry Laptop",
		Platform:   "linux",
	}
	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   devicePairingClaimRoute,
			body:   validRequest,
		},
	)
	assertStatus(t, response, http.StatusNotFound)

	if _, err := store.CreatePairingSession(
		ctx,
		"customer-1",
		"654321",
		now.Add(35*time.Second),
	); err != nil {
		t.Fatalf("CreatePairingSession: %v", err)
	}
	now = now.Add(36 * time.Second)
	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   devicePairingClaimRoute,
			body:   validRequest,
		},
	)
	assertStatus(t, response, http.StatusGone)

	invalidRequests := []DevicePairingClaimRequest{
		{DeviceID: "device-1", DeviceName: "Laptop", Platform: "linux"},
		{Code: "111111", DeviceName: "Laptop", Platform: "linux"},
		{Code: "111111", DeviceID: "device-1", Platform: "linux"},
		{Code: "111111", DeviceID: "device-1", DeviceName: "Laptop"},
	}
	for _, request := range invalidRequests {
		response = serveHTTP(
			t,
			handler,
			testHTTPRequest{
				method: http.MethodPost,
				path:   devicePairingClaimRoute,
				body:   request,
			},
		)
		assertStatus(t, response, http.StatusBadRequest)
	}

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method:  http.MethodPost,
			path:    devicePairingClaimRoute,
			rawBody: []byte("{"),
		},
	)
	assertStatus(t, response, http.StatusBadRequest)
}
