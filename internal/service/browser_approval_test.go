package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
	"github.com/ddrowsy/family-friend-release/internal/service/database"
)

func TestTemporaryBrowserApprovalPersistsInProfilePolicyAndExpires(t *testing.T) {
	ctx := context.Background()
	store, path := newApprovalCodeTestStore(t)
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time {
		return now
	}

	if err := createBrowserApprovalProfile(
		ctx,
		store,
		"customer-1",
		"harry",
	); err != nil {
		t.Fatalf("createBrowserApprovalProfile: %v", err)
	}
	if err := registerAndLinkBrowserApprovalDevice(
		ctx,
		store,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("registerAndLinkBrowserApprovalDevice: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode: %v", err)
	}

	result, err := store.ApproveBrowserURL(
		ctx,
		"customer-1",
		"harry",
		BrowserApprovalRequest{
			DeviceID:             "laptop",
			URL:                  "https://Example.com/path?q=1",
			BrowserPolicyProfile: "study",
			ApprovalCode:         "1234",
			Action:               BrowserApprovalTemporary,
			DurationSeconds:      60,
		},
	)
	if err != nil {
		t.Fatalf("ApproveBrowserURL: %v", err)
	}

	wantExpiry := now.Add(time.Minute)
	if result.Revision != 2 {
		t.Fatalf("temporary revision = %d, want 2", result.Revision)
	}
	if result.ExpiresAt == nil || !result.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("ExpiresAt = %v, want %v", result.ExpiresAt, wantExpiry)
	}

	configuration, err := store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration: %v", err)
	}
	assertTemporaryApproval(
		t,
		browserProfileForTest(t, configuration, "study"),
		"example.com",
		wantExpiry,
	)

	if err := store.db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	db, err := database.Open(ctx, filepath.Clean(path))
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	store = NewStore(db)
	store.now = func() time.Time {
		return now
	}

	configuration, err = store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration after reopen: %v", err)
	}
	assertTemporaryApproval(
		t,
		browserProfileForTest(t, configuration, "study"),
		"example.com",
		wantExpiry,
	)

	now = wantExpiry.Add(time.Second)
	configuration, err = store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration after expiry: %v", err)
	}
	if configuration.Revision != 3 {
		t.Fatalf("cleanup revision = %d, want 3", configuration.Revision)
	}
	if got := len(browserProfileForTest(t, configuration, "study").TemporaryAllowedURLs); got != 0 {
		t.Fatalf("temporary approvals after expiry = %d, want 0", got)
	}

	configuration, err = store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("second ProfileConfiguration after expiry: %v", err)
	}
	if configuration.Revision != 3 {
		t.Fatalf("second cleanup revision = %d, want 3", configuration.Revision)
	}
}

func TestTemporaryBrowserApprovalIsProfileAndCustomerScoped(t *testing.T) {
	ctx := context.Background()
	store, _ := newApprovalCodeTestStore(t)

	for _, identity := range []struct {
		customerID CustomerID
		profileID  ProfileID
	}{
		{customerID: "customer-1", profileID: "harry"},
		{customerID: "customer-1", profileID: "james"},
		{customerID: "customer-2", profileID: "harry"},
	} {
		if err := createBrowserApprovalProfile(
			ctx,
			store,
			identity.customerID,
			identity.profileID,
		); err != nil {
			t.Fatalf(
				"createBrowserApprovalProfile %s/%s: %v",
				identity.customerID,
				identity.profileID,
				err,
			)
		}
	}
	if err := registerAndLinkBrowserApprovalDevice(
		ctx,
		store,
		"customer-1",
		"shared-laptop",
		"harry",
	); err != nil {
		t.Fatalf("registerAndLinkBrowserApprovalDevice: %v", err)
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"shared-laptop",
		"james",
	); err != nil {
		t.Fatalf("LinkDeviceProfile james: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode: %v", err)
	}

	if _, err := store.ApproveBrowserURL(
		ctx,
		"customer-1",
		"harry",
		BrowserApprovalRequest{
			DeviceID:             "shared-laptop",
			URL:                  "example.com",
			BrowserPolicyProfile: "study",
			ApprovalCode:         "1234",
			Action:               BrowserApprovalTemporary,
			DurationSeconds:      60,
		},
	); err != nil {
		t.Fatalf("ApproveBrowserURL: %v", err)
	}

	tests := []struct {
		name       string
		customerID CustomerID
		profileID  ProfileID
		wantCount  int
	}{
		{
			name:       "target profile",
			customerID: "customer-1",
			profileID:  "harry",
			wantCount:  1,
		},
		{
			name:       "other child",
			customerID: "customer-1",
			profileID:  "james",
		},
		{
			name:       "other customer",
			customerID: "customer-2",
			profileID:  "harry",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configuration, err := store.ProfileConfiguration(
				ctx,
				tt.customerID,
				tt.profileID,
			)
			if err != nil {
				t.Fatalf("ProfileConfiguration: %v", err)
			}
			got := len(
				browserProfileForTest(
					t,
					configuration,
					"study",
				).TemporaryAllowedURLs,
			)
			if got != tt.wantCount {
				t.Fatalf("temporary approval count = %d, want %d", got, tt.wantCount)
			}
		})
	}
}

func TestPermanentBrowserApprovalUpdatesOnlyTargetProfileAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, _ := newApprovalCodeTestStore(t)

	for _, profileID := range []ProfileID{"harry", "james"} {
		if err := createBrowserApprovalProfile(
			ctx,
			store,
			"customer-1",
			profileID,
		); err != nil {
			t.Fatalf("createBrowserApprovalProfile %s: %v", profileID, err)
		}
	}
	if err := registerAndLinkBrowserApprovalDevice(
		ctx,
		store,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("registerAndLinkBrowserApprovalDevice: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode: %v", err)
	}

	request := BrowserApprovalRequest{
		DeviceID:             "laptop",
		URL:                  "https://Example.com/path?q=1",
		BrowserPolicyProfile: "study",
		ApprovalCode:         "1234",
		Action:               BrowserApprovalPermanent,
	}
	result, err := store.ApproveBrowserURL(
		ctx,
		"customer-1",
		"harry",
		request,
	)
	if err != nil {
		t.Fatalf("ApproveBrowserURL: %v", err)
	}
	if result.Revision != 2 {
		t.Fatalf("permanent approval revision = %d, want 2", result.Revision)
	}

	harry, err := store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration harry: %v", err)
	}
	harryStudy := browserProfileForTest(t, harry, "study")
	if len(harryStudy.AllowedURLs) != 1 || harryStudy.AllowedURLs[0] != "example.com" {
		t.Fatalf("harry allowed URLs = %v, want [example.com]", harryStudy.AllowedURLs)
	}

	james, err := store.ProfileConfiguration(ctx, "customer-1", "james")
	if err != nil {
		t.Fatalf("ProfileConfiguration james: %v", err)
	}
	if got := len(browserProfileForTest(t, james, "study").AllowedURLs); got != 0 {
		t.Fatalf("james allowed URLs count = %d, want 0", got)
	}

	request.URL = "HTTPS://EXAMPLE.COM./different"
	result, err = store.ApproveBrowserURL(
		ctx,
		"customer-1",
		"harry",
		request,
	)
	if err != nil {
		t.Fatalf("duplicate ApproveBrowserURL: %v", err)
	}
	if result.Revision != 2 {
		t.Fatalf("duplicate approval revision = %d, want 2", result.Revision)
	}
}

func TestBrowserApprovalRejectsInvalidRequests(t *testing.T) {
	ctx := context.Background()
	store, _ := newApprovalCodeTestStore(t)

	if err := createBrowserApprovalProfile(
		ctx,
		store,
		"customer-1",
		"harry",
	); err != nil {
		t.Fatalf("createBrowserApprovalProfile: %v", err)
	}
	if err := store.RegisterDevice(
		ctx,
		DeviceRegistration{
			CustomerID: "customer-1",
			DeviceID:   "laptop",
		},
	); err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}
	if err := store.SetApprovalCode(ctx, "customer-1", "1234"); err != nil {
		t.Fatalf("SetApprovalCode: %v", err)
	}

	base := BrowserApprovalRequest{
		DeviceID:             "laptop",
		URL:                  "example.com",
		BrowserPolicyProfile: "study",
		ApprovalCode:         "1234",
		Action:               BrowserApprovalPermanent,
	}

	_, err := store.ApproveBrowserURL(ctx, "customer-1", "harry", base)
	if !errors.Is(err, ErrDeviceProfileNotLinked) {
		t.Fatalf("unlinked device error = %v, want ErrDeviceProfileNotLinked", err)
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("LinkDeviceProfile: %v", err)
	}

	request := base
	request.ApprovalCode = "wrong"
	_, err = store.ApproveBrowserURL(ctx, "customer-1", "harry", request)
	if !errors.Is(err, ErrInvalidApprovalCode) {
		t.Fatalf("invalid code error = %v, want ErrInvalidApprovalCode", err)
	}

	request = base
	request.BrowserPolicyProfile = "missing"
	_, err = store.ApproveBrowserURL(ctx, "customer-1", "harry", request)
	if !errors.Is(err, ErrBrowserProfileNotFound) {
		t.Fatalf("missing browser profile error = %v, want ErrBrowserProfileNotFound", err)
	}

	request = base
	request.URL = "ftp://example.com/file"
	_, err = store.ApproveBrowserURL(ctx, "customer-1", "harry", request)
	if !errors.Is(err, ErrInvalidBrowserURL) {
		t.Fatalf("invalid URL error = %v, want ErrInvalidBrowserURL", err)
	}

	request = base
	request.Action = BrowserApprovalTemporary
	request.DurationSeconds = 0
	_, err = store.ApproveBrowserURL(ctx, "customer-1", "harry", request)
	if !errors.Is(err, ErrInvalidApprovalDuration) {
		t.Fatalf("invalid duration error = %v, want ErrInvalidApprovalDuration", err)
	}

	request = base
	request.Action = BrowserApprovalAction("unsupported")
	_, err = store.ApproveBrowserURL(ctx, "customer-1", "harry", request)
	if !errors.Is(err, ErrInvalidApprovalAction) {
		t.Fatalf("invalid action error = %v, want ErrInvalidApprovalAction", err)
	}
}

func createBrowserApprovalProfile(
	ctx context.Context,
	store *Store,
	customerID CustomerID,
	profileID ProfileID,
) error {
	if err := store.CreateProfile(ctx, customerID, profileID); err != nil {
		return err
	}
	_, err := store.PutProfileConfiguration(
		ctx,
		ProfileConfiguration{
			CustomerID: customerID,
			ProfileID:  profileID,
			Policy:     browserApprovalPolicy([]string{}),
		},
	)
	return err
}

func registerAndLinkBrowserApprovalDevice(
	ctx context.Context,
	store *Store,
	customerID CustomerID,
	deviceID DeviceID,
	profileID ProfileID,
) error {
	if err := store.RegisterDevice(
		ctx,
		DeviceRegistration{
			CustomerID: customerID,
			DeviceID:   deviceID,
		},
	); err != nil {
		return err
	}
	return store.LinkDeviceProfile(ctx, customerID, deviceID, profileID)
}

func browserProfileForTest(
	t *testing.T,
	configuration ProfileConfiguration,
	name string,
) policy.BrowserProfile {
	t.Helper()

	settings, ok, err := kidControlSettings(configuration.Policy)
	if err != nil {
		t.Fatalf("kidControlSettings: %v", err)
	}
	if !ok {
		t.Fatal("kidcontrol settings are missing")
	}

	profile, ok := settings.BrowserMonitoring.Profiles[name]
	if !ok {
		t.Fatalf("browser profile %q is missing", name)
	}
	return profile
}

func assertTemporaryApproval(
	t *testing.T,
	profile policy.BrowserProfile,
	wantURL string,
	wantExpiry time.Time,
) {
	t.Helper()

	if len(profile.TemporaryAllowedURLs) != 1 {
		t.Fatalf(
			"temporary approval count = %d, want 1",
			len(profile.TemporaryAllowedURLs),
		)
	}
	approval := profile.TemporaryAllowedURLs[0]
	if approval.URL != wantURL {
		t.Fatalf("temporary approval URL = %q, want %q", approval.URL, wantURL)
	}
	if !approval.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf(
			"temporary approval expiry = %v, want %v",
			approval.ExpiresAt,
			wantExpiry,
		)
	}
}

func browserApprovalPolicy(allowedURLs []string) policy.ProfilePolicy {
	return policy.ProfilePolicy{
		SchemaVersion: 1,
		Version:       "1",
		Modules: map[string]policy.ModulePolicy{
			policy.KidControlModuleName: {
				Enabled: true,
				Mode:    "enforce",
				Settings: map[string]any{
					"browser_monitoring": map[string]any{
						"enabled":         true,
						"default_profile": "default",
						"profiles": map[string]any{
							"default": map[string]any{
								"policy":              "block_list",
								"allowed_urls":        []string{},
								"blocked_urls":        []string{},
								"block_inappropriate": true,
							},
							"study": map[string]any{
								"policy":              "allow_list",
								"allowed_urls":        allowedURLs,
								"blocked_urls":        []string{},
								"block_inappropriate": true,
							},
						},
					},
				},
			},
		},
	}
}
