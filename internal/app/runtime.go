package app

import (
	"context"
	"fmt"
	"log"

	"github.com/ddrowsy/family-friend-release/internal/config"
	"github.com/ddrowsy/family-friend-release/internal/modules"
	"github.com/ddrowsy/family-friend-release/internal/platform/windows"
)

// Runtime represents the application runtime.
type Runtime struct {
	PolicyPath     string
	Registry       *modules.Registry
	Logger         *log.Logger
	policyOverride *config.Policy
}

// Start resolves policy, starts enabled modules, and blocks until ctx is canceled.
func (r Runtime) Start(ctx context.Context) error {
	logger := r.Logger
	if logger == nil {
		logger = log.Default()
	}

	policyPath := r.PolicyPath
	if policyPath == "" {
		policyPath = DefaultPolicyPath
	}

	registry := r.Registry
	if registry == nil {
		registry = DefaultRegistry(windows.NewPlatform(logger), logger)
	}

	logger.Printf("drowsyfriend started")

	policyConfig, err := r.loadPolicy(policyPath)
	if err != nil {
		return fmt.Errorf("load policy: %w", err)
	}
	if err := config.ValidatePolicy(policyConfig); err != nil {
		return fmt.Errorf("validate policy: %w", err)
	}

	result, err := registry.StartEnabled(ctx, policyConfig)
	if err != nil {
		return err
	}
	for _, name := range result.Started {
		logger.Printf("module %s started", name)
	}
	for _, name := range result.Skipped {
		logger.Printf("module %s skipped because it is disabled", name)
	}
	for _, name := range result.Unknown {
		logger.Printf("policy module %s has no registered module", name)
	}

	<-ctx.Done()
	logger.Printf("shutdown requested")

	if err := registry.StopStarted(context.Background()); err != nil {
		return err
	}

	return nil
}

func (r Runtime) loadPolicy(policyPath string) (config.Policy, error) {
	if r.policyOverride != nil {
		return *r.policyOverride, nil
	}
	return config.LoadPolicy(policyPath)
}
