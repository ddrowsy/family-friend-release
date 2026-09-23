package kidcontrol

import (
	"context"
	"fmt"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

// Executor runs actions selected by the decision evaluator.
type Executor struct {
	controller platform.Controller
}

// NewExecutor creates an action executor.
func NewExecutor(controller platform.Controller) Executor {
	return Executor{controller: controller}
}

// Execute runs the selected decision action.
func (e Executor) Execute(ctx context.Context, result Decision) error {
	if !result.Matched {
		return nil
	}

	switch result.Action {
	case ActionCloseProcess:
		if e.controller == nil {
			return fmt.Errorf("platform controller is required")
		}
		return e.controller.CloseProcess(ctx, result.ProcessID)
	case ActionCloseWindow:
		if e.controller == nil {
			return fmt.Errorf("platform controller is required")
		}
		return e.controller.CloseWindow(ctx, result.WindowID)
	default:
		return fmt.Errorf("unsupported action %q", result.Action)
	}
}
