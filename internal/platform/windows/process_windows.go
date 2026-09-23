//go:build windows

package windows

import (
	"context"
	"errors"
	"syscall"
	"unsafe"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

const (
	processQueryLimitedInformation = 0x1000
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procQueryFullProcessImageW  = kernel32.NewProc("QueryFullProcessImageNameW")
	errQueryProcessPathNotFound = errors.New("process path not available")
)

// ListProcesses returns running Windows processes using the Tool Help snapshot API.
func (p *Platform) ListProcesses(ctx context.Context) ([]platform.ProcessInfo, error) {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(snapshot)

	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	if err := syscall.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}

	var processes []platform.ProcessInfo
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		pid := int(entry.ProcessID)
		process := platform.ProcessInfo{
			PID:  pid,
			Name: syscall.UTF16ToString(entry.ExeFile[:]),
		}
		if path, err := queryProcessPath(pid); err == nil {
			process.Path = path
		}
		processes = append(processes, process)

		err := syscall.Process32Next(snapshot, &entry)
		if err == syscall.ERROR_NO_MORE_FILES {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return processes, nil
}

func queryProcessPath(pid int) (string, error) {
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return "", errQueryProcessPathNotFound
	}
	defer syscall.CloseHandle(handle)

	buffer := make([]uint16, syscall.MAX_PATH)
	size := uint32(len(buffer))
	r1, _, err := procQueryFullProcessImageW.Call(
		uintptr(handle),
		0,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return "", err
		}
		return "", errQueryProcessPathNotFound
	}

	return syscall.UTF16ToString(buffer[:size]), nil
}
