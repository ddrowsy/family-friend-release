package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

func TestIdentityValidation(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "customer",
			err:  CustomerID(" ").Validate(),
		},
		{
			name: "profile",
			err:  ProfileID("").Validate(),
		},
		{
			name: "device",
			err:  DeviceID("\t").Validate(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("Validate returned nil, want error")
			}
		})
	}
}

func TestProfileConfigurationValidate(t *testing.T) {
	configuration := ProfileConfiguration{
		CustomerID: "customer-1",
		ProfileID:  "harry",
		Revision:   1,
		Policy: policy.ProfilePolicy{
			SchemaVersion: 2,
			Version:       "1.0",
			Modules:       map[string]policy.ModulePolicy{},
		},
	}

	if err := configuration.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestDeviceRegistrationValidate(t *testing.T) {
	registration := DeviceRegistration{
		CustomerID: "customer-1",
		DeviceID:   " ",
	}

	if err := registration.Validate(); err == nil {
		t.Fatal("Validate returned nil, want device ID error")
	}
}

func TestDeviceProfileAssignmentSupportsMultipleProfiles(t *testing.T) {
	assignment := DeviceProfileAssignment{
		CustomerID: "customer-1",
		DeviceID:   "family-laptop",
		ProfileIDs: []ProfileID{"harry", "james"},
	}

	if err := assignment.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestDeviceProfileAssignmentRejectsInvalidProfile(t *testing.T) {
	assignment := DeviceProfileAssignment{
		CustomerID: "customer-1",
		DeviceID:   "family-laptop",
		ProfileIDs: []ProfileID{"harry", " "},
	}

	err := assignment.Validate()
	if err == nil || !strings.Contains(err.Error(), "profile_ids[1]") {
		t.Fatalf("Validate error = %v, want profile_ids[1] error", err)
	}
}

func TestProfilePolicyDoesNotContainDeviceIdentity(t *testing.T) {
	policy := policy.ProfilePolicy{
		SchemaVersion: 2,
		Version:       "1.0",
		Modules:       map[string]policy.ModulePolicy{},
	}

	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if strings.Contains(string(data), "device_id") {
		t.Fatalf("ProfilePolicy JSON = %s, must not contain device_id", data)
	}
}

func TestAPIPaths(t *testing.T) {
	configPath, err := ProfileConfigPath("customer one", "harry")
	if err != nil {
		t.Fatalf("ProfileConfigPath returned error: %v", err)
	}
	if configPath != "/api/v1/customers/customer%20one/profiles/harry/config" {
		t.Fatalf("ProfileConfigPath = %q", configPath)
	}

	approvalPath, err := ProfileBrowserApprovalsPath("customer one", "harry")
	if err != nil {
		t.Fatalf("ProfileBrowserApprovalsPath returned error: %v", err)
	}
	if approvalPath != "/api/v1/customers/customer%20one/profiles/harry/browser/approvals" {
		t.Fatalf("ProfileBrowserApprovalsPath = %q", approvalPath)
	}

	devicePath, err := DevicePath("customer one", "shared laptop")
	if err != nil {
		t.Fatalf("DevicePath returned error: %v", err)
	}
	if devicePath != "/api/v1/customers/customer%20one/devices/shared%20laptop" {
		t.Fatalf("DevicePath = %q", devicePath)
	}

	deviceProfilesPath, err := DeviceProfilesPath("customer one", "shared laptop")
	if err != nil {
		t.Fatalf("DeviceProfilesPath returned error: %v", err)
	}
	if deviceProfilesPath != "/api/v1/customers/customer%20one/devices/shared%20laptop/profiles" {
		t.Fatalf("DeviceProfilesPath = %q", deviceProfilesPath)
	}

	deviceProfilePath, err := DeviceProfilePath("customer one", "shared laptop", "harry")
	if err != nil {
		t.Fatalf("DeviceProfilePath returned error: %v", err)
	}
	if deviceProfilePath != "/api/v1/customers/customer%20one/devices/shared%20laptop/profiles/harry" {
		t.Fatalf("DeviceProfilePath = %q", deviceProfilePath)
	}
}
