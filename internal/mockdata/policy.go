package mockdata

import (
	_ "embed"
	"fmt"

	"github.com/ddrowsy/family-friend-release/internal/config"
)

//go:embed testdata/agent.policy.json
var policyJSON []byte

// Policy returns the deterministic developer policy used by mock-data runtime mode.
func Policy() (config.Policy, error) {
	policyConfig, err := config.ParsePolicy(policyJSON)
	if err != nil {
		return config.Policy{}, fmt.Errorf("parse embedded mock policy: %w", err)
	}
	if err := config.ValidatePolicy(policyConfig); err != nil {
		return config.Policy{}, fmt.Errorf("validate embedded mock policy: %w", err)
	}
	return policyConfig, nil
}
