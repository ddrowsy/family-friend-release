package kidcontrol

import (
	"net/url"
	"strings"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

const (
	BrowserReasonAllowedByAllowList         = "allowed_by_allow_list"
	BrowserReasonAllowedByTemporaryApproval = "allowed_by_temporary_approval"
	BrowserReasonNotInAllowList             = "not_in_allow_list"
	BrowserReasonBlockedByBlockList         = "blocked_by_block_list"
	BrowserReasonAllowedByBlockList         = "allowed_by_block_list"
	BrowserReasonInvalidURL                 = "invalid_url"
)

// BrowserDecision is the result of evaluating a URL against one browser profile.
type BrowserDecision struct {
	Allowed bool
	Reason  string
}

// EvaluateBrowserURL evaluates a URL using the supplied browser profile.
func EvaluateBrowserURL(profile policy.BrowserProfile, rawURL string) BrowserDecision {
	return EvaluateBrowserURLAt(profile, rawURL, time.Now())
}

// EvaluateBrowserURLAt evaluates a URL at a deterministic time.
func EvaluateBrowserURLAt(
	profile policy.BrowserProfile,
	rawURL string,
	now time.Time,
) BrowserDecision {
	if _, ok := parseBrowserURL(rawURL); !ok {
		return BrowserDecision{Allowed: false, Reason: BrowserReasonInvalidURL}
	}

	for _, approval := range profile.TemporaryAllowedURLs {
		if !now.Before(approval.ExpiresAt) {
			continue
		}
		if matchesBrowserRule(rawURL, approval.URL) {
			return BrowserDecision{
				Allowed: true,
				Reason:  BrowserReasonAllowedByTemporaryApproval,
			}
		}
	}

	switch profile.Policy {
	case policy.BrowserPolicyAllowList:
		if matchesAnyBrowserRule(rawURL, profile.AllowedURLs) {
			return BrowserDecision{Allowed: true, Reason: BrowserReasonAllowedByAllowList}
		}
		return BrowserDecision{Allowed: false, Reason: BrowserReasonNotInAllowList}
	case policy.BrowserPolicyBlockList:
		if matchesAnyBrowserRule(rawURL, profile.BlockedURLs) {
			return BrowserDecision{Allowed: false, Reason: BrowserReasonBlockedByBlockList}
		}
		return BrowserDecision{Allowed: true, Reason: BrowserReasonAllowedByBlockList}
	default:
		return BrowserDecision{Allowed: false, Reason: BrowserReasonNotInAllowList}
	}
}

func matchesAnyBrowserRule(rawURL string, rules []string) bool {
	for _, rule := range rules {
		if matchesBrowserRule(rawURL, rule) {
			return true
		}
	}
	return false
}

func matchesBrowserRule(rawURL, rule string) bool {
	target, ok := parseBrowserURL(rawURL)
	if !ok {
		return false
	}
	parsedRule, ok := parseBrowserURL(rule)
	if !ok {
		return false
	}

	targetHost := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	ruleHost := strings.ToLower(strings.TrimSuffix(parsedRule.Hostname(), "."))
	if targetHost != ruleHost && !strings.HasSuffix(targetHost, "."+ruleHost) {
		return false
	}

	rulePath := strings.TrimSuffix(parsedRule.EscapedPath(), "/")
	if rulePath == "" {
		return true
	}

	targetPath := strings.TrimSuffix(target.EscapedPath(), "/")
	return targetPath == rulePath || strings.HasPrefix(targetPath, rulePath+"/")
}

func parseBrowserURL(value string) (*url.URL, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return nil, false
	}
	return parsed, true
}
