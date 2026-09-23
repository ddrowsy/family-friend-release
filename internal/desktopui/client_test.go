package desktopui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/agent"
)

func TestHTTPAgentClientStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != agent.UIStatusPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(agent.UIStatus{
			AgentRunning:      true,
			ProtectionRunning: true,
			Service:           agent.UIServiceConnected,
			Paired:            true,
			Pairing:           agent.UIPairingConfirmed,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewHTTPAgentClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewHTTPAgentClient() error = %v", err)
	}
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.AgentRunning || !status.ProtectionRunning || !status.Paired {
		t.Fatalf("status = %+v", status)
	}
}

func TestHTTPAgentClientRejectsNonLoopbackURL(t *testing.T) {
	if _, err := NewHTTPAgentClient("https://example.com", nil); err == nil {
		t.Fatal("NewHTTPAgentClient() non-loopback error = nil")
	}
	if _, err := NewHTTPAgentClient("http://192.0.2.10:17654", nil); err == nil {
		t.Fatal("NewHTTPAgentClient() remote IP error = nil")
	}
}

func TestHTTPAgentClientReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := NewHTTPAgentClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewHTTPAgentClient() error = %v", err)
	}
	if _, err := client.Status(context.Background()); err == nil {
		t.Fatal("Status() HTTP failure error = nil")
	}
}
