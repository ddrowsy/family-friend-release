package modules

import (
	"context"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/config"
	"github.com/ddrowsy/family-friend-release/internal/policy"
)

type testModule struct {
	name       string
	started    bool
	stopped    bool
	startCount int
	stopCount  int
}

func (m *testModule) Name() string {
	return m.name
}

func (m *testModule) Start(ctx context.Context, modulePolicy policy.ModulePolicy) error {
	m.started = true
	m.startCount++
	return nil
}

func (m *testModule) Stop(ctx context.Context) error {
	m.stopped = true
	m.stopCount++
	return nil
}

func TestRegistryStartsEnabledModules(t *testing.T) {
	registry := NewRegistry()
	module := &testModule{name: "kidcontrol"}
	if err := registry.Register(module); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	result, err := registry.StartEnabled(context.Background(), config.Policy{
		ProfilePolicy: policy.ProfilePolicy{
			Modules: map[string]policy.ModulePolicy{
				"kidcontrol": {Enabled: true, Mode: "enforce"},
			},
		},
	})
	if err != nil {
		t.Fatalf("StartEnabled returned error: %v", err)
	}

	if module.startCount != 1 {
		t.Fatalf("startCount = %d, want 1", module.startCount)
	}
	if len(result.Started) != 1 || result.Started[0] != "kidcontrol" {
		t.Fatalf("Started = %v, want [kidcontrol]", result.Started)
	}
}

func TestRegistrySkipsDisabledModules(t *testing.T) {
	registry := NewRegistry()
	module := &testModule{name: "kidcontrol"}
	if err := registry.Register(module); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	result, err := registry.StartEnabled(context.Background(), config.Policy{
		ProfilePolicy: policy.ProfilePolicy{
			Modules: map[string]policy.ModulePolicy{
				"kidcontrol": {Enabled: false, Mode: "monitor"},
			},
		},
	})
	if err != nil {
		t.Fatalf("StartEnabled returned error: %v", err)
	}

	if module.startCount != 0 {
		t.Fatalf("startCount = %d, want 0", module.startCount)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "kidcontrol" {
		t.Fatalf("Skipped = %v, want [kidcontrol]", result.Skipped)
	}
}

func TestRegistryStopsStartedModules(t *testing.T) {
	registry := NewRegistry()
	module := &testModule{name: "kidcontrol"}
	if err := registry.Register(module); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	_, err := registry.StartEnabled(context.Background(), config.Policy{
		ProfilePolicy: policy.ProfilePolicy{
			Modules: map[string]policy.ModulePolicy{
				"kidcontrol": {Enabled: true, Mode: "enforce"},
			},
		},
	})
	if err != nil {
		t.Fatalf("StartEnabled returned error: %v", err)
	}

	if err := registry.StopStarted(context.Background()); err != nil {
		t.Fatalf("StopStarted returned error: %v", err)
	}

	if module.stopCount != 1 {
		t.Fatalf("stopCount = %d, want 1", module.stopCount)
	}
}
