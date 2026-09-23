package sessionbridge

import (
	"context"
	"errors"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

type desktopStub struct {
	window platform.WindowInfo
	err    error
	closed string
}

func (s *desktopStub) ActiveWindow(context.Context) (platform.WindowInfo, error) {
	return s.window, s.err
}

func (s *desktopStub) CloseWindow(_ context.Context, windowID string) error {
	s.closed = windowID
	return s.err
}

func TestHandlerGetActiveWindow(t *testing.T) {
	desktop := &desktopStub{
		window: platform.WindowInfo{ID: "100", Title: "Study"},
	}
	response, err := NewHandler(desktop).Handle(
		context.Background(),
		NewGetActiveWindowRequest(),
	)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if response.Window == nil || *response.Window != desktop.window {
		t.Fatalf("window = %#v, want %#v", response.Window, desktop.window)
	}
}

func TestHandlerCloseWindow(t *testing.T) {
	desktop := &desktopStub{}
	_, err := NewHandler(desktop).Handle(
		context.Background(),
		NewCloseWindowRequest("55"),
	)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if desktop.closed != "55" {
		t.Fatalf("closed window = %q, want 55", desktop.closed)
	}
}

func TestHandlerRejectsInvalidOrUnavailableRequest(t *testing.T) {
	_, err := NewHandler(&desktopStub{}).Handle(
		context.Background(),
		Request{Version: ProtocolVersion, Operation: Operation("unknown")},
	)
	if !errors.Is(err, ErrUnsupportedOperation) {
		t.Fatalf("invalid request error = %v", err)
	}

	_, err = NewHandler(nil).Handle(
		context.Background(),
		NewGetActiveWindowRequest(),
	)
	if !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("unavailable desktop error = %v", err)
	}
}

func TestHandlerPropagatesContextAndDesktopErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewHandler(&desktopStub{}).Handle(ctx, NewGetActiveWindowRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v", err)
	}

	wantErr := errors.New("desktop failed")
	_, err = NewHandler(&desktopStub{err: wantErr}).Handle(
		context.Background(),
		NewGetActiveWindowRequest(),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("desktop error = %v, want %v", err, wantErr)
	}
}
