package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/service"
)

type pairingClientStub struct {
	claimRequest service.DevicePairingClaimRequest
	claimResult  service.DevicePairingClaimResponse
	claimErr     error
	statusResult service.DevicePairingStatusResponse
	statusErr    error
	sessionID    string
	claimProof   string
}

type uiTestRequest struct {
	method string
	path   string
	body   string
}

func (c *pairingClientStub) ClaimPairing(
	_ context.Context,
	request service.DevicePairingClaimRequest,
) (service.DevicePairingClaimResponse, error) {
	c.claimRequest = request
	return c.claimResult, c.claimErr
}

func (c *pairingClientStub) PairingStatus(
	_ context.Context,
	sessionID string,
	claimProof string,
) (service.DevicePairingStatusResponse, error) {
	c.sessionID = sessionID
	c.claimProof = claimProof
	return c.statusResult, c.statusErr
}

func TestUIAPIStatusDoesNotExposePairingSecrets(t *testing.T) {
	api := newTestUIAPI(t, &pairingClientStub{})
	response := performUIRequest(t, api, uiTestRequest{
		method: http.MethodGet,
		path:   UIStatusPath,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	body := map[string]any{}
	decodeUIResponse(t, response, &body)
	if body["device_id"] != "device-1" {
		t.Fatalf("device_id = %v", body["device_id"])
	}
	if body["pairing"] != "unpaired" {
		t.Fatalf("pairing = %v", body["pairing"])
	}
	for _, forbidden := range []string{"customer_id", "claim_proof", "credential", "bootstrap_proof"} {
		if _, exists := body[forbidden]; exists {
			t.Fatalf("response exposes %q", forbidden)
		}
	}
	profiles, ok := body["profile_ids"].([]any)
	if !ok || len(profiles) != 0 {
		t.Fatalf("profile_ids = %#v", body["profile_ids"])
	}
}

func TestUIAPIPairingClaimKeepsProofInsideAgent(t *testing.T) {
	expiresAt := time.Date(
		2026,
		time.September,
		23,
		11,
		30,
		0,
		0,
		time.UTC,
	)
	client := &pairingClientStub{claimResult: service.DevicePairingClaimResponse{
		SessionID:  "session-1",
		State:      service.PairingStatePendingConfirmation,
		ExpiresAt:  expiresAt,
		ClaimProof: "top-secret-proof",
	}}
	api := newTestUIAPI(t, client)

	response := performUIRequest(t, api, uiTestRequest{
		method: http.MethodPost,
		path:   UIPairingClaimPath,
		body:   `{"code":"123456","device_id":"attacker"}`,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if client.claimRequest.DeviceID != "device-1" || client.claimRequest.Code != "123456" {
		t.Fatalf("claim request = %+v", client.claimRequest)
	}
	if strings.Contains(response.Body.String(), "top-secret-proof") {
		t.Fatal("claim proof leaked to UI response")
	}

	client.statusResult = service.DevicePairingStatusResponse{
		State: service.PairingCompletionPendingConfirmation,
	}
	status := performUIRequest(t, api, uiTestRequest{
		method: http.MethodGet,
		path:   UIPairingStatusPath,
	})
	if status.Code != http.StatusOK {
		t.Fatalf("status poll = %d, body = %s", status.Code, status.Body.String())
	}
	if client.sessionID != "session-1" || client.claimProof != "top-secret-proof" {
		t.Fatalf("remote status identity = %q / %q", client.sessionID, client.claimProof)
	}
}

func TestUIAPIPairingTerminalTransitions(t *testing.T) {
	tests := []struct {
		name   string
		remote service.PairingCompletionState
		want   UIPairingState
		paired bool
	}{
		{
			name:   "confirmed",
			remote: service.PairingCompletionConfirmed,
			want:   UIPairingConfirmed,
			paired: true,
		},
		{
			name:   "cancelled",
			remote: service.PairingCompletionCancelled,
			want:   UIPairingCancelled,
		},
		{
			name:   "expired",
			remote: service.PairingCompletionExpired,
			want:   UIPairingExpired,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &pairingClientStub{
				claimResult: service.DevicePairingClaimResponse{
					SessionID:  "session-1",
					ClaimProof: "proof",
				},
				statusResult: service.DevicePairingStatusResponse{State: test.remote},
			}
			api := newTestUIAPI(t, client)
			claim := performUIRequest(t, api, uiTestRequest{
				method: http.MethodPost,
				path:   UIPairingClaimPath,
				body:   `{"code":"123456"}`,
			})
			if claim.Code != http.StatusOK {
				t.Fatalf("claim status = %d", claim.Code)
			}

			statusResponse := performUIRequest(t, api, uiTestRequest{
				method: http.MethodGet,
				path:   UIPairingStatusPath,
			})
			var pairing UIPairingStatus
			decodeUIResponse(t, statusResponse, &pairing)
			if pairing.State != test.want {
				t.Fatalf("pairing state = %q, want %q", pairing.State, test.want)
			}

			agentResponse := performUIRequest(t, api, uiTestRequest{
				method: http.MethodGet,
				path:   UIStatusPath,
			})
			var status UIStatus
			decodeUIResponse(t, agentResponse, &status)
			if status.Paired != test.paired {
				t.Fatalf("paired = %v, want %v", status.Paired, test.paired)
			}
		})
	}
}

func TestUIAPIPairingSurvivesUIReconnect(t *testing.T) {
	client := &pairingClientStub{
		claimResult: service.DevicePairingClaimResponse{
			SessionID:  "session-1",
			ClaimProof: "proof",
		},
		statusResult: service.DevicePairingStatusResponse{
			State: service.PairingCompletionPendingConfirmation,
		},
	}
	api := newTestUIAPI(t, client)
	claim := performUIRequest(t, api, uiTestRequest{
		method: http.MethodPost,
		path:   UIPairingClaimPath,
		body:   `{"code":"123456"}`,
	})
	if claim.Code != http.StatusOK {
		t.Fatalf("claim status = %d", claim.Code)
	}

	firstUIConnection := api.Handler()
	secondUIConnection := api.Handler()
	_ = firstUIConnection
	request := httptest.NewRequest(http.MethodGet, UIPairingStatusPath, nil)
	response := httptest.NewRecorder()
	secondUIConnection.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("reconnected status = %d, body = %s", response.Code, response.Body.String())
	}
	if client.sessionID != "session-1" || client.claimProof != "proof" {
		t.Fatal("pairing state was not retained by agent API")
	}
}

func TestUIAPIResetPendingPairing(t *testing.T) {
	client := &pairingClientStub{claimResult: service.DevicePairingClaimResponse{
		SessionID:  "session-1",
		ClaimProof: "proof",
	}}
	api := newTestUIAPI(t, client)
	_ = performUIRequest(t, api, uiTestRequest{
		method: http.MethodPost,
		path:   UIPairingClaimPath,
		body:   `{"code":"123456"}`,
	})

	reset := performUIRequest(t, api, uiTestRequest{
		method: http.MethodDelete,
		path:   UIPairingResetPath,
	})
	if reset.Code != http.StatusNoContent {
		t.Fatalf("reset status = %d, body = %s", reset.Code, reset.Body.String())
	}
	statusResponse := performUIRequest(t, api, uiTestRequest{
		method: http.MethodGet,
		path:   UIPairingStatusPath,
	})
	var status UIPairingStatus
	decodeUIResponse(t, statusResponse, &status)
	if status.State != UIPairingUnpaired {
		t.Fatalf("state = %q", status.State)
	}
}

func TestUIAPIReportsRemoteFailureWithoutSecretLeak(t *testing.T) {
	client := &pairingClientStub{claimErr: errors.New("network failed with secret data")}
	api := newTestUIAPI(t, client)
	response := performUIRequest(t, api, uiTestRequest{
		method: http.MethodPost,
		path:   UIPairingClaimPath,
		body:   `{"code":"123456"}`,
	})
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret data") {
		t.Fatal("remote error leaked to UI")
	}
}

func TestUIAPIServerRejectsNonLoopbackAddress(t *testing.T) {
	api := newTestUIAPI(t, &pairingClientStub{})
	if _, err := NewUIAPIServer(api, "0.0.0.0:17654"); err == nil {
		t.Fatal("NewUIAPIServer() non-loopback error = nil")
	}
	if _, err := NewUIAPIServer(api, "127.0.0.1:0"); err != nil {
		t.Fatalf("NewUIAPIServer() loopback error = %v", err)
	}
}

func newTestUIAPI(t *testing.T, client PairingClient) *UIAPI {
	t.Helper()
	api, err := NewUIAPI(client, DeviceIdentity{
		DeviceID:   "device-1",
		DeviceName: "Child PC",
		Platform:   "windows",
	})
	if err != nil {
		t.Fatalf("NewUIAPI() error = %v", err)
	}
	return api
}

func performUIRequest(
	t *testing.T,
	api *UIAPI,
	request uiTestRequest,
) *httptest.ResponseRecorder {
	t.Helper()
	httpRequest := httptest.NewRequest(request.method, request.path, strings.NewReader(request.body))
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, httpRequest)
	return response
}

func decodeUIResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
