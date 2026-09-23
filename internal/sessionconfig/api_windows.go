//go:build windows

package sessionconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Enable configures one Windows child account to start the session worker at login.
func Enable(childSID string, workerPath string) error {
	configuration, err := prepareConfiguration(childSID, workerPath)
	if err != nil {
		return err
	}
	return newManager(registryStore{}).enable(configuration)
}

// Disable removes the configured child's session-worker login startup.
func Disable() error {
	return newManager(registryStore{}).disable()
}

// CurrentStatus returns the configured child and login-startup state.
func CurrentStatus() (Status, error) {
	return newManager(registryStore{}).status()
}

// Load returns the machine-level configured child session worker.
func Load() (Configuration, bool, error) {
	return registryStore{}.load()
}

func prepareConfiguration(childSID string, workerPath string) (Configuration, error) {
	childSID = strings.TrimSpace(childSID)
	sid, err := windows.StringToSid(childSID)
	if err != nil {
		return Configuration{}, fmt.Errorf("parsing child SID %q: %w", childSID, err)
	}

	workerPath = strings.TrimSpace(workerPath)
	if workerPath == "" {
		return Configuration{}, fmt.Errorf("worker executable path is required")
	}
	workerPath, err = filepath.Abs(workerPath)
	if err != nil {
		return Configuration{}, fmt.Errorf("resolving worker executable path: %w", err)
	}
	workerPath = filepath.Clean(workerPath)

	info, err := os.Stat(workerPath)
	if err != nil {
		return Configuration{}, fmt.Errorf("checking worker executable %q: %w", workerPath, err)
	}
	if info.IsDir() {
		return Configuration{}, fmt.Errorf("worker executable %q is a directory", workerPath)
	}

	return Configuration{
		ChildSID:   sid.String(),
		WorkerPath: workerPath,
	}, nil
}
