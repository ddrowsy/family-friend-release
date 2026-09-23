package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBrowserAccessRequestLifecycle(t *testing.T) {
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
		"https://Example.com/path?q=1",
		"study",
	)
	if err != nil {
		t.Fatalf("CreateBrowserAccessRequest: %v", err)
	}
	if request.ID == "" || request.State != BrowserAccessRequestPending {
		t.Fatalf("request = %#v", request)
	}
	if request.Site != "example.com" {
		t.Fatalf("site = %q, want example.com", request.Site)
	}

	duplicate, err := store.CreateBrowserAccessRequest(
		ctx,
		"customer-1",
		"harry",
		"laptop",
		"https://example.com/other",
		"study",
	)
	if err != nil {
		t.Fatalf("duplicate CreateBrowserAccessRequest: %v", err)
	}
	if duplicate.ID != request.ID {
		t.Fatalf("duplicate ID = %q, want %q", duplicate.ID, request.ID)
	}

	approved, err := store.ApproveBrowserAccessRequest(
		ctx,
		"customer-1",
		request.ID,
		BrowserAccessDecision{
			Action:          BrowserApprovalTemporary,
			DurationSeconds: 60,
		},
	)
	if err != nil {
		t.Fatalf("ApproveBrowserAccessRequest: %v", err)
	}
	if approved.State != BrowserAccessRequestApproved || approved.DurationSeconds != 60 {
		t.Fatalf("approved request = %#v", approved)
	}
	if approved.DecidedAt == nil || !approved.DecidedAt.Equal(now) {
		t.Fatalf("DecidedAt = %v, want %v", approved.DecidedAt, now)
	}

	configuration, err := store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration: %v", err)
	}
	assertTemporaryApproval(
		t,
		browserProfileForTest(t, configuration, "study"),
		"example.com",
		now.Add(time.Minute),
	)

	if _, err := store.RejectBrowserAccessRequest(
		ctx,
		"customer-1",
		request.ID,
	); !errors.Is(err, ErrBrowserAccessRequestState) {
		t.Fatalf("second decision error = %v, want ErrBrowserAccessRequestState", err)
	}
}

func TestBrowserAccessRequestPermanentApproval(t *testing.T) {
	ctx := context.Background()
	store, _ := newApprovalCodeTestStore(t)
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
	approved, err := store.ApproveBrowserAccessRequest(
		ctx,
		"customer-1",
		request.ID,
		BrowserAccessDecision{Action: BrowserApprovalPermanent},
	)
	if err != nil {
		t.Fatalf("ApproveBrowserAccessRequest: %v", err)
	}
	if approved.DecisionAction != BrowserApprovalPermanent || approved.DurationSeconds != 0 {
		t.Fatalf("approved decision = %#v", approved)
	}

	configuration, err := store.ProfileConfiguration(ctx, "customer-1", "harry")
	if err != nil {
		t.Fatalf("ProfileConfiguration: %v", err)
	}
	profile := browserProfileForTest(t, configuration, "study")
	if len(profile.AllowedURLs) != 1 || profile.AllowedURLs[0] != "example.com" {
		t.Fatalf("allowed URLs = %v, want [example.com]", profile.AllowedURLs)
	}
}

func TestBrowserAccessRequestIsolationAndExpiry(t *testing.T) {
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

	for _, test := range []struct {
		name       string
		customerID CustomerID
		profileID  ProfileID
		deviceID   DeviceID
	}{
		{name: "customer", customerID: "customer-2", profileID: "harry", deviceID: "laptop"},
		{name: "profile", customerID: "customer-1", profileID: "james", deviceID: "laptop"},
		{name: "device", customerID: "customer-1", profileID: "harry", deviceID: "other"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.BrowserAccessRequestByID(
				ctx,
				test.customerID,
				test.profileID,
				test.deviceID,
				request.ID,
			)
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
		})
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
		t.Fatalf("BrowserAccessRequestByID after expiry: %v", err)
	}
	if expired.State != BrowserAccessRequestExpired {
		t.Fatalf("state = %q, want expired", expired.State)
	}

	if _, err := store.ApproveBrowserAccessRequest(
		ctx,
		"customer-1",
		request.ID,
		BrowserAccessDecision{Action: BrowserApprovalPermanent},
	); !errors.Is(err, ErrBrowserAccessRequestState) {
		t.Fatalf("approval after expiry error = %v, want ErrBrowserAccessRequestState", err)
	}
}

func TestBrowserAccessDecisionValidation(t *testing.T) {
	tests := []struct {
		name     string
		decision BrowserAccessDecision
		wantErr  error
	}{
		{
			name:     "temporary requires duration",
			decision: BrowserAccessDecision{Action: BrowserApprovalTemporary},
			wantErr:  ErrInvalidApprovalDuration,
		},
		{
			name:     "permanent rejects duration",
			decision: BrowserAccessDecision{Action: BrowserApprovalPermanent, DurationSeconds: 60},
			wantErr:  ErrInvalidApprovalDuration,
		},
		{
			name:     "unknown action",
			decision: BrowserAccessDecision{Action: BrowserApprovalAction("unknown")},
			wantErr:  ErrInvalidApprovalAction,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateBrowserAccessDecision(test.decision); !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
		})
	}
}
