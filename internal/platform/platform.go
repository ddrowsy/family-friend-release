package platform

import "context"

// ProcessInfo describes a running process.
type ProcessInfo struct {
	PID  int
	Name string
	Path string
}

// WindowInfo describes a window visible to the operating system.
type WindowInfo struct {
	ID          string
	Title       string
	ProcessID   int
	ProcessName string
}

// ScreenFrame contains a captured screen frame or placeholder data.
type ScreenFrame struct {
	Width  int
	Height int
	Source string
	Data   []byte
}

// ProcessProvider lists running processes.
type ProcessProvider interface {
	ListProcesses(ctx context.Context) ([]ProcessInfo, error)
}

// WindowProvider reports active window information.
type WindowProvider interface {
	ActiveWindow(ctx context.Context) (WindowInfo, error)
}

// ScreenProvider captures a screen frame.
type ScreenProvider interface {
	CaptureScreen(ctx context.Context) (ScreenFrame, error)
}

// Controller controls processes and windows.
type Controller interface {
	CloseProcess(ctx context.Context, pid int) error
	CloseWindow(ctx context.Context, windowID string) error
}

// Platform is the full platform capability set used by modules.
type Platform interface {
	ProcessProvider
	WindowProvider
	ScreenProvider
	Controller
}
