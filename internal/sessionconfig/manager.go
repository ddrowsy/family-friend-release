package sessionconfig

import (
	"errors"
	"fmt"
)

var (
	ErrConfigurationConflict = errors.New(
		"session worker is already configured differently; disable it before changing child or path",
	)
	ErrUserHiveUnavailable = errors.New("configured child registry hive is unavailable")
	ErrWindowsOnly         = errors.New("session worker configuration requires Windows")
)

// Configuration identifies the Windows child session worker selected for protection.
type Configuration struct {
	ChildSID   string
	WorkerPath string
}

// Status describes the configured worker and its login startup registration.
type Status struct {
	Configured        bool
	Configuration     Configuration
	StartupRegistered bool
	StartupMatches    bool
	StartupCommand    string
}

type store interface {
	load() (Configuration, bool, error)
	save(Configuration) error
	clear() error
	startupCommand(childSID string) (string, bool, error)
	setStartup(childSID string, command string) error
	clearStartup(childSID string) error
}

type manager struct {
	store store
}

func newManager(store store) *manager {
	return &manager{store: store}
}

func (m *manager) enable(configuration Configuration) error {
	existing, configured, err := m.store.load()
	if err != nil {
		return err
	}
	if configured && existing != configuration {
		return fmt.Errorf(
			"%w: child SID %q, worker %q",
			ErrConfigurationConflict,
			existing.ChildSID,
			existing.WorkerPath,
		)
	}

	command := startupCommand(configuration.WorkerPath)
	if err := m.store.setStartup(configuration.ChildSID, command); err != nil {
		return err
	}
	if configured {
		return nil
	}

	if err := m.store.save(configuration); err != nil {
		_ = m.store.clearStartup(configuration.ChildSID)
		return err
	}
	return nil
}

func (m *manager) disable() error {
	configuration, configured, err := m.store.load()
	if err != nil {
		return err
	}
	if !configured {
		return nil
	}

	if err := m.store.clearStartup(configuration.ChildSID); err != nil {
		return err
	}
	return m.store.clear()
}

func (m *manager) status() (Status, error) {
	configuration, configured, err := m.store.load()
	if err != nil {
		return Status{}, err
	}
	if !configured {
		return Status{}, nil
	}

	command, registered, err := m.store.startupCommand(configuration.ChildSID)
	if err != nil {
		return Status{}, err
	}
	wantCommand := startupCommand(configuration.WorkerPath)
	return Status{
		Configured:        true,
		Configuration:     configuration,
		StartupRegistered: registered,
		StartupMatches:    registered && command == wantCommand,
		StartupCommand:    command,
	}, nil
}

func startupCommand(workerPath string) string {
	return `"` + workerPath + `"`
}
