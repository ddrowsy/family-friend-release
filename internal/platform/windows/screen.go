package windows

import (
	"context"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

// CaptureScreen returns a placeholder screen frame.
func (p *Platform) CaptureScreen(ctx context.Context) (platform.ScreenFrame, error) {
	return platform.ScreenFrame{
		Width:  1920,
		Height: 1080,
		Source: "windows-mock",
		Data:   []byte("mock-screen-frame"),
	}, nil
}
