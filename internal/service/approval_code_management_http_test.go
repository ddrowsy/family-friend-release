package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestManagementApprovalCodeHTTPSetReplaceAndIsolation(t *testing.T) {
	store, handler, db := newHTTPTestHandler(t)
	store.approvalCodeIterations = testApprovalCodeIterations
	ctx := context.Background()

	for _, customerID := range []CustomerID{"customer-1", "customer-2"} {
		if err := store.CreateProfile(ctx, customerID, "child"); err != nil {
			t.Fatalf("CreateProfile %s: %v", customerID, err)
		}
	}
	if err := store.SetApprovalCode(ctx, "customer-2", "other-code"); err != nil {
		t.Fatalf("SetApprovalCode customer-2: %v", err)
	}

	managementHandler := withManagementCaller(
		handler,
		ManagementCaller{CustomerID: "customer-1"},
	)
	const initialCode = "initial-secret-code"
	response := serveHTTP(
		t,
		managementHandler,
		testHTTPRequest{
			method: http.MethodPut,
			path:   managementApprovalCodeRoute,
			body: managementApprovalCodeHTTPBody{
				ApprovalCode: initialCode,
			},
		},
	)
	assertStatus(t, response, http.StatusNoContent)
	if err := store.VerifyApprovalCode(ctx, "customer-1", initialCode); err != nil {
		t.Fatalf("VerifyApprovalCode initial: %v", err)
	}

	var persisted []byte
	if err := db.QueryRow(
		`SELECT approval_code_hash FROM parent_credentials WHERE customer_id = ?`,
		"customer-1",
	).Scan(&persisted); err != nil {
		t.Fatalf("read persisted verifier: %v", err)
	}
	if bytes.Contains(persisted, []byte(initialCode)) {
		t.Fatal("persisted verifier contains plaintext approval code")
	}

	for attempt := 0; attempt < maxApprovalCodeFailures; attempt++ {
		_ = store.VerifyApprovalCode(ctx, "customer-1", "wrong-code")
	}
	if err := store.VerifyApprovalCode(ctx, "customer-1", initialCode); !errors.Is(err, ErrApprovalCodeThrottled) {
		t.Fatalf("locked verification error = %v, want ErrApprovalCodeThrottled", err)
	}

	const replacementCode = "replacement-secret-code"
	response = serveHTTP(
		t,
		managementHandler,
		testHTTPRequest{
			method: http.MethodPut,
			path:   managementApprovalCodeRoute,
			body: managementApprovalCodeHTTPBody{
				ApprovalCode: replacementCode,
			},
		},
	)
	assertStatus(t, response, http.StatusNoContent)
	if err := store.VerifyApprovalCode(ctx, "customer-1", replacementCode); err != nil {
		t.Fatalf("VerifyApprovalCode replacement: %v", err)
	}
	if err := store.VerifyApprovalCode(ctx, "customer-1", initialCode); !errors.Is(err, ErrInvalidApprovalCode) {
		t.Fatalf("old approval code error = %v, want ErrInvalidApprovalCode", err)
	}
	if err := store.VerifyApprovalCode(ctx, "customer-2", "other-code"); err != nil {
		t.Fatalf("customer-2 approval code changed: %v", err)
	}
}

func TestManagementApprovalCodeHTTPRejectsUnauthenticatedAndInvalidInput(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	store.approvalCodeIterations = testApprovalCodeIterations
	if err := store.CreateProfile(context.Background(), "customer-1", "child"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPut,
			path:   managementApprovalCodeRoute,
			body: managementApprovalCodeHTTPBody{
				ApprovalCode: "must-not-leak",
			},
		},
	)
	assertStatus(t, response, http.StatusUnauthorized)
	if strings.Contains(response.Body.String(), "must-not-leak") {
		t.Fatal("unauthorized response leaked approval code")
	}

	managementHandler := withManagementCaller(
		handler,
		ManagementCaller{CustomerID: "customer-1"},
	)
	tests := []struct {
		name string
		code string
	}{
		{name: "blank", code: "   "},
		{name: "too long", code: strings.Repeat("x", maxApprovalCodeLength+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveHTTP(
				t,
				managementHandler,
				testHTTPRequest{
					method: http.MethodPut,
					path:   managementApprovalCodeRoute,
					body: managementApprovalCodeHTTPBody{
						ApprovalCode: test.code,
					},
				},
			)
			assertStatus(t, response, http.StatusBadRequest)
		})
	}
}
