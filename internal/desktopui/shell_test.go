package desktopui

import (
	"context"
	"errors"
	"sync"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ddrowsy/family-friend-release/internal/agent"
)

type fakeAgentClient struct {
	mu     sync.Mutex
	status agent.UIStatus
	err    error
	calls  int
}

func (c *fakeAgentClient) Status(_ context.Context) (agent.UIStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return c.status, c.err
}

func (c *fakeAgentClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestNewShellRequiresDependencies(t *testing.T) {
	application := test.NewApp()

	if _, err := NewShell(nil, &fakeAgentClient{}); err == nil {
		t.Fatal("NewShell() missing application error = nil")
	}
	if _, err := NewShell(application, nil); err == nil {
		t.Fatal("NewShell() missing agent client error = nil")
	}
}

func TestShellStartsHiddenAndReusesWindow(t *testing.T) {
	application := test.NewApp()
	client := &fakeAgentClient{}
	windowsBefore := len(application.Driver().AllWindows())

	shell, err := NewShell(application, client)
	if err != nil {
		t.Fatalf("NewShell() error = %v", err)
	}
	if shell.Visible() {
		t.Fatal("shell starts visible")
	}
	createdWindows := len(application.Driver().AllWindows())
	if createdWindows != windowsBefore+1 {
		t.Fatalf("windows = %d, want %d", createdWindows, windowsBefore+1)
	}
	window := shell.window

	shell.Open()
	shell.Open()
	if !shell.Visible() {
		t.Fatal("shell not visible after Open()")
	}
	if shell.window != window {
		t.Fatal("Open() replaced the existing window")
	}
	if got := len(application.Driver().AllWindows()); got != createdWindows {
		t.Fatalf("windows after repeated Open() = %d, want %d", got, createdWindows)
	}
}

func TestShellHideKeepsWindow(t *testing.T) {
	application := test.NewApp()
	shell, err := NewShell(application, &fakeAgentClient{})
	if err != nil {
		t.Fatalf("NewShell() error = %v", err)
	}
	window := shell.window

	shell.Open()
	shell.Hide()
	if shell.Visible() {
		t.Fatal("shell remains visible after Hide()")
	}
	if shell.window != window {
		t.Fatal("Hide() replaced the window")
	}
}

func TestShellShowsDisconnectedAgentState(t *testing.T) {
	application := test.NewApp()
	client := &fakeAgentClient{err: errors.New("agent unavailable")}
	shell, err := NewShell(application, client)
	if err != nil {
		t.Fatalf("NewShell() error = %v", err)
	}

	shell.refreshStatus(context.Background())
	if shell.statusLabel.Text != "Agent: unavailable\nProtection status: unknown" {
		t.Fatalf("status label = %q", shell.statusLabel.Text)
	}
	if shell.trayStatus.Label != "Status: disconnected" {
		t.Fatalf("tray status = %q", shell.trayStatus.Label)
	}
}

func TestShellShowsProtectedAgentState(t *testing.T) {
	application := test.NewApp()
	client := &fakeAgentClient{status: agent.UIStatus{
		AgentRunning:      true,
		ProtectionRunning: true,
		Service:           agent.UIServiceConnected,
		Paired:            true,
	}}
	shell, err := NewShell(application, client)
	if err != nil {
		t.Fatalf("NewShell() error = %v", err)
	}

	shell.refreshStatus(context.Background())
	if shell.trayStatus.Label != "Status: protected" {
		t.Fatalf("tray status = %q", shell.trayStatus.Label)
	}
}

func TestShellExitDoesNotCallAgent(t *testing.T) {
	application := test.NewApp()
	client := &fakeAgentClient{}
	shell, err := NewShell(application, client)
	if err != nil {
		t.Fatalf("NewShell() error = %v", err)
	}

	shell.Exit()
	if calls := client.callCount(); calls != 0 {
		t.Fatalf("agent calls after Exit() = %d, want 0", calls)
	}
}
