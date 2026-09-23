//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ddrowsy/family-friend-release/internal/agent"
	"github.com/ddrowsy/family-friend-release/internal/app"
	"github.com/ddrowsy/family-friend-release/internal/config"
	"github.com/ddrowsy/family-friend-release/internal/platform"
	"github.com/ddrowsy/family-friend-release/internal/service"
	serviceclient "github.com/ddrowsy/family-friend-release/internal/service/client"
)

const controlServiceURLEnvironment = "DROWSYFRIEND_CONTROL_SERVICE_URL"

type windowsAgentRuntime struct {
	runtime  runtimeStarter
	uiServer *agent.UIAPIServer
}

func (r *windowsAgentRuntime) Start(ctx context.Context) error {
	if err := r.uiServer.Start(ctx); err != nil {
		return fmt.Errorf("starting localhost UI API: %w", err)
	}

	runtimeErr := r.runtime.Start(ctx)
	stopErr := r.uiServer.Stop(context.Background())
	return errors.Join(runtimeErr, stopErr)
}

func newWindowsAgentRuntime(
	platformLayer platform.Platform,
	logger *log.Logger,
) (runtimeStarter, error) {
	policyConfig, err := config.LoadPolicy(app.DefaultPolicyPath)
	if err != nil {
		return nil, fmt.Errorf("loading device identity: %w", err)
	}
	deviceName, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("resolving device name: %w", err)
	}
	pairingClient, err := newPairingClientFromEnvironment()
	if err != nil {
		return nil, err
	}

	uiAPI, err := agent.NewUIAPI(pairingClient, agent.DeviceIdentity{
		DeviceID:   service.DeviceID(policyConfig.DeviceID),
		DeviceName: deviceName,
		Platform:   "windows",
	})
	if err != nil {
		return nil, fmt.Errorf("creating localhost UI API: %w", err)
	}
	uiServer, err := agent.NewUIAPIServer(uiAPI, agent.DefaultUIAPIAddress)
	if err != nil {
		return nil, fmt.Errorf("creating localhost UI API server: %w", err)
	}

	return &windowsAgentRuntime{
		runtime:  app.BootstrapWithPlatform(platformLayer, logger),
		uiServer: uiServer,
	}, nil
}

func newPairingClientFromEnvironment() (agent.PairingClient, error) {
	baseURL := strings.TrimSpace(os.Getenv(controlServiceURLEnvironment))
	if baseURL == "" {
		return nil, nil
	}

	client, err := serviceclient.NewAgent(baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf(
			"configuring control service from %s: %w",
			controlServiceURLEnvironment,
			err,
		)
	}
	return client, nil
}
