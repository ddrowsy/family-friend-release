package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	BrowserAccessRequestPending  BrowserAccessRequestState = "pending"
	BrowserAccessRequestApproved BrowserAccessRequestState = "approved"
	BrowserAccessRequestRejected BrowserAccessRequestState = "rejected"
	BrowserAccessRequestExpired  BrowserAccessRequestState = "expired"

	browserAccessRequestIDBytes         = 24
	browserAccessRequestPendingLifetime = 10 * time.Minute
	browserAccessRequestResultLifetime  = 10 * time.Minute
	browserAccessRequestTimeLayout      = "2006-01-02T15:04:05.000000000Z"
)

var ErrBrowserAccessRequestState = errors.New("invalid browser access request state")

// BrowserAccessRequestState describes the lifecycle of a remote parent approval request.
type BrowserAccessRequestState string

// BrowserAccessRequest records one blocked-site request and its parent decision.
type BrowserAccessRequest struct {
	ID                   string
	CustomerID           CustomerID
	ProfileID            ProfileID
	SourceDeviceID       DeviceID
	URL                  string
	Site                 string
	BrowserPolicyProfile string
	State                BrowserAccessRequestState
	DecisionAction       BrowserApprovalAction
	DurationSeconds      int64
	CreatedAt            time.Time
	ExpiresAt            time.Time
	DecidedAt            *time.Time
}

// BrowserAccessDecision is selected only by the authenticated parent side.
type BrowserAccessDecision struct {
	Action          BrowserApprovalAction
	DurationSeconds int64
}

// CreateBrowserAccessRequest records a blocked-site request from an assigned device.
func (s *Store) CreateBrowserAccessRequest(
	ctx context.Context,
	customerID CustomerID,
	profileID ProfileID,
	deviceID DeviceID,
	rawURL string,
	browserProfile string,
) (BrowserAccessRequest, error) {
	if err := customerID.Validate(); err != nil {
		return BrowserAccessRequest{}, err
	}
	if err := profileID.Validate(); err != nil {
		return BrowserAccessRequest{}, err
	}
	if err := deviceID.Validate(); err != nil {
		return BrowserAccessRequest{}, err
	}
	browserProfile = strings.TrimSpace(browserProfile)
	if browserProfile == "" {
		return BrowserAccessRequest{}, ErrBrowserProfileNotFound
	}
	site, err := normalizeBrowserApprovalURL(rawURL)
	if err != nil {
		return BrowserAccessRequest{}, err
	}
	if err := s.ValidateDeviceProfileAccess(ctx, customerID, deviceID, profileID); err != nil {
		return BrowserAccessRequest{}, err
	}

	now := s.now().UTC()
	if existing, err := s.pendingBrowserAccessRequest(
		ctx,
		customerID,
		profileID,
		deviceID,
		site,
		browserProfile,
		now,
	); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return BrowserAccessRequest{}, err
	}

	id, err := newBrowserAccessRequestID()
	if err != nil {
		return BrowserAccessRequest{}, err
	}
	request := BrowserAccessRequest{
		ID:                   id,
		CustomerID:           customerID,
		ProfileID:            profileID,
		SourceDeviceID:       deviceID,
		URL:                  strings.TrimSpace(rawURL),
		Site:                 site,
		BrowserPolicyProfile: browserProfile,
		State:                BrowserAccessRequestPending,
		CreatedAt:            now,
		ExpiresAt:            now.Add(browserAccessRequestPendingLifetime),
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO browser_access_requests(
			id, customer_id, profile_id, source_device_id, url, site,
			browser_profile, state, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		request.ID,
		request.CustomerID,
		request.ProfileID,
		request.SourceDeviceID,
		request.URL,
		request.Site,
		request.BrowserPolicyProfile,
		request.State,
		formatBrowserAccessRequestTime(request.CreatedAt),
		formatBrowserAccessRequestTime(request.ExpiresAt),
	)
	if err != nil {
		return BrowserAccessRequest{}, fmt.Errorf("creating browser access request: %w", err)
	}
	return request, nil
}

// BrowserAccessRequestByID reads one request within its customer/profile/device boundary.
func (s *Store) BrowserAccessRequestByID(
	ctx context.Context,
	customerID CustomerID,
	profileID ProfileID,
	deviceID DeviceID,
	requestID string,
) (BrowserAccessRequest, error) {
	if err := customerID.Validate(); err != nil {
		return BrowserAccessRequest{}, err
	}
	if err := profileID.Validate(); err != nil {
		return BrowserAccessRequest{}, err
	}
	if err := deviceID.Validate(); err != nil {
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
		WHERE id = ? AND customer_id = ? AND profile_id = ? AND source_device_id = ?`,
		requestID,
		customerID,
		profileID,
		deviceID,
	))
	if err != nil {
		return BrowserAccessRequest{}, err
	}
	return s.expireBrowserAccessRequest(ctx, request)
}

// ApproveBrowserAccessRequest applies the parent-selected approval and records the decision.
func (s *Store) ApproveBrowserAccessRequest(
	ctx context.Context,
	customerID CustomerID,
	requestID string,
	decision BrowserAccessDecision,
) (BrowserAccessRequest, error) {
	request, err := s.browserAccessRequestForParent(ctx, customerID, requestID)
	if err != nil {
		return BrowserAccessRequest{}, err
	}
	if request.State != BrowserAccessRequestPending {
		return BrowserAccessRequest{}, ErrBrowserAccessRequestState
	}
	now := s.now().UTC()
	if !request.ExpiresAt.After(now) {
		return s.markBrowserAccessRequestExpired(ctx, request, now)
	}
	if err := validateBrowserAccessDecision(decision); err != nil {
		return BrowserAccessRequest{}, err
	}

	scope := browserApprovalScope{
		customerID: request.CustomerID,
		profileID:  request.ProfileID,
	}
	target := browserApprovalTarget{
		browserProfile: request.BrowserPolicyProfile,
		rule:           request.Site,
	}
	switch decision.Action {
	case BrowserApprovalTemporary:
		approval := BrowserApprovalRequest{DurationSeconds: decision.DurationSeconds}
		if _, _, err := s.approveTemporaryBrowserURL(ctx, scope, approval, target); err != nil {
			return BrowserAccessRequest{}, err
		}
	case BrowserApprovalPermanent:
		if _, err := s.approvePermanentBrowserURL(ctx, scope, target); err != nil {
			return BrowserAccessRequest{}, err
		}
	}

	return s.recordBrowserAccessDecision(
		ctx,
		request,
		BrowserAccessRequestApproved,
		decision,
		now,
	)
}

// RejectBrowserAccessRequest records a parent rejection without changing browser policy.
func (s *Store) RejectBrowserAccessRequest(
	ctx context.Context,
	customerID CustomerID,
	requestID string,
) (BrowserAccessRequest, error) {
	request, err := s.browserAccessRequestForParent(ctx, customerID, requestID)
	if err != nil {
		return BrowserAccessRequest{}, err
	}
	if request.State != BrowserAccessRequestPending {
		return BrowserAccessRequest{}, ErrBrowserAccessRequestState
	}
	now := s.now().UTC()
	if !request.ExpiresAt.After(now) {
		return s.markBrowserAccessRequestExpired(ctx, request, now)
	}
	return s.recordBrowserAccessDecision(
		ctx,
		request,
		BrowserAccessRequestRejected,
		BrowserAccessDecision{},
		now,
	)
}

func (s *Store) pendingBrowserAccessRequest(
	ctx context.Context,
	customerID CustomerID,
	profileID ProfileID,
	deviceID DeviceID,
	site string,
	browserProfile string,
	now time.Time,
) (BrowserAccessRequest, error) {
	request, err := scanBrowserAccessRequest(s.db.QueryRowContext(
		ctx,
		`SELECT id, customer_id, profile_id, source_device_id, url, site,
			browser_profile, state, decision_action, duration_seconds,
			created_at, expires_at, decided_at
		FROM browser_access_requests
		WHERE customer_id = ? AND profile_id = ? AND source_device_id = ?
			AND site = ? AND browser_profile = ? AND state = ?
		ORDER BY created_at DESC LIMIT 1`,
		customerID,
		profileID,
		deviceID,
		site,
		browserProfile,
		BrowserAccessRequestPending,
	))
	if err != nil {
		return BrowserAccessRequest{}, err
	}
	if request.ExpiresAt.After(now) {
		return request, nil
	}
	if _, err := s.markBrowserAccessRequestExpired(ctx, request, now); err != nil {
		return BrowserAccessRequest{}, err
	}
	return BrowserAccessRequest{}, ErrNotFound
}

func (s *Store) browserAccessRequestForParent(
	ctx context.Context,
	customerID CustomerID,
	requestID string,
) (BrowserAccessRequest, error) {
	if err := customerID.Validate(); err != nil {
		return BrowserAccessRequest{}, err
	}
	if strings.TrimSpace(requestID) == "" {
		return BrowserAccessRequest{}, ErrNotFound
	}
	return scanBrowserAccessRequest(s.db.QueryRowContext(
		ctx,
		`SELECT id, customer_id, profile_id, source_device_id, url, site,
			browser_profile, state, decision_action, duration_seconds,
			created_at, expires_at, decided_at
		FROM browser_access_requests WHERE id = ? AND customer_id = ?`,
		requestID,
		customerID,
	))
}

func (s *Store) expireBrowserAccessRequest(
	ctx context.Context,
	request BrowserAccessRequest,
) (BrowserAccessRequest, error) {
	if request.State != BrowserAccessRequestPending || request.ExpiresAt.After(s.now().UTC()) {
		return request, nil
	}
	return s.markBrowserAccessRequestExpired(ctx, request, s.now().UTC())
}

func (s *Store) markBrowserAccessRequestExpired(
	ctx context.Context,
	request BrowserAccessRequest,
	now time.Time,
) (BrowserAccessRequest, error) {
	expiresAt := now.Add(browserAccessRequestResultLifetime)
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE browser_access_requests SET state = ?, expires_at = ?
		WHERE id = ? AND customer_id = ? AND state = ?`,
		BrowserAccessRequestExpired,
		formatBrowserAccessRequestTime(expiresAt),
		request.ID,
		request.CustomerID,
		BrowserAccessRequestPending,
	)
	if err != nil {
		return BrowserAccessRequest{}, fmt.Errorf("expiring browser access request: %w", err)
	}
	if err := requireBrowserAccessRequestTransition(result); err != nil {
		return BrowserAccessRequest{}, err
	}
	request.State = BrowserAccessRequestExpired
	request.ExpiresAt = expiresAt
	return request, nil
}

func (s *Store) recordBrowserAccessDecision(
	ctx context.Context,
	request BrowserAccessRequest,
	state BrowserAccessRequestState,
	decision BrowserAccessDecision,
	now time.Time,
) (BrowserAccessRequest, error) {
	expiresAt := now.Add(browserAccessRequestResultLifetime)
	var action any
	var duration any
	if state == BrowserAccessRequestApproved {
		action = decision.Action
		if decision.Action == BrowserApprovalTemporary {
			duration = decision.DurationSeconds
		}
	}
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE browser_access_requests
		SET state = ?, decision_action = ?, duration_seconds = ?, decided_at = ?, expires_at = ?
		WHERE id = ? AND customer_id = ? AND state = ?`,
		state,
		action,
		duration,
		formatBrowserAccessRequestTime(now),
		formatBrowserAccessRequestTime(expiresAt),
		request.ID,
		request.CustomerID,
		BrowserAccessRequestPending,
	)
	if err != nil {
		return BrowserAccessRequest{}, fmt.Errorf("recording browser access decision: %w", err)
	}
	if err := requireBrowserAccessRequestTransition(result); err != nil {
		return BrowserAccessRequest{}, err
	}
	request.State = state
	request.DecisionAction = decision.Action
	request.DurationSeconds = decision.DurationSeconds
	request.DecidedAt = &now
	request.ExpiresAt = expiresAt
	return request, nil
}

func validateBrowserAccessDecision(decision BrowserAccessDecision) error {
	switch decision.Action {
	case BrowserApprovalTemporary:
		if decision.DurationSeconds <= 0 {
			return ErrInvalidApprovalDuration
		}
	case BrowserApprovalPermanent:
		if decision.DurationSeconds != 0 {
			return ErrInvalidApprovalDuration
		}
	default:
		return ErrInvalidApprovalAction
	}
	return nil
}

func scanBrowserAccessRequest(row *sql.Row) (BrowserAccessRequest, error) {
	var request BrowserAccessRequest
	var action sql.NullString
	var duration sql.NullInt64
	var createdAt string
	var expiresAt string
	var decidedAt sql.NullString
	if err := row.Scan(
		&request.ID,
		&request.CustomerID,
		&request.ProfileID,
		&request.SourceDeviceID,
		&request.URL,
		&request.Site,
		&request.BrowserPolicyProfile,
		&request.State,
		&action,
		&duration,
		&createdAt,
		&expiresAt,
		&decidedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BrowserAccessRequest{}, ErrNotFound
		}
		return BrowserAccessRequest{}, fmt.Errorf("reading browser access request: %w", err)
	}

	var err error
	request.CreatedAt, err = parseBrowserAccessRequestTime(createdAt)
	if err != nil {
		return BrowserAccessRequest{}, err
	}
	request.ExpiresAt, err = parseBrowserAccessRequestTime(expiresAt)
	if err != nil {
		return BrowserAccessRequest{}, err
	}
	if action.Valid {
		request.DecisionAction = BrowserApprovalAction(action.String)
	}
	if duration.Valid {
		request.DurationSeconds = duration.Int64
	}
	if decidedAt.Valid {
		value, err := parseBrowserAccessRequestTime(decidedAt.String)
		if err != nil {
			return BrowserAccessRequest{}, err
		}
		request.DecidedAt = &value
	}
	return request, nil
}

func requireBrowserAccessRequestTransition(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking browser access request transition: %w", err)
	}
	if rows != 1 {
		return ErrBrowserAccessRequestState
	}
	return nil
}

func newBrowserAccessRequestID() (string, error) {
	random := make([]byte, browserAccessRequestIDBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generating browser access request ID: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}

func formatBrowserAccessRequestTime(value time.Time) string {
	return value.UTC().Format(browserAccessRequestTimeLayout)
}

func parseBrowserAccessRequestTime(value string) (time.Time, error) {
	parsed, err := time.Parse(browserAccessRequestTimeLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing browser access request time: %w", err)
	}
	return parsed.UTC(), nil
}
