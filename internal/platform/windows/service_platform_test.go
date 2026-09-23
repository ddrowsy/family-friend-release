package windows

import (
	"context"
	"errors"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/platform"
	"github.com/ddrowsy/family-friend-release/internal/sessionbridge"
)

func TestServicePlatformRoutesInteractiveWindowOperationsThroughBridge(t *testing.T) {
	local := &platformStub{}
	bridge := &sessionBridgeStub{
		window: platform.WindowInfo{ID: "42", Title: "Homework"},
	}
	platformLayer := newServicePlatform(local, bridge)

	window, err := platformLayer.ActiveWindow(context.Background())
	if err != nil {
		t.Fatalf("ActiveWindow() error = %v", err)
	}
	if window != bridge.window {
		t.Fatalf("window = %#v, want %#v", window, bridge.window)
	}
	if local.activeWindowCalls != 0 {
		t.Fatalf("local ActiveWindow() calls = %d, want 0", local.activeWindowCalls)
	}

	if err := platformLayer.CloseWindow(context.Background(), "73"); err != nil {
		t.Fatalf("CloseWindow() error = %v", err)
	}
	if bridge.closedWindow != "73" {
		t.Fatalf("bridge closed window = %q, want 73", bridge.closedWindow)
	}
	if local.closedWindow != "" {
		t.Fatalf("local closed window = %q, want empty", local.closedWindow)
	}
}

func TestServicePlatformKeepsServiceSafeOperationsLocal(t *testing.T) {
	local := &platformStub{
		processes: []platform.ProcessInfo{{PID: 7, Name: "browser.exe"}},
		screen:    platform.ScreenFrame{Width: 1, Height: 1, Source: "placeholder"},
	}
	platformLayer := newServicePlatform(local, &sessionBridgeStub{})

	processes, err := platformLayer.ListProcesses(context.Background())
	if err != nil {
		t.Fatalf("ListProcesses() error = %v", err)
	}
	if len(processes) != 1 || processes[0].PID != 7 {
		t.Fatalf("processes = %#v, want PID 7", processes)
	}

	if err := platformLayer.CloseProcess(context.Background(), 7); err != nil {
		t.Fatalf("CloseProcess() error = %v", err)
	}
	if local.closedProcess != 7 {
		t.Fatalf("closed process = %d, want 7", local.closedProcess)
	}

	screen, err := platformLayer.CaptureScreen(context.Background())
	if err != nil {
		t.Fatalf("CaptureScreen() error = %v", err)
	}
	if screen.Source != "placeholder" {
		t.Fatalf("screen source = %q, want placeholder", screen.Source)
	}
}

func TestServicePlatformPropagatesBridgeErrors(t *testing.T) {
	bridgeErr := errors.New("bridge unavailable")
	platformLayer := newServicePlatform(
		&platformStub{},
		&sessionBridgeStub{err: bridgeErr},
	)

	_, err := platformLayer.ActiveWindow(context.Background())
	if !errors.Is(err, bridgeErr) {
		t.Fatalf("ActiveWindow() error = %v, want %v", err, bridgeErr)
	}
	if err := platformLayer.CloseWindow(context.Background(), "42"); !errors.Is(err, bridgeErr) {
		t.Fatalf("CloseWindow() error = %v, want %v", err, bridgeErr)
	}
}

func TestServicePlatformWithoutBridgeReportsDesktopUnavailable(t *testing.T) {
	platformLayer := NewServicePlatform(&platformStub{}, nil)

	_, err := platformLayer.ActiveWindow(context.Background())
	if !errors.Is(err, sessionbridge.ErrDesktopUnavailable) {
		t.Fatalf(
			"ActiveWindow() error = %v, want %v",
			err,
			sessionbridge.ErrDesktopUnavailable,
		)
	}
	if err := platformLayer.CloseWindow(context.Background(), "42"); !errors.Is(
		err,
		sessionbridge.ErrDesktopUnavailable,
	) {
		t.Fatalf(
			"CloseWindow() error = %v, want %v",
			err,
			sessionbridge.ErrDesktopUnavailable,
		)
	}
}

type sessionBridgeStub struct {
	window       platform.WindowInfo
	closedWindow string
	err          error
}

func (s *sessionBridgeStub) GetActiveWindow(context.Context) (platform.WindowInfo, error) {
	if s.err != nil {
		return platform.WindowInfo{}, s.err
	}
	return s.window, nil
}

func (s *sessionBridgeStub) CloseWindow(_ context.Context, windowID string) error {
	if s.err != nil {
		return s.err
	}
	s.closedWindow = windowID
	return nil
}

type platformStub struct {
	processes         []platform.ProcessInfo
	screen            platform.ScreenFrame
	activeWindowCalls int
	closedProcess     int
	closedWindow      string
}

func (s *platformStub) ListProcesses(context.Context) ([]platform.ProcessInfo, error) {
	if s.processes == nil {
		return []platform.ProcessInfo{}, nil
	}
	return s.processes, nil
}

func (s *platformStub) ActiveWindow(context.Context) (platform.WindowInfo, error) {
	s.activeWindowCalls++
	return platform.WindowInfo{}, nil
}

func (s *platformStub) CaptureScreen(context.Context) (platform.ScreenFrame, error) {
	return s.screen, nil
}

func (s *platformStub) CloseProcess(_ context.Context, pid int) error {
	s.closedProcess = pid
	return nil
}

func (s *platformStub) CloseWindow(_ context.Context, windowID string) error {
	s.closedWindow = windowID
	return nil
}
