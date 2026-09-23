package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"
)

func TestManagementPairingServiceCreateAndHeartbeat(t *testing.T) {
	ctx := context.Background()
	store := newPairingStoreWithCustomers(
		t,
		ctx,
		"customer-1",
		"customer-2",
	)
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

	service := newManagementPairingService(store)
	codes := []string{"111111", "111111", "222222", "222222", "333333"}
	service.generateCode = func() (string, error) {
		code := codes[0]
		codes = codes[1:]
		return code, nil
	}

	first, err := service.create(ctx, "customer-1")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if first.Code != "111111" {
		t.Fatalf("first code = %q", first.Code)
	}
	if !first.ExpiresAt.Equal(now.Add(35 * time.Second)) {
		t.Fatalf("first expiry = %v", first.ExpiresAt)
	}

	replacement, err := service.create(ctx, "customer-1")
	if err != nil {
		t.Fatalf("create replacement: %v", err)
	}
	if replacement.ID == first.ID {
		t.Fatal("replacement reused session ID")
	}
	if replacement.Code != "222222" {
		t.Fatalf("replacement code = %q", replacement.Code)
	}
	if _, err := store.PairingSession(
		ctx,
		"customer-1",
		first.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replaced session error = %v, want ErrNotFound", err)
	}

	now = now.Add(29 * time.Second)
	unchanged, err := service.heartbeat(ctx, "customer-1", replacement.ID)
	if err != nil {
		t.Fatalf("heartbeat unchanged: %v", err)
	}
	if unchanged.Code != "222222" || !unchanged.ExpiresAt.Equal(replacement.ExpiresAt) {
		t.Fatalf("unchanged heartbeat = %#v", unchanged)
	}

	now = now.Add(time.Second)
	rotated, err := service.heartbeat(ctx, "customer-1", replacement.ID)
	if err != nil {
		t.Fatalf("heartbeat rotated: %v", err)
	}
	if rotated.Code != "333333" {
		t.Fatalf("rotated code = %q", rotated.Code)
	}
	if !rotated.ExpiresAt.Equal(now.Add(35 * time.Second)) {
		t.Fatalf("rotated expiry = %v", rotated.ExpiresAt)
	}

	claimed, err := store.ClaimPairingSession(
		ctx,
		rotated.Code,
		PairingDeviceClaim{
			DeviceID:   "device-1",
			DeviceName: "Harry Laptop",
			Platform:   "linux",
		},
		[]byte{},
	)
	if err != nil {
		t.Fatalf("ClaimPairingSession: %v", err)
	}
	pending, err := service.heartbeat(ctx, "customer-1", claimed.ID)
	if err != nil {
		t.Fatalf("heartbeat pending: %v", err)
	}
	if pending.State != PairingStatePendingConfirmation {
		t.Fatalf("pending state = %q", pending.State)
	}
	if pending.DeviceID != "device-1" || pending.Code != "" {
		t.Fatalf("pending session = %#v", pending)
	}

	if _, err := service.heartbeat(
		ctx,
		"customer-2",
		claimed.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other customer heartbeat error = %v, want ErrNotFound", err)
	}
}

func TestManagementPairingServiceRejectsExpiredWaitingSession(t *testing.T) {
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

	service := newManagementPairingService(store)
	service.generateCode = func() (string, error) { return "111111", nil }
	session, err := service.create(ctx, "customer-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	now = now.Add(36 * time.Second)
	if _, err := service.heartbeat(
		ctx,
		"customer-1",
		session.ID,
	); !errors.Is(err, ErrPairingSessionExpired) {
		t.Fatalf("expired heartbeat error = %v", err)
	}
}

type managementPairingHTTPRequest struct {
	method     string
	path       string
	customerID CustomerID
}

func TestManagementPairingHTTPCreateHeartbeatAndCallerBoundary(t *testing.T) {
	ctx := context.Background()
	store := newPairingStoreWithCustomers(
		t,
		ctx,
		"customer-1",
		"customer-2",
	)
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

	response := serveManagementPairingRequest(
		t,
		handler,
		managementPairingHTTPRequest{
			method: http.MethodPost,
			path:   managementPairingRoute,
		},
	)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing caller status = %d", response.Code)
	}

	response = serveManagementPairingRequest(
		t,
		handler,
		managementPairingHTTPRequest{
			method:     http.MethodPost,
			path:       managementPairingRoute,
			customerID: "customer-1",
		},
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body = %s", response.Code, response.Body.String())
	}
	createBody := response.Body.Bytes()
	var created ManagementPairingResponse
	if err := json.Unmarshal(createBody, &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	var createFields map[string]any
	if err := json.Unmarshal(createBody, &createFields); err != nil {
		t.Fatalf("decode create fields: %v", err)
	}
	if _, ok := createFields["customer_id"]; ok {
		t.Fatal("create response exposed customer_id")
	}
	if created.SessionID == "" || created.State != PairingStateWaiting {
		t.Fatalf("create response = %#v", created)
	}
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(created.Code) {
		t.Fatalf("create code = %q", created.Code)
	}
	if !created.ExpiresAt.Equal(now.Add(35 * time.Second)) {
		t.Fatalf("create expiry = %v", created.ExpiresAt)
	}

	heartbeatPath := managementPairingRoute + "/" + created.SessionID + "/heartbeat"
	response = serveManagementPairingRequest(
		t,
		handler,
		managementPairingHTTPRequest{
			method:     http.MethodPost,
			path:       heartbeatPath,
			customerID: "customer-2",
		},
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("other caller status = %d", response.Code)
	}

	claimed, err := store.ClaimPairingSession(
		ctx,
		created.Code,
		PairingDeviceClaim{
			DeviceID:   "device-1",
			DeviceName: "Harry Laptop",
			Platform:   "linux",
		},
		[]byte{},
	)
	if err != nil {
		t.Fatalf("ClaimPairingSession: %v", err)
	}

	response = serveManagementPairingRequest(
		t,
		handler,
		managementPairingHTTPRequest{
			method:     http.MethodPost,
			path:       heartbeatPath,
			customerID: "customer-1",
		},
	)
	if response.Code != http.StatusOK {
		t.Fatalf("pending heartbeat status = %d; body = %s", response.Code, response.Body.String())
	}
	var pending ManagementPairingResponse
	if err := json.NewDecoder(response.Body).Decode(&pending); err != nil {
		t.Fatalf("decode pending response: %v", err)
	}
	if pending.State != PairingStatePendingConfirmation || pending.Code != "" {
		t.Fatalf("pending response = %#v", pending)
	}
	if pending.Device == nil || pending.Device.DeviceID != claimed.DeviceID {
		t.Fatalf("pending device = %#v", pending.Device)
	}
}

func serveManagementPairingRequest(
	t *testing.T,
	handler http.Handler,
	testRequest managementPairingHTTPRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(testRequest.method, testRequest.path, nil)
	if testRequest.customerID != "" {
		request = request.WithContext(
			WithManagementCaller(
				request.Context(),
				ManagementCaller{CustomerID: testRequest.customerID},
			),
		)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
