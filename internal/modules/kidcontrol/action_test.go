package kidcontrol

import (
	"context"
	"testing"
)

type fakeActionController struct {
	closedPID      int
	closedWindowID string
}

func (f *fakeActionController) CloseProcess(ctx context.Context, pid int) error {
	f.closedPID = pid
	return nil
}

func (f *fakeActionController) CloseWindow(ctx context.Context, windowID string) error {
	f.closedWindowID = windowID
	return nil
}

func TestExecutorHandlesCloseProcess(t *testing.T) {
	controller := &fakeActionController{}
	executor := NewExecutor(controller)

	err := executor.Execute(context.Background(), Decision{
		Matched:   true,
		Action:    ActionCloseProcess,
		ProcessID: 99,
		AppName:   "msedge.exe",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if controller.closedPID != 99 {
		t.Fatalf("closedPID = %d, want 99", controller.closedPID)
	}
}

func TestExecutorHandlesCloseWindow(t *testing.T) {
	controller := &fakeActionController{}
	executor := NewExecutor(controller)

	err := executor.Execute(context.Background(), Decision{
		Matched:  true,
		Action:   ActionCloseWindow,
		WindowID: "window-1",
		AppName:  "msedge.exe",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if controller.closedWindowID != "window-1" {
		t.Fatalf("closedWindowID = %q, want window-1", controller.closedWindowID)
	}
}
