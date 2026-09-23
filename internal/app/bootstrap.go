package app

import (
	"log"

	"github.com/ddrowsy/family-friend-release/internal/config"
	"github.com/ddrowsy/family-friend-release/internal/modules"
	"github.com/ddrowsy/family-friend-release/internal/modules/kidcontrol"
	"github.com/ddrowsy/family-friend-release/internal/platform"
	"github.com/ddrowsy/family-friend-release/internal/platform/windows"
)

const DefaultPolicyPath = "configs/default.policy.json"

// Bootstrap prepares application dependencies.
func Bootstrap(logger *log.Logger) Runtime {
	return BootstrapWithPlatform(windows.NewPlatform(logger), logger)
}

// BootstrapWithPolicy prepares application dependencies using an in-memory policy.
func BootstrapWithPolicy(policyConfig config.Policy, logger *log.Logger) Runtime {
	return Runtime{
		Registry:       DefaultRegistry(windows.NewPlatform(logger), logger),
		Logger:         logger,
		policyOverride: &policyConfig,
	}
}

// BootstrapWithPlatform prepares application dependencies with the supplied platform.
func BootstrapWithPlatform(platformLayer platform.Platform, logger *log.Logger) Runtime {
	return Runtime{
		PolicyPath: DefaultPolicyPath,
		Registry:   DefaultRegistry(platformLayer, logger),
		Logger:     logger,
	}
}

// DefaultRegistry returns the modules available in this build.
func DefaultRegistry(platformLayer platform.Platform, logger *log.Logger) *modules.Registry {
	registry := modules.NewRegistry()
	if err := registry.Register(kidcontrol.New(platformLayer, logger)); err != nil {
		panic(err)
	}
	return registry
}
