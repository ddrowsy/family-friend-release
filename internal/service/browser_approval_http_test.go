package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

type approvalDeviceLink struct {
	customerID CustomerID
	deviceID   DeviceID
	profileID  ProfileID
}

func TestHTTPBrowserApprovalTemporaryAndPermanent(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	store.approvalCodeIterations = testApprovalCodeIterations
	ctx := context.Background()

	if err := createHTTPApprovalProfile(
		ctx,
		store,
		"customer-1",
		"harry",
	); err != nil {
		t.Fatalf("createHTTPApprovalProfile: %v", err)
	}
	for _, deviceID := range []DeviceID{"laptop", "desktop"} {
		if err := registerAndLinkHTTPApprovalDevice(
			ctx,
			store,
			approvalDeviceLink{
				customerID: "customer-1",
				deviceID:   deviceID,
				profileID:  "harry",
			},
		); err != nil {
			t.Fatalf("registerAndLinkHTTPApprovalDevice %s: %v", deviceID, err)
		}
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode: %v", err)
	}

	path := mustProfileBrowserApprovalsPath(t, "customer-1", "harry")
	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   path,
			body: BrowserApprovalRequest{
				DeviceID:             "laptop",
				URL:                  "https://Example.com/path",
				BrowserPolicyProfile: "study",
				ApprovalCode:         "1234",
				Action:               BrowserApprovalTemporary,
				DurationSeconds:      60,
			},
		},
	)
	assertStatus(t, response, http.StatusOK)

	var temporaryResult BrowserApprovalResult
	decodeHTTPResponse(t, response, &temporaryResult)
	if temporaryResult.ExpiresAt == nil {
		t.Fatal("temporary approval result has nil expiry")
	}
	if temporaryResult.Revision != 2 {
		t.Fatalf("temporary revision = %d, want 2", temporaryResult.Revision)
	}
	if temporaryResult.CustomerID != "customer-1" {
		t.Fatalf("temporary customer ID = %q", temporaryResult.CustomerID)
	}
	if temporaryResult.ProfileID != "harry" {
		t.Fatalf("temporary profile ID = %q", temporaryResult.ProfileID)
	}
	if temporaryResult.DeviceID != "laptop" {
		t.Fatalf("temporary device ID = %q", temporaryResult.DeviceID)
	}

	configuration, err := store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration after temporary approval: %v", err)
	}
	study := browserProfileForTest(t, configuration, "study")
	if len(study.TemporaryAllowedURLs) != 1 {
		t.Fatalf(
			"temporary approval count = %d, want 1",
			len(study.TemporaryAllowedURLs),
		)
	}

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   path,
			body: BrowserApprovalRequest{
				DeviceID:             "desktop",
				URL:                  "https://KhanAcademy.org/lesson",
				BrowserPolicyProfile: "study",
				ApprovalCode:         "1234",
				Action:               BrowserApprovalPermanent,
			},
		},
	)
	assertStatus(t, response, http.StatusOK)

	var permanentResult BrowserApprovalResult
	decodeHTTPResponse(t, response, &permanentResult)
	if permanentResult.Revision != 3 {
		t.Fatalf("permanent revision = %d, want 3", permanentResult.Revision)
	}

	configuration, err = store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration: %v", err)
	}
	study = browserProfileForTest(t, configuration, "study")
	if len(study.AllowedURLs) != 1 || study.AllowedURLs[0] != "khanacademy.org" {
		t.Fatalf("allowed URLs = %v, want [khanacademy.org]", study.AllowedURLs)
	}
}

func TestHTTPBrowserApprovalDeviceAndProfileIsolation(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	store.approvalCodeIterations = testApprovalCodeIterations
	ctx := context.Background()

	for _, identity := range []struct {
		customerID CustomerID
		profileID  ProfileID
	}{
		{customerID: "customer-1", profileID: "harry"},
		{customerID: "customer-1", profileID: "james"},
		{customerID: "customer-2", profileID: "harry"},
	} {
		if err := createHTTPApprovalProfile(
			ctx,
			store,
			identity.customerID,
			identity.profileID,
		); err != nil {
			t.Fatalf(
				"createHTTPApprovalProfile %s/%s: %v",
				identity.customerID,
				identity.profileID,
				err,
			)
		}
	}

	if err := registerAndLinkHTTPApprovalDevice(
		ctx,
		store,
		approvalDeviceLink{
			customerID: "customer-1",
			deviceID:   "shared-laptop",
			profileID:  "harry",
		},
	); err != nil {
		t.Fatalf("link shared-laptop/harry: %v", err)
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"shared-laptop",
		"james",
	); err != nil {
		t.Fatalf("link shared-laptop/james: %v", err)
	}
	if err := registerAndLinkHTTPApprovalDevice(
		ctx,
		store,
		approvalDeviceLink{
			customerID: "customer-1",
			deviceID:   "harry-only",
			profileID:  "harry",
		},
	); err != nil {
		t.Fatalf("link harry-only: %v", err)
	}
	if err := registerAndLinkHTTPApprovalDevice(
		ctx,
		store,
		approvalDeviceLink{
			customerID: "customer-2",
			deviceID:   "other-laptop",
			profileID:  "harry",
		},
	); err != nil {
		t.Fatalf("link other-laptop: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode customer-1: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-2", "9876"); err != nil {
		t.Fatalf("SetApprovalCode customer-2: %v", err)
	}

	for _, profileID := range []ProfileID{"harry", "james"} {
		response := serveHTTP(
			t,
			handler,
			testHTTPRequest{
				method: http.MethodPost,
				path: mustProfileBrowserApprovalsPath(
					t,
					"customer-1",
					profileID,
				),
				body: BrowserApprovalRequest{
					DeviceID:             "shared-laptop",
					URL:                  "example.com",
					BrowserPolicyProfile: "study",
					ApprovalCode:         "1234",
					Action:               BrowserApprovalTemporary,
					DurationSeconds:      60,
				},
			},
		)
		assertStatus(t, response, http.StatusOK)
	}

	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path: mustProfileBrowserApprovalsPath(
				t,
				"customer-1",
				"james",
			),
			body: BrowserApprovalRequest{
				DeviceID:             "harry-only",
				URL:                  "example.net",
				BrowserPolicyProfile: "study",
				ApprovalCode:         "1234",
				Action:               BrowserApprovalTemporary,
				DurationSeconds:      60,
			},
		},
	)
	assertStatus(t, response, http.StatusForbidden)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path: mustProfileBrowserApprovalsPath(
				t,
				"customer-2",
				"harry",
			),
			body: BrowserApprovalRequest{
				DeviceID:             "shared-laptop",
				URL:                  "example.net",
				BrowserPolicyProfile: "study",
				ApprovalCode:         "9876",
				Action:               BrowserApprovalTemporary,
				DurationSeconds:      60,
			},
		},
	)
	assertStatus(t, response, http.StatusNotFound)
}

func TestHTTPBrowserApprovalImportantErrors(t *testing.T) {
	store, handler, db := newHTTPTestHandler(t)
	store.approvalCodeIterations = testApprovalCodeIterations
	ctx := context.Background()

	if err := createHTTPApprovalProfile(
		ctx,
		store,
		"customer-1",
		"harry",
	); err != nil {
		t.Fatalf("createHTTPApprovalProfile: %v", err)
	}
	if err := registerAndLinkHTTPApprovalDevice(
		ctx,
		store,
		approvalDeviceLink{
			customerID: "customer-1",
			deviceID:   "laptop",
			profileID:  "harry",
		},
	); err != nil {
		t.Fatalf("registerAndLinkHTTPApprovalDevice: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode: %v", err)
	}

	path := mustProfileBrowserApprovalsPath(t, "customer-1", "harry")
	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method:  http.MethodPost,
			path:    path,
			rawBody: []byte("{"),
		},
	)
	assertStatus(t, response, http.StatusBadRequest)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   path,
			body: BrowserApprovalRequest{
				DeviceID:             "laptop",
				URL:                  "example.com",
				BrowserPolicyProfile: "study",
				ApprovalCode:         "1234",
				Action:               BrowserApprovalTemporary,
				DurationSeconds:      maxTemporaryBrowserApprovalDurationSeconds + 1,
			},
		},
	)
	assertStatus(t, response, http.StatusBadRequest)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   path,
			body: BrowserApprovalRequest{
				DeviceID:             "laptop",
				URL:                  "ftp://example.com/file",
				BrowserPolicyProfile: "study",
				ApprovalCode:         "1234",
				Action:               BrowserApprovalPermanent,
			},
		},
	)
	assertStatus(t, response, http.StatusBadRequest)

	secret := "definitely-not-the-code"
	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   path,
			body: BrowserApprovalRequest{
				DeviceID:             "laptop",
				URL:                  "example.com",
				BrowserPolicyProfile: "study",
				ApprovalCode:         secret,
				Action:               BrowserApprovalPermanent,
			},
		},
	)
	assertStatus(t, response, http.StatusUnauthorized)
	if strings.Contains(response.Body.String(), secret) {
		t.Fatal("approval response echoed parent approval code")
	}

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   path,
			body: BrowserApprovalRequest{
				DeviceID:             "laptop",
				URL:                  "example.com",
				BrowserPolicyProfile: "missing",
				ApprovalCode:         "1234",
				Action:               BrowserApprovalPermanent,
			},
		},
	)
	assertStatus(t, response, http.StatusNotFound)

	for range maxApprovalCodeFailures {
		response = serveHTTP(
			t,
			handler,
			testHTTPRequest{
				method: http.MethodPost,
				path:   path,
				body: BrowserApprovalRequest{
					DeviceID:             "laptop",
					URL:                  "example.com",
					BrowserPolicyProfile: "study",
					ApprovalCode:         "wrong",
					Action:               BrowserApprovalPermanent,
				},
			},
		)
		assertStatus(t, response, http.StatusUnauthorized)
	}
	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   path,
			body: BrowserApprovalRequest{
				DeviceID:             "laptop",
				URL:                  "example.com",
				BrowserPolicyProfile: "study",
				ApprovalCode:         "1234",
				Action:               BrowserApprovalPermanent,
			},
		},
	)
	assertStatus(t, response, http.StatusTooManyRequests)

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   path,
			body: BrowserApprovalRequest{
				DeviceID:             "laptop",
				URL:                  "example.com",
				BrowserPolicyProfile: "study",
				ApprovalCode:         "1234",
				Action:               BrowserApprovalPermanent,
			},
		},
	)
	assertStatus(t, response, http.StatusInternalServerError)
}

func TestHTTPBrowserApprovalMissingDeviceInvalidActionAndDuration(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	store.approvalCodeIterations = testApprovalCodeIterations
	ctx := context.Background()

	if err := createHTTPApprovalProfile(
		ctx,
		store,
		"customer-1",
		"harry",
	); err != nil {
		t.Fatalf("createHTTPApprovalProfile: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode: %v", err)
	}

	path := mustProfileBrowserApprovalsPath(t, "customer-1", "harry")
	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPost,
			path:   path,
			body: BrowserApprovalRequest{
				DeviceID:             "missing-device",
				URL:                  "example.com",
				BrowserPolicyProfile: "study",
				ApprovalCode:         "1234",
				Action:               BrowserApprovalPermanent,
			},
		},
	)
	assertStatus(t, response, http.StatusNotFound)

	if err := registerAndLinkHTTPApprovalDevice(
		ctx,
		store,
		approvalDeviceLink{
			customerID: "customer-1",
			deviceID:   "laptop",
			profileID:  "harry",
		},
	); err != nil {
		t.Fatalf("registerAndLinkHTTPApprovalDevice: %v", err)
	}

	for _, missingPath := range []string{
		mustProfileBrowserApprovalsPath(t, "customer-1", "missing-profile"),
		mustProfileBrowserApprovalsPath(t, "missing-customer", "harry"),
	} {
		response = serveHTTP(
			t,
			handler,
			testHTTPRequest{
				method: http.MethodPost,
				path:   missingPath,
				body: BrowserApprovalRequest{
					DeviceID:             "laptop",
					URL:                  "example.com",
					BrowserPolicyProfile: "study",
					ApprovalCode:         "1234",
					Action:               BrowserApprovalPermanent,
				},
			},
		)
		assertStatus(t, response, http.StatusNotFound)
	}

	tests := []BrowserApprovalRequest{
		{
			DeviceID:             "laptop",
			URL:                  "example.com",
			BrowserPolicyProfile: "study",
			ApprovalCode:         "1234",
			Action:               BrowserApprovalTemporary,
			DurationSeconds:      0,
		},
		{
			DeviceID:             "laptop",
			URL:                  "example.com",
			BrowserPolicyProfile: "study",
			ApprovalCode:         "1234",
			Action:               BrowserApprovalAction("unsupported"),
		},
	}
	for _, request := range tests {
		response = serveHTTP(
			t,
			handler,
			testHTTPRequest{
				method: http.MethodPost,
				path:   path,
				body:   request,
			},
		)
		assertStatus(t, response, http.StatusBadRequest)
	}
}

func createHTTPApprovalProfile(
	ctx context.Context,
	store *Store,
	customerID CustomerID,
	profileID ProfileID,
) error {
	if err := store.CreateProfile(ctx, customerID, profileID); err != nil {
		return err
	}
	_, err := store.PutProfileConfiguration(
		ctx,
		ProfileConfiguration{
			CustomerID: customerID,
			ProfileID:  profileID,
			Policy:     browserApprovalPolicy([]string{}),
		},
	)
	return err
}

func registerAndLinkHTTPApprovalDevice(
	ctx context.Context,
	store *Store,
	link approvalDeviceLink,
) error {
	if err := store.RegisterDevice(
		ctx,
		DeviceRegistration{
			CustomerID: link.customerID,
			DeviceID:   link.deviceID,
		},
	); err != nil {
		return err
	}
	return store.LinkDeviceProfile(
		ctx,
		link.customerID,
		link.deviceID,
		link.profileID,
	)
}

func mustProfileBrowserApprovalsPath(
	t *testing.T,
	customerID CustomerID,
	profileID ProfileID,
) string {
	t.Helper()
	path, err := ProfileBrowserApprovalsPath(customerID, profileID)
	if err != nil {
		t.Fatalf("ProfileBrowserApprovalsPath: %v", err)
	}
	return path
}
