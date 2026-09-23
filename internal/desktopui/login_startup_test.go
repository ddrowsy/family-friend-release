package desktopui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeLoginStartupStore struct {
	command   string
	exists    bool
	readErr   error
	writeErr  error
	deleteErr error
}

func (s *fakeLoginStartupStore) Read() (string, bool, error) {
	return s.command, s.exists, s.readErr
}

func (s *fakeLoginStartupStore) Write(command string) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.command = command
	s.exists = true
	return nil
}

func (s *fakeLoginStartupStore) Delete() error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.command = ""
	s.exists = false
	return nil
}

func TestLoginStartupEnableIsIdempotent(t *testing.T) {
	executablePath := createTestExecutable(t)
	store := &fakeLoginStartupStore{}
	startup := newLoginStartup(store)

	if err := startup.Enable(executablePath); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	firstCommand := store.command
	if err := startup.Enable(executablePath); err != nil {
		t.Fatalf("Enable() second call error = %v", err)
	}

	absolutePath, err := filepath.Abs(executablePath)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	want := `"` + absolutePath + `" --startup`
	if store.command != want {
		t.Fatalf("stored command = %q, want %q", store.command, want)
	}
	if store.command != firstCommand {
		t.Fatalf("second Enable() changed command from %q to %q", firstCommand, store.command)
	}
}

func TestLoginStartupEnabledMatchesCurrentExecutable(t *testing.T) {
	executablePath := createTestExecutable(t)
	store := &fakeLoginStartupStore{}
	startup := newLoginStartup(store)
	if err := startup.Enable(executablePath); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	enabled, err := startup.Enabled(executablePath)
	if err != nil {
		t.Fatalf("Enabled() error = %v", err)
	}
	if !enabled {
		t.Fatal("Enabled() = false, want true")
	}

	otherExecutablePath := createTestExecutable(t)
	enabled, err = startup.Enabled(otherExecutablePath)
	if err != nil {
		t.Fatalf("Enabled() for moved executable error = %v", err)
	}
	if enabled {
		t.Fatal("Enabled() for moved executable = true, want false")
	}
}

func TestLoginStartupDisableIsIdempotent(t *testing.T) {
	store := &fakeLoginStartupStore{}
	startup := newLoginStartup(store)

	if err := startup.Disable(); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if err := startup.Disable(); err != nil {
		t.Fatalf("Disable() second call error = %v", err)
	}
}

func TestLoginStartupRejectsMissingExecutable(t *testing.T) {
	startup := newLoginStartup(&fakeLoginStartupStore{})
	missingPath := filepath.Join(t.TempDir(), "missing.exe")

	if err := startup.Enable(missingPath); err == nil {
		t.Fatal("Enable() error = nil, want missing executable error")
	}
}

func TestLoginStartupPropagatesStoreErrors(t *testing.T) {
	executablePath := createTestExecutable(t)
	writeErr := errors.New("write failed")
	startup := newLoginStartup(&fakeLoginStartupStore{writeErr: writeErr})

	if err := startup.Enable(executablePath); !errors.Is(err, writeErr) {
		t.Fatalf("Enable() error = %v, want wrapped %v", err, writeErr)
	}
}

func createTestExecutable(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "drowsyfriend-ui.exe")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	return path
}
