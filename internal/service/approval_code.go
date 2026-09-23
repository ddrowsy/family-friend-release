package service

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	approvalCodeVerifierVersion   = 1
	defaultApprovalCodeIterations = 600_000
	approvalCodeSaltBytes         = 16
	approvalCodeHashBytes         = 32
	maxApprovalCodeFailures       = 5
	approvalCodeLockDuration      = time.Minute
)

var (
	ErrInvalidApprovalCode    = errors.New("invalid approval code")
	ErrApprovalCodeThrottled  = errors.New("approval code verification throttled")
	ErrDeviceProfileNotLinked = errors.New("device is not linked to profile")
)

type approvalCodeVerifier struct {
	Version    int    `json:"version"`
	Iterations int    `json:"iterations"`
	Salt       []byte `json:"salt"`
	Hash       []byte `json:"hash"`
}

// SetApprovalCode sets or replaces the parent approval code for one customer.
func (s *Store) SetApprovalCode(
	ctx context.Context,
	customerID CustomerID,
	code string,
) error {
	if err := customerID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(code) == "" {
		return errors.New("approval code is required")
	}

	encodedVerifier, err := encodeApprovalCodeVerifier(
		code,
		s.approvalCodeIterations,
	)
	if err != nil {
		return err
	}

	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO parent_credentials(
			customer_id,
			approval_code_hash,
			failed_attempts,
			locked_until,
			updated_at
		)
		SELECT ?, ?, 0, NULL, ?
		WHERE EXISTS (
			SELECT 1 FROM customers WHERE id = ?
		)
		ON CONFLICT(customer_id) DO UPDATE SET
			approval_code_hash = excluded.approval_code_hash,
			failed_attempts = 0,
			locked_until = NULL,
			updated_at = excluded.updated_at`,
		customerID,
		encodedVerifier,
		s.now().UTC(),
		customerID,
	)
	if err != nil {
		return fmt.Errorf("storing approval code verifier: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking approval code update: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// VerifyApprovalCode validates one customer-owned parent approval code.
func (s *Store) VerifyApprovalCode(
	ctx context.Context,
	customerID CustomerID,
	code string,
) error {
	if err := customerID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(code) == "" {
		return ErrInvalidApprovalCode
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting approval code verification: %w", err)
	}
	defer tx.Rollback()

	var encodedVerifier []byte
	var failedAttempts int
	var lockedUntil sql.NullString
	err = tx.QueryRowContext(
		ctx,
		`SELECT approval_code_hash, failed_attempts, locked_until
		FROM parent_credentials
		WHERE customer_id = ?`,
		customerID,
	).Scan(&encodedVerifier, &failedAttempts, &lockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidApprovalCode
	}
	if err != nil {
		return fmt.Errorf("reading approval code verifier: %w", err)
	}

	now := s.now().UTC()
	if lockedUntil.Valid {
		lockExpiry, err := time.Parse(time.RFC3339Nano, lockedUntil.String)
		if err != nil {
			return fmt.Errorf("parsing approval code lock: %w", err)
		}
		if now.Before(lockExpiry) {
			return ErrApprovalCodeThrottled
		}
		failedAttempts = 0
	}

	valid, err := approvalCodeMatches(encodedVerifier, code)
	if err != nil {
		return err
	}
	if valid {
		if err := resetApprovalCodeFailures(ctx, tx, customerID, now); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing approval code verification: %w", err)
		}
		return nil
	}

	failedAttempts++
	var newLock any
	if failedAttempts >= maxApprovalCodeFailures {
		newLock = now.Add(approvalCodeLockDuration).Format(time.RFC3339Nano)
	}
	_, err = tx.ExecContext(
		ctx,
		`UPDATE parent_credentials
		SET failed_attempts = ?, locked_until = ?, updated_at = ?
		WHERE customer_id = ?`,
		failedAttempts,
		newLock,
		now,
		customerID,
	)
	if err != nil {
		return fmt.Errorf("recording approval code failure: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing approval code failure: %w", err)
	}
	return ErrInvalidApprovalCode
}

// ValidateDeviceProfileAccess verifies a device can act for one child profile.
func (s *Store) ValidateDeviceProfileAccess(
	ctx context.Context,
	customerID CustomerID,
	deviceID DeviceID,
	profileID ProfileID,
) error {
	linked, err := s.DeviceProfileLinked(
		ctx,
		customerID,
		deviceID,
		profileID,
	)
	if err != nil {
		return err
	}
	if !linked {
		return ErrDeviceProfileNotLinked
	}
	return nil
}

func encodeApprovalCodeVerifier(code string, iterations int) ([]byte, error) {
	salt := make([]byte, approvalCodeSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating approval code salt: %w", err)
	}

	hash, err := pbkdf2.Key(
		sha256.New,
		code,
		salt,
		iterations,
		approvalCodeHashBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("deriving approval code verifier: %w", err)
	}

	encoded, err := json.Marshal(
		approvalCodeVerifier{
			Version:    approvalCodeVerifierVersion,
			Iterations: iterations,
			Salt:       salt,
			Hash:       hash,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("encoding approval code verifier: %w", err)
	}
	return encoded, nil
}

func approvalCodeMatches(encodedVerifier []byte, code string) (bool, error) {
	var verifier approvalCodeVerifier
	if err := json.Unmarshal(encodedVerifier, &verifier); err != nil {
		return false, fmt.Errorf("decoding approval code verifier: %w", err)
	}

	isSupportedVersion := verifier.Version == approvalCodeVerifierVersion
	hasIterations := verifier.Iterations > 0
	hasValidSalt := len(verifier.Salt) == approvalCodeSaltBytes
	hasValidHash := len(verifier.Hash) == approvalCodeHashBytes
	if !isSupportedVersion || !hasIterations || !hasValidSalt || !hasValidHash {
		return false, errors.New("invalid stored approval code verifier")
	}

	hash, err := pbkdf2.Key(
		sha256.New,
		code,
		verifier.Salt,
		verifier.Iterations,
		len(verifier.Hash),
	)
	if err != nil {
		return false, fmt.Errorf("deriving approval code verifier: %w", err)
	}
	return subtle.ConstantTimeCompare(hash, verifier.Hash) == 1, nil
}

func resetApprovalCodeFailures(
	ctx context.Context,
	tx *sql.Tx,
	customerID CustomerID,
	now time.Time,
) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE parent_credentials
		SET failed_attempts = 0, locked_until = NULL, updated_at = ?
		WHERE customer_id = ?`,
		now,
		customerID,
	)
	if err != nil {
		return fmt.Errorf("resetting approval code failures: %w", err)
	}
	return nil
}
