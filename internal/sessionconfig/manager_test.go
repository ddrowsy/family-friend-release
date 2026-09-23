package sessionconfig

import (
	"errors"
	"testing"
)

func TestEnableCreatesMatchingStartupAndConfiguration(t *testing.T) {
	store := &storeStub{}
	configuration := Configuration{
		ChildSID:   "S-1-5-21-1001",
		WorkerPath: `C:\Family Friend\worker.exe`,
	}

	if err := newManager(store).enable(configuration); err != nil {
		t.Fatalf("enable() error = %v", err)
	}
	if store.configuration != configuration {
		t.Fatalf("configuration = %#v, want %#v", store.configuration, configuration)
	}
	if store.startupSID != configuration.ChildSID {
		t.Fatalf("startup SID = %q, want %q", store.startupSID, configuration.ChildSID)
	}
	if store.startup != startupCommand(configuration.WorkerPath) {
		t.Fatalf("startup command = %q", store.startup)
	}
}

func TestEnableSameConfigurationIsIdempotentAndRepairsStartup(t *testing.T) {
	configuration := Configuration{
		ChildSID:   "S-1-5-21-1001",
		WorkerPath: `C:\Family Friend\worker.exe`,
	}
	store := &storeStub{
		configuration: configuration,
		configured:    true,
		startup:       `"C:\old-worker.exe"`,
	}

	if err := newManager(store).enable(configuration); err != nil {
		t.Fatalf("enable() error = %v", err)
	}
	if store.saveCalls != 0 {
		t.Fatalf("save calls = %d, want 0", store.saveCalls)
	}
	if store.startup != startupCommand(configuration.WorkerPath) {
		t.Fatalf("startup command = %q", store.startup)
	}
}

func TestEnableRejectsDifferentConfiguration(t *testing.T) {
	store := &storeStub{
		configuration: Configuration{
			ChildSID:   "S-1-5-21-1001",
			WorkerPath: `C:\worker.exe`,
		},
		configured: true,
	}

	err := newManager(store).enable(Configuration{
		ChildSID:   "S-1-5-21-2002",
		WorkerPath: `D:\worker.exe`,
	})
	if !errors.Is(err, ErrConfigurationConflict) {
		t.Fatalf("enable() error = %v, want %v", err, ErrConfigurationConflict)
	}
	if store.setStartupCalls != 0 {
		t.Fatalf("set startup calls = %d, want 0", store.setStartupCalls)
	}
}

func TestEnableRollsBackStartupWhenConfigurationSaveFails(t *testing.T) {
	saveErr := errors.New("save failed")
	store := &storeStub{saveErr: saveErr}

	err := newManager(store).enable(Configuration{
		ChildSID:   "S-1-5-21-1001",
		WorkerPath: `C:\worker.exe`,
	})
	if !errors.Is(err, saveErr) {
		t.Fatalf("enable() error = %v, want %v", err, saveErr)
	}
	if store.clearStartupCalls != 1 {
		t.Fatalf("clear startup calls = %d, want 1", store.clearStartupCalls)
	}
}

func TestDisableRemovesConfiguredStartupBeforeConfiguration(t *testing.T) {
	configuration := Configuration{
		ChildSID:   "S-1-5-21-1001",
		WorkerPath: `C:\worker.exe`,
	}
	store := &storeStub{
		configuration: configuration,
		configured:    true,
		startup:       startupCommand(configuration.WorkerPath),
	}

	if err := newManager(store).disable(); err != nil {
		t.Fatalf("disable() error = %v", err)
	}
	if store.clearStartupCalls != 1 {
		t.Fatalf("clear startup calls = %d, want 1", store.clearStartupCalls)
	}
	if store.clearCalls != 1 {
		t.Fatalf("clear calls = %d, want 1", store.clearCalls)
	}
}

func TestStatusReportsMissingAndMismatchedStartup(t *testing.T) {
	configuration := Configuration{
		ChildSID:   "S-1-5-21-1001",
		WorkerPath: `C:\worker.exe`,
	}
	store := &storeStub{
		configuration:     configuration,
		configured:        true,
		startup:           `"C:\other.exe"`,
		startupRegistered: true,
	}

	status, err := newManager(store).status()
	if err != nil {
		t.Fatalf("status() error = %v", err)
	}
	if !status.Configured || !status.StartupRegistered {
		t.Fatalf("status = %#v", status)
	}
	if status.StartupMatches {
		t.Fatalf("startup unexpectedly matches: %#v", status)
	}
}

type storeStub struct {
	configuration     Configuration
	configured        bool
	startup           string
	startupSID        string
	startupRegistered bool
	saveErr           error
	saveCalls         int
	clearCalls        int
	setStartupCalls   int
	clearStartupCalls int
}

func (s *storeStub) load() (Configuration, bool, error) {
	return s.configuration, s.configured, nil
}

func (s *storeStub) save(configuration Configuration) error {
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.configuration = configuration
	s.configured = true
	return nil
}

func (s *storeStub) clear() error {
	s.clearCalls++
	s.configuration = Configuration{}
	s.configured = false
	return nil
}

func (s *storeStub) startupCommand(string) (string, bool, error) {
	return s.startup, s.startupRegistered, nil
}

func (s *storeStub) setStartup(childSID string, command string) error {
	s.setStartupCalls++
	s.startupSID = childSID
	s.startup = command
	s.startupRegistered = true
	return nil
}

func (s *storeStub) clearStartup(string) error {
	s.clearStartupCalls++
	s.startup = ""
	s.startupRegistered = false
	return nil
}
