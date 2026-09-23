//go:build windows

package sessionconfig

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	machineKeyPath   = `SOFTWARE\FamilyFriend\SessionWorker`
	childSIDValue    = "ChildSID"
	workerPathValue  = "WorkerPath"
	startupValueName = "FamilyFriendSessionWorker"
	startupKeySuffix = `\Software\Microsoft\Windows\CurrentVersion\Run`
)

type registryStore struct{}

func (registryStore) load() (Configuration, bool, error) {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		machineKeyPath,
		registry.READ|registry.WOW64_64KEY,
	)
	if errors.Is(err, registry.ErrNotExist) {
		return Configuration{}, false, nil
	}
	if err != nil {
		return Configuration{}, false, fmt.Errorf("opening session worker configuration: %w", err)
	}
	defer key.Close()

	childSID, _, sidErr := key.GetStringValue(childSIDValue)
	workerPath, _, pathErr := key.GetStringValue(workerPathValue)
	sidMissing := errors.Is(sidErr, registry.ErrNotExist)
	pathMissing := errors.Is(pathErr, registry.ErrNotExist)
	if sidMissing && pathMissing {
		return Configuration{}, false, nil
	}
	if sidErr != nil {
		return Configuration{}, false, fmt.Errorf("reading configured child SID: %w", sidErr)
	}
	if pathErr != nil {
		return Configuration{}, false, fmt.Errorf("reading configured worker path: %w", pathErr)
	}

	return Configuration{
		ChildSID:   childSID,
		WorkerPath: workerPath,
	}, true, nil
}

func (registryStore) save(configuration Configuration) error {
	key, _, err := registry.CreateKey(
		registry.LOCAL_MACHINE,
		machineKeyPath,
		registry.SET_VALUE|registry.WOW64_64KEY,
	)
	if err != nil {
		return fmt.Errorf("creating session worker configuration: %w", err)
	}
	defer key.Close()

	if err := key.SetStringValue(childSIDValue, configuration.ChildSID); err != nil {
		return fmt.Errorf("storing configured child SID: %w", err)
	}
	if err := key.SetStringValue(workerPathValue, configuration.WorkerPath); err != nil {
		_ = key.DeleteValue(childSIDValue)
		return fmt.Errorf("storing configured worker path: %w", err)
	}
	return nil
}

func (registryStore) clear() error {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		machineKeyPath,
		registry.SET_VALUE|registry.WOW64_64KEY,
	)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("opening session worker configuration: %w", err)
	}
	defer key.Close()

	if err := deleteValue(key, childSIDValue); err != nil {
		return fmt.Errorf("removing configured child SID: %w", err)
	}
	if err := deleteValue(key, workerPathValue); err != nil {
		return fmt.Errorf("removing configured worker path: %w", err)
	}
	return nil
}

func (registryStore) startupCommand(childSID string) (string, bool, error) {
	if err := ensureUserHive(childSID); err != nil {
		return "", false, err
	}

	key, err := registry.OpenKey(
		registry.USERS,
		startupKeyPath(childSID),
		registry.QUERY_VALUE,
	)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("opening child login startup key: %w", err)
	}
	defer key.Close()

	command, _, err := key.GetStringValue(startupValueName)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading session worker login startup: %w", err)
	}
	return command, true, nil
}

func (registryStore) setStartup(childSID string, command string) error {
	if err := ensureUserHive(childSID); err != nil {
		return err
	}

	key, _, err := registry.CreateKey(
		registry.USERS,
		startupKeyPath(childSID),
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("creating child login startup key: %w", err)
	}
	defer key.Close()

	if err := key.SetStringValue(startupValueName, command); err != nil {
		return fmt.Errorf("setting session worker login startup: %w", err)
	}
	return nil
}

func (registryStore) clearStartup(childSID string) error {
	if err := ensureUserHive(childSID); err != nil {
		return err
	}

	key, err := registry.OpenKey(
		registry.USERS,
		startupKeyPath(childSID),
		registry.SET_VALUE,
	)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("opening child login startup key: %w", err)
	}
	defer key.Close()

	if err := deleteValue(key, startupValueName); err != nil {
		return fmt.Errorf("removing session worker login startup: %w", err)
	}
	return nil
}

func ensureUserHive(childSID string) error {
	key, err := registry.OpenKey(registry.USERS, childSID, registry.READ)
	if errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("%w: %q", ErrUserHiveUnavailable, childSID)
	}
	if err != nil {
		return fmt.Errorf("opening configured child registry hive %q: %w", childSID, err)
	}
	return key.Close()
}

func startupKeyPath(childSID string) string {
	return childSID + startupKeySuffix
}

func deleteValue(key registry.Key, name string) error {
	err := key.DeleteValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}
