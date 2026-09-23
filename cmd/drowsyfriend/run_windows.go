//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	platformwindows "github.com/ddrowsy/family-friend-release/internal/platform/windows"
	"github.com/ddrowsy/family-friend-release/internal/servicecontrol"
	"github.com/ddrowsy/family-friend-release/internal/sessionbridge"
	"github.com/ddrowsy/family-friend-release/internal/sessionconfig"
	"golang.org/x/sys/windows/svc"
)

const serviceStopTimeout = 30 * time.Second

func run(logger *log.Logger, options runtimeOptions) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("detecting Windows service session: %w", err)
	}
	if !isService {
		runtime, err := newInteractiveRuntime(logger, options)
		if err != nil {
			return err
		}
		return runInteractive(runtime)
	}
	if options.MockData {
		return fmt.Errorf("--mock-data is only supported in interactive mode")
	}

	if err := useServiceWorkingDirectory(); err != nil {
		return err
	}

	localPlatform := platformwindows.NewPlatform(logger)
	bridge := configuredSessionBridge(
		logger,
		sessionconfig.Load,
		sessionbridge.NewPipeClient,
	)
	servicePlatform := platformwindows.NewServicePlatform(localPlatform, bridge)
	runtime, err := newWindowsAgentRuntime(servicePlatform, logger)
	if err != nil {
		return err
	}
	handler := &windowsServiceHandler{
		runtime:     runtime,
		logger:      logger,
		stopTimeout: serviceStopTimeout,
	}
	if err := svc.Run(servicecontrol.Name, handler); err != nil {
		return fmt.Errorf("running Windows service: %w", err)
	}
	return nil
}

func useServiceWorkingDirectory() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving service executable: %w", err)
	}

	if err := os.Chdir(filepath.Dir(executable)); err != nil {
		return fmt.Errorf("setting service working directory: %w", err)
	}
	return nil
}
