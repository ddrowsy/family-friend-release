package config

import (
	"errors"
	"fmt"
)

var validModuleModes = map[string]struct{}{
	"":        {},
	"monitor": {},
	"enforce": {},
}

// ValidatePolicy verifies the policy contains the required runtime fields.
func ValidatePolicy(policy Policy) error {
	if policy.SchemaVersion <= 0 {
		return errors.New("policy schema_version is required")
	}
	if policy.Version == "" {
		return errors.New("policy version is required")
	}
	if policy.DeviceID == "" {
		return errors.New("policy device_id is required")
	}
	if policy.Modules == nil {
		return errors.New("policy modules map is required")
	}

	for name, module := range policy.Modules {
		if _, ok := validModuleModes[module.Mode]; !ok {
			return fmt.Errorf("module %q has invalid mode %q", name, module.Mode)
		}
	}

	return nil
}
