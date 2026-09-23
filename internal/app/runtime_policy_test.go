package app

import (
	"io"
	"log"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/config"
	"github.com/ddrowsy/family-friend-release/internal/policy"
)

func TestBootstrapWithPolicyUsesInMemoryPolicy(t *testing.T) {
	policyConfig := config.Policy{
		DeviceID: "mock-device",
		ProfilePolicy: policy.ProfilePolicy{
			SchemaVersion: 2,
			Version:       "mock",
			Modules:       map[string]policy.ModulePolicy{},
		},
	}
	logger := log.New(io.Discard, "", 0)
	runtime := BootstrapWithPolicy(policyConfig, logger)

	got, err := runtime.loadPolicy("does-not-exist.json")
	if err != nil {
		t.Fatalf("loadPolicy returned error: %v", err)
	}
	if got.DeviceID != policyConfig.DeviceID || got.Version != policyConfig.Version {
		t.Fatalf("policy = %#v, want in-memory policy %#v", got, policyConfig)
	}
}
