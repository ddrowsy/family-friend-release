package sessionbridge

import (
	"context"
	"errors"
	"fmt"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

var ErrDesktopUnavailable = errors.New("session desktop endpoint is unavailable")

type Desktop interface {
	ActiveWindow(ctx context.Context) (platform.WindowInfo, error)
	CloseWindow(ctx context.Context, windowID string) error
}

type Handler struct {
	desktop Desktop
}

func NewHandler(desktop Desktop) *Handler {
	return &Handler{desktop: desktop}
}

func (h *Handler) Handle(ctx context.Context, request Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if err := request.Validate(); err != nil {
		return Response{}, err
	}
	if h == nil || h.desktop == nil {
		return Response{}, ErrDesktopUnavailable
	}

	switch request.Operation {
	case OperationGetActiveWindow:
		window, err := h.desktop.ActiveWindow(ctx)
		if err != nil {
			return Response{}, fmt.Errorf("getting active window: %w", err)
		}
		return Response{Version: ProtocolVersion, Window: &window}, nil
	case OperationCloseWindow:
		if err := h.desktop.CloseWindow(ctx, request.WindowID); err != nil {
			return Response{}, fmt.Errorf("closing window %q: %w", request.WindowID, err)
		}
		return Response{Version: ProtocolVersion}, nil
	default:
		return Response{}, fmt.Errorf("%w: %q", ErrUnsupportedOperation, request.Operation)
	}
}
