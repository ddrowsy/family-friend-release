package policy

import (
	"testing"
	"time"
)

func TestCleanupExpiredTemporaryApprovals(t *testing.T) {
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	settings := BrowserMonitoringSettings{
		Profiles: map[string]BrowserProfile{
			"study": {
				TemporaryAllowedURLs: []TemporaryBrowserApproval{
					{URL: "expired.example", ExpiresAt: now},
					{URL: "active.example", ExpiresAt: now.Add(time.Minute)},
				},
			},
			"default": {
				TemporaryAllowedURLs: []TemporaryBrowserApproval{},
			},
		},
	}

	if !settings.CleanupExpiredTemporaryApprovals(now) {
		t.Fatal("CleanupExpiredTemporaryApprovals = false, want true")
	}

	approvals := settings.Profiles["study"].TemporaryAllowedURLs
	if len(approvals) != 1 || approvals[0].URL != "active.example" {
		t.Fatalf("temporary approvals = %#v, want active approval only", approvals)
	}
	if settings.CleanupExpiredTemporaryApprovals(now) {
		t.Fatal("second cleanup changed already-clean settings")
	}
}
