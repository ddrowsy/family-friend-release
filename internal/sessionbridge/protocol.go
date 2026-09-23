package sessionbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

const ProtocolVersion = 1

type Operation string

const (
	OperationGetActiveWindow Operation = "get_active_window"
	OperationCloseWindow     Operation = "close_window"
)

type ErrorCode string

const (
	ErrorCodeInvalidRequest       ErrorCode = "invalid_request"
	ErrorCodeUnsupportedVersion   ErrorCode = "unsupported_version"
	ErrorCodeUnsupportedOperation ErrorCode = "unsupported_operation"
	ErrorCodeOperationFailed      ErrorCode = "operation_failed"
)

var (
	ErrInvalidRequest       = errors.New("invalid session bridge request")
	ErrUnsupportedVersion   = errors.New("unsupported session bridge protocol version")
	ErrUnsupportedOperation = errors.New("unsupported session bridge operation")
)

type Request struct {
	Version   int       `json:"version"`
	Operation Operation `json:"operation"`
	WindowID  string    `json:"window_id,omitempty"`
}

type Response struct {
	Version int                  `json:"version"`
	Window  *platform.WindowInfo `json:"window,omitempty"`
	Error   *ResponseError       `json:"error,omitempty"`
}

type ResponseError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func NewGetActiveWindowRequest() Request {
	return Request{Version: ProtocolVersion, Operation: OperationGetActiveWindow}
}

func NewCloseWindowRequest(windowID string) Request {
	return Request{
		Version:   ProtocolVersion,
		Operation: OperationCloseWindow,
		WindowID:  windowID,
	}
}

func (r Request) Validate() error {
	if r.Version != ProtocolVersion {
		return fmt.Errorf("%w: %d", ErrUnsupportedVersion, r.Version)
	}

	switch r.Operation {
	case OperationGetActiveWindow:
		if r.WindowID != "" {
			return fmt.Errorf("%w: unexpected window id", ErrInvalidRequest)
		}
	case OperationCloseWindow:
		if strings.TrimSpace(r.WindowID) == "" {
			return fmt.Errorf("%w: window id is required", ErrInvalidRequest)
		}
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOperation, r.Operation)
	}
	return nil
}

func (r Response) Validate() error {
	if r.Version != ProtocolVersion {
		return fmt.Errorf("%w: %d", ErrUnsupportedVersion, r.Version)
	}
	if r.Window != nil && r.Error != nil {
		return fmt.Errorf("%w: window and error both set", ErrInvalidRequest)
	}
	if r.Error == nil {
		return nil
	}

	switch r.Error.Code {
	case ErrorCodeInvalidRequest,
		ErrorCodeUnsupportedVersion,
		ErrorCodeUnsupportedOperation,
		ErrorCodeOperationFailed:
		return nil
	default:
		return fmt.Errorf("%w: unknown error code %q", ErrInvalidRequest, r.Error.Code)
	}
}

func encodeRequest(w io.Writer, request Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	return encodeJSON(w, request)
}

func decodeRequest(r io.Reader) (Request, error) {
	var request Request
	if err := decodeJSON(r, &request); err != nil {
		return Request{}, err
	}
	return request, request.Validate()
}

func encodeResponse(w io.Writer, response Response) error {
	if err := response.Validate(); err != nil {
		return err
	}
	return encodeJSON(w, response)
}

func decodeResponse(r io.Reader) (Response, error) {
	var response Response
	if err := decodeJSON(r, &response); err != nil {
		return Response{}, err
	}
	return response, response.Validate()
}

func encodeJSON(w io.Writer, value any) error {
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return fmt.Errorf("encoding session bridge message: %w", err)
	}
	return nil
}

func decodeJSON(r io.Reader, value any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("%w: decoding session bridge message: %v", ErrInvalidRequest, err)
	}

	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return ErrInvalidRequest
	}
	return fmt.Errorf("%w: decoding trailing session bridge data: %v", ErrInvalidRequest, err)
}
