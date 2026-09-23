package kidcontrol

import (
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

func TestEvaluateBrowserURLAllowList(t *testing.T) {
	profile := policy.BrowserProfile{
		Policy:      policy.BrowserPolicyAllowList,
		AllowedURLs: []string{"wikipedia.org", "youtube.com/education"},
	}

	tests := []struct {
		url     string
		allowed bool
		reason  string
	}{
		{"https://wikipedia.org/wiki/Go", true, BrowserReasonAllowedByAllowList},
		{"https://en.wikipedia.org/wiki/Go", true, BrowserReasonAllowedByAllowList},
		{"https://youtube.com/education/math", true, BrowserReasonAllowedByAllowList},
		{"https://youtube.com/shorts/abc", false, BrowserReasonNotInAllowList},
		{"https://notwikipedia.org", false, BrowserReasonNotInAllowList},
	}

	for _, tt := range tests {
		got := EvaluateBrowserURL(profile, tt.url)
		if got.Allowed != tt.allowed || got.Reason != tt.reason {
			t.Fatalf("EvaluateBrowserURL(%q) = %#v, want allowed=%v reason=%q", tt.url, got, tt.allowed, tt.reason)
		}
	}
}

func TestEvaluateBrowserURLBlockList(t *testing.T) {
	profile := policy.BrowserProfile{
		Policy:      policy.BrowserPolicyBlockList,
		BlockedURLs: []string{"youtube.com/shorts", "reddit.com"},
	}

	tests := []struct {
		url     string
		allowed bool
	}{
		{"https://youtube.com/shorts/abc", false},
		{"https://www.reddit.com/r/golang", false},
		{"https://youtube.com/watch?v=1", true},
		{"https://reddit.com.example.org", true},
	}

	for _, tt := range tests {
		got := EvaluateBrowserURL(profile, tt.url)
		if got.Allowed != tt.allowed {
			t.Fatalf("EvaluateBrowserURL(%q).Allowed = %v, want %v", tt.url, got.Allowed, tt.allowed)
		}
	}
}

func TestBrowserURLRulePathBoundary(t *testing.T) {
	profile := policy.BrowserProfile{
		Policy:      policy.BrowserPolicyBlockList,
		BlockedURLs: []string{"example.com/games"},
	}

	if got := EvaluateBrowserURL(profile, "https://example.com/games2"); !got.Allowed {
		t.Fatal("example.com/games2 was blocked, want allowed")
	}
	if got := EvaluateBrowserURL(profile, "https://example.com/games/play"); got.Allowed {
		t.Fatal("example.com/games/play was allowed, want blocked")
	}
}

func TestBrowserURLRuleAcceptsScheme(t *testing.T) {
	profile := policy.BrowserProfile{
		Policy:      policy.BrowserPolicyAllowList,
		AllowedURLs: []string{"https://school.example.com/course"},
	}

	if got := EvaluateBrowserURL(profile, "https://school.example.com/course/math"); !got.Allowed {
		t.Fatalf("URL was blocked: %#v", got)
	}
}

func TestEvaluateBrowserURLAtAllowsActiveTemporaryApproval(t *testing.T) {
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	profile := policy.BrowserProfile{
		Policy:      policy.BrowserPolicyAllowList,
		AllowedURLs: []string{"wikipedia.org"},
		TemporaryAllowedURLs: []policy.TemporaryBrowserApproval{
			{
				URL:       "youtube.com/education",
				ExpiresAt: now.Add(time.Minute),
			},
		},
	}

	got := EvaluateBrowserURLAt(
		profile,
		"https://youtube.com/education/math",
		now,
	)
	if !got.Allowed || got.Reason != BrowserReasonAllowedByTemporaryApproval {
		t.Fatalf("temporary approval decision = %#v, want allowed", got)
	}
}

func TestEvaluateBrowserURLAtIgnoresExpiredTemporaryApproval(t *testing.T) {
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	profile := policy.BrowserProfile{
		Policy:      policy.BrowserPolicyAllowList,
		AllowedURLs: []string{"wikipedia.org"},
		TemporaryAllowedURLs: []policy.TemporaryBrowserApproval{
			{
				URL:       "youtube.com",
				ExpiresAt: now,
			},
		},
	}

	got := EvaluateBrowserURLAt(profile, "https://youtube.com/watch?v=1", now)
	if got.Allowed || got.Reason != BrowserReasonNotInAllowList {
		t.Fatalf("expired temporary approval decision = %#v, want blocked", got)
	}
}
