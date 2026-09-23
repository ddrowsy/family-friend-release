package kidcontrol

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

// WindowObservationHandler receives active window observations.
type WindowObservationHandler func(ctx context.Context, observation WindowObservation) error

// WindowObservation describes active window state.
type WindowObservation struct {
	Window platform.WindowInfo
}

// WindowMonitor monitors the active window.
type WindowMonitor struct {
	windowProvider platform.WindowProvider
	scanInterval   time.Duration
	handler        WindowObservationHandler
	logger         *log.Logger
	cancel         context.CancelFunc
	done           chan struct{}
	mu             sync.Mutex
}

// NewWindowMonitor creates a window monitor.
func NewWindowMonitor(windowProvider platform.WindowProvider, scanInterval time.Duration, logger *log.Logger) *WindowMonitor {
	if logger == nil {
		logger = log.Default()
	}

	return &WindowMonitor{
		windowProvider: windowProvider,
		scanInterval:   scanInterval,
		logger:         logger,
	}
}

// SetHandler sets the callback used for active window observations.
func (m *WindowMonitor) SetHandler(handler WindowObservationHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.handler = handler
}

// Start starts the window monitor.
func (m *WindowMonitor) Start(ctx context.Context) error {
	if m.windowProvider == nil {
		return errors.New("window provider is required")
	}
	if m.scanInterval <= 0 {
		return errors.New("window scan interval must be positive")
	}

	m.mu.Lock()
	if m.handler == nil {
		m.mu.Unlock()
		return errors.New("window observation handler is required")
	}
	if m.cancel != nil {
		m.mu.Unlock()
		return errors.New("window monitor already started")
	}
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	m.cancel = cancel
	m.done = done
	m.mu.Unlock()

	m.logger.Printf("kidcontrol window monitor started; scan_interval_seconds=%.0f", m.scanInterval.Seconds())
	go m.run(loopCtx, done)
	return nil
}

// Observe reads active window information once.
func (m *WindowMonitor) Observe(ctx context.Context) (WindowObservation, error) {
	if m.windowProvider == nil {
		return WindowObservation{}, errors.New("window provider is required")
	}

	window, err := m.windowProvider.ActiveWindow(ctx)
	if err != nil {
		return WindowObservation{}, err
	}

	m.logger.Printf("kidcontrol active window title=%q process=%s", window.Title, window.ProcessName)
	return WindowObservation{Window: window}, nil
}

// Stop stops the window monitor.
func (m *WindowMonitor) Stop(ctx context.Context) error {
	m.mu.Lock()
	cancel := m.cancel
	done := m.done
	m.cancel = nil
	m.done = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	m.logger.Printf("kidcontrol window monitor stopped")
	return nil
}

func (m *WindowMonitor) run(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	m.observeAndHandle(ctx)

	ticker := time.NewTicker(m.scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.observeAndHandle(ctx)
		}
	}
}

func (m *WindowMonitor) observeAndHandle(ctx context.Context) {
	observation, err := m.Observe(ctx)
	if err != nil {
		if ctx.Err() == nil {
			m.logger.Printf("kidcontrol window monitor scan failed: %v", err)
		}
		return
	}

	m.mu.Lock()
	handler := m.handler
	m.mu.Unlock()
	if handler == nil {
		return
	}

	if err := handler(ctx, observation); err != nil && ctx.Err() == nil {
		m.logger.Printf("kidcontrol window monitor handler failed: %v", err)
	}
}
