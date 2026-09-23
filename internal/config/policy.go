package config

import "github.com/ddrowsy/family-friend-release/internal/policy"

// Policy describes the JSON policy file used by one agent installation.
// DeviceID is agent identity; the embedded ProfilePolicy is shared with the service.
type Policy struct {
	DeviceID string `json:"device_id"`
	policy.ProfilePolicy
}
