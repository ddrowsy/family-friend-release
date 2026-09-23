//go:build windows

package sessionworker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	platformwindows "github.com/ddrowsy/family-friend-release/internal/platform/windows"
	"github.com/ddrowsy/family-friend-release/internal/sessionbridge"
	"golang.org/x/sys/windows"
)

const (
	instanceMutexName = `Local\FamilyFriendSessionWorker`
	localSystemSID    = "S-1-5-18"
)

// Run serves interactive desktop operations for the current Windows user session.
func Run(ctx context.Context, logger *log.Logger) error {
	return run(
		ctx,
		acquireInstance,
		func() (server, error) {
			userSID, err := currentUserSID()
			if err != nil {
				return nil, err
			}

			desktop := platformwindows.NewPlatform(logger)
			bridgeServer, err := sessionbridge.ListenPipe(
				desktop,
				userSID,
				localSystemSID,
			)
			if err != nil {
				return nil, err
			}
			return bridgeServer, nil
		},
	)
}

func currentUserSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("reading current Windows user: %w", err)
	}
	return user.User.Sid.String(), nil
}

func acquireInstance() (io.Closer, bool, error) {
	name, err := windows.UTF16PtrFromString(instanceMutexName)
	if err != nil {
		return nil, false, fmt.Errorf("encoding session worker mutex name: %w", err)
	}

	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("creating session worker mutex: %w", err)
	}
	return instanceHandle{handle: handle}, true, nil
}

type instanceHandle struct {
	handle windows.Handle
}

func (h instanceHandle) Close() error {
	return windows.CloseHandle(h.handle)
}
