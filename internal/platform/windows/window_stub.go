//go:build !windows

package windows

import (
	"context"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

// ActiveWindow returns a sample active window on non-Windows development machines.
func (p *Platform) ActiveWindow(ctx context.Context) (platform.WindowInfo, error) {
	return platform.WindowInfo{
		ID:          "mock-window-1",
		Title:       "Example Search - Microsoft Edge",
		ProcessID:   200,
		ProcessName: "msedge.exe",
	}, nil
}

// CloseWindow logs the window it would close on non-Windows development machines.
func (p *Platform) CloseWindow(ctx context.Context, windowID string) error {
	p.logger.Printf("windows platform dry-run close window id=%s", windowID)
	return nil
}
