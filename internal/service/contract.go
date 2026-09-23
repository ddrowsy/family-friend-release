package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

const APIV1Path = "/api/v1"

// CustomerID identifies one customer account boundary.
type CustomerID string

// ProfileID identifies one child profile owned by a customer.
type ProfileID string

// DeviceID identifies one installed Family Friend agent.
type DeviceID string

// BrowserApprovalAction selects how long a browser approval should apply.
type BrowserApprovalAction string

const (
	BrowserApprovalTemporary BrowserApprovalAction = "temporary"
	BrowserApprovalPermanent BrowserApprovalAction = "permanent"
)

// ProfileConfiguration is the versioned policy for one child profile.
type ProfileConfiguration struct {
	CustomerID CustomerID           `json:"customer_id"`
	ProfileID  ProfileID            `json:"profile_id"`
	Revision   int64                `json:"revision"`
	Policy     policy.ProfilePolicy `json:"policy"`
}

// DeviceRegistration identifies one device owned by a customer.
type DeviceRegistration struct {
	CustomerID CustomerID `json:"customer_id"`
	DeviceID   DeviceID   `json:"device_id"`
}

// DeviceProfileAssignment describes the child profiles available on one device.
type DeviceProfileAssignment struct {
	CustomerID CustomerID  `json:"customer_id"`
	DeviceID   DeviceID    `json:"device_id"`
	ProfileIDs []ProfileID `json:"profile_ids"`
}

// ActiveBrowserProfileRequest changes the selected browser policy profile.
type ActiveBrowserProfileRequest struct {
	Name            string     `json:"name"`
	DurationSeconds int64      `json:"duration_seconds,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	FallbackProfile string     `json:"fallback_profile,omitempty"`
}

// BrowserApprovalRequest is sent by an agent for a blocked browser URL.
type BrowserApprovalRequest struct {
	DeviceID             DeviceID              `json:"device_id"`
	URL                  string                `json:"url"`
	BrowserPolicyProfile string                `json:"browser_profile"`
	ApprovalCode         string                `json:"approval_code"`
	Action               BrowserApprovalAction `json:"action"`
	DurationSeconds      int64                 `json:"duration_seconds,omitempty"`
}

// BrowserApprovalResult describes a successful browser approval.
type BrowserApprovalResult struct {
	CustomerID CustomerID `json:"customer_id"`
	ProfileID  ProfileID  `json:"profile_id"`
	DeviceID   DeviceID   `json:"device_id"`
	Revision   int64      `json:"revision,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// Validate verifies a customer ID can identify a service resource.
func (id CustomerID) Validate() error {
	return validateID("customer ID", string(id))
}

// Validate verifies a child profile ID can identify a service resource.
func (id ProfileID) Validate() error {
	return validateID("profile ID", string(id))
}

// Validate verifies a device ID can identify a service resource.
func (id DeviceID) Validate() error {
	return validateID("device ID", string(id))
}

// Validate verifies the configuration has valid service identity.
func (c ProfileConfiguration) Validate() error {
	if err := c.CustomerID.Validate(); err != nil {
		return err
	}
	if err := c.ProfileID.Validate(); err != nil {
		return err
	}
	return validateProfilePolicy(c.Policy)
}

func validateProfilePolicy(profilePolicy policy.ProfilePolicy) error {
	if profilePolicy.SchemaVersion <= 0 {
		return errors.New("profile policy schema_version is required")
	}
	if profilePolicy.Version == "" {
		return errors.New("profile policy version is required")
	}
	if profilePolicy.Modules == nil {
		return errors.New("profile policy modules map is required")
	}

	for name, module := range profilePolicy.Modules {
		switch module.Mode {
		case "", "monitor", "enforce":
		default:
			return fmt.Errorf("module %q has invalid mode %q", name, module.Mode)
		}
	}
	return nil
}

// Validate verifies the device registration has valid service identity.
func (r DeviceRegistration) Validate() error {
	if err := r.CustomerID.Validate(); err != nil {
		return err
	}
	return r.DeviceID.Validate()
}

// Validate verifies the device assignment has valid service identity.
func (a DeviceProfileAssignment) Validate() error {
	if err := a.CustomerID.Validate(); err != nil {
		return err
	}
	if err := a.DeviceID.Validate(); err != nil {
		return err
	}
	for i, profileID := range a.ProfileIDs {
		if err := profileID.Validate(); err != nil {
			return fmt.Errorf("profile_ids[%d]: %w", i, err)
		}
	}
	return nil
}

// ProfileConfigPath returns the API path for one child profile configuration.
func ProfileConfigPath(customerID CustomerID, profileID ProfileID) (string, error) {
	if err := customerID.Validate(); err != nil {
		return "", err
	}
	if err := profileID.Validate(); err != nil {
		return "", err
	}

	return profilePath(customerID, profileID) + "/config", nil
}

// ProfileActiveBrowserPath returns the API path for active browser profile state.
func ProfileActiveBrowserPath(customerID CustomerID, profileID ProfileID) (string, error) {
	if err := customerID.Validate(); err != nil {
		return "", err
	}
	if err := profileID.Validate(); err != nil {
		return "", err
	}

	return profilePath(customerID, profileID) + "/browser/active-profile", nil
}

// ProfileBrowserApprovalsPath returns the API path for browser approvals.
func ProfileBrowserApprovalsPath(customerID CustomerID, profileID ProfileID) (string, error) {
	if err := customerID.Validate(); err != nil {
		return "", err
	}
	if err := profileID.Validate(); err != nil {
		return "", err
	}

	return profilePath(customerID, profileID) + "/browser/approvals", nil
}

// DevicePath returns the API path used to register or address one device.
func DevicePath(customerID CustomerID, deviceID DeviceID) (string, error) {
	if err := customerID.Validate(); err != nil {
		return "", err
	}
	if err := deviceID.Validate(); err != nil {
		return "", err
	}

	return APIV1Path +
		"/customers/" + url.PathEscape(string(customerID)) +
		"/devices/" + url.PathEscape(string(deviceID)), nil
}

// DeviceProfilesPath returns the API path for one device's child-profile links.
func DeviceProfilesPath(customerID CustomerID, deviceID DeviceID) (string, error) {
	devicePath, err := DevicePath(customerID, deviceID)
	if err != nil {
		return "", err
	}

	return devicePath + "/profiles", nil
}

// DeviceProfilePath returns the API path for one device-to-profile link.
func DeviceProfilePath(
	customerID CustomerID,
	deviceID DeviceID,
	profileID ProfileID,
) (string, error) {
	deviceProfilesPath, err := DeviceProfilesPath(customerID, deviceID)
	if err != nil {
		return "", err
	}
	if err := profileID.Validate(); err != nil {
		return "", err
	}

	return deviceProfilesPath + "/" + url.PathEscape(string(profileID)), nil
}

func profilePath(customerID CustomerID, profileID ProfileID) string {
	return APIV1Path +
		"/customers/" + url.PathEscape(string(customerID)) +
		"/profiles/" + url.PathEscape(string(profileID))
}

func validateID(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
