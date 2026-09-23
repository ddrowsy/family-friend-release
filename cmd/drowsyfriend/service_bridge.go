package main

import (
	"log"

	"github.com/ddrowsy/family-friend-release/internal/sessionbridge"
	"github.com/ddrowsy/family-friend-release/internal/sessionconfig"
)

type sessionConfigLoader func() (sessionconfig.Configuration, bool, error)
type sessionBridgeFactory func(childSID string) (*sessionbridge.Client, error)

func configuredSessionBridge(
	logger *log.Logger,
	load sessionConfigLoader,
	newClient sessionBridgeFactory,
) *sessionbridge.Client {
	configuration, configured, err := load()
	if err != nil {
		logSessionBridgeUnavailable(logger, "loading configured child session", err)
		return nil
	}
	if !configured {
		return nil
	}

	client, err := newClient(configuration.ChildSID)
	if err != nil {
		logSessionBridgeUnavailable(logger, "creating child session bridge", err)
		return nil
	}
	return client
}

func logSessionBridgeUnavailable(logger *log.Logger, message string, err error) {
	if logger == nil {
		return
	}
	logger.Printf("%s: %v", message, err)
}
