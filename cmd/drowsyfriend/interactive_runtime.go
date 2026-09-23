package main

import (
	"fmt"
	"log"

	"github.com/ddrowsy/family-friend-release/internal/app"
	"github.com/ddrowsy/family-friend-release/internal/mockdata"
)

func newInteractiveRuntime(logger *log.Logger, options runtimeOptions) (app.Runtime, error) {
	if !options.MockData {
		return app.Bootstrap(logger), nil
	}

	policyConfig, err := mockdata.Policy()
	if err != nil {
		return app.Runtime{}, fmt.Errorf("loading mock data: %w", err)
	}
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("WARNING: mock data mode enabled; using embedded developer policy data")

	return app.BootstrapWithPolicy(policyConfig, logger), nil
}
