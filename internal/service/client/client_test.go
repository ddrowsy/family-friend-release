package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
	"github.com/ddrowsy/family-friend-release/internal/service"
)

func TestAgentProfileConfiguration(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/customers/customer-1/profiles/profile-2/config" {
			t.Errorf("path = %q", r.URL.Path)
		}
		writeTestJSON(t, w, service.ProfileConfiguration{
			CustomerID: "customer-1",
			ProfileID:  "profile-2",
			Revision:   7,
			Policy: policy.ProfilePolicy{
				SchemaVersion: 1,
				Version:       "v7",
				Modules:       map[string]policy.ModulePolicy{},
			},
		})
	}))
	defer server.Close()

	client := newTestAgentClient(t, server)
	configuration, err := client.ProfileConfiguration(
		t.Context(),
		"customer-1",
		"profile-2",
	)
	if err != nil {
		t.Fatalf("ProfileConfiguration() error = %v", err)
	}
	if configuration.Revision != 7 {
		t.Fatalf("revision = %d, want 7", configuration.Revision)
	}
}

func TestManagementPutProfileConfiguration(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var profilePolicy policy.ProfilePolicy
		if err := json.NewDecoder(r.Body).Decode(&profilePolicy); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if profilePolicy.Version != "v2" {
			t.Errorf("version = %q, want v2", profilePolicy.Version)
		}
		writeTestJSON(t, w, service.ProfileConfiguration{
			CustomerID: "customer-1",
			ProfileID:  "profile-1",
			Revision:   2,
			Policy:     profilePolicy,
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	profilePolicy := policy.ProfilePolicy{
		SchemaVersion: 1,
		Version:       "v2",
		Modules:       map[string]policy.ModulePolicy{},
	}
	configuration, err := client.PutProfileConfiguration(
		t.Context(),
		"customer-1",
		"profile-1",
		profilePolicy,
	)
	if err != nil {
		t.Fatalf("PutProfileConfiguration() error = %v", err)
	}
	if configuration.Revision != 2 {
		t.Fatalf("revision = %d, want 2", configuration.Revision)
	}
}

func TestManagementSetActiveBrowserProfile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != "/api/v1/customers/customer-1/profiles/profile-1/browser/active-profile" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var request service.ActiveBrowserProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Name != "study" || request.DurationSeconds != 900 {
			t.Errorf("request = %#v", request)
		}
		writeTestJSON(t, w, service.ProfileConfiguration{
			CustomerID: "customer-1",
			ProfileID:  "profile-1",
			Revision:   2,
			Policy: policy.ProfilePolicy{
				SchemaVersion: 1,
				Version:       "v1",
				Modules:       map[string]policy.ModulePolicy{},
			},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	configuration, err := client.SetActiveBrowserProfile(
		t.Context(),
		"customer-1",
		"profile-1",
		service.ActiveBrowserProfileRequest{
			Name:            "study",
			DurationSeconds: 900,
		},
	)
	if err != nil {
		t.Fatalf("SetActiveBrowserProfile() error = %v", err)
	}
	if configuration.Revision != 2 {
		t.Fatalf("revision = %d, want 2", configuration.Revision)
	}
}

func TestAgentDeviceOperationsSupportMultipleProfiles(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/customers/customer-1/devices/device-1":
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/customers/customer-1/devices/device-1/profiles":
			writeTestJSON(t, w, service.DeviceProfileAssignment{
				CustomerID: "customer-1",
				DeviceID:   "device-1",
				ProfileIDs: []service.ProfileID{"profile-a", "profile-b"},
			})
		case "/api/v1/customers/customer-1/devices/device-1/profiles/profile-a":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestAgentClient(t, server)
	if err := client.RegisterDevice(t.Context(), "customer-1", "device-1"); err != nil {
		t.Fatalf("RegisterDevice() error = %v", err)
	}
	assignment, err := client.DeviceProfiles(
		t.Context(),
		"customer-1",
		"device-1",
	)
	if err != nil {
		t.Fatalf("DeviceProfiles() error = %v", err)
	}
	if len(assignment.ProfileIDs) != 2 {
		t.Fatalf("profile count = %d, want 2", len(assignment.ProfileIDs))
	}
	linked, err := client.DeviceProfileLinked(
		t.Context(),
		"customer-1",
		"device-1",
		"profile-a",
	)
	if err != nil || !linked {
		t.Fatalf("DeviceProfileLinked() = %v, %v; want true, nil", linked, err)
	}
	linked, err = client.DeviceProfileLinked(
		t.Context(),
		"customer-1",
		"device-1",
		"missing",
	)
	if err != nil || linked {
		t.Fatalf("DeviceProfileLinked() = %v, %v; want false, nil", linked, err)
	}
}

func TestAgentBrowserApprovalMethodsSupplyDeviceAndAction(t *testing.T) {
	t.Parallel()

	requests := make(chan service.BrowserApprovalRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request service.BrowserApprovalRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests <- request
		writeTestJSON(t, w, service.BrowserApprovalResult{DeviceID: request.DeviceID})
	}))
	defer server.Close()

	client := newTestAgentClient(t, server)
	request := service.BrowserApprovalRequest{
		DeviceID:             "device-1",
		URL:                  "https://example.com",
		BrowserPolicyProfile: "study",
		ApprovalCode:         "secret-code",
		DurationSeconds:      300,
	}
	if _, err := client.ApproveBrowserTemporary(
		t.Context(),
		"customer-1",
		"profile-1",
		request,
	); err != nil {
		t.Fatalf("ApproveBrowserTemporary() error = %v", err)
	}
	if _, err := client.ApproveBrowserPermanent(
		t.Context(),
		"customer-1",
		"profile-1",
		request,
	); err != nil {
		t.Fatalf("ApproveBrowserPermanent() error = %v", err)
	}

	temporary := <-requests
	permanent := <-requests
	if temporary.DeviceID != "device-1" || temporary.Action != service.BrowserApprovalTemporary {
		t.Fatalf("temporary request = %#v", temporary)
	}
	if temporary.DurationSeconds != 300 {
		t.Fatalf("temporary duration = %d, want 300", temporary.DurationSeconds)
	}
	if permanent.DeviceID != "device-1" || permanent.Action != service.BrowserApprovalPermanent {
		t.Fatalf("permanent request = %#v", permanent)
	}
	if permanent.DurationSeconds != 0 {
		t.Fatalf("permanent duration = %d, want 0", permanent.DurationSeconds)
	}
}

func TestAgentApprovalCodeDoesNotLeakThroughResponseError(t *testing.T) {
	t.Parallel()

	const approvalCode = "top-secret-approval-code"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		writeTestJSON(t, w, map[string]string{"error": approvalCode})
	}))
	defer server.Close()

	client := newTestAgentClient(t, server)
	_, err := client.ApproveBrowserPermanent(
		t.Context(),
		"customer-1",
		"profile-1",
		service.BrowserApprovalRequest{
			DeviceID:             "device-1",
			URL:                  "https://example.com",
			BrowserPolicyProfile: "study",
			ApprovalCode:         approvalCode,
		},
	)
	if err == nil {
		t.Fatal("ApproveBrowserPermanent() error = nil")
	}
	if strings.Contains(err.Error(), approvalCode) {
		t.Fatalf("error leaks approval code: %v", err)
	}
	var responseErr *ResponseError
	if !errors.As(err, &responseErr) {
		t.Fatalf("error = %v, want ResponseError", err)
	}
	if responseErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", responseErr.StatusCode)
	}
}

func TestAgentContextCancellation(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	client := newTestAgentClient(t, server)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := client.ProfileConfiguration(ctx, "customer-1", "profile-1")
		done <- err
	}()
	<-started
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not stop after context cancellation")
	}
}

func TestAgentResponseSizeLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBodyBytes+1)))
	}))
	defer server.Close()

	client := newTestAgentClient(t, server)
	_, err := client.ProfileConfiguration(
		t.Context(),
		"customer-1",
		"profile-1",
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("error = %v, want response size error", err)
	}
}

func newTestAgentClient(t *testing.T, server *httptest.Server) *AgentClient {
	t.Helper()
	client, err := NewAgent(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	return client
}

func newTestManagementClient(t *testing.T, server *httptest.Server) *ManagementClient {
	t.Helper()
	client, err := NewManagement(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewManagement() error = %v", err)
	}
	return client
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
