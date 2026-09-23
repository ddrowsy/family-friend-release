//go:build windows

package windows

import (
	"context"
	"fmt"
	"syscall"
)

const (
	processTerminate = 0x0001
)

// CloseProcess force-closes a Windows process by PID.
func (p *Platform) CloseProcess(ctx context.Context, pid int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if pid <= 0 {
		return fmt.Errorf("invalid process pid %d", pid)
	}

	handle, err := syscall.OpenProcess(processTerminate, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open process pid=%d: %w", pid, err)
	}
	defer syscall.CloseHandle(handle)

	p.logger.Printf("windows platform force close process pid=%d", pid)
	if err := syscall.TerminateProcess(handle, 1); err != nil {
		return fmt.Errorf("terminate process pid=%d: %w", pid, err)
	}
	return nil
}
