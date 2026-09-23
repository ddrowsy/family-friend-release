package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/policy"
	"github.com/ddrowsy/family-friend-release/internal/service"
)

type syncClientStub struct {
	profiles       []service.ProfileID
	configurations map[service.ProfileID]service.ProfileConfiguration
	profileErr     map[service.ProfileID]error
	fetchCount     map[service.ProfileID]int
}

func (c *syncClientStub) DeviceProfiles(
	context.Context,
	service.CustomerID,
	service.DeviceID,
) (service.DeviceProfileAssignment, error) {
	return service.DeviceProfileAssignment{ProfileIDs: c.profiles}, nil
}

func (c *syncClientStub) ProfileConfiguration(
	_ context.Context,
	_ service.CustomerID,
	profileID service.ProfileID,
) (service.ProfileConfiguration, error) {
	c.fetchCount[profileID]++
	if err := c.profileErr[profileID]; err != nil {
		return service.ProfileConfiguration{}, err
	}
	return c.configurations[profileID], nil
}

type syncApplierStub struct {
	applied  []service.ProfileID
	applyErr map[service.ProfileID]error
}

func (a *syncApplierStub) ApplyProfilePolicy(
	_ context.Context,
	profileID service.ProfileID,
	_ policy.ProfilePolicy,
) error {
	if err := a.applyErr[profileID]; err != nil {
		return err
	}
	a.applied = append(a.applied, profileID)
	return nil
}

func TestProfilePolicySynchronizerInitialUnchangedAndNewerRevision(t *testing.T) {
	client, applier, synchronizer := newSynchronizerTestFixture()

	if err := synchronizer.Sync(t.Context()); err != nil {
		t.Fatalf("initial Sync() error = %v", err)
	}
	if len(applier.applied) != 1 {
		t.Fatalf("initial apply count = %d, want 1", len(applier.applied))
	}

	if err := synchronizer.Sync(t.Context()); err != nil {
		t.Fatalf("unchanged Sync() error = %v", err)
	}
	if len(applier.applied) != 1 {
		t.Fatalf("unchanged apply count = %d, want 1", len(applier.applied))
	}

	configuration := client.configurations["harry"]
	configuration.Revision = 2
	client.configurations["harry"] = configuration
	if err := synchronizer.Sync(t.Context()); err != nil {
		t.Fatalf("newer Sync() error = %v", err)
	}
	if len(applier.applied) != 2 {
		t.Fatalf("newer apply count = %d, want 2", len(applier.applied))
	}
}

func TestProfilePolicySynchronizerMultipleProfiles(t *testing.T) {
	client, applier, synchronizer := newSynchronizerTestFixture()
	client.profiles = []service.ProfileID{"harry", "james"}
	client.configurations["james"] = testConfiguration("james", 4)

	if err := synchronizer.Sync(t.Context()); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(applier.applied) != 2 {
		t.Fatalf("apply count = %d, want 2", len(applier.applied))
	}
}

func TestProfilePolicySynchronizerContinuesAfterProfileFailure(t *testing.T) {
	t.Run("fetch", func(t *testing.T) {
		client, applier, synchronizer := newSynchronizerTestFixture()
		client.profiles = []service.ProfileID{"harry", "james"}
		client.configurations["james"] = testConfiguration("james", 1)
		client.profileErr["harry"] = errors.New("fetch failed")

		if err := synchronizer.Sync(t.Context()); err == nil {
			t.Fatal("Sync() error = nil")
		}
		if len(applier.applied) != 1 || applier.applied[0] != "james" {
			t.Fatalf("applied profiles = %v, want [james]", applier.applied)
		}
	})

	t.Run("apply", func(t *testing.T) {
		client, applier, synchronizer := newSynchronizerTestFixture()
		client.profiles = []service.ProfileID{"harry", "james"}
		client.configurations["james"] = testConfiguration("james", 1)
		applier.applyErr["harry"] = errors.New("apply failed")

		if err := synchronizer.Sync(t.Context()); err == nil {
			t.Fatal("Sync() error = nil")
		}
		if len(applier.applied) != 1 || applier.applied[0] != "james" {
			t.Fatalf("applied profiles = %v, want [james]", applier.applied)
		}
	})
}

func TestProfilePolicySynchronizerFailureDoesNotAdvanceRevision(t *testing.T) {
	t.Run("fetch", func(t *testing.T) {
		client, applier, synchronizer := newSynchronizerTestFixture()
		client.profileErr["harry"] = errors.New("fetch failed")
		if err := synchronizer.Sync(t.Context()); err == nil {
			t.Fatal("Sync() error = nil")
		}
		delete(client.profileErr, "harry")
		if err := synchronizer.Sync(t.Context()); err != nil {
			t.Fatalf("retry Sync() error = %v", err)
		}
		if len(applier.applied) != 1 {
			t.Fatalf("apply count = %d, want 1", len(applier.applied))
		}
	})

	t.Run("apply", func(t *testing.T) {
		_, applier, synchronizer := newSynchronizerTestFixture()
		applier.applyErr["harry"] = errors.New("apply failed")
		if err := synchronizer.Sync(t.Context()); err == nil {
			t.Fatal("Sync() error = nil")
		}
		delete(applier.applyErr, "harry")
		if err := synchronizer.Sync(t.Context()); err != nil {
			t.Fatalf("retry Sync() error = %v", err)
		}
		if len(applier.applied) != 1 {
			t.Fatalf("apply count = %d, want 1", len(applier.applied))
		}
	})
}

func newSynchronizerTestFixture() (
	*syncClientStub,
	*syncApplierStub,
	*ProfilePolicySynchronizer,
) {
	client := &syncClientStub{
		profiles: []service.ProfileID{"harry"},
		configurations: map[service.ProfileID]service.ProfileConfiguration{
			"harry": testConfiguration("harry", 1),
		},
		profileErr: map[service.ProfileID]error{},
		fetchCount: map[service.ProfileID]int{},
	}
	applier := &syncApplierStub{
		applied:  []service.ProfileID{},
		applyErr: map[service.ProfileID]error{},
	}
	return client, applier, NewProfilePolicySynchronizer(
		client,
		applier,
		"customer-1",
		"device-1",
	)
}

func testConfiguration(
	profileID service.ProfileID,
	revision int64,
) service.ProfileConfiguration {
	return service.ProfileConfiguration{
		CustomerID: "customer-1",
		ProfileID:  profileID,
		Revision:   revision,
		Policy: policy.ProfilePolicy{
			SchemaVersion: 1,
			Version:       "v1",
			Modules:       map[string]policy.ModulePolicy{},
		},
	}
}
