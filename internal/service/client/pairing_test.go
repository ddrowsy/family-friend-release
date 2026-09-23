package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/service"
)

func TestAgentClientClaimPairing(t *testing.T) {
	expiresAt := time.Date(
		2026,
		time.September,
		23,
		11,
		0,
		0,
		0,
		time.UTC,
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/pairing/claim" {
			t.Fatalf("path = %q", r.URL.Path)
		}

		var request service.DevicePairingClaimRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Code != "123456" || request.DeviceID != "device-1" {
			t.Fatalf("request = %+v", request)
		}

		writeTestJSON(t, w, service.DevicePairingClaimResponse{
			SessionID:  "session-1",
			State:      service.PairingStatePendingConfirmation,
			ExpiresAt:  expiresAt,
			ClaimProof: "secret-proof",
		})
	}))
	defer server.Close()

	client := newTestAgentClient(t, server)
	response, err := client.ClaimPairing(context.Background(), service.DevicePairingClaimRequest{
		Code:       "123456",
		DeviceID:   "device-1",
		DeviceName: "Child PC",
		Platform:   "windows",
	})
	if err != nil {
		t.Fatalf("ClaimPairing() error = %v", err)
	}
	if response.SessionID != "session-1" || response.ClaimProof != "secret-proof" {
		t.Fatalf("response = %+v", response)
	}
}

func TestAgentClientPairingStatusUsesBootstrapProof(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/pairing/session-1/status" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Pairing secret-proof" {
			t.Fatalf("Authorization = %q", got)
		}
		writeTestJSON(t, w, service.DevicePairingStatusResponse{
			State: service.PairingCompletionConfirmed,
		})
	}))
	defer server.Close()

	client := newTestAgentClient(t, server)
	response, err := client.PairingStatus(context.Background(), "session-1", "secret-proof")
	if err != nil {
		t.Fatalf("PairingStatus() error = %v", err)
	}
	if response.State != service.PairingCompletionConfirmed {
		t.Fatalf("state = %q", response.State)
	}
}

func TestAgentClientPairingStatusRejectsMissingBootstrapIdentity(t *testing.T) {
	client, err := NewAgent("https://example.test", nil)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if _, err := client.PairingStatus(context.Background(), "", "proof"); err == nil {
		t.Fatal("PairingStatus() missing session error = nil")
	}
	if _, err := client.PairingStatus(context.Background(), "session-1", " "); err == nil {
		t.Fatal("PairingStatus() missing proof error = nil")
	}
}
