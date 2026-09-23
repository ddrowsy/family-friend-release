package mockdata

import (
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/modules/kidcontrol"
	"github.com/ddrowsy/family-friend-release/internal/policy"
)

func TestPolicyUsesProductionModelsAndExpectedFixtures(t *testing.T) {
	policyConfig, err := Policy()
	if err != nil {
		t.Fatalf("Policy returned error: %v", err)
	}

	modulePolicy, ok := policyConfig.Modules[policy.KidControlModuleName]
	if !ok {
		t.Fatal("mock policy is missing kidcontrol module")
	}
	settings, err := kidcontrol.ParseSettings(modulePolicy.Settings)
	if err != nil {
		t.Fatalf("ParseSettings returned error: %v", err)
	}
	if !settings.BrowserMonitoring.Enabled {
		t.Fatal("browser monitoring is disabled")
	}

	defaultProfile := settings.BrowserMonitoring.Profiles["default"]
	assertStringsContain(
		t,
		defaultProfile.BlockedURLs,
		"facebook.com",
		"tiktok.com",
	)
	if len(defaultProfile.TemporaryAllowedURLs) != 1 {
		t.Fatalf("temporary approvals = %d, want 1", len(defaultProfile.TemporaryAllowedURLs))
	}
	approval := defaultProfile.TemporaryAllowedURLs[0]
	if approval.URL != "facebook.com/school" {
		t.Fatalf("temporary approval URL = %q", approval.URL)
	}
	wantExpiry := time.Date(
		2099,
		time.January,
		1,
		0,
		0,
		0,
		0,
		time.UTC,
	)
	if !approval.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("temporary approval expiry = %s, want %s", approval.ExpiresAt, wantExpiry)
	}

	studyProfile := settings.BrowserMonitoring.Profiles["study"]
	if studyProfile.Policy != policy.BrowserPolicyAllowList {
		t.Fatalf("study policy = %q, want %q", studyProfile.Policy, policy.BrowserPolicyAllowList)
	}
	assertStringsContain(
		t,
		studyProfile.AllowedURLs,
		"google.com",
		"wikipedia.org",
	)
}

func assertStringsContain(t *testing.T, values []string, expected ...string) {
	t.Helper()

	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := seen[value]; !ok {
			t.Fatalf("values = %q, missing %q", values, value)
		}
	}
}
