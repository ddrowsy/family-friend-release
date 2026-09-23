package config

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

func TestLoadPolicyValidDefault(t *testing.T) {
	policy, err := LoadPolicy(filepath.Join("..", "..", "configs", "default.policy.json"))
	if err != nil {
		t.Fatalf("LoadPolicy returned error: %v", err)
	}

	if policy.Version != "1.0" {
		t.Fatalf("Version = %q, want %q", policy.Version, "1.0")
	}
	if policy.SchemaVersion != 2 {
		t.Fatalf("SchemaVersion = %d, want 2", policy.SchemaVersion)
	}
	if policy.DeviceID != "child-windows-001" {
		t.Fatalf("DeviceID = %q, want %q", policy.DeviceID, "child-windows-001")
	}
	if _, ok := policy.Modules["kidcontrol"]; !ok {
		t.Fatal("default policy does not include kidcontrol")
	}
}

func TestAgentPolicyRoundTripsSharedProfilePolicy(t *testing.T) {
	shared := policy.ProfilePolicy{
		SchemaVersion: 2,
		Version:       "1.0",
		Modules: map[string]policy.ModulePolicy{
			"kidcontrol": {
				Enabled: true,
				Mode:    "enforce",
				Settings: map[string]any{
					"dry_run": false,
				},
			},
		},
	}
	agentPolicy := Policy{
		DeviceID:      "device-1",
		ProfilePolicy: shared,
	}

	data, err := json.Marshal(agentPolicy)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var decoded Policy
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !reflect.DeepEqual(decoded.ProfilePolicy, shared) {
		t.Fatalf("ProfilePolicy = %#v, want %#v", decoded.ProfilePolicy, shared)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("Unmarshal fields returned error: %v", err)
	}
	for _, name := range []string{"device_id", "schema_version", "version", "modules"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("agent policy JSON is missing %q: %s", name, data)
		}
	}
	if _, ok := fields["ProfilePolicy"]; ok {
		t.Fatalf("shared profile policy must be embedded, got nested JSON: %s", data)
	}
}

func TestValidatePolicyMissingSchemaVersion(t *testing.T) {
	policy := validPolicy()
	policy.SchemaVersion = 0

	err := ValidatePolicy(policy)
	if err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("ValidatePolicy error = %v, want schema_version error", err)
	}
}

func TestValidatePolicyMissingVersion(t *testing.T) {
	policy := validPolicy()
	policy.Version = ""

	err := ValidatePolicy(policy)
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("ValidatePolicy error = %v, want version error", err)
	}
}

func TestValidatePolicyMissingDeviceID(t *testing.T) {
	policy := validPolicy()
	policy.DeviceID = ""

	err := ValidatePolicy(policy)
	if err == nil || !strings.Contains(err.Error(), "device_id") {
		t.Fatalf("ValidatePolicy error = %v, want device_id error", err)
	}
}

func TestValidatePolicyInvalidModuleMode(t *testing.T) {
	policy := validPolicy()
	module := policy.Modules["kidcontrol"]
	module.Mode = "block"
	policy.Modules["kidcontrol"] = module

	err := ValidatePolicy(policy)
	if err == nil || !strings.Contains(err.Error(), "invalid mode") {
		t.Fatalf("ValidatePolicy error = %v, want invalid mode error", err)
	}
}

func validPolicy() Policy {
	return Policy{
		DeviceID: "child-windows-001",
		ProfilePolicy: policy.ProfilePolicy{
			SchemaVersion: 2,
			Version:       "1.0",
			Modules: map[string]policy.ModulePolicy{
				"kidcontrol": {
					Enabled:  true,
					Mode:     "enforce",
					Settings: map[string]interface{}{},
				},
			},
		},
	}
}
