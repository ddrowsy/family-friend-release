package policy

import "time"

// CleanupExpiredTemporaryApprovals removes browser approvals that are no longer active.
func (s *BrowserMonitoringSettings) CleanupExpiredTemporaryApprovals(now time.Time) bool {
	changed := false

	for name, profile := range s.Profiles {
		if len(profile.TemporaryAllowedURLs) == 0 {
			continue
		}

		active := make([]TemporaryBrowserApproval, 0, len(profile.TemporaryAllowedURLs))
		for _, approval := range profile.TemporaryAllowedURLs {
			if !now.Before(approval.ExpiresAt) {
				changed = true
				continue
			}
			active = append(active, approval)
		}

		if len(active) == len(profile.TemporaryAllowedURLs) {
			continue
		}

		profile.TemporaryAllowedURLs = active
		s.Profiles[name] = profile
	}

	return changed
}
