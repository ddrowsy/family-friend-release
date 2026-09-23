package kidcontrol

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

const (
	DefaultAppControlScanIntervalSeconds       = 2
	DefaultWindowMonitoringScanIntervalSeconds = 2
)

// Config contains kidcontrol settings.
type Config struct {
	Mode     string
	Settings policy.KidControlSettings
}

// ParseSettings converts raw module settings into typed KidControl settings.
func ParseSettings(raw map[string]any) (policy.KidControlSettings, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return policy.KidControlSettings{}, fmt.Errorf("encode settings: %w", err)
	}

	var settings policy.KidControlSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return policy.KidControlSettings{}, fmt.Errorf("decode settings: %w", err)
	}

	applyDefaults(&settings, raw)
	return settings, nil
}

func applyDefaults(settings *policy.KidControlSettings, raw map[string]any) {
	if _, ok := raw["dry_run"]; !ok {
		settings.DryRun = true
	}
	if settings.AppControl.ScanIntervalSeconds == 0 {
		settings.AppControl.ScanIntervalSeconds = DefaultAppControlScanIntervalSeconds
	}
	if settings.WindowMonitoring.ScanIntervalSeconds == 0 {
		settings.WindowMonitoring.ScanIntervalSeconds = DefaultWindowMonitoringScanIntervalSeconds
	}
}

func validateSettings(settings policy.KidControlSettings) error {
	if settings.AppControl.Enabled && settings.AppControl.ScanIntervalSeconds < 0 {
		return fmt.Errorf("app_control.scan_interval_seconds must be positive when app control is enabled")
	}
	if settings.WindowMonitoring.Enabled && settings.WindowMonitoring.ScanIntervalSeconds < 0 {
		return fmt.Errorf("window_monitoring.scan_interval_seconds must be positive when window monitoring is enabled")
	}

	for i, app := range settings.AppControl.BlockedApps {
		if strings.TrimSpace(app) == "" {
			return fmt.Errorf("app_control.blocked_apps[%d] is empty", i)
		}
	}
	for i, keyword := range settings.WindowMonitoring.BlockedTitleKeywords {
		if strings.TrimSpace(keyword) == "" {
			return fmt.Errorf("window_monitoring.blocked_title_keywords[%d] is empty", i)
		}
	}

	if settings.ScreenMonitoring.Enabled && settings.ScreenMonitoring.SampleIntervalSeconds <= 0 {
		return fmt.Errorf("screen_monitoring.sample_interval_seconds must be positive when screen monitoring is enabled")
	}

	return validateBrowserMonitoringSettings(settings.BrowserMonitoring)
}

func validateBrowserMonitoringSettings(settings policy.BrowserMonitoringSettings) error {
	if !settings.Enabled {
		return nil
	}
	if strings.TrimSpace(settings.DefaultProfile) == "" {
		return fmt.Errorf("browser_monitoring.default_profile is required when browser monitoring is enabled")
	}
	if _, ok := settings.Profiles[settings.DefaultProfile]; !ok {
		return fmt.Errorf("browser_monitoring.default_profile %q does not exist", settings.DefaultProfile)
	}

	for name, profile := range settings.Profiles {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("browser_monitoring.profiles contains an empty profile name")
		}
		if err := validateBrowserProfile(profile); err != nil {
			return fmt.Errorf("browser_monitoring.profiles[%q]: %w", name, err)
		}
	}

	return nil
}

func validateBrowserProfile(profile policy.BrowserProfile) error {
	switch profile.Policy {
	case policy.BrowserPolicyAllowList, policy.BrowserPolicyBlockList:
	default:
		return fmt.Errorf(
			"policy must be %q or %q",
			policy.BrowserPolicyAllowList,
			policy.BrowserPolicyBlockList,
		)
	}

	for i, rule := range profile.AllowedURLs {
		if strings.TrimSpace(rule) == "" {
			return fmt.Errorf("allowed_urls[%d] is empty", i)
		}
	}
	for i, rule := range profile.BlockedURLs {
		if strings.TrimSpace(rule) == "" {
			return fmt.Errorf("blocked_urls[%d] is empty", i)
		}
	}

	return nil
}
