package sessionbridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

func TestTransportRoundTrip(t *testing.T) {
	desktop := &desktopStub{
		window: platform.WindowInfo{ID: "42", Title: "Homework"},
	}
	server := newServer(nil, NewHandler(desktop))
	client := newTestClient(server, time.Second)

	window, err := client.GetActiveWindow(context.Background())
	if err != nil {
		t.Fatalf("GetActiveWindow() error = %v", err)
	}
	if window != desktop.window {
		t.Fatalf("window = %#v, want %#v", window, desktop.window)
	}

	if err := client.CloseWindow(context.Background(), "73"); err != nil {
		t.Fatalf("CloseWindow() error = %v", err)
	}
	if desktop.closed != "73" {
		t.Fatalf("closed window = %q, want 73", desktop.closed)
	}
}

func TestClientReconnectsAfterUnavailableEndpoint(t *testing.T) {
	server := newServer(
		nil,
		NewHandler(&desktopStub{window: platform.WindowInfo{ID: "9"}}),
	)
	available := false
	client := newClient(
		func(ctx context.Context) (net.Conn, error) {
			if !available {
				return nil, errors.New("pipe unavailable")
			}
			return testConnection(ctx, server), nil
		},
		time.Second,
	)

	if _, err := client.GetActiveWindow(context.Background()); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("unavailable error = %v, want %v", err, ErrDesktopUnavailable)
	}

	available = true
	window, err := client.GetActiveWindow(context.Background())
	if err != nil {
		t.Fatalf("GetActiveWindow() after restart error = %v", err)
	}
	if window.ID != "9" {
		t.Fatalf("window ID = %q, want 9", window.ID)
	}
}

func TestClientCallIsBounded(t *testing.T) {
	client := newClient(
		func(ctx context.Context) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		20*time.Millisecond,
	)

	_, err := client.GetActiveWindow(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded call error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestServerRejectsMalformedAndWrongVersionRequests(t *testing.T) {
	server := newServer(nil, NewHandler(&desktopStub{}))
	tests := []struct {
		name    string
		payload []byte
		code    ErrorCode
	}{
		{
			name:    "malformed",
			payload: []byte("{"),
			code:    ErrorCodeInvalidRequest,
		},
		{
			name:    "wrong version",
			payload: []byte(`{"version":2,"operation":"get_active_window"}`),
			code:    ErrorCodeUnsupportedVersion,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := exchangeRaw(t, server, test.payload)
			if response.Error == nil || response.Error.Code != test.code {
				t.Fatalf("response error = %#v, want code %q", response.Error, test.code)
			}
		})
	}
}

func TestServerRejectsOversizedMessage(t *testing.T) {
	server := newServer(nil, NewHandler(&desktopStub{}))
	clientConn, serverConn := net.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- server.handleConnection(context.Background(), serverConn)
	}()

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], maxMessageSize+1)
	if _, err := clientConn.Write(header[:]); err != nil {
		t.Fatalf("write header: %v", err)
	}

	response, err := readResponse(clientConn)
	if err != nil {
		t.Fatalf("readResponse() error = %v", err)
	}
	if response.Error == nil || response.Error.Code != ErrorCodeInvalidRequest {
		t.Fatalf("response error = %#v, want invalid_request", response.Error)
	}
	_ = clientConn.Close()

	if err := <-done; err != nil {
		t.Fatalf("handleConnection() error = %v", err)
	}
}

func newTestClient(server *Server, timeout time.Duration) *Client {
	return newClient(
		func(ctx context.Context) (net.Conn, error) {
			return testConnection(ctx, server), nil
		},
		timeout,
	)
}

func testConnection(ctx context.Context, server *Server) net.Conn {
	clientConn, serverConn := net.Pipe()
	go func() {
		_ = server.handleConnection(ctx, serverConn)
	}()
	return clientConn
}

func exchangeRaw(t *testing.T, server *Server, payload []byte) Response {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- server.handleConnection(context.Background(), serverConn)
	}()

	if err := writeFrame(clientConn, payload); err != nil {
		t.Fatalf("writeFrame() error = %v", err)
	}
	response, err := readResponse(clientConn)
	if err != nil {
		t.Fatalf("readResponse() error = %v", err)
	}
	_ = clientConn.Close()

	if err := <-done; err != nil {
		t.Fatalf("handleConnection() error = %v", err)
	}
	return response
}

type shortWriter struct {
	buffer bytes.Buffer
	limit  int
}

func (w *shortWriter) Write(data []byte) (int, error) {
	if len(data) > w.limit {
		data = data[:w.limit]
	}
	return w.buffer.Write(data)
}

func TestWriteFrameHandlesShortWrites(t *testing.T) {
	writer := &shortWriter{limit: 2}
	want := []byte("session bridge")

	if err := writeFrame(writer, want); err != nil {
		t.Fatalf("writeFrame() error = %v", err)
	}

	got, err := readFrame(bytes.NewReader(writer.buffer.Bytes()))
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("payload = %q, want %q", got, want)
	}
}
