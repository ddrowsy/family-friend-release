package service

import (
	"context"
	"strings"
)

func (s *Store) browserAccessRequestForDevice(
	ctx context.Context,
	caller DeviceCaller,
	requestID string,
) (BrowserAccessRequest, error) {
	if err := caller.Validate(); err != nil {
		return BrowserAccessRequest{}, err
	}
	if strings.TrimSpace(requestID) == "" {
		return BrowserAccessRequest{}, ErrNotFound
	}

	request, err := scanBrowserAccessRequest(s.db.QueryRowContext(
		ctx,
		`SELECT id, customer_id, profile_id, source_device_id, url, site,
			browser_profile, state, decision_action, duration_seconds,
			created_at, expires_at, decided_at
		FROM browser_access_requests
		WHERE id = ? AND customer_id = ? AND source_device_id = ?`,
		requestID,
		caller.CustomerID,
		caller.DeviceID,
	))
	if err != nil {
		return BrowserAccessRequest{}, err
	}
	return s.expireBrowserAccessRequest(ctx, request)
}
