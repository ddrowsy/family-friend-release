package kidcontrol

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

func TestParseSettingsValid(t *testing.T) {
	settings, err := ParseSettings(map[string]interface{}{
		"app_control": map[string]interface{}{
			"enabled":      true,
			"blocked_apps": []interface{}{"msedge.exe", "brave.exe"},
		},
		"window_monitoring": map[string]interface{}{
			"enabled":                true,
			"blocked_title_keywords": []interface{}{"inprivate", "restricted"},
		},
		"screen_monitoring": map[string]interface{}{
			"enabled":                 true,
			"sample_interval_seconds": float64(3),
		},
	})
	if err != nil {
		t.Fatalf("ParseSettings returned error: %v", err)
	}
	if err := validateSettings(settings); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if !settings.AppControl.Enabled {
		t.Fatal("AppControl.Enabled = false, want true")
	}
	if !settings.DryRun {
		t.Fatal("DryRun = false, want true")
	}
	if got, want := settings.AppControl.ScanIntervalSeconds, DefaultAppControlScanIntervalSeconds; got != want {
		t.Fatalf("ScanIntervalSeconds = %d, want %d", got, want)
	}
	if got, want := settings.AppControl.BlockedApps[0], "msedge.exe"; got != want {
		t.Fatalf("BlockedApps[0] = %q, want %q", got, want)
	}
	if !settings.WindowMonitoring.Enabled {
		t.Fatal("WindowMonitoring.Enabled = false, want true")
	}
	if got, want := settings.WindowMonitoring.ScanIntervalSeconds, DefaultWindowMonitoringScanIntervalSeconds; got != want {
		t.Fatalf("Window ScanIntervalSeconds = %d, want %d", got, want)
	}
	if got, want := settings.WindowMonitoring.BlockedTitleKeywords[0], "inprivate"; got != want {
		t.Fatalf("BlockedTitleKeywords[0] = %q, want %q", got, want)
	}
	if got, want := settings.ScreenMonitoring.SampleIntervalSeconds, 3; got != want {
		t.Fatalf("SampleIntervalSeconds = %d, want %d", got, want)
	}
}

func TestSettingsValidateRejectsNegativeWindowScanInterval(t *testing.T) {
	settings := policy.KidControlSettings{
		WindowMonitoring: policy.WindowMonitoringSettings{
			Enabled:             true,
			ScanIntervalSeconds: -1,
		},
	}

	err := validateSettings(settings)
	if err == nil || !strings.Contains(err.Error(), "window_monitoring.scan_interval_seconds") {
		t.Fatalf("Validate error = %v, want window scan interval error", err)
	}
}

func TestSettingsValidateRejectsEmptyBlockedTitleKeyword(t *testing.T) {
	settings := policy.KidControlSettings{
		WindowMonitoring: policy.WindowMonitoringSettings{
			BlockedTitleKeywords: []string{"restricted", " "},
		},
	}

	err := validateSettings(settings)
	if err == nil || !strings.Contains(err.Error(), "blocked_title_keywords") {
		t.Fatalf("Validate error = %v, want blocked_title_keywords error", err)
	}
}

func TestParseSettingsPreservesExplicitDryRunFalse(t *testing.T) {
	settings, err := ParseSettings(map[string]interface{}{
		"dry_run": false,
		"app_control": map[string]interface{}{
			"enabled":      true,
			"blocked_apps": []interface{}{"msedge.exe"},
		},
	})
	if err != nil {
		t.Fatalf("ParseSettings returned error: %v", err)
	}

	if settings.DryRun {
		t.Fatal("DryRun = true, want false")
	}
}

func TestAppControlSettingsDoesNotOwnDryRun(t *testing.T) {
	if _, ok := reflect.TypeOf(policy.AppControlSettings{}).FieldByName("DryRun"); ok {
		t.Fatal("AppControlSettings has DryRun field, want root Settings.DryRun only")
	}
}

func TestSettingsValidateRejectsNegativeAppScanInterval(t *testing.T) {
	settings := policy.KidControlSettings{
		AppControl: policy.AppControlSettings{
			Enabled:             true,
			ScanIntervalSeconds: -1,
			BlockedApps:         []string{"msedge.exe"},
		},
	}

	err := validateSettings(settings)
	if err == nil || !strings.Contains(err.Error(), "scan_interval_seconds") {
		t.Fatalf("Validate error = %v, want scan_interval_seconds error", err)
	}
}

func TestSettingsValidateRejectsInvalidScreenSampleInterval(t *testing.T) {
	settings := policy.KidControlSettings{
		ScreenMonitoring: policy.ScreenMonitoringSettings{
			Enabled:               true,
			SampleIntervalSeconds: 0,
		},
	}

	err := validateSettings(settings)
	if err == nil || !strings.Contains(err.Error(), "sample_interval_seconds") {
		t.Fatalf("Validate error = %v, want sample interval error", err)
	}
}

func TestSettingsValidateRejectsEmptyBlockedApp(t *testing.T) {
	settings := policy.KidControlSettings{
		AppControl: policy.AppControlSettings{
			BlockedApps: []string{"msedge.exe", " "},
		},
	}

	err := validateSettings(settings)
	if err == nil || !strings.Contains(err.Error(), "blocked_apps") {
		t.Fatalf("Validate error = %v, want blocked_apps error", err)
	}
}

func TestParseSettingsBrowserProfiles(t *testing.T) {
	settings, err := ParseSettings(map[string]interface{}{
		"browser_monitoring": map[string]interface{}{
			"enabled":         true,
			"default_profile": "default",
			"profiles": map[string]interface{}{
				"default": map[string]interface{}{
					"policy":              "block_list",
					"blocked_urls":        []interface{}{"youtube.com/shorts"},
					"block_inappropriate": true,
				},
				"study": map[string]interface{}{
					"policy":              "allow_list",
					"allowed_urls":        []interface{}{"wikipedia.org", "khanacademy.org"},
					"block_inappropriate": true,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ParseSettings returned error: %v", err)
	}
	if err := validateSettings(settings); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	browser := settings.BrowserMonitoring
	if !browser.Enabled {
		t.Fatal("BrowserMonitoring.Enabled = false, want true")
	}
	if browser.DefaultProfile != "default" {
		t.Fatalf("DefaultProfile = %q, want default", browser.DefaultProfile)
	}
	study := browser.Profiles["study"]
	if study.Policy != policy.BrowserPolicyAllowList {
		t.Fatalf("study.Policy = %q, want %q", study.Policy, policy.BrowserPolicyAllowList)
	}
	if len(study.AllowedURLs) != 2 || study.AllowedURLs[0] != "wikipedia.org" {
		t.Fatalf("study.AllowedURLs = %#v", study.AllowedURLs)
	}
	if !study.BlockInappropriate {
		t.Fatal("study.BlockInappropriate = false, want true")
	}
}

func TestBrowserMonitoringValidateRejectsMissingDefaultProfile(t *testing.T) {
	settings := policy.BrowserMonitoringSettings{
		Enabled:        true,
		DefaultProfile: "missing",
		Profiles: map[string]policy.BrowserProfile{
			"default": {Policy: policy.BrowserPolicyBlockList},
		},
	}

	err := validateBrowserMonitoringSettings(settings)
	if err == nil || !strings.Contains(err.Error(), "default_profile") {
		t.Fatalf("Validate error = %v, want default_profile error", err)
	}
}
