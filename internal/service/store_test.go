package service

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
	"github.com/ddrowsy/family-friend-release/internal/service/database"
)

func TestStorePersistsProfilesConfigurationAndDeviceLinks(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "service.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	store := NewStore(db)

	if err := store.CreateProfile(ctx, "customer-1", "harry"); err != nil {
		t.Fatalf("CreateProfile harry: %v", err)
	}
	if err := store.CreateProfile(ctx, "customer-1", "james"); err != nil {
		t.Fatalf("CreateProfile james: %v", err)
	}

	configuration := ProfileConfiguration{
		CustomerID: "customer-1",
		ProfileID:  "harry",
		Policy: policy.ProfilePolicy{
			SchemaVersion: 1,
			Version:       "1",
			Modules:       map[string]policy.ModulePolicy{},
		},
	}
	first, err := store.PutProfileConfiguration(ctx, configuration)
	if err != nil {
		t.Fatalf("PutProfileConfiguration first: %v", err)
	}
	second, err := store.PutProfileConfiguration(ctx, configuration)
	if err != nil {
		t.Fatalf("PutProfileConfiguration second: %v", err)
	}
	if first.Revision != 1 || second.Revision != 1 {
		t.Fatalf("revisions = %d, %d; want 1, 1", first.Revision, second.Revision)
	}

	registration := DeviceRegistration{
		CustomerID: "customer-1",
		DeviceID:   "laptop",
	}
	if err := store.RegisterDevice(ctx, registration); err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("LinkDeviceProfile harry: %v", err)
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"laptop",
		"james",
	); err != nil {
		t.Fatalf("LinkDeviceProfile james: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	db, err = database.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer db.Close()
	store = NewStore(db)

	persisted, err := store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration: %v", err)
	}
	if persisted.Revision != 1 {
		t.Fatalf("persisted revision = %d, want 1", persisted.Revision)
	}
	profileIDs, err := store.DeviceProfiles(ctx, "customer-1", "laptop")
	if err != nil {
		t.Fatalf("DeviceProfiles: %v", err)
	}
	want := []ProfileID{"harry", "james"}
	if !reflect.DeepEqual(profileIDs, want) {
		t.Fatalf("DeviceProfiles = %v, want %v", profileIDs, want)
	}
}

func TestStoreKeepsCustomersAndProfileRevisionsIsolated(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	store := NewStore(db)

	for _, customerID := range []CustomerID{"customer-1", "customer-2"} {
		if err := store.CreateProfile(ctx, customerID, "child"); err != nil {
			t.Fatalf("CreateProfile %s: %v", customerID, err)
		}
	}
	profilePolicy := policy.ProfilePolicy{
		SchemaVersion: 1,
		Version:       "1",
		Modules:       map[string]policy.ModulePolicy{},
	}
	first, err := store.PutProfileConfiguration(
		ctx,
		ProfileConfiguration{
			CustomerID: "customer-1",
			ProfileID:  "child",
			Policy:     profilePolicy,
		},
	)
	if err != nil {
		t.Fatalf("Put customer-1: %v", err)
	}
	second, err := store.PutProfileConfiguration(
		ctx,
		ProfileConfiguration{
			CustomerID: "customer-2",
			ProfileID:  "child",
			Policy:     profilePolicy,
		},
	)
	if err != nil {
		t.Fatalf("Put customer-2: %v", err)
	}
	if first.Revision != 1 || second.Revision != 1 {
		t.Fatalf("isolated revisions = %d, %d; want 1, 1", first.Revision, second.Revision)
	}
}

func TestStoreRejectsInvalidOrMissingResourcesWithoutMutation(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	store := NewStore(db)

	if err := store.CreateProfile(ctx, "customer-1", "harry"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	_, err = store.PutProfileConfiguration(
		ctx,
		ProfileConfiguration{
			CustomerID: "customer-1",
			ProfileID:  "missing",
			Policy: policy.ProfilePolicy{
				SchemaVersion: 1,
				Version:       "1",
				Modules:       map[string]policy.ModulePolicy{},
			},
		},
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing profile error = %v, want ErrNotFound", err)
	}
	_, err = store.PutProfileConfiguration(
		ctx,
		ProfileConfiguration{
			CustomerID: "",
			ProfileID:  "harry",
		},
	)
	if err == nil {
		t.Fatal("invalid configuration returned nil error")
	}
	_, err = store.ProfileConfiguration(ctx, "customer-1", "harry")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ProfileConfiguration error = %v, want ErrNotFound", err)
	}
}

func TestStoreInvalidPolicyUpdateLeavesPreviousConfigurationIntact(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	store := NewStore(db)

	if err := store.CreateProfile(ctx, "customer-1", "harry"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	original := ProfileConfiguration{
		CustomerID: "customer-1",
		ProfileID:  "harry",
		Policy: policy.ProfilePolicy{
			SchemaVersion: 1,
			Version:       "1",
			Modules:       map[string]policy.ModulePolicy{},
		},
	}
	stored, err := store.PutProfileConfiguration(ctx, original)
	if err != nil {
		t.Fatalf("PutProfileConfiguration original: %v", err)
	}
	if stored.Revision != 1 {
		t.Fatalf("original revision = %d, want 1", stored.Revision)
	}

	invalid := original
	invalid.Policy.SchemaVersion = 0
	if _, err := store.PutProfileConfiguration(ctx, invalid); err == nil {
		t.Fatal("invalid policy update returned nil error")
	}

	persisted, err := store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration: %v", err)
	}
	if persisted.Revision != 1 {
		t.Fatalf("revision after invalid update = %d, want 1", persisted.Revision)
	}
	if !reflect.DeepEqual(persisted.Policy, original.Policy) {
		t.Fatalf("policy after invalid update = %#v, want %#v", persisted.Policy, original.Policy)
	}

	updated, err := store.PutProfileConfiguration(ctx, original)
	if err != nil {
		t.Fatalf("PutProfileConfiguration valid retry: %v", err)
	}
	if updated.Revision != 1 {
		t.Fatalf("revision after valid retry = %d, want 1", updated.Revision)
	}
}

func TestStoreRoundTripsActiveBrowserProfile(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	store := NewStore(db)

	if err := store.CreateProfile(ctx, "customer-1", "harry"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	profilePolicy := browserApprovalPolicy([]string{})
	settings, ok, err := kidControlSettings(profilePolicy)
	if err != nil {
		t.Fatalf("kidControlSettings: %v", err)
	}
	if !ok {
		t.Fatal("kidcontrol settings are missing")
	}

	expiry := time.Date(2026, time.September, 22, 13, 0, 0, 0, time.UTC)
	settings.BrowserMonitoring.ActiveProfile = &policy.ActiveBrowserProfile{
		Name:            "study",
		ExpiresAt:       &expiry,
		FallbackProfile: "default",
	}
	if err := setKidControlSettings(&profilePolicy, settings); err != nil {
		t.Fatalf("setKidControlSettings: %v", err)
	}

	stored, err := store.PutProfileConfiguration(
		ctx,
		ProfileConfiguration{
			CustomerID: "customer-1",
			ProfileID:  "harry",
			Policy:     profilePolicy,
		},
	)
	if err != nil {
		t.Fatalf("PutProfileConfiguration: %v", err)
	}
	if stored.Revision != 1 {
		t.Fatalf("stored revision = %d, want 1", stored.Revision)
	}

	persisted, err := store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration: %v", err)
	}
	persistedSettings, ok, err := kidControlSettings(persisted.Policy)
	if err != nil {
		t.Fatalf("kidControlSettings persisted: %v", err)
	}
	if !ok || persistedSettings.BrowserMonitoring.ActiveProfile == nil {
		t.Fatal("persisted active browser profile is missing")
	}

	active := persistedSettings.BrowserMonitoring.ActiveProfile
	if active.Name != "study" || active.FallbackProfile != "default" {
		t.Fatalf("active browser profile = %#v", active)
	}
	if active.ExpiresAt == nil || !active.ExpiresAt.Equal(expiry) {
		t.Fatalf("active profile expiry = %v, want %v", active.ExpiresAt, expiry)
	}
}

func TestStoreSupportsManyToManyDeviceProfileLinks(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	store := NewStore(db)

	for _, profileID := range []ProfileID{"harry", "james"} {
		if err := store.CreateProfile(ctx, "customer-1", profileID); err != nil {
			t.Fatalf("CreateProfile %s: %v", profileID, err)
		}
	}
	for _, deviceID := range []DeviceID{"laptop", "desktop"} {
		registration := DeviceRegistration{
			CustomerID: "customer-1",
			DeviceID:   deviceID,
		}
		if err := store.RegisterDevice(ctx, registration); err != nil {
			t.Fatalf("RegisterDevice %s: %v", deviceID, err)
		}
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("LinkDeviceProfile laptop/harry: %v", err)
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"laptop",
		"james",
	); err != nil {
		t.Fatalf("LinkDeviceProfile laptop/james: %v", err)
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"desktop",
		"harry",
	); err != nil {
		t.Fatalf("LinkDeviceProfile desktop/harry: %v", err)
	}

	laptopProfiles, err := store.DeviceProfiles(ctx, "customer-1", "laptop")
	if err != nil {
		t.Fatalf("DeviceProfiles laptop: %v", err)
	}
	if want := []ProfileID{"harry", "james"}; !reflect.DeepEqual(laptopProfiles, want) {
		t.Fatalf("laptop profiles = %v, want %v", laptopProfiles, want)
	}

	desktopProfiles, err := store.DeviceProfiles(ctx, "customer-1", "desktop")
	if err != nil {
		t.Fatalf("DeviceProfiles desktop: %v", err)
	}
	if want := []ProfileID{"harry"}; !reflect.DeepEqual(desktopProfiles, want) {
		t.Fatalf("desktop profiles = %v, want %v", desktopProfiles, want)
	}
}

func TestStoreRejectsMissingDeviceProfileRelationships(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	store := NewStore(db)

	missingCustomerDevice := DeviceRegistration{
		CustomerID: "missing",
		DeviceID:   "laptop",
	}
	if err := store.RegisterDevice(ctx, missingCustomerDevice); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing customer registration error = %v, want ErrNotFound", err)
	}

	if err := store.CreateProfile(ctx, "customer-1", "harry"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	registration := DeviceRegistration{
		CustomerID: "customer-1",
		DeviceID:   "laptop",
	}
	if err := store.RegisterDevice(ctx, registration); err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}

	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"missing-device",
		"harry",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing device link error = %v, want ErrNotFound", err)
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"laptop",
		"missing-profile",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing profile link error = %v, want ErrNotFound", err)
	}
	if _, err := store.DeviceProfiles(
		ctx,
		"customer-1",
		"missing-device",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing DeviceProfiles error = %v, want ErrNotFound", err)
	}
}

func TestStoreUnlinksAndValidatesDeviceProfileLinks(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	store := NewStore(db)

	if err := store.CreateProfile(ctx, "customer-1", "harry"); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	registration := DeviceRegistration{
		CustomerID: "customer-1",
		DeviceID:   "laptop",
	}
	if err := store.RegisterDevice(ctx, registration); err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}
	if err := store.LinkDeviceProfile(
		ctx,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("LinkDeviceProfile: %v", err)
	}

	linked, err := store.DeviceProfileLinked(
		ctx,
		"customer-1",
		"laptop",
		"harry",
	)
	if err != nil {
		t.Fatalf("DeviceProfileLinked: %v", err)
	}
	if !linked {
		t.Fatal("DeviceProfileLinked = false, want true")
	}

	if err := store.UnlinkDeviceProfile(
		ctx,
		"customer-1",
		"laptop",
		"harry",
	); err != nil {
		t.Fatalf("UnlinkDeviceProfile: %v", err)
	}
	linked, err = store.DeviceProfileLinked(
		ctx,
		"customer-1",
		"laptop",
		"harry",
	)
	if err != nil {
		t.Fatalf("DeviceProfileLinked after unlink: %v", err)
	}
	if linked {
		t.Fatal("DeviceProfileLinked after unlink = true, want false")
	}

	if err := store.UnlinkDeviceProfile(
		ctx,
		"customer-1",
		"laptop",
		"harry",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second UnlinkDeviceProfile error = %v, want ErrNotFound", err)
	}
	_, err = store.DeviceProfileLinked(
		ctx,
		"customer-1",
		"laptop",
		"missing",
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing profile link check error = %v, want ErrNotFound", err)
	}
}
