package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

var ErrInvalidActiveBrowserProfile = errors.New("invalid active browser profile request")

// SetActiveBrowserProfile changes or clears the active browser policy profile.
func (s *Store) SetActiveBrowserProfile(
	ctx context.Context,
	customerID CustomerID,
	profileID ProfileID,
	request ActiveBrowserProfileRequest,
) (ProfileConfiguration, error) {
	if request.DurationSeconds < 0 {
		return ProfileConfiguration{}, ErrInvalidActiveBrowserProfile
	}
	if request.DurationSeconds > 0 && request.ExpiresAt != nil {
		return ProfileConfiguration{}, ErrInvalidActiveBrowserProfile
	}

	now := s.now().UTC()
	return s.mutateProfilePolicy(
		ctx,
		customerID,
		profileID,
		func(profilePolicy *policy.ProfilePolicy) (bool, error) {
			return setActiveBrowserProfile(profilePolicy, request, now)
		},
	)
}

func setActiveBrowserProfile(
	profilePolicy *policy.ProfilePolicy,
	request ActiveBrowserProfileRequest,
	now time.Time,
) (bool, error) {
	settings, ok, err := kidControlSettings(*profilePolicy)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrBrowserProfileNotFound
	}

	name := strings.TrimSpace(request.Name)
	if name == "" {
		if settings.BrowserMonitoring.ActiveProfile == nil {
			return false, nil
		}
		settings.BrowserMonitoring.ActiveProfile = nil
		return true, setKidControlSettings(profilePolicy, settings)
	}
	if _, exists := settings.BrowserMonitoring.Profiles[name]; !exists {
		return false, ErrBrowserProfileNotFound
	}

	fallback := strings.TrimSpace(request.FallbackProfile)
	if fallback == "" {
		fallback = settings.BrowserMonitoring.DefaultProfile
	}
	if fallback != "" {
		if _, exists := settings.BrowserMonitoring.Profiles[fallback]; !exists {
			return false, ErrBrowserProfileNotFound
		}
	}

	expiresAt := request.ExpiresAt
	if request.DurationSeconds > 0 {
		value := now.Add(time.Duration(request.DurationSeconds) * time.Second)
		expiresAt = &value
	}
	if expiresAt != nil {
		value := expiresAt.UTC()
		if !value.After(now) {
			return false, ErrInvalidActiveBrowserProfile
		}
		expiresAt = &value
	}

	active := &policy.ActiveBrowserProfile{
		Name:            name,
		ExpiresAt:       expiresAt,
		FallbackProfile: fallback,
	}
	if activeBrowserProfilesEqual(settings.BrowserMonitoring.ActiveProfile, active) {
		return false, nil
	}

	settings.BrowserMonitoring.ActiveProfile = active
	return true, setKidControlSettings(profilePolicy, settings)
}

func activeBrowserProfilesEqual(
	current *policy.ActiveBrowserProfile,
	next *policy.ActiveBrowserProfile,
) bool {
	if current == nil || next == nil {
		return current == next
	}
	if current.Name != next.Name || current.FallbackProfile != next.FallbackProfile {
		return false
	}
	if current.ExpiresAt == nil || next.ExpiresAt == nil {
		return current.ExpiresAt == next.ExpiresAt
	}
	return current.ExpiresAt.Equal(*next.ExpiresAt)
}
