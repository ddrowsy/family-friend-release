package service

import (
	"context"
	"testing"
	"time"
)

func TestBrowserAccessRequestExpiryPersistsResultRetention(t *testing.T) {
	ctx := context.Background()
	store, _ := newApprovalCodeTestStore(t)
	now := time.Date(2026, time.September, 23, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	if err := createBrowserApprovalProfile(ctx, store, "customer-1", "harry"); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := registerAndLinkBrowserApprovalDevice(ctx, store, "customer-1", "laptop", "harry"); err != nil {
		t.Fatalf("link device: %v", err)
	}

	request, err := store.CreateBrowserAccessRequest(
		ctx,
		"customer-1",
		"harry",
		"laptop",
		"example.com",
		"study",
	)
	if err != nil {
		t.Fatalf("CreateBrowserAccessRequest: %v", err)
	}

	now = request.ExpiresAt.Add(time.Second)
	expired, err := store.BrowserAccessRequestByID(
		ctx,
		"customer-1",
		"harry",
		"laptop",
		request.ID,
	)
	if err != nil {
		t.Fatalf("BrowserAccessRequestByID: %v", err)
	}
	if expired.State != BrowserAccessRequestExpired {
		t.Fatalf("state = %q, want %q", expired.State, BrowserAccessRequestExpired)
	}

	wantExpiresAt := now.Add(browserAccessRequestResultLifetime)
	if !expired.ExpiresAt.Equal(wantExpiresAt) {
		t.Fatalf("ExpiresAt = %v, want %v", expired.ExpiresAt, wantExpiresAt)
	}

	var storedExpiresAt string
	if err := store.db.QueryRowContext(
		ctx,
		"SELECT expires_at FROM browser_access_requests WHERE id = ?",
		request.ID,
	).Scan(&storedExpiresAt); err != nil {
		t.Fatalf("read stored expiry: %v", err)
	}

	persisted, err := parseBrowserAccessRequestTime(storedExpiresAt)
	if err != nil {
		t.Fatalf("parse stored expiry: %v", err)
	}
	if !persisted.Equal(wantExpiresAt) {
		t.Fatalf("stored expires_at = %v, want %v", persisted, wantExpiresAt)
	}
}
