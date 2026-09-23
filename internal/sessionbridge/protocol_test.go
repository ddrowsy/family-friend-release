package sessionbridge

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

func TestRequestRoundTrip(t *testing.T) {
	tests := []Request{
		NewGetActiveWindowRequest(),
		NewCloseWindowRequest("123"),
	}

	for _, want := range tests {
		var buffer bytes.Buffer
		if err := encodeRequest(&buffer, want); err != nil {
			t.Fatalf("encodeRequest() error = %v", err)
		}
		got, err := decodeRequest(&buffer)
		if err != nil {
			t.Fatalf("decodeRequest() error = %v", err)
		}
		if got != want {
			t.Fatalf("request = %#v, want %#v", got, want)
		}
	}
}

func TestRequestValidation(t *testing.T) {
	tests := []struct {
		request Request
		wantErr error
	}{
		{
			request: Request{
				Version:   ProtocolVersion + 1,
				Operation: OperationGetActiveWindow,
			},
			wantErr: ErrUnsupportedVersion,
		},
		{
			request: Request{
				Version:   ProtocolVersion,
				Operation: Operation("unknown"),
			},
			wantErr: ErrUnsupportedOperation,
		},
		{request: NewCloseWindowRequest(" "), wantErr: ErrInvalidRequest},
		{
			request: Request{
				Version:   ProtocolVersion,
				Operation: OperationGetActiveWindow,
				WindowID:  "123",
			},
			wantErr: ErrInvalidRequest,
		},
	}

	for _, test := range tests {
		if err := test.request.Validate(); !errors.Is(err, test.wantErr) {
			t.Fatalf("Validate() error = %v, want %v", err, test.wantErr)
		}
	}
}

func TestDecodeRequestIsStrict(t *testing.T) {
	inputs := []string{
		`{"version":1,"operation":"get_active_window","unexpected":true}`,
		`{"version":1,"operation":"get_active_window"} {"version":1}`,
	}

	for _, input := range inputs {
		_, err := decodeRequest(bytes.NewBufferString(input))
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("decodeRequest(%q) error = %v, want %v", input, err, ErrInvalidRequest)
		}
	}
}

func TestResponseValidationAndRoundTrip(t *testing.T) {
	window := platform.WindowInfo{ID: "88", Title: "Homework"}
	want := Response{Version: ProtocolVersion, Window: &window}

	var buffer bytes.Buffer
	if err := encodeResponse(&buffer, want); err != nil {
		t.Fatalf("encodeResponse() error = %v", err)
	}
	got, err := decodeResponse(&buffer)
	if err != nil {
		t.Fatalf("decodeResponse() error = %v", err)
	}
	if got.Window == nil || *got.Window != window {
		t.Fatalf("window = %#v, want %#v", got.Window, window)
	}

	invalid := []Response{
		{
			Version: ProtocolVersion,
			Window:  &window,
			Error: &ResponseError{
				Code: ErrorCodeOperationFailed,
			},
		},
		{
			Version: ProtocolVersion,
			Error: &ResponseError{
				Code: ErrorCode("unknown"),
			},
		},
	}
	for _, response := range invalid {
		if err := response.Validate(); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidRequest)
		}
	}
}
