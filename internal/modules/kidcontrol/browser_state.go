package kidcontrol

import (
	"fmt"
	"strings"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

// SetBrowserProfile adds or replaces a browser profile.
func (m *Module) SetBrowserProfile(name string, profile policy.BrowserProfile) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("browser profile name is required")
	}
	if err := validateBrowserProfile(profile); err != nil {
		return fmt.Errorf("browser profile %q: %w", name, err)
	}

	m.browserMu.Lock()
	defer m.browserMu.Unlock()

	if m.config.Settings.BrowserMonitoring.Profiles == nil {
		m.config.Settings.BrowserMonitoring.Profiles = make(map[string]policy.BrowserProfile)
	}
	m.config.Settings.BrowserMonitoring.Profiles[name] = profile
	return nil
}

// DeleteBrowserProfile deletes a browser profile.
func (m *Module) DeleteBrowserProfile(name string) error {
	m.browserMu.Lock()
	defer m.browserMu.Unlock()

	settings := &m.config.Settings.BrowserMonitoring
	if name == settings.DefaultProfile {
		return fmt.Errorf("cannot delete default browser profile %q", name)
	}
	if _, ok := settings.Profiles[name]; !ok {
		return fmt.Errorf("browser profile %q does not exist", name)
	}
	delete(settings.Profiles, name)
	return nil
}

// ActivateBrowserProfile selects a browser profile temporarily.
// A duration <= 0 keeps the profile active until changed.
func (m *Module) ActivateBrowserProfile(
	name string,
	duration time.Duration,
	fallbackProfile string,
) error {
	return m.activateBrowserProfile(name, duration, fallbackProfile, time.Now())
}

func (m *Module) activateBrowserProfile(
	name string,
	duration time.Duration,
	fallbackProfile string,
	now time.Time,
) error {
	m.browserMu.Lock()
	defer m.browserMu.Unlock()

	settings := &m.config.Settings.BrowserMonitoring
	if _, ok := settings.Profiles[name]; !ok {
		return fmt.Errorf("browser profile %q does not exist", name)
	}
	if fallbackProfile == "" {
		fallbackProfile = settings.DefaultProfile
	}
	if _, ok := settings.Profiles[fallbackProfile]; !ok {
		return fmt.Errorf("browser fallback profile %q does not exist", fallbackProfile)
	}

	var expiresAt *time.Time
	if duration > 0 {
		expiry := now.Add(duration)
		expiresAt = &expiry
	}

	settings.ActiveProfile = &policy.ActiveBrowserProfile{
		Name:            name,
		ExpiresAt:       expiresAt,
		FallbackProfile: fallbackProfile,
	}
	return nil
}

// ResolveBrowserProfile returns the profile that applies at the supplied time.
func (m *Module) ResolveBrowserProfile(
	now time.Time,
) (string, policy.BrowserProfile, error) {
	m.browserMu.Lock()
	defer m.browserMu.Unlock()

	settings := &m.config.Settings.BrowserMonitoring
	if !settings.Enabled {
		return "", policy.BrowserProfile{}, fmt.Errorf("browser monitoring is disabled")
	}

	name := settings.DefaultProfile
	if settings.ActiveProfile != nil {
		active := settings.ActiveProfile
		expired := active.ExpiresAt != nil && !now.Before(*active.ExpiresAt)
		_, activeExists := settings.Profiles[active.Name]

		if !expired && activeExists {
			name = active.Name
		} else {
			if _, fallbackExists := settings.Profiles[active.FallbackProfile]; fallbackExists {
				name = active.FallbackProfile
			}
			settings.ActiveProfile = nil
		}
	}

	profile, ok := settings.Profiles[name]
	if !ok {
		return "", policy.BrowserProfile{}, fmt.Errorf("browser profile %q does not exist", name)
	}
	return name, profile, nil
}
