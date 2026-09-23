package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ConfirmPairingSession registers the claimed device and completes pairing.
func (s *Store) ConfirmPairingSession(
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PairingSession{}, fmt.Errorf("starting pairing confirm transaction: %w", err)
	}
	defer tx.Rollback()

	session, err := pairingSessionForManagementTransition(
		ctx,
		tx,
		customerID,
		sessionID,
	)
	if err != nil {
		return PairingSession{}, err
	}
	now := s.now().UTC()
	if err := validatePendingPairingSession(session, now); err != nil {
		return PairingSession{}, err
	}
	if err := session.DeviceID.Validate(); err != nil {
		return PairingSession{}, fmt.Errorf("validating claimed device: %w", err)
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO devices(
			customer_id,
			id,
			name,
			created_at,
			last_seen_at
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(customer_id, id) DO UPDATE SET
			name = excluded.name,
			last_seen_at = excluded.last_seen_at`,
		customerID,
		session.DeviceID,
		session.DeviceName,
		now,
		now,
	)
	if err != nil {
		return PairingSession{}, fmt.Errorf("registering paired device: %w", err)
	}

	expiresAt := now.Add(pairingCompletionTimeout)
	result, err := tx.ExecContext(
		ctx,
		`UPDATE pairing_sessions
		SET state = ?, expires_at = ?, updated_at = ?
		WHERE customer_id = ? AND id = ? AND state = ?`,
		PairingStateConfirmed,
		formatPairingTime(expiresAt),
		formatPairingTime(now),
		customerID,
		sessionID,
		PairingStatePendingConfirmation,
	)
	if err != nil {
		return PairingSession{}, fmt.Errorf("confirming pairing session: %w", err)
	}
	if err := requirePairingTransitionRow(result); err != nil {
		return PairingSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return PairingSession{}, fmt.Errorf("committing pairing confirmation: %w", err)
	}

	session.State = PairingStateConfirmed
	session.ExpiresAt = expiresAt
	session.UpdatedAt = now
	return session, nil
}

// CancelPairingSession terminates a pending pairing attempt for completion polling.
func (s *Store) CancelPairingSession(
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PairingSession{}, fmt.Errorf("starting pairing cancel transaction: %w", err)
	}
	defer tx.Rollback()

	session, err := pairingSessionForManagementTransition(
		ctx,
		tx,
		customerID,
		sessionID,
	)
	if err != nil {
		return PairingSession{}, err
	}
	now := s.now().UTC()
	if err := validatePendingPairingSession(session, now); err != nil {
		return PairingSession{}, err
	}

	expiresAt := now.Add(pairingCompletionTimeout)
	result, err := tx.ExecContext(
		ctx,
		`UPDATE pairing_sessions
		SET state = ?, expires_at = ?, updated_at = ?
		WHERE customer_id = ? AND id = ? AND state = ?`,
		PairingStateCancelled,
		formatPairingTime(expiresAt),
		formatPairingTime(now),
		customerID,
		sessionID,
		PairingStatePendingConfirmation,
	)
	if err != nil {
		return PairingSession{}, fmt.Errorf("cancelling pairing session: %w", err)
	}
	if err := requirePairingTransitionRow(result); err != nil {
		return PairingSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return PairingSession{}, fmt.Errorf("committing pairing cancellation: %w", err)
	}

	session.State = PairingStateCancelled
	session.ExpiresAt = expiresAt
	session.UpdatedAt = now
	return session, nil
}

func pairingSessionForManagementTransition(
	ctx context.Context,
	tx *sql.Tx,
	customerID CustomerID,
	sessionID string,
) (PairingSession, error) {
	return scanPairingSession(
		tx.QueryRowContext(
			ctx,
			"SELECT "+pairingSessionColumns+
				" FROM pairing_sessions WHERE customer_id = ? AND id = ?",
			customerID,
			sessionID,
		),
	)
}

func validatePendingPairingSession(session PairingSession, now time.Time) error {
	if session.State != PairingStatePendingConfirmation {
		return ErrPairingSessionState
	}
	if !session.ExpiresAt.After(now) {
		return ErrPairingSessionExpired
	}
	return nil
}

func requirePairingTransitionRow(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking pairing transition: %w", err)
	}
	if rows != 1 {
		return ErrPairingSessionState
	}
	return nil
}

func (s *managementPairingService) confirm(
	ctx context.Context,
	customerID CustomerID,
	sessionID string,
) (PairingSession, error) {
	return s.store.ConfirmPairingSession(ctx, customerID, sessionID)
}

func (s *managementPairingService) cancel(
	ctx context.Context,
	customerID CustomerID,
	sessionID string,
) (PairingSession, error) {
	return s.store.CancelPairingSession(ctx, customerID, sessionID)
}
