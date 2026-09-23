package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/ddrowsy/family-friend-release/internal/policy"
	"github.com/ddrowsy/family-friend-release/internal/service"
)

// ProfilePolicyClient provides the control-service reads needed for synchronization.
type ProfilePolicyClient interface {
	DeviceProfiles(
		ctx context.Context,
		customerID service.CustomerID,
		deviceID service.DeviceID,
	) (service.DeviceProfileAssignment, error)
	ProfileConfiguration(
		ctx context.Context,
		customerID service.CustomerID,
		profileID service.ProfileID,
	) (service.ProfileConfiguration, error)
}

// ProfilePolicyApplier applies one shared profile policy to the local runtime.
type ProfilePolicyApplier interface {
	ApplyProfilePolicy(
		ctx context.Context,
		profileID service.ProfileID,
		profilePolicy policy.ProfilePolicy,
	) error
}

// ProfilePolicySynchronizer applies newer linked-profile revisions from the service.
type ProfilePolicySynchronizer struct {
	client       ProfilePolicyClient
	applier      ProfilePolicyApplier
	customerID   service.CustomerID
	deviceID     service.DeviceID
	lastRevision map[service.ProfileID]int64
}

// NewProfilePolicySynchronizer creates a synchronizer for one local agent installation.
func NewProfilePolicySynchronizer(
	client ProfilePolicyClient,
	applier ProfilePolicyApplier,
	customerID service.CustomerID,
	deviceID service.DeviceID,
) *ProfilePolicySynchronizer {
	return &ProfilePolicySynchronizer{
		client:       client,
		applier:      applier,
		customerID:   customerID,
		deviceID:     deviceID,
		lastRevision: map[service.ProfileID]int64{},
	}
}

// Sync fetches linked profiles and applies configurations newer than the last successful revision.
func (s *ProfilePolicySynchronizer) Sync(ctx context.Context) error {
	assignment, err := s.client.DeviceProfiles(ctx, s.customerID, s.deviceID)
	if err != nil {
		return fmt.Errorf("listing linked profiles: %w", err)
	}

	syncErrors := []error{}
	for _, profileID := range assignment.ProfileIDs {
		if err := s.syncProfile(ctx, profileID); err != nil {
			syncErrors = append(syncErrors, err)
		}
	}
	return errors.Join(syncErrors...)
}

func (s *ProfilePolicySynchronizer) syncProfile(
	ctx context.Context,
	profileID service.ProfileID,
) error {
	configuration, err := s.client.ProfileConfiguration(ctx, s.customerID, profileID)
	if err != nil {
		return fmt.Errorf("fetching profile %q configuration: %w", profileID, err)
	}
	if configuration.Revision <= s.lastRevision[profileID] {
		return nil
	}

	if err := s.applier.ApplyProfilePolicy(ctx, profileID, configuration.Policy); err != nil {
		return fmt.Errorf("applying profile %q policy: %w", profileID, err)
	}
	s.lastRevision[profileID] = configuration.Revision
	return nil
}
