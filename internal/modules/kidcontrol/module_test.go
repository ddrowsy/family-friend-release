package kidcontrol

import (
	"context"
	"io"
	"log"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/platform/windows"
	"github.com/ddrowsy/family-friend-release/internal/policy"
)

func TestModuleStartsAndStopsCleanly(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	module := New(windows.NewPlatform(logger), logger)
	modulePolicy := policy.ModulePolicy{
		Enabled: true,
		Mode:    "enforce",
		Settings: map[string]interface{}{
			"app_control": map[string]interface{}{
				"enabled":      true,
				"blocked_apps": []interface{}{"msedge.exe"},
			},
			"window_monitoring": map[string]interface{}{
				"enabled": true,
			},
			"screen_monitoring": map[string]interface{}{
				"enabled":                 false,
				"sample_interval_seconds": float64(3),
			},
		},
	}

	if err := module.Start(context.Background(), modulePolicy); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if !module.running {
		t.Fatal("module is not running after Start")
	}

	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if module.running {
		t.Fatal("module is still running after Stop")
	}
}

func TestNewAcceptsPlatformDependency(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	module := New(windows.NewPlatform(logger), logger)

	if module.platform == nil {
		t.Fatal("module platform is nil")
	}
}
