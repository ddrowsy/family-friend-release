package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/service"
	serviceclient "github.com/ddrowsy/family-friend-release/internal/service/client"
)

type browserApprovalClientStub struct {
	createStatus service.DeviceBrowserAccessStatus
	createErr    error
	createInput  service.DeviceBrowserAccessRequest
	createCalls  int

	statusResult service.DeviceBrowserAccessStatus
	statusErr    error
	statusID     string

	temporaryInput service.BrowserApprovalRequest
	temporaryErr   error
	temporaryCalls int
	permanentInput service.BrowserApprovalRequest
	permanentErr   error
	permanentCalls int
}

func (c *browserApprovalClientStub) CreateBrowserAccessRequest(
	_ context.Context,
	request service.DeviceBrowserAccessRequest,
) (service.DeviceBrowserAccessStatus, error) {
	c.createCalls++
	c.createInput = request
	return c.createStatus, c.createErr
}

func (c *browserApprovalClientStub) BrowserAccessRequestStatus(
	_ context.Context,
	requestID string,
) (service.DeviceBrowserAccessStatus, error) {
	c.statusID = requestID
	return c.statusResult, c.statusErr
}

func (c *browserApprovalClientStub) ApproveBrowserTemporary(
	_ context.Context,
	_ service.CustomerID,
	_ service.ProfileID,
	request service.BrowserApprovalRequest,
) (service.BrowserApprovalResult, error) {
	c.temporaryCalls++
	c.temporaryInput = request
	return service.BrowserApprovalResult{}, c.temporaryErr
}

func (c *browserApprovalClientStub) ApproveBrowserPermanent(
	_ context.Context,
	_ service.CustomerID,
	_ service.ProfileID,
	request service.BrowserApprovalRequest,
) (service.BrowserApprovalResult, error) {
	c.permanentCalls++
	c.permanentInput = request
	return service.BrowserApprovalResult{}, c.permanentErr
}

func TestBrowserApprovalAPICreateRequest(t *testing.T) {
	expiresAt := time.Date(2026, time.September, 23, 5, 0, 0, 0, time.UTC)
	client := &browserApprovalClientStub{
		createStatus: service.DeviceBrowserAccessStatus{
			RequestID: "request-1",
			State:     service.BrowserAccessRequestPending,
			ExpiresAt: expiresAt,
		},
	}
	handler := newBrowserApprovalTestHandler(t, client)

	response := serveBrowserApprovalRequest(
		t,
		handler,
		http.MethodPost,
		BrowserAccessRequestsPath,
		map[string]any{
			"url":             "https://example.com/private",
			"browser_profile": "study",
		},
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if client.createInput.ProfileID != "harry" {
		t.Fatalf("profile ID = %q, want harry", client.createInput.ProfileID)
	}
	if client.createInput.URL != "https://example.com/private" {
		t.Fatalf("URL = %q", client.createInput.URL)
	}

	var body map[string]any
	decodeBrowserApprovalResponse(t, response, &body)
	if body["request_id"] != "request-1" || body["state"] != "pending" {
		t.Fatalf("response = %#v", body)
	}
	for _, forbidden := range []string{"customer_id", "device_id", "profile_id"} {
		if _, exists := body[forbidden]; exists {
			t.Fatalf("response exposed %s", forbidden)
		}
	}
}

func TestBrowserApprovalAPICreateRequestRejectsChildDecision(t *testing.T) {
	client := &browserApprovalClientStub{}
	handler := newBrowserApprovalTestHandler(t, client)

	response := serveBrowserApprovalRequest(
		t,
		handler,
		http.MethodPost,
		BrowserAccessRequestsPath,
		map[string]any{
			"url":              "https://example.com",
			"browser_profile":  "study",
			"action":           "permanent",
			"duration_seconds": 3600,
		},
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if client.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", client.createCalls)
	}
}

func TestBrowserApprovalAPIRequestStatusStates(t *testing.T) {
	states := []service.BrowserAccessRequestState{
		service.BrowserAccessRequestPending,
		service.BrowserAccessRequestApproved,
		service.BrowserAccessRequestRejected,
		service.BrowserAccessRequestExpired,
	}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			client := &browserApprovalClientStub{
				statusResult: service.DeviceBrowserAccessStatus{
					RequestID: "request-1",
					State:     state,
				},
			}
			handler := newBrowserApprovalTestHandler(t, client)
			response := serveBrowserApprovalRequest(
				t,
				handler,
				http.MethodGet,
				BrowserAccessRequestsPath+"/request-1",
				nil,
			)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			var body browserAccessStatusResponse
			decodeBrowserApprovalResponse(t, response, &body)
			if body.State != state {
				t.Fatalf("state = %q, want %q", body.State, state)
			}
			if client.statusID != "request-1" {
				t.Fatalf("request ID = %q", client.statusID)
			}
		})
	}
}

func TestBrowserApprovalAPIDirectTemporaryApproval(t *testing.T) {
	client := &browserApprovalClientStub{}
	handler := newBrowserApprovalTestHandler(t, client)
	response := serveBrowserApprovalRequest(
		t,
		handler,
		http.MethodPost,
		BrowserDirectApprovalPath,
		map[string]any{
			"url":              "https://example.com",
			"browser_profile":  "study",
			"approval_code":    "parent-secret",
			"action":           "temporary",
			"duration_seconds": 900,
		},
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if client.temporaryCalls != 1 || client.permanentCalls != 0 {
		t.Fatalf("temporary/permanent calls = %d/%d", client.temporaryCalls, client.permanentCalls)
	}
	if client.temporaryInput.DeviceID != "device-1" || client.temporaryInput.DurationSeconds != 900 {
		t.Fatalf("temporary request = %#v", client.temporaryInput)
	}
	if client.temporaryInput.ApprovalCode != "parent-secret" {
		t.Fatal("parent code was not forwarded unchanged")
	}
}

func TestBrowserApprovalAPIDirectPermanentApproval(t *testing.T) {
	client := &browserApprovalClientStub{}
	handler := newBrowserApprovalTestHandler(t, client)
	response := serveBrowserApprovalRequest(
		t,
		handler,
		http.MethodPost,
		BrowserDirectApprovalPath,
		map[string]any{
			"url":             "https://example.com",
			"browser_profile": "study",
			"approval_code":   "parent-secret",
			"action":          "permanent",
		},
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if client.permanentCalls != 1 || client.temporaryCalls != 0 {
		t.Fatalf("permanent/temporary calls = %d/%d", client.permanentCalls, client.temporaryCalls)
	}
}

func TestBrowserApprovalAPIRejectsInvalidTemporaryDuration(t *testing.T) {
	client := &browserApprovalClientStub{}
	handler := newBrowserApprovalTestHandler(t, client)
	response := serveBrowserApprovalRequest(
		t,
		handler,
		http.MethodPost,
		BrowserDirectApprovalPath,
		map[string]any{
			"url":              "https://example.com",
			"browser_profile":  "study",
			"approval_code":    "parent-secret",
			"action":           "temporary",
			"duration_seconds": 0,
		},
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if client.temporaryCalls != 0 {
		t.Fatalf("temporary calls = %d, want 0", client.temporaryCalls)
	}
}

func TestBrowserApprovalAPIRemoteFailureDoesNotLeakSecret(t *testing.T) {
	client := &browserApprovalClientStub{
		temporaryErr: errors.New("remote failure for parent-secret"),
	}
	handler := newBrowserApprovalTestHandler(t, client)
	response := serveBrowserApprovalRequest(
		t,
		handler,
		http.MethodPost,
		BrowserDirectApprovalPath,
		map[string]any{
			"url":              "https://example.com",
			"browser_profile":  "study",
			"approval_code":    "parent-secret",
			"action":           "temporary",
			"duration_seconds": 300,
		},
	)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
	}
	if strings.Contains(response.Body.String(), "parent-secret") {
		t.Fatalf("response leaked parent code: %s", response.Body.String())
	}
}

func TestBrowserApprovalAPIMapsInvalidParentCode(t *testing.T) {
	client := &browserApprovalClientStub{
		permanentErr: &serviceclient.ResponseError{
			Method:     http.MethodPost,
			Path:       "/remote",
			StatusCode: http.StatusUnauthorized,
		},
	}
	handler := newBrowserApprovalTestHandler(t, client)
	response := serveBrowserApprovalRequest(
		t,
		handler,
		http.MethodPost,
		BrowserDirectApprovalPath,
		map[string]any{
			"url":             "https://example.com",
			"browser_profile": "study",
			"approval_code":   "wrong-code",
			"action":          "permanent",
		},
	)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if strings.Contains(response.Body.String(), "wrong-code") {
		t.Fatalf("response leaked parent code: %s", response.Body.String())
	}
}

func newBrowserApprovalTestHandler(
	t *testing.T,
	client BrowserApprovalClient,
) http.Handler {
	t.Helper()
	api, err := NewBrowserApprovalAPI(
		client,
		BrowserApprovalIdentityResolverFunc(func(context.Context) (BrowserApprovalIdentity, error) {
			return BrowserApprovalIdentity{
				CustomerID: "customer-1",
				ProfileID:  "harry",
				DeviceID:   "device-1",
			}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewBrowserApprovalAPI() error = %v", err)
	}
	return api.Handler()
}

func serveBrowserApprovalRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		requestBody = bytes.NewReader(data)
	}
	request := httptest.NewRequest(method, path, requestBody)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeBrowserApprovalResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	target any,
) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
