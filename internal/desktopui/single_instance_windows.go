//go:build windows

package desktopui

import (
	"errors"
	"fmt"
	"io"

	"golang.org/x/sys/windows"
)

const desktopInstanceMutexName = `Local\FamilyFriendDesktopUI`

type desktopInstanceLock struct {
	handle windows.Handle
}

// AcquireDesktopInstance prevents duplicate tray UI processes in the same Windows session.
func AcquireDesktopInstance() (io.Closer, bool, error) {
	mutexName, err := windows.UTF16PtrFromString(desktopInstanceMutexName)
	if err != nil {
		return nil, false, fmt.Errorf("encoding desktop instance mutex name: %w", err)
	}

	handle, err := windows.CreateMutex(nil, false, mutexName)
	if err != nil {
		return nil, false, fmt.Errorf("creating desktop instance mutex: %w", err)
	}
	if errors.Is(windows.GetLastError(), windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(handle)
		return nil, false, nil
	}

	return &desktopInstanceLock{handle: handle}, true, nil
}

func (l *desktopInstanceLock) Close() error {
	if err := windows.CloseHandle(l.handle); err != nil {
		return fmt.Errorf("closing desktop instance mutex: %w", err)
	}
	return nil
}
