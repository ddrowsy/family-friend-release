package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/sessionbridge"
	"github.com/ddrowsy/family-friend-release/internal/sessionconfig"
)

func TestConfiguredSessionBridgeUsesConfiguredChildSID(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	wantClient := &sessionbridge.Client{}
	var gotSID string

	client := configuredSessionBridge(
		logger,
		func() (sessionconfig.Configuration, bool, error) {
			return sessionconfig.Configuration{
				ChildSID: "S-1-5-21-1001",
			}, true, nil
		},
		func(childSID string) (*sessionbridge.Client, error) {
			gotSID = childSID
			return wantClient, nil
		},
	)

	if client != wantClient {
		t.Fatalf("client = %p, want %p", client, wantClient)
	}
	if gotSID != "S-1-5-21-1001" {
		t.Fatalf("child SID = %q", gotSID)
	}
}

func TestConfiguredSessionBridgeWithoutConfigurationStaysUnavailable(t *testing.T) {
	factoryCalls := 0

	client := configuredSessionBridge(
		log.New(io.Discard, "", 0),
		func() (sessionconfig.Configuration, bool, error) {
			return sessionconfig.Configuration{}, false, nil
		},
		func(string) (*sessionbridge.Client, error) {
			factoryCalls++
			return &sessionbridge.Client{}, nil
		},
	)

	if client != nil {
		t.Fatalf("client = %p, want nil", client)
	}
	if factoryCalls != 0 {
		t.Fatalf("factory calls = %d, want 0", factoryCalls)
	}
}

func TestConfiguredSessionBridgeLogsConfigurationFailure(t *testing.T) {
	loadErr := errors.New("configuration unavailable")
	var output bytes.Buffer

	client := configuredSessionBridge(
		log.New(&output, "", 0),
		func() (sessionconfig.Configuration, bool, error) {
			return sessionconfig.Configuration{}, false, loadErr
		},
		func(string) (*sessionbridge.Client, error) {
			t.Fatal("client factory called")
			return nil, nil
		},
	)

	if client != nil {
		t.Fatalf("client = %p, want nil", client)
	}
	if !strings.Contains(output.String(), "loading configured child session") {
		t.Fatalf("log = %q", output.String())
	}
}

func TestConfiguredSessionBridgeIgnoresInvalidConfiguredSID(t *testing.T) {
	clientErr := errors.New("invalid SID")

	client := configuredSessionBridge(
		log.New(io.Discard, "", 0),
		func() (sessionconfig.Configuration, bool, error) {
			return sessionconfig.Configuration{
				ChildSID: "invalid",
			}, true, nil
		},
		func(string) (*sessionbridge.Client, error) {
			return nil, clientErr
		},
	)

	if client != nil {
		t.Fatalf("client = %p, want nil", client)
	}
}
