package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PendingBrowserAccessRequests lists pending blocked-site requests for a customer.
func (s *Store) PendingBrowserAccessRequests(
	ctx context.Context,
	customerID CustomerID,
) ([]BrowserAccessRequest, error) {
	if err := customerID.Validate(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id FROM browser_access_requests
		WHERE customer_id = ? AND state = ?
		ORDER BY created_at ASC`,
		customerID,
		BrowserAccessRequestPending,
	)
	if err != nil {
		return nil, fmt.Errorf("listing browser access requests: %w", err)
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reading browser access request ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing browser access requests: %w", err)
	}

	requests := make([]BrowserAccessRequest, 0, len(ids))
	for _, id := range ids {
		request, err := s.browserAccessRequestForParent(ctx, customerID, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		request, err = s.expireBrowserAccessRequest(ctx, request)
		if err != nil {
			return nil, err
		}
		if request.State == BrowserAccessRequestPending {
			requests = append(requests, request)
		}
	}
	return requests, nil
}
