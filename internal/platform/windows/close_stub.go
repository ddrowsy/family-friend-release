//go:build !windows

package windows

import "context"

// CloseProcess logs the process it would close on non-Windows development machines.
func (p *Platform) CloseProcess(ctx context.Context, pid int) error {
	p.logger.Printf("windows platform dry-run close process pid=%d", pid)
	return nil
}
