package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/service"
)

func TestAgentCreateBrowserAccessRequest(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, time.September, 23, 4, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/browser/access-requests" {
			t.Errorf("path = %q", r.URL.Path)
		}

		var request service.DeviceBrowserAccessRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.ProfileID != "harry" || request.BrowserPolicyProfile != "study" {
			t.Errorf("request = %#v", request)
		}
		writeTestJSON(t, w, service.DeviceBrowserAccessStatus{
			RequestID: "request-1",
			State:     service.BrowserAccessRequestPending,
			ExpiresAt: expiresAt,
		})
	}))
	defer server.Close()

	client := newTestAgentClient(t, server)
	status, err := client.CreateBrowserAccessRequest(
		t.Context(),
		service.DeviceBrowserAccessRequest{
			ProfileID:            "harry",
			URL:                  "https://example.com/private",
			BrowserPolicyProfile: "study",
		},
	)
	if err != nil {
		t.Fatalf("CreateBrowserAccessRequest() error = %v", err)
	}
	if status.RequestID != "request-1" || status.State != service.BrowserAccessRequestPending {
		t.Fatalf("status = %#v", status)
	}
	if !status.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expires_at = %v, want %v", status.ExpiresAt, expiresAt)
	}
}

func TestAgentBrowserAccessRequestStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/browser/access-requests/request-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		writeTestJSON(t, w, service.DeviceBrowserAccessStatus{
			RequestID: "request-1",
			State:     service.BrowserAccessRequestApproved,
		})
	}))
	defer server.Close()

	client := newTestAgentClient(t, server)
	status, err := client.BrowserAccessRequestStatus(t.Context(), "request-1")
	if err != nil {
		t.Fatalf("BrowserAccessRequestStatus() error = %v", err)
	}
	if status.State != service.BrowserAccessRequestApproved {
		t.Fatalf("state = %q, want approved", status.State)
	}
}

func TestAgentBrowserAccessRequestStatusRejectsEmptyID(t *testing.T) {
	t.Parallel()

	client, err := NewAgent("https://example.invalid", nil)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if _, err := client.BrowserAccessRequestStatus(t.Context(), "  "); err == nil {
		t.Fatal("BrowserAccessRequestStatus() error = nil")
	}
}
