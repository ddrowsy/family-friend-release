package sessionbridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

const (
	maxMessageSize     = 64 * 1024
	defaultCallTimeout = 5 * time.Second
)

var (
	ErrMessageTooLarge = errors.New("session bridge message is too large")
	ErrOperationFailed = errors.New("session bridge operation failed")
	ErrUntrustedPeer   = errors.New("untrusted session bridge peer")
)

type dialFunc func(ctx context.Context) (net.Conn, error)

type Client struct {
	dial    dialFunc
	timeout time.Duration
}

func newClient(dial dialFunc, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultCallTimeout
	}
	return &Client{
		dial:    dial,
		timeout: timeout,
	}
}

func (c *Client) GetActiveWindow(ctx context.Context) (platform.WindowInfo, error) {
	response, err := c.call(ctx, NewGetActiveWindowRequest())
	if err != nil {
		return platform.WindowInfo{}, err
	}
	if response.Window == nil {
		return platform.WindowInfo{}, fmt.Errorf("%w: active window missing", ErrInvalidRequest)
	}
	return *response.Window, nil
}

func (c *Client) CloseWindow(ctx context.Context, windowID string) error {
	_, err := c.call(ctx, NewCloseWindowRequest(windowID))
	return err
}

func (c *Client) call(ctx context.Context, request Request) (Response, error) {
	if c == nil || c.dial == nil {
		return Response{}, ErrDesktopUnavailable
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	conn, err := c.dial(callCtx)
	if err != nil {
		if callCtx.Err() != nil {
			return Response{}, callCtx.Err()
		}
		return Response{}, fmt.Errorf("%w: %v", ErrDesktopUnavailable, err)
	}
	defer conn.Close()

	stopInterrupt := context.AfterFunc(callCtx, func() {
		_ = conn.Close()
	})
	defer stopInterrupt()
	if deadline, ok := callCtx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return Response{}, fmt.Errorf("setting session bridge deadline: %w", err)
		}
	}

	if err := writeRequest(conn, request); err != nil {
		return Response{}, ioResult(callCtx, "writing session bridge request", err)
	}
	response, err := readResponse(conn)
	if err != nil {
		return Response{}, ioResult(callCtx, "reading session bridge response", err)
	}
	if response.Error != nil {
		return Response{}, remoteError(response.Error)
	}
	return response, nil
}

type Server struct {
	listener net.Listener
	handler  *Handler
}

func newServer(listener net.Listener, handler *Handler) *Server {
	return &Server{
		listener: listener,
		handler:  handler,
	}
}

func (s *Server) Serve(ctx context.Context) error {
	if s == nil || s.listener == nil {
		return ErrDesktopUnavailable
	}

	stopClose := context.AfterFunc(ctx, func() {
		_ = s.listener.Close()
	})
	defer stopClose()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accepting session bridge connection: %w", err)
		}

		_ = s.handleConnection(ctx, conn)
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) error {
	defer conn.Close()

	requestCtx, cancel := context.WithTimeout(ctx, defaultCallTimeout)
	defer cancel()

	stopInterrupt := context.AfterFunc(requestCtx, func() {
		_ = conn.Close()
	})
	defer stopInterrupt()
	if deadline, ok := requestCtx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return fmt.Errorf("setting session bridge deadline: %w", err)
		}
	}

	request, err := readRequest(conn)
	if err != nil {
		return writeErrorResponse(conn, err)
	}

	response, err := s.handler.Handle(requestCtx, request)
	if err != nil {
		return writeErrorResponse(conn, err)
	}
	return writeResponse(conn, response)
}

func writeRequest(w io.Writer, request Request) error {
	var payload bytes.Buffer
	if err := encodeRequest(&payload, request); err != nil {
		return err
	}
	return writeFrame(w, payload.Bytes())
}

func readRequest(r io.Reader) (Request, error) {
	payload, err := readFrame(r)
	if err != nil {
		return Request{}, err
	}
	return decodeRequest(bytes.NewReader(payload))
}

func writeResponse(w io.Writer, response Response) error {
	var payload bytes.Buffer
	if err := encodeResponse(&payload, response); err != nil {
		return err
	}
	return writeFrame(w, payload.Bytes())
}

func readResponse(r io.Reader) (Response, error) {
	payload, err := readFrame(r)
	if err != nil {
		return Response{}, err
	}
	return decodeResponse(bytes.NewReader(payload))
}

func writeFrame(w io.Writer, payload []byte) error {
	if len(payload) == 0 {
		return ErrInvalidRequest
	}
	if len(payload) > maxMessageSize {
		return ErrMessageTooLarge
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeFull(w, header[:]); err != nil {
		return err
	}
	return writeFull(w, payload)
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	size := binary.BigEndian.Uint32(header[:])
	if size == 0 {
		return nil, ErrInvalidRequest
	}
	if size > maxMessageSize {
		return nil, ErrMessageTooLarge
	}

	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeErrorResponse(w io.Writer, err error) error {
	response := Response{
		Version: ProtocolVersion,
		Error: &ResponseError{
			Code:    errorCode(err),
			Message: err.Error(),
		},
	}
	return writeResponse(w, response)
}

func errorCode(err error) ErrorCode {
	switch {
	case errors.Is(err, ErrUnsupportedVersion):
		return ErrorCodeUnsupportedVersion
	case errors.Is(err, ErrUnsupportedOperation):
		return ErrorCodeUnsupportedOperation
	case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrMessageTooLarge):
		return ErrorCodeInvalidRequest
	default:
		return ErrorCodeOperationFailed
	}
}

func remoteError(responseErr *ResponseError) error {
	var base error
	switch responseErr.Code {
	case ErrorCodeInvalidRequest:
		base = ErrInvalidRequest
	case ErrorCodeUnsupportedVersion:
		base = ErrUnsupportedVersion
	case ErrorCodeUnsupportedOperation:
		base = ErrUnsupportedOperation
	default:
		base = ErrOperationFailed
	}
	return fmt.Errorf("%w: %s", base, responseErr.Message)
}

func ioResult(ctx context.Context, action string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("%s: %w", action, err)
}
