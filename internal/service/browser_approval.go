package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

var (
	ErrBrowserProfileNotFound  = errors.New("browser policy profile not found")
	ErrInvalidBrowserURL       = errors.New("invalid browser URL")
	ErrInvalidApprovalAction   = errors.New("invalid browser approval action")
	ErrInvalidApprovalDuration = errors.New("temporary approval duration must be positive")
)

type browserApprovalScope struct {
	customerID CustomerID
	profileID  ProfileID
}

type browserApprovalTarget struct {
	browserProfile string
	rule           string
}

// ApproveBrowserURL applies a parent-approved temporary or permanent browser rule.
func (s *Store) ApproveBrowserURL(
	ctx context.Context,
	customerID CustomerID,
	profileID ProfileID,
	request BrowserApprovalRequest,
) (BrowserApprovalResult, error) {
	if err := customerID.Validate(); err != nil {
		return BrowserApprovalResult{}, err
	}
	if err := profileID.Validate(); err != nil {
		return BrowserApprovalResult{}, err
	}
	if err := validateBrowserApprovalRequest(request); err != nil {
		return BrowserApprovalResult{}, err
	}

	rule, err := normalizeBrowserApprovalURL(request.URL)
	if err != nil {
		return BrowserApprovalResult{}, err
	}

	scope := browserApprovalScope{
		customerID: customerID,
		profileID:  profileID,
	}
	target := browserApprovalTarget{
		browserProfile: request.BrowserPolicyProfile,
		rule:           rule,
	}

	if err := s.ValidateDeviceProfileAccess(
		ctx,
		customerID,
		request.DeviceID,
		profileID,
	); err != nil {
		return BrowserApprovalResult{}, err
	}
	if err := s.VerifyApprovalCode(ctx, customerID, request.ApprovalCode); err != nil {
		return BrowserApprovalResult{}, err
	}

	result := BrowserApprovalResult{
		CustomerID: customerID,
		ProfileID:  profileID,
		DeviceID:   request.DeviceID,
	}
	switch request.Action {
	case BrowserApprovalTemporary:
		revision, expiresAt, err := s.approveTemporaryBrowserURL(
			ctx,
			scope,
			request,
			target,
		)
		if err != nil {
			return BrowserApprovalResult{}, err
		}
		result.Revision = revision
		result.ExpiresAt = &expiresAt
	case BrowserApprovalPermanent:
		revision, err := s.approvePermanentBrowserURL(ctx, scope, target)
		if err != nil {
			return BrowserApprovalResult{}, err
		}
		result.Revision = revision
	}

	return result, nil
}

func validateBrowserApprovalRequest(request BrowserApprovalRequest) error {
	if err := request.DeviceID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(request.BrowserPolicyProfile) == "" {
		return ErrBrowserProfileNotFound
	}

	switch request.Action {
	case BrowserApprovalTemporary:
		if request.DurationSeconds <= 0 {
			return ErrInvalidApprovalDuration
		}
	case BrowserApprovalPermanent:
	default:
		return ErrInvalidApprovalAction
	}
	return nil
}

func (s *Store) approveTemporaryBrowserURL(
	ctx context.Context,
	scope browserApprovalScope,
	request BrowserApprovalRequest,
	target browserApprovalTarget,
) (int64, time.Time, error) {
	now := s.now().UTC()
	expiresAt := now.Add(time.Duration(request.DurationSeconds) * time.Second)

	configuration, err := s.mutateProfilePolicy(
		ctx,
		scope.customerID,
		scope.profileID,
		func(profilePolicy *policy.ProfilePolicy) (bool, error) {
			return addTemporaryBrowserRule(
				profilePolicy,
				target,
				expiresAt,
			)
		},
	)
	if err != nil {
		return 0, time.Time{}, err
	}

	return configuration.Revision, expiresAt, nil
}

func (s *Store) approvePermanentBrowserURL(
	ctx context.Context,
	scope browserApprovalScope,
	target browserApprovalTarget,
) (int64, error) {
	configuration, err := s.mutateProfilePolicy(
		ctx,
		scope.customerID,
		scope.profileID,
		func(profilePolicy *policy.ProfilePolicy) (bool, error) {
			return addPermanentBrowserRule(profilePolicy, target)
		},
	)
	if err != nil {
		return 0, err
	}
	return configuration.Revision, nil
}

func addTemporaryBrowserRule(
	profilePolicy *policy.ProfilePolicy,
	target browserApprovalTarget,
	expiresAt time.Time,
) (bool, error) {
	settings, profile, err := browserProfileSettings(
		*profilePolicy,
		target.browserProfile,
	)
	if err != nil {
		return false, err
	}

	approvals := profile.TemporaryAllowedURLs
	if approvals == nil {
		approvals = []policy.TemporaryBrowserApproval{}
	}
	for i, approval := range approvals {
		if approval.URL != target.rule {
			continue
		}
		if approval.ExpiresAt.Equal(expiresAt) {
			return false, nil
		}

		approvals[i].ExpiresAt = expiresAt
		profile.TemporaryAllowedURLs = approvals
		settings.BrowserMonitoring.Profiles[target.browserProfile] = profile
		if err := setKidControlSettings(profilePolicy, settings); err != nil {
			return false, err
		}
		return true, nil
	}

	profile.TemporaryAllowedURLs = append(
		approvals,
		policy.TemporaryBrowserApproval{
			URL:       target.rule,
			ExpiresAt: expiresAt,
		},
	)
	settings.BrowserMonitoring.Profiles[target.browserProfile] = profile
	if err := setKidControlSettings(profilePolicy, settings); err != nil {
		return false, err
	}
	return true, nil
}

func addPermanentBrowserRule(
	profilePolicy *policy.ProfilePolicy,
	target browserApprovalTarget,
) (bool, error) {
	settings, profile, err := browserProfileSettings(
		*profilePolicy,
		target.browserProfile,
	)
	if err != nil {
		return false, err
	}

	allowedURLs := profile.AllowedURLs
	if allowedURLs == nil {
		allowedURLs = []string{}
	}
	for _, existingRule := range allowedURLs {
		if existingRule == target.rule {
			return false, nil
		}
	}

	profile.AllowedURLs = append(allowedURLs, target.rule)
	settings.BrowserMonitoring.Profiles[target.browserProfile] = profile
	if err := setKidControlSettings(profilePolicy, settings); err != nil {
		return false, err
	}
	return true, nil
}

func browserProfileSettings(
	profilePolicy policy.ProfilePolicy,
	browserProfile string,
) (policy.KidControlSettings, policy.BrowserProfile, error) {
	settings, ok, err := kidControlSettings(profilePolicy)
	if err != nil {
		return policy.KidControlSettings{}, policy.BrowserProfile{}, err
	}
	if !ok {
		return policy.KidControlSettings{}, policy.BrowserProfile{}, ErrBrowserProfileNotFound
	}

	profile, ok := settings.BrowserMonitoring.Profiles[browserProfile]
	if !ok {
		return policy.KidControlSettings{}, policy.BrowserProfile{}, ErrBrowserProfileNotFound
	}
	return settings, profile, nil
}

func normalizeBrowserApprovalURL(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return "", ErrInvalidBrowserURL
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", ErrInvalidBrowserURL
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", ErrInvalidBrowserURL
	}

	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return "", ErrInvalidBrowserURL
	}
	return host, nil
}
