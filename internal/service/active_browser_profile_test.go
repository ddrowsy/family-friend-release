package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSetActiveBrowserProfileActivateClearAndRevision(t *testing.T) {
	store, _, _ := newHTTPTestHandler(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	if err := createHTTPApprovalProfile(ctx, store, "customer-1", "harry"); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	configuration, err := store.SetActiveBrowserProfile(
		ctx,
		"customer-1",
		"harry",
		ActiveBrowserProfileRequest{
			Name:            "study",
			DurationSeconds: 900,
		},
	)
	if err != nil {
		t.Fatalf("SetActiveBrowserProfile activate: %v", err)
	}
	if configuration.Revision != 2 {
		t.Fatalf("activate revision = %d, want 2", configuration.Revision)
	}
	settings, ok, err := kidControlSettings(configuration.Policy)
	if err != nil || !ok {
		t.Fatalf("kidControlSettings() = %v, %v", ok, err)
	}
	active := settings.BrowserMonitoring.ActiveProfile
	if active == nil || active.Name != "study" {
		t.Fatalf("active profile = %#v", active)
	}
	if active.FallbackProfile != settings.BrowserMonitoring.DefaultProfile {
		t.Fatalf("fallback = %q, want %q", active.FallbackProfile, settings.BrowserMonitoring.DefaultProfile)
	}
	wantExpiry := now.Add(15 * time.Minute)
	if active.ExpiresAt == nil || !active.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("expiry = %v, want %v", active.ExpiresAt, wantExpiry)
	}

	configuration, err = store.SetActiveBrowserProfile(
		ctx,
		"customer-1",
		"harry",
		ActiveBrowserProfileRequest{},
	)
	if err != nil {
		t.Fatalf("SetActiveBrowserProfile clear: %v", err)
	}
	if configuration.Revision != 3 {
		t.Fatalf("clear revision = %d, want 3", configuration.Revision)
	}
	settings, _, _ = kidControlSettings(configuration.Policy)
	if settings.BrowserMonitoring.ActiveProfile != nil {
		t.Fatalf("active profile after clear = %#v", settings.BrowserMonitoring.ActiveProfile)
	}

	configuration, err = store.SetActiveBrowserProfile(
		ctx,
		"customer-1",
		"harry",
		ActiveBrowserProfileRequest{},
	)
	if err != nil {
		t.Fatalf("SetActiveBrowserProfile idempotent clear: %v", err)
	}
	if configuration.Revision != 3 {
		t.Fatalf("idempotent clear revision = %d, want 3", configuration.Revision)
	}
}

func TestSetActiveBrowserProfileRejectsInvalidProfilesAndExpiry(t *testing.T) {
	store, _, _ := newHTTPTestHandler(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	if err := createHTTPApprovalProfile(ctx, store, "customer-1", "harry"); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	tests := []ActiveBrowserProfileRequest{
		{Name: "missing"},
		{Name: "study", FallbackProfile: "missing"},
		{Name: "study", ExpiresAt: &now},
		{Name: "study", DurationSeconds: -1},
		{Name: "study", DurationSeconds: 60, ExpiresAt: ptrTime(now.Add(time.Hour))},
	}
	for _, request := range tests {
		_, err := store.SetActiveBrowserProfile(ctx, "customer-1", "harry", request)
		if err == nil {
			t.Fatalf("request %#v unexpectedly succeeded", request)
		}
	}

	_, err := store.SetActiveBrowserProfile(
		ctx,
		"customer-1",
		"harry",
		ActiveBrowserProfileRequest{Name: "missing"},
	)
	if !errors.Is(err, ErrBrowserProfileNotFound) {
		t.Fatalf("missing profile error = %v", err)
	}
}

func TestHTTPSetActiveBrowserProfile(t *testing.T) {
	store, handler, _ := newHTTPTestHandler(t)
	ctx := context.Background()
	if err := createHTTPApprovalProfile(ctx, store, "customer-1", "harry"); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	path, err := ProfileActiveBrowserPath("customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileActiveBrowserPath: %v", err)
	}
	response := serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: "PUT",
			path:   path,
			body: ActiveBrowserProfileRequest{
				Name:            "study",
				FallbackProfile: "default",
			},
		},
	)
	assertStatus(t, response, 200)

	response = serveHTTP(
		t,
		handler,
		testHTTPRequest{
			method: "PUT",
			path:   path,
			body:   ActiveBrowserProfileRequest{Name: "missing"},
		},
	)
	assertStatus(t, response, 404)
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
