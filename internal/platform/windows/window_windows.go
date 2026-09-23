//go:build windows

package windows

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

const (
	wmClose = 0x0010
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	procGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW         = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procPostMessageW           = user32.NewProc("PostMessageW")
)

// ActiveWindow returns the foreground Windows window.
func (p *Platform) ActiveWindow(ctx context.Context) (platform.WindowInfo, error) {
	if err := ctx.Err(); err != nil {
		return platform.WindowInfo{}, err
	}

	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return platform.WindowInfo{}, nil
	}

	var pid uint32
	procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))

	title := windowTitle(hwnd)
	processName := ""
	if path, err := queryProcessPath(int(pid)); err == nil {
		processName = filepath.Base(path)
	}
	if processName == "" {
		processName = processNameFromSnapshot(ctx, int(pid))
	}

	return platform.WindowInfo{
		ID:          strconv.FormatUint(uint64(hwnd), 10),
		Title:       title,
		ProcessID:   int(pid),
		ProcessName: processName,
	}, nil
}

// CloseWindow asks a window to close by sending WM_CLOSE.
func (p *Platform) CloseWindow(ctx context.Context, windowID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	hwnd, err := strconv.ParseUint(windowID, 10, 64)
	if err != nil || hwnd == 0 {
		return fmt.Errorf("invalid window id %q", windowID)
	}

	p.logger.Printf("windows platform close window id=%s", windowID)
	r1, _, callErr := procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
	if r1 == 0 {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("send close message window id=%s: %w", windowID, callErr)
		}
		return fmt.Errorf("send close message window id=%s failed", windowID)
	}
	return nil
}

func windowTitle(hwnd uintptr) string {
	buffer := make([]uint16, 512)
	r1, _, _ := procGetWindowTextW.Call(
		hwnd,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if r1 == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer[:r1])
}

func processNameFromSnapshot(ctx context.Context, pid int) string {
	processes, err := (&Platform{}).ListProcesses(ctx)
	if err != nil {
		return ""
	}
	for _, process := range processes {
		if process.PID == pid {
			return process.Name
		}
	}
	return ""
}
