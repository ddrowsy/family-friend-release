//go:build !windows

package sessionworker

import (
	"context"
	"fmt"
	"log"
)

// Run reports that the session worker requires Windows.
func Run(context.Context, *log.Logger) error {
	return fmt.Errorf("session worker requires Windows")
}
