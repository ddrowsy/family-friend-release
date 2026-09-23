//go:build !windows

package windows

import (
	"context"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

// ListProcesses returns sample processes on non-Windows development machines.
func (p *Platform) ListProcesses(ctx context.Context) ([]platform.ProcessInfo, error) {
	return []platform.ProcessInfo{
		{PID: 100, Name: "explorer.exe", Path: `C:\Windows\explorer.exe`},
		{PID: 200, Name: "msedge.exe", Path: `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`},
		{PID: 300, Name: "notepad.exe", Path: `C:\Windows\System32\notepad.exe`},
	}, nil
}
