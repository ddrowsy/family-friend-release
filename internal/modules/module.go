package modules

import (
	"context"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

// Module is the common lifecycle interface for runnable modules.
type Module interface {
	Name() string
	Start(ctx context.Context, modulePolicy policy.ModulePolicy) error
	Stop(ctx context.Context) error
}
