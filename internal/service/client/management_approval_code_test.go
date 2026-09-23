package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestManagementSetApprovalCode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != managementApprovalCodePath {
			t.Errorf("path = %q, want %q", r.URL.Path, managementApprovalCodePath)
		}

		var request managementApprovalCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.ApprovalCode != "new-secret-code" {
			t.Errorf("approval code = %q, want new-secret-code", request.ApprovalCode)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	if err := client.SetApprovalCode(t.Context(), "new-secret-code"); err != nil {
		t.Fatalf("SetApprovalCode() error = %v", err)
	}
}
