package config

import (
	"encoding/json"
	"os"
)

// LoadPolicy reads a policy file from disk.
func LoadPolicy(path string) (Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, err
	}
	return ParsePolicy(data)
}

// ParsePolicy decodes policy JSON using the production policy contract.
func ParsePolicy(data []byte) (Policy, error) {
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, err
	}
	return policy, nil
}
