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
	PairingStateWaiting             PairingSessionState = "waiting"
	PairingStatePendingConfirmation PairingSessionState = "pending_confirmation"
	PairingStateConfirmed           PairingSessionState = "confirmed"
	PairingStateCancelled           PairingSessionState = "cancelled"

	pairingSessionIDBytes             = 24
	pairingPendingConfirmationTimeout = 10 * time.Minute
	pairingCompletionTimeout          = 10 * time.Minute
	pairingTimeLayout                 = "2006-01-02T15:04:05.000000000Z"

	pairingSessionColumns = "id, customer_id, state, code, device_id, device_name, " +
		"platform, expires_at, created_at, updated_at"
)

var (
	ErrPairingSessionExpired = errors.New("pairing session expired")
	ErrPairingSessionState   = errors.New("invalid pairing session state")
)

// PairingSessionState describes the persisted pairing lifecycle state.
type PairingSessionState string

// PairingSession is one active parent-to-device pairing attempt.
type PairingSession struct {
	ID         string
	CustomerID CustomerID
	State      PairingSessionState
	Code       string
	DeviceID   DeviceID
	DeviceName string
	Platform   string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// PairingCodeRotation describes a new code and expiry for a waiting session.
type PairingCodeRotation struct {
	Code      string
	ExpiresAt time.Time
}

// PairingDeviceClaim identifies the device claiming a waiting pairing code.
type PairingDeviceClaim struct {
	DeviceID   DeviceID
	DeviceName string
	Platform   string
}

// CreatePairingSession replaces the customer's active pairing attempt.
func (s *Store) CreatePairingSession(
	ctx context.Context,
	customerID CustomerID,
	code string,
	expiresAt time.Time,
) (PairingSession, error) {
	if err := customerID.Validate(); err != nil {
		return PairingSession{}, err
	}
	if strings.TrimSpace(code) == "" {
		return PairingSession{}, errors.New("pairing code is required")
	}

	now := s.now().UTC()
	if !expiresAt.After(now) {
		return PairingSession{}, errors.New("pairing expiry must be in the future")
	}

	sessionID, err := newPairingSessionID()
	if err != nil {
		return PairingSession{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PairingSession{}, fmt.Errorf("starting pairing session transaction: %w", err)
	}
	defer tx.Rollback()

	if err := requirePairingCustomer(ctx, tx, customerID); err != nil {
		return PairingSession{}, err
	}
	if _, err := tx.ExecContext(
		ctx,
		"DELETE FROM pairing_sessions WHERE customer_id = ?",
		customerID,
	); err != nil {
		return PairingSession{}, fmt.Errorf("replacing pairing session: %w", err)
	}

	createdAt := formatPairingTime(now)
	_, err = tx.ExecContext(
		ctx,
		"INSERT INTO pairing_sessions("+
			"id, customer_id, state, code, expires_at, created_at, updated_at"+
			") VALUES (?, ?, ?, ?, ?, ?, ?)",
		sessionID,
		customerID,
		PairingStateWaiting,
		code,
		formatPairingTime(expiresAt),
		createdAt,
		createdAt,
	)
	if err != nil {
		return PairingSession{}, fmt.Errorf("creating pairing session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PairingSession{}, fmt.Errorf("committing pairing session: %w", err)
	}

	return PairingSession{
		ID:         sessionID,
		CustomerID: customerID,
		State:      PairingStateWaiting,
		Code:       code,
		ExpiresAt:  expiresAt.UTC(),
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// PairingSession returns one unexpired pairing session for its customer.
func (s *Store) PairingSession(
	ctx context.Context,
	customerID CustomerID,
	sessionID string,
) (PairingSession, error) {
	if err := customerID.Validate(); err != nil {
		return PairingSession{}, err
	}
	if strings.TrimSpace(sessionID) == "" {
		return PairingSession{}, errors.New("pairing session ID is required")
	}

	session, err := scanPairingSession(
		s.db.QueryRowContext(
			ctx,
			"SELECT "+pairingSessionColumns+
				" FROM pairing_sessions WHERE customer_id = ? AND id = ?",
			customerID,
			sessionID,
		),
	)
	if err != nil {
		return PairingSession{}, err
	}
	if !session.ExpiresAt.After(s.now().UTC()) {
		return PairingSession{}, ErrPairingSessionExpired
	}
	return session, nil
}

// RotatePairingSessionCode replaces the code and expiry on a waiting session.
func (s *Store) RotatePairingSessionCode(
	ctx context.Context,
	customerID CustomerID,
	sessionID string,
	rotation PairingCodeRotation,
) (PairingSession, error) {
	if err := customerID.Validate(); err != nil {
		return PairingSession{}, err
	}
	if strings.TrimSpace(sessionID) == "" {
		return PairingSession{}, errors.New("pairing session ID is required")
	}
	if strings.TrimSpace(rotation.Code) == "" {
		return PairingSession{}, errors.New("pairing code is required")
	}

	now := s.now().UTC()
	if !rotation.ExpiresAt.After(now) {
		return PairingSession{}, errors.New("pairing expiry must be in the future")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PairingSession{}, fmt.Errorf("starting pairing rotation transaction: %w", err)
	}
	defer tx.Rollback()

	session, err := scanPairingSession(
		tx.QueryRowContext(
			ctx,
			"SELECT "+pairingSessionColumns+
				" FROM pairing_sessions WHERE customer_id = ? AND id = ?",
			customerID,
			sessionID,
		),
	)
	if err != nil {
		return PairingSession{}, err
	}
	if session.State != PairingStateWaiting {
		return PairingSession{}, ErrPairingSessionState
	}
	if !session.ExpiresAt.After(now) {
		return PairingSession{}, ErrPairingSessionExpired
	}

	result, err := tx.ExecContext(
		ctx,
		"UPDATE pairing_sessions SET code = ?, expires_at = ?, updated_at = ? "+
			"WHERE customer_id = ? AND id = ? AND state = ?",
		rotation.Code,
		formatPairingTime(rotation.ExpiresAt),
		formatPairingTime(now),
		customerID,
		sessionID,
		PairingStateWaiting,
	)
	if err != nil {
		return PairingSession{}, fmt.Errorf("rotating pairing code: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return PairingSession{}, fmt.Errorf("checking pairing rotation: %w", err)
	}
	if rows != 1 {
		return PairingSession{}, ErrPairingSessionState
	}
	if err := tx.Commit(); err != nil {
		return PairingSession{}, fmt.Errorf("committing pairing rotation: %w", err)
	}

	session.Code = rotation.Code
	session.ExpiresAt = rotation.ExpiresAt.UTC()
	session.UpdatedAt = now
	return session, nil
}

// PairingSessionByCode resolves one unexpired waiting pairing code.
func (s *Store) PairingSessionByCode(
	ctx context.Context,
	code string,
) (PairingSession, error) {
	if strings.TrimSpace(code) == "" {
		return PairingSession{}, errors.New("pairing code is required")
	}

	session, err := scanPairingSession(
		s.db.QueryRowContext(
			ctx,
			"SELECT "+pairingSessionColumns+
				" FROM pairing_sessions WHERE code = ?",
			code,
		),
	)
	if err != nil {
		return PairingSession{}, err
	}
	if session.State != PairingStateWaiting {
		return PairingSession{}, ErrPairingSessionState
	}
	if !session.ExpiresAt.After(s.now().UTC()) {
		return PairingSession{}, ErrPairingSessionExpired
	}
	return session, nil
}

// ClaimPairingSession moves a valid waiting code to pending confirmation.
func (s *Store) ClaimPairingSession(
	ctx context.Context,
	code string,
	claim PairingDeviceClaim,
	claimProofHash []byte,
) (PairingSession, error) {
	if strings.TrimSpace(code) == "" {
		return PairingSession{}, errors.New("pairing code is required")
	}
	if err := claim.DeviceID.Validate(); err != nil {
		return PairingSession{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PairingSession{}, fmt.Errorf("starting pairing claim transaction: %w", err)
	}
	defer tx.Rollback()

	session, err := scanPairingSession(
		tx.QueryRowContext(
			ctx,
			"SELECT "+pairingSessionColumns+
				" FROM pairing_sessions WHERE code = ?",
			code,
		),
	)
	if err != nil {
		return PairingSession{}, err
	}

	now := s.now().UTC()
	if session.State != PairingStateWaiting {
		return PairingSession{}, ErrPairingSessionState
	}
	if !session.ExpiresAt.After(now) {
		return PairingSession{}, ErrPairingSessionExpired
	}

	expiresAt := now.Add(pairingPendingConfirmationTimeout)
	result, err := tx.ExecContext(
		ctx,
		"UPDATE pairing_sessions SET "+
			"state = ?, code = NULL, device_id = ?, device_name = ?, "+
			"platform = ?, claim_proof_hash = ?, expires_at = ?, updated_at = ? "+
			"WHERE id = ? AND state = ? AND code = ?",
		PairingStatePendingConfirmation,
		claim.DeviceID,
		claim.DeviceName,
		claim.Platform,
		claimProofHash,
		formatPairingTime(expiresAt),
		formatPairingTime(now),
		session.ID,
		PairingStateWaiting,
		code,
	)
	if err != nil {
		return PairingSession{}, fmt.Errorf("claiming pairing session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return PairingSession{}, fmt.Errorf("checking pairing claim: %w", err)
	}
	if rows != 1 {
		return PairingSession{}, ErrPairingSessionState
	}
	if err := tx.Commit(); err != nil {
		return PairingSession{}, fmt.Errorf("committing pairing claim: %w", err)
	}

	session.State = PairingStatePendingConfirmation
	session.Code = ""
	session.DeviceID = claim.DeviceID
	session.DeviceName = claim.DeviceName
	session.Platform = claim.Platform
	session.ExpiresAt = expiresAt
	session.UpdatedAt = now
	return session, nil
}

// DeletePairingSession removes one active pairing attempt.
func (s *Store) DeletePairingSession(
	ctx context.Context,
	customerID CustomerID,
	sessionID string,
) error {
	if err := customerID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("pairing session ID is required")
	}

	result, err := s.db.ExecContext(
		ctx,
		"DELETE FROM pairing_sessions WHERE customer_id = ? AND id = ?",
		customerID,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("deleting pairing session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking pairing session delete: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

type pairingSessionScanner interface {
	Scan(dest ...any) error
}

func scanPairingSession(scanner pairingSessionScanner) (PairingSession, error) {
	var session PairingSession
	var code sql.NullString
	var deviceID sql.NullString
	var deviceName sql.NullString
	var platform sql.NullString
	var expiresAt string
	var createdAt string
	var updatedAt string

	err := scanner.Scan(
		&session.ID,
		&session.CustomerID,
		&session.State,
		&code,
		&deviceID,
		&deviceName,
		&platform,
		&expiresAt,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PairingSession{}, ErrNotFound
	}
	if err != nil {
		return PairingSession{}, fmt.Errorf("reading pairing session: %w", err)
	}

	session.Code = code.String
	session.DeviceID = DeviceID(deviceID.String)
	session.DeviceName = deviceName.String
	session.Platform = platform.String

	session.ExpiresAt, err = parsePairingTime(expiresAt)
	if err != nil {
		return PairingSession{}, err
	}
	session.CreatedAt, err = parsePairingTime(createdAt)
	if err != nil {
		return PairingSession{}, err
	}
	session.UpdatedAt, err = parsePairingTime(updatedAt)
	if err != nil {
		return PairingSession{}, err
	}
	return session, nil
}

func requirePairingCustomer(
	ctx context.Context,
	tx *sql.Tx,
	customerID CustomerID,
) error {
	var exists int
	err := tx.QueryRowContext(
		ctx,
		"SELECT 1 FROM customers WHERE id = ?",
		customerID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("finding pairing customer: %w", err)
	}
	return nil
}

func newPairingSessionID() (string, error) {
	random := make([]byte, pairingSessionIDBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generating pairing session ID: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}

func formatPairingTime(value time.Time) string {
	return value.UTC().Format(pairingTimeLayout)
}

func parsePairingTime(value string) (time.Time, error) {
	parsed, err := time.Parse(pairingTimeLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing pairing time: %w", err)
	}
	return parsed, nil
}
