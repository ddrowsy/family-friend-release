package windows

import (
	"log"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

// Platform provides Windows platform behavior.
type Platform struct {
	logger *log.Logger
}

var _ platform.Platform = (*Platform)(nil)

// NewPlatform creates a Windows platform implementation.
func NewPlatform(logger *log.Logger) *Platform {
	if logger == nil {
		logger = log.Default()
	}

	return &Platform{logger: logger}
}

// NewMockPlatform keeps older callers working.
func NewMockPlatform(logger *log.Logger) *Platform {
	return NewPlatform(logger)
}
