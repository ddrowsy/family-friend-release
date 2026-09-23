//go:build windows

package desktopui

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	loginStartupRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	loginStartupValueName    = "FamilyFriendUI"
)

type windowsLoginStartupStore struct{}

// NewLoginStartup creates the Windows per-user login startup manager.
func NewLoginStartup() *LoginStartup {
	return newLoginStartup(windowsLoginStartupStore{})
}

func (windowsLoginStartupStore) Read() (string, bool, error) {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		loginStartupRegistryPath,
		registry.QUERY_VALUE,
	)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("opening startup registry key: %w", err)
	}
	defer key.Close()

	command, _, err := key.GetStringValue(loginStartupValueName)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading startup registry value: %w", err)
	}
	return command, true, nil
}

func (windowsLoginStartupStore) Write(command string) error {
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		loginStartupRegistryPath,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("opening startup registry key for writing: %w", err)
	}
	defer key.Close()

	if err := key.SetStringValue(loginStartupValueName, command); err != nil {
		return fmt.Errorf("writing startup registry value: %w", err)
	}
	return nil
}

func (windowsLoginStartupStore) Delete() error {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		loginStartupRegistryPath,
		registry.SET_VALUE,
	)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("opening startup registry key for deletion: %w", err)
	}
	defer key.Close()

	if err := key.DeleteValue(loginStartupValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("deleting startup registry value: %w", err)
	}
	return nil
}
