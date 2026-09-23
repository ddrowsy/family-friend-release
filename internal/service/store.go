package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("service resource not found")

// Store persists profile configuration and device assignments.
type Store struct {
	db                     *sql.DB
	now                    func() time.Time
	approvalCodeIterations int
}

// NewStore creates a service store backed by an initialized SQLite database.
func NewStore(db *sql.DB) *Store {
	return &Store{
		db:                     db,
		now:                    time.Now,
		approvalCodeIterations: defaultApprovalCodeIterations,
	}
}

// CreateProfile creates a customer when needed and adds one child profile.
func (s *Store) CreateProfile(ctx context.Context, customerID CustomerID, profileID ProfileID) error {
	if err := customerID.Validate(); err != nil {
		return err
	}
	if err := profileID.Validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting profile transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(
		ctx,
		"INSERT OR IGNORE INTO customers(id, created_at) VALUES (?, ?)",
		customerID,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("creating customer: %w", err)
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO profiles(
			customer_id,
			id,
			name,
			created_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?)`,
		customerID,
		profileID,
		profileID,
		time.Now().UTC(),
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("creating profile: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing profile transaction: %w", err)
	}
	return nil
}

// PutProfileConfiguration stores policy and increments the profile revision when it changes.
func (s *Store) PutProfileConfiguration(
	ctx context.Context,
	configuration ProfileConfiguration,
) (ProfileConfiguration, error) {
	if err := configuration.CustomerID.Validate(); err != nil {
		return ProfileConfiguration{}, err
	}
	if err := configuration.ProfileID.Validate(); err != nil {
		return ProfileConfiguration{}, err
	}
	if _, err := cleanupExpiredTemporaryApprovals(
		&configuration.Policy,
		s.now().UTC(),
	); err != nil {
		return ProfileConfiguration{}, err
	}
	if err := configuration.Validate(); err != nil {
		return ProfileConfiguration{}, err
	}

	policyJSON, err := json.Marshal(configuration.Policy)
	if err != nil {
		return ProfileConfiguration{}, fmt.Errorf("encoding profile policy: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProfileConfiguration{}, fmt.Errorf("starting configuration transaction: %w", err)
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(
		ctx,
		"SELECT 1 FROM profiles WHERE customer_id = ? AND id = ?",
		configuration.CustomerID,
		configuration.ProfileID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ProfileConfiguration{}, ErrNotFound
	}
	if err != nil {
		return ProfileConfiguration{}, fmt.Errorf("finding profile: %w", err)
	}

	var revision int64
	var existingPolicyJSON []byte
	err = tx.QueryRowContext(
		ctx,
		`SELECT revision, policy_json
		FROM profile_configs
		WHERE customer_id = ? AND profile_id = ?`,
		configuration.CustomerID,
		configuration.ProfileID,
	).Scan(&revision, &existingPolicyJSON)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ProfileConfiguration{}, fmt.Errorf("reading profile configuration: %w", err)
	}

	if err == nil && bytes.Equal(existingPolicyJSON, policyJSON) {
		configuration.Revision = revision
		return configuration, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		revision = 1
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO profile_configs(
				customer_id,
				profile_id,
				revision,
				policy_json,
				updated_at
			) VALUES (?, ?, ?, ?, ?)`,
			configuration.CustomerID,
			configuration.ProfileID,
			revision,
			policyJSON,
			s.now().UTC(),
		)
	} else {
		revision++
		_, err = tx.ExecContext(
			ctx,
			`UPDATE profile_configs
			SET revision = ?, policy_json = ?, updated_at = ?
			WHERE customer_id = ? AND profile_id = ?`,
			revision,
			policyJSON,
			s.now().UTC(),
			configuration.CustomerID,
			configuration.ProfileID,
		)
	}
	if err != nil {
		return ProfileConfiguration{}, fmt.Errorf("storing profile configuration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProfileConfiguration{}, fmt.Errorf("committing configuration transaction: %w", err)
	}

	configuration.Revision = revision
	return configuration, nil
}

// ProfileConfiguration returns the persisted configuration for one child profile.
// Expired temporary browser approvals are removed and persisted before returning.
func (s *Store) ProfileConfiguration(
	ctx context.Context,
	customerID CustomerID,
	profileID ProfileID,
) (ProfileConfiguration, error) {
	return s.mutateProfilePolicy(ctx, customerID, profileID, nil)
}

// RegisterDevice registers an agent installation under a customer.
func (s *Store) RegisterDevice(ctx context.Context, registration DeviceRegistration) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO devices(customer_id, id, created_at)
		SELECT ?, ?, ?
		WHERE EXISTS (SELECT 1 FROM customers WHERE id = ?)`,
		registration.CustomerID,
		registration.DeviceID,
		time.Now().UTC(),
		registration.CustomerID,
	)
	if err != nil {
		return fmt.Errorf("registering device: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking device registration: %w", err)
	}
	if rows != 0 {
		return nil
	}

	var exists int
	err = s.db.QueryRowContext(
		ctx,
		"SELECT 1 FROM devices WHERE customer_id = ? AND id = ?",
		registration.CustomerID,
		registration.DeviceID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("finding registered device: %w", err)
	}
	return nil
}

// LinkDeviceProfile links a registered device to a child profile in the same customer.
func (s *Store) LinkDeviceProfile(
	ctx context.Context,
	customerID CustomerID,
	deviceID DeviceID,
	profileID ProfileID,
) error {
	if err := validateDeviceProfileIdentity(customerID, deviceID, profileID); err != nil {
		return err
	}

	result, err := s.db.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO device_profiles(
			customer_id,
			device_id,
			profile_id,
			created_at
		)
		SELECT ?, ?, ?, ?
		WHERE EXISTS (
			SELECT 1 FROM devices WHERE customer_id = ? AND id = ?
		)
		AND EXISTS (
			SELECT 1 FROM profiles WHERE customer_id = ? AND id = ?
		)`,
		customerID,
		deviceID,
		profileID,
		time.Now().UTC(),
		customerID,
		deviceID,
		customerID,
		profileID,
	)
	if err != nil {
		return fmt.Errorf("linking device profile: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking device profile link: %w", err)
	}
	if rows != 0 {
		return nil
	}

	var exists int
	err = s.db.QueryRowContext(
		ctx,
		`SELECT 1 FROM device_profiles
		WHERE customer_id = ? AND device_id = ? AND profile_id = ?`,
		customerID,
		deviceID,
		profileID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("finding device profile link: %w", err)
	}
	return nil
}

// UnlinkDeviceProfile removes one device-to-profile association.
func (s *Store) UnlinkDeviceProfile(
	ctx context.Context,
	customerID CustomerID,
	deviceID DeviceID,
	profileID ProfileID,
) error {
	if err := validateDeviceProfileIdentity(customerID, deviceID, profileID); err != nil {
		return err
	}

	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM device_profiles
		WHERE customer_id = ? AND device_id = ? AND profile_id = ?`,
		customerID,
		deviceID,
		profileID,
	)
	if err != nil {
		return fmt.Errorf("unlinking device profile: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking device profile unlink: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeviceProfileLinked reports whether a registered device is linked to a profile.
func (s *Store) DeviceProfileLinked(
	ctx context.Context,
	customerID CustomerID,
	deviceID DeviceID,
	profileID ProfileID,
) (bool, error) {
	if err := validateDeviceProfileIdentity(customerID, deviceID, profileID); err != nil {
		return false, err
	}

	var deviceExists int
	var profileExists int
	var linked int
	err := s.db.QueryRowContext(
		ctx,
		`SELECT
			EXISTS(
				SELECT 1 FROM devices
				WHERE customer_id = ? AND id = ?
			),
			EXISTS(
				SELECT 1 FROM profiles
				WHERE customer_id = ? AND id = ?
			),
			EXISTS(
				SELECT 1 FROM device_profiles
				WHERE customer_id = ? AND device_id = ? AND profile_id = ?
			)`,
		customerID,
		deviceID,
		customerID,
		profileID,
		customerID,
		deviceID,
		profileID,
	).Scan(&deviceExists, &profileExists, &linked)
	if err != nil {
		return false, fmt.Errorf("checking device profile link: %w", err)
	}
	if deviceExists == 0 || profileExists == 0 {
		return false, ErrNotFound
	}
	return linked != 0, nil
}

// DeviceProfiles returns the child profiles linked to one registered device.
func (s *Store) DeviceProfiles(
	ctx context.Context,
	customerID CustomerID,
	deviceID DeviceID,
) ([]ProfileID, error) {
	if err := customerID.Validate(); err != nil {
		return nil, err
	}
	if err := deviceID.Validate(); err != nil {
		return nil, err
	}

	var exists int
	err := s.db.QueryRowContext(
		ctx,
		"SELECT 1 FROM devices WHERE customer_id = ? AND id = ?",
		customerID,
		deviceID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finding device: %w", err)
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT profile_id
		FROM device_profiles
		WHERE customer_id = ? AND device_id = ?
		ORDER BY profile_id`,
		customerID,
		deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing device profiles: %w", err)
	}
	defer rows.Close()

	profileIDs := []ProfileID{}
	for rows.Next() {
		var profileID ProfileID
		if err := rows.Scan(&profileID); err != nil {
			return nil, fmt.Errorf("scanning device profile: %w", err)
		}
		profileIDs = append(profileIDs, profileID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing device profiles: %w", err)
	}
	return profileIDs, nil
}

func validateDeviceProfileIdentity(
	customerID CustomerID,
	deviceID DeviceID,
	profileID ProfileID,
) error {
	if err := customerID.Validate(); err != nil {
		return err
	}
	if err := deviceID.Validate(); err != nil {
		return err
	}
	return profileID.Validate()
}
