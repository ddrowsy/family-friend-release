package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

type profilePolicyMutation func(*policy.ProfilePolicy) (bool, error)

func (s *Store) mutateProfilePolicy(
	ctx context.Context,
	customerID CustomerID,
	profileID ProfileID,
	mutation profilePolicyMutation,
) (ProfileConfiguration, error) {
	if err := customerID.Validate(); err != nil {
		return ProfileConfiguration{}, err
	}
	if err := profileID.Validate(); err != nil {
		return ProfileConfiguration{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProfileConfiguration{}, fmt.Errorf("starting policy mutation: %w", err)
	}
	defer tx.Rollback()

	var revision int64
	var policyJSON []byte
	err = tx.QueryRowContext(
		ctx,
		`SELECT revision, policy_json
		FROM profile_configs
		WHERE customer_id = ? AND profile_id = ?`,
		customerID,
		profileID,
	).Scan(&revision, &policyJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ProfileConfiguration{}, ErrNotFound
	}
	if err != nil {
		return ProfileConfiguration{}, fmt.Errorf("reading profile configuration: %w", err)
	}

	var profilePolicy policy.ProfilePolicy
	if err := json.Unmarshal(policyJSON, &profilePolicy); err != nil {
		return ProfileConfiguration{}, fmt.Errorf("decoding profile policy: %w", err)
	}

	changed, err := cleanupExpiredTemporaryApprovals(
		&profilePolicy,
		s.now().UTC(),
	)
	if err != nil {
		return ProfileConfiguration{}, err
	}
	if mutation != nil {
		mutated, mutationErr := mutation(&profilePolicy)
		if mutationErr != nil {
			return ProfileConfiguration{}, mutationErr
		}
		changed = changed || mutated
	}

	configuration := ProfileConfiguration{
		CustomerID: customerID,
		ProfileID:  profileID,
		Revision:   revision,
		Policy:     profilePolicy,
	}
	if !changed {
		return configuration, nil
	}
	if err := validateProfilePolicy(profilePolicy); err != nil {
		return ProfileConfiguration{}, err
	}

	policyJSON, err = json.Marshal(profilePolicy)
	if err != nil {
		return ProfileConfiguration{}, fmt.Errorf("encoding profile policy: %w", err)
	}

	revision++
	_, err = tx.ExecContext(
		ctx,
		`UPDATE profile_configs
		SET revision = ?, policy_json = ?, updated_at = ?
		WHERE customer_id = ? AND profile_id = ?`,
		revision,
		policyJSON,
		s.now().UTC(),
		customerID,
		profileID,
	)
	if err != nil {
		return ProfileConfiguration{}, fmt.Errorf("storing profile policy mutation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProfileConfiguration{}, fmt.Errorf("committing profile policy mutation: %w", err)
	}

	configuration.Revision = revision
	configuration.Policy = profilePolicy
	return configuration, nil
}

func cleanupExpiredTemporaryApprovals(
	profilePolicy *policy.ProfilePolicy,
	now time.Time,
) (bool, error) {
	settings, ok, err := kidControlSettings(*profilePolicy)
	if err != nil || !ok {
		return false, err
	}
	if !settings.BrowserMonitoring.CleanupExpiredTemporaryApprovals(now) {
		return false, nil
	}
	if err := setKidControlSettings(profilePolicy, settings); err != nil {
		return false, err
	}
	return true, nil
}

func kidControlSettings(
	profilePolicy policy.ProfilePolicy,
) (policy.KidControlSettings, bool, error) {
	module, ok := profilePolicy.Modules[policy.KidControlModuleName]
	if !ok {
		return policy.KidControlSettings{}, false, nil
	}

	data, err := json.Marshal(module.Settings)
	if err != nil {
		return policy.KidControlSettings{}, false, fmt.Errorf(
			"encoding kidcontrol settings: %w",
			err,
		)
	}

	var settings policy.KidControlSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return policy.KidControlSettings{}, false, fmt.Errorf(
			"decoding kidcontrol settings: %w",
			err,
		)
	}
	if settings.BrowserMonitoring.Profiles == nil {
		settings.BrowserMonitoring.Profiles = map[string]policy.BrowserProfile{}
	}

	return settings, true, nil
}

func setKidControlSettings(
	profilePolicy *policy.ProfilePolicy,
	settings policy.KidControlSettings,
) error {
	module, ok := profilePolicy.Modules[policy.KidControlModuleName]
	if !ok {
		return ErrBrowserProfileNotFound
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encoding kidcontrol settings: %w", err)
	}

	settingsMap := map[string]any{}
	if err := json.Unmarshal(data, &settingsMap); err != nil {
		return fmt.Errorf("decoding kidcontrol settings map: %w", err)
	}

	module.Settings = settingsMap
	profilePolicy.Modules[policy.KidControlModuleName] = module
	return nil
}
