package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/policy"
	"github.com/ddrowsy/family-friend-release/internal/service/database"
)

type testHTTPRequest struct {
	method  string
	path    string
	body    any
	rawBody []byte
}

func TestHTTPProfileConfigurationReadUpdateAndIsolation(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)

	for _, identity := range []struct {
		customerID CustomerID
		profileID  ProfileID
	}{
		{customerID: "customer-1", profileID: "harry"},
		{customerID: "customer-1", profileID: "james"},
		{customerID: "customer-2", profileID: "harry"},
	} {
		if err := store.CreateProfile(
			context.Background(),
			identity.customerID,
			identity.profileID,
		); err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
	}

	harryPath := mustProfileConfigPath(t, "customer-1", "harry")
	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPut,
			path:   harryPath,
			body:   testProfilePolicy("harry-v1"),
		},
	)
	assertStatus(t, response, http.StatusOK)
	var first ProfileConfiguration
	decodeHTTPResponse(t, response, &first)
	if first.Revision != 1 {
		t.Fatalf("first revision = %d, want 1", first.Revision)
	}

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPut,
			path:   harryPath,
			body:   testProfilePolicy("harry-v2"),
		},
	)
	assertStatus(t, response, http.StatusOK)
	var second ProfileConfiguration
	decodeHTTPResponse(t, response, &second)
	if second.Revision != 2 {
		t.Fatalf("second revision = %d, want 2", second.Revision)
	}

	jamesPath := mustProfileConfigPath(t, "customer-1", "james")
	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPut,
			path:   jamesPath,
			body:   testProfilePolicy("james-v1"),
		},
	)
	assertStatus(t, response, http.StatusOK)

	otherHarryPath := mustProfileConfigPath(t, "customer-2", "harry")
	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPut,
			path:   otherHarryPath,
			body:   testProfilePolicy("other-harry-v1"),
		},
	)
	assertStatus(t, response, http.StatusOK)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   harryPath,
		},
	)
	assertStatus(t, response, http.StatusOK)
	var harry ProfileConfiguration
	decodeHTTPResponse(t, response, &harry)
	if harry.Revision != 2 || harry.Policy.Version != "harry-v2" {
		t.Fatalf("harry configuration = %#v", harry)
	}

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   jamesPath,
		},
	)
	assertStatus(t, response, http.StatusOK)
	var james ProfileConfiguration
	decodeHTTPResponse(t, response, &james)
	if james.Revision != 1 || james.Policy.Version != "james-v1" {
		t.Fatalf("james configuration = %#v", james)
	}

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   otherHarryPath,
		},
	)
	assertStatus(t, response, http.StatusOK)
	var otherHarry ProfileConfiguration
	decodeHTTPResponse(t, response, &otherHarry)
	if otherHarry.Revision != 1 || otherHarry.Policy.Version != "other-harry-v1" {
		t.Fatalf("other customer configuration = %#v", otherHarry)
	}
}

func TestHTTPDeviceAssociationLifecycle(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	ctx := context.Background()

	for _, profileID := range []ProfileID{"harry", "james"} {
		if err := store.CreateProfile(ctx, "customer-1", profileID); err != nil {
			t.Fatalf("CreateProfile %s: %v", profileID, err)
		}
	}
	for _, deviceID := range []DeviceID{"laptop", "desktop"} {
		response := serveHTTP(
			t,
			handler,
			testHTTPRequest{
				method: http.MethodPut,
				path:   mustDevicePath(t, "customer-1", deviceID),
			},
		)
		assertStatus(t, response, http.StatusNoContent)
	}

	for _, link := range []struct {
		deviceID  DeviceID
		profileID ProfileID
	}{
		{deviceID: "laptop", profileID: "harry"},
		{deviceID: "laptop", profileID: "james"},
		{deviceID: "desktop", profileID: "harry"},
	} {
		response := serveHTTP(
			t,
			handler,
			testHTTPRequest{
				method: http.MethodPut,
				path: mustDeviceProfilePath(
					t,
					"customer-1",
					link.deviceID,
					link.profileID,
				),
			},
		)
		assertStatus(t, response, http.StatusNoContent)
	}

	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodGet,
			path: mustDeviceProfilesPath(
				t,
				"customer-1",
				"laptop",
			),
		},
	)
	assertStatus(t, response, http.StatusOK)
	var assignment DeviceProfileAssignment
	decodeHTTPResponse(t, response, &assignment)
	wantProfiles := []ProfileID{"harry", "james"}
	if !reflect.DeepEqual(assignment.ProfileIDs, wantProfiles) {
		t.Fatalf("laptop profiles = %v, want %v", assignment.ProfileIDs, wantProfiles)
	}

	harryLinkPath := mustDeviceProfilePath(
		t,
		"customer-1",
		"laptop",
		"harry",
	)
	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   harryLinkPath,
		},
	)
	assertStatus(t, response, http.StatusNoContent)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodDelete,
			path:   harryLinkPath,
		},
	)
	assertStatus(t, response, http.StatusNoContent)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodGet,
			path:   harryLinkPath,
		},
	)
	assertStatus(t, response, http.StatusNotFound)
}

func TestHTTPRejectsInvalidAndMissingResources(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	ctx := context.Background()

	if err := store.CreateProfile(ctx, "customer-1", "harry"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	configPath := mustProfileConfigPath(t, "customer-1", "harry")

	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPut,
			path:   configPath,
			body: map[string]any{
				"schema_version": 0,
				"version":        "invalid",
				"modules":        map[string]any{},
			},
		},
	)
	assertStatus(t, response, http.StatusBadRequest)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method:  http.MethodPut,
			path:    configPath,
			rawBody: []byte("{"),
		},
	)
	assertStatus(t, response, http.StatusBadRequest)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodGet,
			path: mustProfileConfigPath(
				t,
				"customer-1",
				"missing",
			),
		},
	)
	assertStatus(t, response, http.StatusNotFound)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPut,
			path: mustDevicePath(
				t,
				"missing-customer",
				"laptop",
			),
		},
	)
	assertStatus(t, response, http.StatusNotFound)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodPut,
			path:   "/api/v1/customers/%20/profiles/harry/config",
			body:   testProfilePolicy("ignored"),
		},
	)
	assertStatus(t, response, http.StatusBadRequest)
}

func TestHTTPReturnsInternalServerError(t *testing.T) {
	store, handler, db := newHTTPTestHandler(t)
	ctx := context.Background()

	if err := store.CreateProfile(ctx, "customer-1", "harry"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: http.MethodGet,
			path: mustProfileConfigPath(
				t,
				"customer-1",
				"harry",
			),
		},
	)
	assertStatus(t, response, http.StatusInternalServerError)
}

func newHTTPTestHandler(
	t *testing.T,
) (*Store, http.Handler, *sql.DB) {
	t.Helper()
	db, err := database.Open(
		context.Background(),
		filepath.Join(t.TempDir(), "service.db"),
	)
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	store := NewStore(db)
	return store, NewHTTPHandler(store), db
}

func testProfilePolicy(version string) policy.ProfilePolicy {
	return policy.ProfilePolicy{
		SchemaVersion: 1,
		Version:       version,
		Modules:       map[string]policy.ModulePolicy{},
	}
}

func serveHTTP(
	t *testing.T,
	handler http.Handler,
	request testHTTPRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	body := request.rawBody
	if body == nil && request.body != nil {
		var err error
		body, err = json.Marshal(request.body)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
	}

	httpRequest := httptest.NewRequest(
		request.method,
		request.path,
		bytes.NewReader(body),
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	return response
}

func decodeHTTPResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	target any,
) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
}

func assertStatus(
	t *testing.T,
	response *httptest.ResponseRecorder,
	want int,
) {
	t.Helper()
	if response.Code != want {
		t.Fatalf(
			"status = %d, want %d; body = %s",
			response.Code,
			want,
			response.Body.String(),
		)
	}
}

func mustProfileConfigPath(
	t *testing.T,
	customerID CustomerID,
	profileID ProfileID,
) string {
	t.Helper()
	path, err := ProfileConfigPath(customerID, profileID)
	if err != nil {
		t.Fatalf("ProfileConfigPath: %v", err)
	}
	return path
}

func mustDevicePath(
	t *testing.T,
	customerID CustomerID,
	deviceID DeviceID,
) string {
	t.Helper()
	path, err := DevicePath(customerID, deviceID)
	if err != nil {
		t.Fatalf("DevicePath: %v", err)
	}
	return path
}

func mustDeviceProfilesPath(
	t *testing.T,
	customerID CustomerID,
	deviceID DeviceID,
) string {
	t.Helper()
	path, err := DeviceProfilesPath(customerID, deviceID)
	if err != nil {
		t.Fatalf("DeviceProfilesPath: %v", err)
	}
	return path
}

func mustDeviceProfilePath(
	t *testing.T,
	customerID CustomerID,
	deviceID DeviceID,
	profileID ProfileID,
) string {
	t.Helper()
	path, err := DeviceProfilePath(customerID, deviceID, profileID)
	if err != nil {
		t.Fatalf("DeviceProfilePath: %v", err)
	}
	return path
}
