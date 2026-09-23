package service

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// DeviceBrowserAccessRequest is sent by an installed agent for a blocked URL.
type DeviceBrowserAccessRequest struct {
	ProfileID            ProfileID `json:"profile_id"`
	URL                  string    `json:"url"`
	BrowserPolicyProfile string    `json:"browser_policy_profile"`
}

// DeviceBrowserAccessStatus is the identity-free request state returned to an agent.
type DeviceBrowserAccessStatus struct {
	RequestID       string                    `json:"request_id"`
	State           BrowserAccessRequestState `json:"state"`
	ExpiresAt       time.Time                 `json:"expires_at"`
	Action          BrowserApprovalAction     `json:"action,omitempty"`
	DurationSeconds int64                     `json:"duration_seconds,omitempty"`
}

// DeviceBrowserAccessRequestsPath returns the installed-device request collection path.
func DeviceBrowserAccessRequestsPath() string {
	return APIV1Path + "/browser/access-requests"
}

// DeviceBrowserAccessRequestPath returns the path for one opaque device request ID.
func DeviceBrowserAccessRequestPath(requestID string) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", errors.New("browser access request ID is required")
	}
	return DeviceBrowserAccessRequestsPath() + "/" + url.PathEscape(requestID), nil
}
