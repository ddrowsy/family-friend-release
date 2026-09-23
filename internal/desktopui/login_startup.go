package desktopui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const loginStartupArgument = "--startup"

type loginStartupStore interface {
	Read() (string, bool, error)
	Write(command string) error
	Delete() error
}

// LoginStartup manages per-user desktop UI startup registration.
type LoginStartup struct {
	store loginStartupStore
}

func newLoginStartup(store loginStartupStore) *LoginStartup {
	return &LoginStartup{store: store}
}

// Enable registers the current UI executable to start at user login.
func (s *LoginStartup) Enable(executablePath string) error {
	command, err := loginStartupCommand(executablePath)
	if err != nil {
		return err
	}
	if err := s.store.Write(command); err != nil {
		return fmt.Errorf("enabling login startup: %w", err)
	}
	return nil
}

// Disable removes the Family Friend login startup registration.
func (s *LoginStartup) Disable() error {
	if err := s.store.Delete(); err != nil {
		return fmt.Errorf("disabling login startup: %w", err)
	}
	return nil
}

// Enabled reports whether login startup points at the supplied UI executable.
func (s *LoginStartup) Enabled(executablePath string) (bool, error) {
	expectedCommand, err := loginStartupCommand(executablePath)
	if err != nil {
		return false, err
	}

	command, exists, err := s.store.Read()
	if err != nil {
		return false, fmt.Errorf("checking login startup: %w", err)
	}
	if !exists {
		return false, nil
	}
	return command == expectedCommand, nil
}

func loginStartupCommand(executablePath string) (string, error) {
	if strings.TrimSpace(executablePath) == "" {
		return "", errors.New("UI executable path is required")
	}

	absolutePath, err := filepath.Abs(executablePath)
	if err != nil {
		return "", fmt.Errorf("resolving UI executable path %q: %w", executablePath, err)
	}
	if strings.Contains(absolutePath, `"`) {
		return "", fmt.Errorf("UI executable path %q contains an unsupported quote", absolutePath)
	}

	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("checking UI executable %q: %w", absolutePath, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("UI executable %q is not a regular file", absolutePath)
	}

	return `"` + absolutePath + `" ` + loginStartupArgument, nil
}
