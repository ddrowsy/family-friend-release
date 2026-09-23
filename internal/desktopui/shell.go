package desktopui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ddrowsy/family-friend-release/internal/agent"
)

const (
	windowTitle          = "Family Friend"
	statusRefreshTimeout = 3 * time.Second
)

type statusPresentation struct {
	windowText string
	trayText   string
}

// Shell owns the user-session desktop window and system tray lifecycle.
type Shell struct {
	application fyne.App
	client      AgentClient
	window      fyne.Window
	statusLabel *widget.Label
	trayMenu    *fyne.Menu
	trayStatus  *fyne.MenuItem

	mu      sync.RWMutex
	visible bool
}

// NewShell creates a hidden desktop shell. It never starts or stops protection.
func NewShell(application fyne.App, client AgentClient) (*Shell, error) {
	if application == nil {
		return nil, errors.New("desktop application is required")
	}
	if client == nil {
		return nil, errors.New("agent client is required")
	}

	application.SetIcon(theme.ComputerIcon())
	window := application.NewWindow(windowTitle)
	window.Resize(fyne.NewSize(380, 180))

	statusLabel := widget.NewLabel("Agent: checking")
	window.SetContent(container.NewVBox(
		widget.NewLabel("Family Friend"),
		statusLabel,
		widget.NewLabel("Protection continues when this window is hidden or closed."),
	))

	shell := &Shell{
		application: application,
		client:      client,
		window:      window,
		statusLabel: statusLabel,
	}
	window.SetCloseIntercept(shell.Hide)
	shell.configureTray()
	return shell, nil
}

// Run starts the user-session UI event loop without showing the main window.
func (s *Shell) Run() {
	s.RefreshStatus(context.Background())
	s.application.Run()
}

// Open shows and focuses the existing main window.
func (s *Shell) Open() {
	s.mu.Lock()
	s.visible = true
	s.mu.Unlock()

	s.window.Show()
	s.window.RequestFocus()
	s.RefreshStatus(context.Background())
}

// Hide hides the desktop window without changing protection state.
func (s *Shell) Hide() {
	s.mu.Lock()
	s.visible = false
	s.mu.Unlock()
	s.window.Hide()
}

// Exit exits only the desktop UI process.
func (s *Shell) Exit() {
	s.application.Quit()
}

// Visible reports whether the desktop window is currently shown.
func (s *Shell) Visible() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.visible
}

// RefreshStatus refreshes the display from the localhost agent without blocking the UI thread.
func (s *Shell) RefreshStatus(ctx context.Context) {
	go s.refreshStatus(ctx)
}

func (s *Shell) configureTray() {
	openItem := fyne.NewMenuItem("Open Family Friend", s.Open)
	s.trayStatus = fyne.NewMenuItem("Status: checking", func() {})
	s.trayStatus.Disabled = true
	exitItem := fyne.NewMenuItem("Exit UI", s.Exit)
	exitItem.IsQuit = true
	s.trayMenu = fyne.NewMenu(
		windowTitle,
		openItem,
		s.trayStatus,
		fyne.NewMenuItemSeparator(),
		exitItem,
	)

	desktopApplication, ok := s.application.(desktop.App)
	if !ok {
		return
	}
	desktopApplication.SetSystemTrayIcon(theme.ComputerIcon())
	desktopApplication.SetSystemTrayMenu(s.trayMenu)
	desktopApplication.SetSystemTrayWindow(s.window)
}

func (s *Shell) refreshStatus(ctx context.Context) {
	requestContext, cancel := context.WithTimeout(ctx, statusRefreshTimeout)
	defer cancel()

	status, err := s.client.Status(requestContext)
	presentation := presentStatus(status, err)
	fyne.DoAndWait(func() {
		s.statusLabel.SetText(presentation.windowText)
		s.trayStatus.Label = presentation.trayText
		s.trayMenu.Refresh()
	})
}

func presentStatus(status agent.UIStatus, err error) statusPresentation {
	if err != nil {
		return statusPresentation{
			windowText: "Agent: unavailable\nProtection status: unknown",
			trayText:   "Status: disconnected",
		}
	}
	if !status.AgentRunning {
		return statusPresentation{
			windowText: "Agent: unavailable\nProtection status: unknown",
			trayText:   "Status: attention",
		}
	}

	protectionState := "not running"
	trayText := "Status: attention"
	if status.ProtectionRunning {
		protectionState = "running"
		trayText = "Status: protected"
	}
	pairingState := "unpaired"
	if status.Paired {
		pairingState = "paired"
	}
	return statusPresentation{
		windowText: fmt.Sprintf(
			"Agent: running\nProtection: %s\nService: %s\nDevice: %s",
			protectionState,
			status.Service,
			pairingState,
		),
		trayText: trayText,
	}
}
