//go:build windows

package sessionbridge

import (
	"context"
	"fmt"
	"net"

	winio "github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

const (
	pipePath = `\\.\pipe\family-friend-session`

	pipeClientAccess = windows.FILE_GENERIC_READ |
		(windows.FILE_GENERIC_WRITE &^ windows.FILE_APPEND_DATA)
)

type handleConn interface {
	net.Conn
	Fd() uintptr
}

func NewPipeClient(serverSID string) (*Client, error) {
	expectedServer, err := windows.StringToSid(serverSID)
	if err != nil {
		return nil, fmt.Errorf("parsing session bridge server SID %q: %w", serverSID, err)
	}

	return newClient(
		func(ctx context.Context) (net.Conn, error) {
			conn, err := winio.DialPipeAccess(
				ctx,
				pipePath,
				uint32(pipeClientAccess),
			)
			if err != nil {
				return nil, err
			}
			if err := verifyPipeServerSID(conn, expectedServer); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return conn, nil
		},
		defaultCallTimeout,
	), nil
}

func ListenPipe(
	desktop Desktop,
	serverSID string,
	clientSID string,
) (*Server, error) {
	securityDescriptor, err := pipeSecurityDescriptor(serverSID, clientSID)
	if err != nil {
		return nil, err
	}

	listener, err := winio.ListenPipe(
		pipePath,
		&winio.PipeConfig{
			SecurityDescriptor: securityDescriptor,
			InputBufferSize:    int32(maxMessageSize),
			OutputBufferSize:   int32(maxMessageSize),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("listening on session bridge pipe: %w", err)
	}

	return newServer(
		listener,
		NewHandler(desktop),
	), nil
}

func pipeSecurityDescriptor(serverSID string, clientSID string) (string, error) {
	server, err := windows.StringToSid(serverSID)
	if err != nil {
		return "", fmt.Errorf("parsing session bridge server SID %q: %w", serverSID, err)
	}
	client, err := windows.StringToSid(clientSID)
	if err != nil {
		return "", fmt.Errorf("parsing session bridge client SID %q: %w", clientSID, err)
	}

	return fmt.Sprintf(
		"D:P(A;;GA;;;%s)(A;;0x%08x;;;%s)",
		server.String(),
		uint32(pipeClientAccess),
		client.String(),
	), nil
}

func verifyPipeServerSID(conn net.Conn, expectedSID *windows.SID) error {
	pipe, ok := conn.(handleConn)
	if !ok {
		return fmt.Errorf("%w: pipe handle unavailable", ErrUntrustedPeer)
	}

	var processID uint32
	if err := windows.GetNamedPipeServerProcessId(
		windows.Handle(pipe.Fd()),
		&processID,
	); err != nil {
		return fmt.Errorf("%w: resolving pipe server process: %v", ErrUntrustedPeer, err)
	}

	process, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		processID,
	)
	if err != nil {
		return fmt.Errorf("%w: opening pipe server process: %v", ErrUntrustedPeer, err)
	}
	defer windows.CloseHandle(process)

	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return fmt.Errorf("%w: opening pipe server token: %v", ErrUntrustedPeer, err)
	}
	defer token.Close()

	user, err := token.GetTokenUser()
	if err != nil {
		return fmt.Errorf("%w: reading pipe server identity: %v", ErrUntrustedPeer, err)
	}
	if !user.User.Sid.Equals(expectedSID) {
		return fmt.Errorf(
			"%w: server SID %s does not match %s",
			ErrUntrustedPeer,
			user.User.Sid.String(),
			expectedSID.String(),
		)
	}
	return nil
}
