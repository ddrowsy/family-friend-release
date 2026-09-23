package kidcontrol

import (
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

func TestResolveBrowserProfileUsesCurrentProfileValue(t *testing.T) {
	module := testBrowserModule()

	start := time.Unix(100, 0)
	if err := module.activateBrowserProfile(
		"study",
		15*time.Minute,
		"default",
		start,
	); err != nil {
		t.Fatalf("activateBrowserProfile returned error: %v", err)
	}
	active := module.config.Settings.BrowserMonitoring.ActiveProfile
	if active == nil || active.Name != "study" {
		t.Fatalf("active profile = %#v, want study", active)
	}
	wantExpiry := start.Add(15 * time.Minute)
	if active.ExpiresAt == nil || !active.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("active profile expiry = %v, want %v", active.ExpiresAt, wantExpiry)
	}

	updated := policy.BrowserProfile{
		Policy:      policy.BrowserPolicyAllowList,
		AllowedURLs: []string{"khanacademy.org"},
	}
	if err := module.SetBrowserProfile("study", updated); err != nil {
		t.Fatalf("SetBrowserProfile returned error: %v", err)
	}

	name, profile, err := module.ResolveBrowserProfile(time.Unix(200, 0))
	if err != nil {
		t.Fatalf("ResolveBrowserProfile returned error: %v", err)
	}
	if name != "study" {
		t.Fatalf("profile name = %q, want study", name)
	}
	if len(profile.AllowedURLs) != 1 || profile.AllowedURLs[0] != "khanacademy.org" {
		t.Fatalf("profile = %#v, want updated study profile", profile)
	}
}

func TestResolveBrowserProfileFallsBackAfterExpiry(t *testing.T) {
	module := testBrowserModule()
	start := time.Unix(100, 0)

	if err := module.activateBrowserProfile("study", 15*time.Minute, "default", start); err != nil {
		t.Fatalf("activateBrowserProfile returned error: %v", err)
	}

	name, _, err := module.ResolveBrowserProfile(start.Add(16 * time.Minute))
	if err != nil {
		t.Fatalf("ResolveBrowserProfile returned error: %v", err)
	}
	if name != "default" {
		t.Fatalf("profile name = %q, want default", name)
	}
	if module.config.Settings.BrowserMonitoring.ActiveProfile != nil {
		t.Fatal("expired active profile was not cleared")
	}
}

func TestResolveBrowserProfileUsesPersistedSharedState(t *testing.T) {
	module := testBrowserModule()
	start := time.Unix(100, 0)
	expiry := start.Add(15 * time.Minute)
	module.config.Settings.BrowserMonitoring.ActiveProfile = &policy.ActiveBrowserProfile{
		Name:            "study",
		ExpiresAt:       &expiry,
		FallbackProfile: "default",
	}

	name, _, err := module.ResolveBrowserProfile(start.Add(time.Minute))
	if err != nil {
		t.Fatalf("ResolveBrowserProfile returned error: %v", err)
	}
	if name != "study" {
		t.Fatalf("profile name = %q, want study", name)
	}
}

func TestResolveBrowserProfileFallsBackWhenActiveProfileDeleted(t *testing.T) {
	module := testBrowserModule()
	start := time.Unix(100, 0)

	if err := module.activateBrowserProfile("study", 15*time.Minute, "default", start); err != nil {
		t.Fatalf("activateBrowserProfile returned error: %v", err)
	}
	if err := module.DeleteBrowserProfile("study"); err != nil {
		t.Fatalf("DeleteBrowserProfile returned error: %v", err)
	}

	name, _, err := module.ResolveBrowserProfile(start.Add(time.Minute))
	if err != nil {
		t.Fatalf("ResolveBrowserProfile returned error: %v", err)
	}
	if name != "default" {
		t.Fatalf("profile name = %q, want default", name)
	}
}

func TestDeleteBrowserProfileRejectsDefault(t *testing.T) {
	module := testBrowserModule()

	if err := module.DeleteBrowserProfile("default"); err == nil {
		t.Fatal("DeleteBrowserProfile returned nil, want error")
	}
}

func TestResolveBrowserProfileUsesDefaultWhenFallbackDeleted(t *testing.T) {
	module := testBrowserModule()
	start := time.Unix(100, 0)

	if err := module.SetBrowserProfile("free", policy.BrowserProfile{Policy: policy.BrowserPolicyBlockList}); err != nil {
		t.Fatalf("SetBrowserProfile returned error: %v", err)
	}
	if err := module.activateBrowserProfile("study", 15*time.Minute, "free", start); err != nil {
		t.Fatalf("activateBrowserProfile returned error: %v", err)
	}
	if err := module.DeleteBrowserProfile("free"); err != nil {
		t.Fatalf("DeleteBrowserProfile returned error: %v", err)
	}

	name, _, err := module.ResolveBrowserProfile(start.Add(16 * time.Minute))
	if err != nil {
		t.Fatalf("ResolveBrowserProfile returned error: %v", err)
	}
	if name != "default" {
		t.Fatalf("profile name = %q, want default", name)
	}
}

func TestActivateBrowserProfileWithNoDurationDoesNotExpire(t *testing.T) {
	module := testBrowserModule()
	start := time.Unix(100, 0)

	if err := module.activateBrowserProfile("study", 0, "default", start); err != nil {
		t.Fatalf("activateBrowserProfile returned error: %v", err)
	}

	name, _, err := module.ResolveBrowserProfile(start.Add(24 * time.Hour))
	if err != nil {
		t.Fatalf("ResolveBrowserProfile returned error: %v", err)
	}
	if name != "study" {
		t.Fatalf("profile name = %q, want study", name)
	}
	active := module.config.Settings.BrowserMonitoring.ActiveProfile
	if active == nil {
		t.Fatal("active profile is nil")
	}
	if active.ExpiresAt != nil {
		t.Fatalf("active profile expiry = %v, want nil", active.ExpiresAt)
	}
}

func testBrowserModule() *Module {
	module := &Module{}
	module.config.Settings.BrowserMonitoring = policy.BrowserMonitoringSettings{
		Enabled:        true,
		DefaultProfile: "default",
		Profiles: map[string]policy.BrowserProfile{
			"default": {Policy: policy.BrowserPolicyBlockList},
			"study": {
				Policy:      policy.BrowserPolicyAllowList,
				AllowedURLs: []string{"wikipedia.org"},
			},
		},
	}
	return module
}
