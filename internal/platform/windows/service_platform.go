package windows

import (
	"context"

	"github.com/ddrowsy/family-friend-release/internal/platform"
	"github.com/ddrowsy/family-friend-release/internal/sessionbridge"
)

type sessionBridge interface {
	GetActiveWindow(ctx context.Context) (platform.WindowInfo, error)
	CloseWindow(ctx context.Context, windowID string) error
}

type servicePlatform struct {
	local  platform.Platform
	bridge sessionBridge
}

var _ platform.Platform = (*servicePlatform)(nil)

// NewServicePlatform routes interactive window operations through the session bridge.
func NewServicePlatform(local platform.Platform, bridge *sessionbridge.Client) platform.Platform {
	return newServicePlatform(local, bridge)
}

func newServicePlatform(local platform.Platform, bridge sessionBridge) *servicePlatform {
	return &servicePlatform{
		local:  local,
		bridge: bridge,
	}
}

func (p *servicePlatform) ListProcesses(ctx context.Context) ([]platform.ProcessInfo, error) {
	return p.local.ListProcesses(ctx)
}

func (p *servicePlatform) ActiveWindow(ctx context.Context) (platform.WindowInfo, error) {
	if p.bridge == nil {
		return platform.WindowInfo{}, sessionbridge.ErrDesktopUnavailable
	}
	return p.bridge.GetActiveWindow(ctx)
}

func (p *servicePlatform) CaptureScreen(ctx context.Context) (platform.ScreenFrame, error) {
	return p.local.CaptureScreen(ctx)
}

func (p *servicePlatform) CloseProcess(ctx context.Context, pid int) error {
	return p.local.CloseProcess(ctx, pid)
}

func (p *servicePlatform) CloseWindow(ctx context.Context, windowID string) error {
	if p.bridge == nil {
		return sessionbridge.ErrDesktopUnavailable
	}
	return p.bridge.CloseWindow(ctx, windowID)
}
