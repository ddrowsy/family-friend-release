//go:build windows

package servicecontrol

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const uninstallStopTimeout = 45 * time.Second

func Install(executablePath string) error {
	resolvedPath, err := resolveExecutablePath(executablePath)
	if err != nil {
		return err
	}

	manager, err := mgr.Connect()
	if err != nil {
		return serviceManagerError("connecting to Windows Service Control Manager", err)
	}
	defer manager.Disconnect()

	existing, err := manager.OpenService(Name)
	if err == nil {
		existing.Close()
		return fmt.Errorf("service %q is already installed", Name)
	}
	if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return serviceManagerError("checking existing Family Friend service", err)
	}

	service, err := manager.CreateService(
		Name,
		resolvedPath,
		mgr.Config{
			StartType:   mgr.StartAutomatic,
			DisplayName: DisplayName,
			Description: Description,
		},
	)
	if err != nil {
		return serviceManagerError("installing Family Friend service", err)
	}
	defer service.Close()

	return nil
}

func Uninstall() error {
	manager, err := mgr.Connect()
	if err != nil {
		return serviceManagerError("connecting to Windows Service Control Manager", err)
	}
	defer manager.Disconnect()

	service, err := manager.OpenService(Name)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return fmt.Errorf("service %q is not installed", Name)
	}
	if err != nil {
		return serviceManagerError("opening Family Friend service", err)
	}
	defer service.Close()

	if err := stopService(service); err != nil {
		return err
	}
	if err := service.Delete(); err != nil {
		return serviceManagerError("removing Family Friend service", err)
	}
	return nil
}

func Start() error {
	manager, err := mgr.Connect()
	if err != nil {
		return serviceManagerError("connecting to Windows Service Control Manager", err)
	}
	defer manager.Disconnect()

	service, err := manager.OpenService(Name)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return fmt.Errorf("service %q is not installed", Name)
	}
	if err != nil {
		return serviceManagerError("opening Family Friend service", err)
	}
	defer service.Close()

	status, err := service.Query()
	if err != nil {
		return serviceManagerError("querying Family Friend service", err)
	}
	if status.State == svc.Running || status.State == svc.StartPending {
		return nil
	}
	if err := service.Start(); err != nil {
		return serviceManagerError("starting Family Friend service", err)
	}
	return nil
}

func CurrentStatus() (Status, error) {
	manager, err := mgr.Connect()
	if err != nil {
		return Status{}, serviceManagerError("connecting to Windows Service Control Manager", err)
	}
	defer manager.Disconnect()

	service, err := manager.OpenService(Name)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return Status{Installed: false}, nil
	}
	if err != nil {
		return Status{}, serviceManagerError("opening Family Friend service", err)
	}
	defer service.Close()

	status, err := service.Query()
	if err != nil {
		return Status{}, serviceManagerError("querying Family Friend service", err)
	}
	configuration, err := service.Config()
	if err != nil {
		return Status{}, serviceManagerError("reading Family Friend service configuration", err)
	}

	return Status{
		Installed:      true,
		State:          serviceState(status.State),
		StartMode:      serviceStartMode(configuration.StartType),
		ExecutablePath: configuration.BinaryPathName,
	}, nil
}

func resolveExecutablePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("service executable path is required")
	}

	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving service executable path: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("checking service executable %q: %w", resolved, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("service executable %q is a directory", resolved)
	}
	return resolved, nil
}

func stopService(service *mgr.Service) error {
	status, err := service.Query()
	if err != nil {
		return serviceManagerError("querying Family Friend service", err)
	}
	if status.State == svc.Stopped {
		return nil
	}

	if _, err := service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return serviceManagerError("stopping Family Friend service", err)
	}

	deadline := time.Now().Add(uninstallStopTimeout)
	for time.Now().Before(deadline) {
		status, err = service.Query()
		if err != nil {
			return serviceManagerError("querying Family Friend service while stopping", err)
		}
		if status.State == svc.Stopped {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}

	return fmt.Errorf("stopping Family Friend service: timed out after %s", uninstallStopTimeout)
}

func serviceState(state svc.State) State {
	switch state {
	case svc.Stopped:
		return StateStopped
	case svc.Running:
		return StateRunning
	case svc.Paused:
		return StatePaused
	case svc.StartPending, svc.StopPending, svc.ContinuePending, svc.PausePending:
		return StatePending
	default:
		return StateUnknown
	}
}

func serviceStartMode(startType uint32) StartMode {
	switch startType {
	case mgr.StartAutomatic:
		return StartModeAutomatic
	case mgr.StartManual:
		return StartModeManual
	case mgr.StartDisabled:
		return StartModeDisabled
	default:
		return StartModeUnknown
	}
}

func serviceManagerError(action string, err error) error {
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return fmt.Errorf("%s: administrator privileges are required: %w", action, err)
	}
	return fmt.Errorf("%s: %w", action, err)
}
