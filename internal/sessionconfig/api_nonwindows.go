//go:build !windows

package sessionconfig

// Enable reports that session-worker configuration requires Windows.
func Enable(string, string) error {
	return ErrWindowsOnly
}

// Disable reports that session-worker configuration requires Windows.
func Disable() error {
	return ErrWindowsOnly
}

// CurrentStatus reports that session-worker configuration requires Windows.
func CurrentStatus() (Status, error) {
	return Status{}, ErrWindowsOnly
}

// Load reports that session-worker configuration requires Windows.
func Load() (Configuration, bool, error) {
	return Configuration{}, false, ErrWindowsOnly
}
