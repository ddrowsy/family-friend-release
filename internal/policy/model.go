package policy

import "time"

const (
	KidControlModuleName = "kidcontrol"

	BrowserPolicyAllowList = "allow_list"
	BrowserPolicyBlockList = "block_list"
)

// ProfilePolicy describes the policy JSON owned by one child profile.
type ProfilePolicy struct {
	SchemaVersion int                     `json:"schema_version"`
	Version       string                  `json:"version"`
	Modules       map[string]ModulePolicy `json:"modules"`
}

// ModulePolicy describes one configured policy module.
type ModulePolicy struct {
	Enabled  bool           `json:"enabled"`
	Mode     string         `json:"mode"`
	Settings map[string]any `json:"settings"`
}

// KidControlSettings describes the persisted KidControl policy settings.
type KidControlSettings struct {
	DryRun            bool                      `json:"dry_run"`
	AppControl        AppControlSettings        `json:"app_control"`
	WindowMonitoring  WindowMonitoringSettings  `json:"window_monitoring"`
	ScreenMonitoring  ScreenMonitoringSettings  `json:"screen_monitoring"`
	BrowserMonitoring BrowserMonitoringSettings `json:"browser_monitoring"`
}

// AppControlSettings describes process-level application controls.
type AppControlSettings struct {
	Enabled             bool     `json:"enabled"`
	ScanIntervalSeconds int      `json:"scan_interval_seconds"`
	BlockedApps         []string `json:"blocked_apps"`
}

// WindowMonitoringSettings describes active-window controls.
type WindowMonitoringSettings struct {
	Enabled              bool     `json:"enabled"`
	ScanIntervalSeconds  int      `json:"scan_interval_seconds"`
	BlockedTitleKeywords []string `json:"blocked_title_keywords"`
}

// ScreenMonitoringSettings describes screen-sampling controls.
type ScreenMonitoringSettings struct {
	Enabled               bool `json:"enabled"`
	SampleIntervalSeconds int  `json:"sample_interval_seconds"`
}

// BrowserMonitoringSettings describes browser profiles and current profile state.
type BrowserMonitoringSettings struct {
	Enabled        bool                      `json:"enabled"`
	DefaultProfile string                    `json:"default_profile"`
	ActiveProfile  *ActiveBrowserProfile     `json:"active_profile,omitempty"`
	Profiles       map[string]BrowserProfile `json:"profiles"`
}

// BrowserProfile describes one named browser policy profile.
type BrowserProfile struct {
	Policy               string                     `json:"policy"`
	AllowedURLs          []string                   `json:"allowed_urls"`
	BlockedURLs          []string                   `json:"blocked_urls"`
	BlockInappropriate   bool                       `json:"block_inappropriate"`
	TemporaryAllowedURLs []TemporaryBrowserApproval `json:"temporary_allowed_urls,omitempty"`
}

// ActiveBrowserProfile describes the selected browser profile and optional expiry.
type ActiveBrowserProfile struct {
	Name            string     `json:"name"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	FallbackProfile string     `json:"fallback_profile"`
}

// TemporaryBrowserApproval describes one time-limited allowed URL rule.
type TemporaryBrowserApproval struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}
