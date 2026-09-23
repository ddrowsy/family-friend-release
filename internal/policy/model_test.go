package policy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCurrentKidControlJSONRemainsCompatible(t *testing.T) {
	raw := []byte(`{
		"dry_run": true,
		"app_control": {
			"enabled": true,
			"scan_interval_seconds": 2,
			"blocked_apps": ["msedge.exe"]
		},
		"window_monitoring": {
			"enabled": true,
			"scan_interval_seconds": 2,
			"blocked_title_keywords": ["inprivate"]
		},
		"screen_monitoring": {
			"enabled": false,
			"sample_interval_seconds": 3
		},
		"browser_monitoring": {
			"enabled": true,
			"default_profile": "default",
			"profiles": {
				"default": {
					"policy": "block_list",
					"allowed_urls": [],
					"blocked_urls": ["example.com"],
					"block_inappropriate": true
				}
			}
		}
	}`)

	var settings KidControlSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("unmarshal current KidControl JSON: %v", err)
	}

	if settings.BrowserMonitoring.DefaultProfile != "default" {
		t.Fatalf(
			"default profile = %q, want default",
			settings.BrowserMonitoring.DefaultProfile,
		)
	}
	if settings.BrowserMonitoring.ActiveProfile != nil {
		t.Fatal("current JSON unexpectedly created active profile state")
	}

	defaultProfile := settings.BrowserMonitoring.Profiles["default"]
	if defaultProfile.Policy != BrowserPolicyBlockList {
		t.Fatalf("browser policy = %q, want %q", defaultProfile.Policy, BrowserPolicyBlockList)
	}
	if len(defaultProfile.TemporaryAllowedURLs) != 0 {
		t.Fatalf(
			"temporary allowed URLs = %d, want 0",
			len(defaultProfile.TemporaryAllowedURLs),
		)
	}

	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal current KidControl JSON: %v", err)
	}
	if strings.Contains(string(encoded), "active_profile") {
		t.Fatalf("encoded current JSON unexpectedly contains active_profile: %s", encoded)
	}
	if strings.Contains(string(encoded), "temporary_allowed_urls") {
		t.Fatalf(
			"encoded current JSON unexpectedly contains temporary_allowed_urls: %s",
			encoded,
		)
	}
}

func TestBrowserStateJSONRoundTrip(t *testing.T) {
	expiresAt := time.Date(2026, time.September, 22, 10, 30, 0, 0, time.UTC)
	settings := BrowserMonitoringSettings{
		Enabled:        true,
		DefaultProfile: "default",
		ActiveProfile: &ActiveBrowserProfile{
			Name:            "study",
			ExpiresAt:       &expiresAt,
			FallbackProfile: "default",
		},
		Profiles: map[string]BrowserProfile{
			"default": {
				Policy:               BrowserPolicyBlockList,
				AllowedURLs:          []string{},
				BlockedURLs:          []string{},
				BlockInappropriate:   true,
				TemporaryAllowedURLs: []TemporaryBrowserApproval{},
			},
			"study": {
				Policy:             BrowserPolicyAllowList,
				AllowedURLs:        []string{"wikipedia.org"},
				BlockedURLs:        []string{},
				BlockInappropriate: true,
				TemporaryAllowedURLs: []TemporaryBrowserApproval{
					{
						URL:       "youtube.com",
						ExpiresAt: expiresAt,
					},
				},
			},
		},
	}

	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal browser settings: %v", err)
	}

	var decoded BrowserMonitoringSettings
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal browser settings: %v", err)
	}

	if decoded.ActiveProfile == nil {
		t.Fatal("active profile is nil")
	}
	if decoded.ActiveProfile.Name != "study" {
		t.Fatalf("active profile = %q, want study", decoded.ActiveProfile.Name)
	}
	if decoded.ActiveProfile.ExpiresAt == nil ||
		!decoded.ActiveProfile.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("active profile expiry = %v, want %v", decoded.ActiveProfile.ExpiresAt, expiresAt)
	}

	temporary := decoded.Profiles["study"].TemporaryAllowedURLs
	if len(temporary) != 1 {
		t.Fatalf("temporary approvals = %d, want 1", len(temporary))
	}
	if temporary[0].URL != "youtube.com" {
		t.Fatalf("temporary approval URL = %q, want youtube.com", temporary[0].URL)
	}
	if !temporary[0].ExpiresAt.Equal(expiresAt) {
		t.Fatalf(
			"temporary approval expiry = %v, want %v",
			temporary[0].ExpiresAt,
			expiresAt,
		)
	}
}
