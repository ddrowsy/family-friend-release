package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBrowserAccessRequestHTTPDeviceFlow(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	ctx := context.Background()
	if err := createBrowserApprovalProfile(ctx, store, "customer-1", "harry"); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := registerAndLinkBrowserApprovalDevice(
		ctx,
		store,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("link device: %v", err)
	}

	body := createBrowserAccessRequestHTTPBody{
		ProfileID:            "harry",
		URL:                  "https://example.com/home",
		BrowserPolicyProfile: "study",
	}
	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   deviceBrowserAccessRequestsRoute,
			body:   body,
		},
	)
	assertStatus(t, response, http.StatusUnauthorized)

	deviceHandler := withDeviceCaller(
		handler,
		DeviceCaller{CustomerID: "customer-1", DeviceID: "laptop"},
	)
	response = serveHTTP(
		t,
		deviceHandler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   deviceBrowserAccessRequestsRoute,
			body:   body,
		},
	)
	assertStatus(t, response, http.StatusCreated)
	responseBody := response.Body.String()
	if strings.Contains(responseBody, "customer") || strings.Contains(responseBody, "device_id") {
		t.Fatalf("device response leaks caller identity: %s", responseBody)
	}
	var created browserAccessRequestStatusResponse
	if err := json.Unmarshal([]byte(responseBody), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.RequestID == "" || created.State != BrowserAccessRequestPending {
		t.Fatalf("created response = %#v", created)
	}

	body.URL = "https://example.com/other"
	response = serveHTTP(
		t,
		deviceHandler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   deviceBrowserAccessRequestsRoute,
			body:   body,
		},
	)
	assertStatus(t, response, http.StatusCreated)
	var duplicate browserAccessRequestStatusResponse
	decodeHTTPResponse(t, response, &duplicate)
	if duplicate.RequestID != created.RequestID {
		t.Fatalf("duplicate request ID = %q, want %q", duplicate.RequestID, created.RequestID)
	}

	response = serveHTTP(
		t,
		deviceHandler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   deviceBrowserAccessRequestsRoute + "/" + created.RequestID,
		},
	)
	assertStatus(t, response, http.StatusOK)

	otherDeviceHandler := withDeviceCaller(
		handler,
		DeviceCaller{CustomerID: "customer-1", DeviceID: "other-device"},
	)
	response = serveHTTP(
		t,
		otherDeviceHandler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   deviceBrowserAccessRequestsRoute + "/" + created.RequestID,
		},
	)
	assertStatus(t, response, http.StatusNotFound)
}

func TestBrowserAccessRequestHTTPCreateValidation(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	ctx := context.Background()
	if err := createBrowserApprovalProfile(ctx, store, "customer-1", "harry"); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := createBrowserApprovalProfile(ctx, store, "customer-1", "james"); err != nil {
		t.Fatalf("create second profile: %v", err)
	}
	if err := registerAndLinkBrowserApprovalDevice(
		ctx,
		store,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("link device: %v", err)
	}

	handler = withDeviceCaller(
		handler,
		DeviceCaller{CustomerID: "customer-1", DeviceID: "laptop"},
	)
	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name: "invalid profile ID",
			body: createBrowserAccessRequestHTTPBody{
				ProfileID:            " ",
				URL:                  "example.com",
				BrowserPolicyProfile: "study",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unknown browser profile",
			body: createBrowserAccessRequestHTTPBody{
				ProfileID:            "harry",
				URL:                  "example.com",
				BrowserPolicyProfile: "missing",
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "invalid URL",
			body: createBrowserAccessRequestHTTPBody{
				ProfileID:            "harry",
				URL:                  "not a host/path",
				BrowserPolicyProfile: "study",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unlinked profile",
			body: createBrowserAccessRequestHTTPBody{
				ProfileID:            "james",
				URL:                  "example.com",
				BrowserPolicyProfile: "study",
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "child cannot choose approval action",
			body: map[string]any{
				"profile_id":             "harry",
				"url":                    "example.com",
				"browser_policy_profile": "study",
				"action":                 "permanent",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveHTTP(
				t,
				handler,
				testHTTPRequest{
					method: http.MethodPost,
					path:   deviceBrowserAccessRequestsRoute,
					body:   test.body,
				},
			)
			assertStatus(t, response, test.wantStatus)
		})
	}
}

func TestBrowserAccessRequestHTTPManagementFlow(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	ctx := context.Background()
	if err := createBrowserApprovalProfile(ctx, store, "customer-1", "harry"); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := registerAndLinkBrowserApprovalDevice(
		ctx,
		store,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("link device: %v", err)
	}

	request, err := store.CreateBrowserAccessRequest(
		ctx,
		"customer-1",
		"harry",
		"laptop",
		"example.com",
		"study",
	)
	if err != nil {
		t.Fatalf("CreateBrowserAccessRequest: %v", err)
	}

	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   managementBrowserAccessRequestsRoute,
		},
	)
	assertStatus(t, response, http.StatusUnauthorized)

	managementHandler := withManagementCaller(
		handler,
		ManagementCaller{CustomerID: "customer-1"},
	)
	response = serveHTTP(
		t,
		managementHandler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   managementBrowserAccessRequestsRoute,
		},
	)
	assertStatus(t, response, http.StatusOK)
	var pending []managementBrowserAccessRequestResponse
	decodeHTTPResponse(t, response, &pending)
	if len(pending) != 1 || pending[0].RequestID != request.ID {
		t.Fatalf("pending requests = %#v", pending)
	}

	otherManagementHandler := withManagementCaller(
		handler,
		ManagementCaller{CustomerID: "customer-2"},
	)
	response = serveHTTP(
		t,
		otherManagementHandler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   managementBrowserAccessRequestsRoute + "/" + request.ID,
		},
	)
	assertStatus(t, response, http.StatusNotFound)

	response = serveHTTP(
		t,
		managementHandler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   managementBrowserAccessRequestsRoute + "/" + request.ID + "/approve",
			body: browserAccessDecisionHTTPBody{
				Action:          BrowserApprovalTemporary,
				DurationSeconds: 60,
			},
		},
	)
	assertStatus(t, response, http.StatusOK)
	var approved managementBrowserAccessRequestResponse
	decodeHTTPResponse(t, response, &approved)
	hasExpectedDecision := approved.State == BrowserAccessRequestApproved &&
		approved.Action == BrowserApprovalTemporary
	hasExpectedDuration := approved.DurationSeconds == 60
	if !hasExpectedDecision || !hasExpectedDuration {
		t.Fatalf("approved response = %#v", approved)
	}

	deviceHandler := withDeviceCaller(
		handler,
		DeviceCaller{CustomerID: "customer-1", DeviceID: "laptop"},
	)
	response = serveHTTP(
		t,
		deviceHandler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   deviceBrowserAccessRequestsRoute + "/" + request.ID,
		},
	)
	assertStatus(t, response, http.StatusOK)
	var status browserAccessRequestStatusResponse
	decodeHTTPResponse(t, response, &status)
	if status.State != BrowserAccessRequestApproved || status.Action != BrowserApprovalTemporary {
		t.Fatalf("device status = %#v", status)
	}
}

func TestBrowserAccessRequestHTTPRejectAndDurationValidation(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	ctx := context.Background()
	if err := createBrowserApprovalProfile(ctx, store, "customer-1", "harry"); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := registerAndLinkBrowserApprovalDevice(
		ctx,
		store,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("link device: %v", err)
	}

	request, err := store.CreateBrowserAccessRequest(
		ctx,
		"customer-1",
		"harry",
		"laptop",
		"reject.example.com",
		"study",
	)
	if err != nil {
		t.Fatalf("CreateBrowserAccessRequest: %v", err)
	}
	managementHandler := withManagementCaller(
		handler,
		ManagementCaller{CustomerID: "customer-1"},
	)

	response := serveHTTP(
		t,
		managementHandler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   managementBrowserAccessRequestsRoute + "/" + request.ID + "/approve",
			body: browserAccessDecisionHTTPBody{
				Action:          BrowserApprovalTemporary,
				DurationSeconds: maxTemporaryBrowserApprovalDurationSeconds + 1,
			},
		},
	)
	assertStatus(t, response, http.StatusBadRequest)

	response = serveHTTP(
		t,
		managementHandler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   managementBrowserAccessRequestsRoute + "/" + request.ID + "/reject",
		},
	)
	assertStatus(t, response, http.StatusOK)
	var rejected managementBrowserAccessRequestResponse
	decodeHTTPResponse(t, response, &rejected)
	if rejected.State != BrowserAccessRequestRejected {
		t.Fatalf("rejected response = %#v", rejected)
	}
}

func TestBrowserAccessRequestHTTPDeviceStatusExpiresRequest(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	ctx := context.Background()
	now := time.Date(2026, time.September, 23, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	if err := createBrowserApprovalProfile(ctx, store, "customer-1", "harry"); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := registerAndLinkBrowserApprovalDevice(
		ctx,
		store,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("link device: %v", err)
	}

	request, err := store.CreateBrowserAccessRequest(
		ctx,
		"customer-1",
		"harry",
		"laptop",
		"expired.example.com",
		"study",
	)
	if err != nil {
		t.Fatalf("CreateBrowserAccessRequest: %v", err)
	}
	now = request.ExpiresAt.Add(time.Second)

	handler = withDeviceCaller(
		handler,
		DeviceCaller{CustomerID: "customer-1", DeviceID: "laptop"},
	)
	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   deviceBrowserAccessRequestsRoute + "/" + request.ID,
		},
	)
	assertStatus(t, response, http.StatusOK)
	var status browserAccessRequestStatusResponse
	decodeHTTPResponse(t, response, &status)
	if status.State != BrowserAccessRequestExpired {
		t.Fatalf("status = %#v", status)
	}
}

func withDeviceCaller(handler http.Handler, caller DeviceCaller) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithDeviceCaller(r.Context(), caller)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withManagementCaller(handler http.Handler, caller ManagementCaller) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithManagementCaller(r.Context(), caller)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}
