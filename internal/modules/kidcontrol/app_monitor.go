package kidcontrol

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

// AppObservationHandler receives observations from app monitor scans.
type AppObservationHandler func(ctx context.Context, observations []AppObservation) error

// AppObservation describes a process seen by the app monitor.
type AppObservation struct {
	Process platform.ProcessInfo
}

// AppMonitor monitors running applications.
type AppMonitor struct {
	processProvider platform.ProcessProvider
	scanInterval    time.Duration
	blockedApps     []string
	handler         AppObservationHandler
	logger          *log.Logger
	cancel          context.CancelFunc
	done            chan struct{}
	mu              sync.Mutex
}

// NewAppMonitor creates an app monitor.
func NewAppMonitor(processProvider platform.ProcessProvider, scanInterval time.Duration, blockedApps []string, logger *log.Logger) *AppMonitor {
	if logger == nil {
		logger = log.Default()
	}

	return &AppMonitor{
		processProvider: processProvider,
		scanInterval:    scanInterval,
		blockedApps:     append([]string(nil), blockedApps...),
		logger:          logger,
	}
}

// SetHandler sets the callback used for scan observations.
func (m *AppMonitor) SetHandler(handler AppObservationHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.handler = handler
}

// Start starts the app monitor.
func (m *AppMonitor) Start(ctx context.Context) error {
	if m.processProvider == nil {
		return errors.New("process provider is required")
	}
	if m.scanInterval <= 0 {
		return errors.New("scan interval must be positive")
	}

	m.mu.Lock()
	if m.handler == nil {
		m.mu.Unlock()
		return errors.New("app observation handler is required")
	}
	if m.cancel != nil {
		m.mu.Unlock()
		return errors.New("app monitor already started")
	}
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	m.cancel = cancel
	m.done = done
	m.mu.Unlock()

	m.logger.Printf("kidcontrol app monitor started; scan_interval_seconds=%.0f blocked_apps=%s", m.scanInterval.Seconds(), strings.Join(m.blockedApps, ","))
	go m.run(loopCtx, done)
	return nil
}

// Scan lists running processes and returns app observations.
func (m *AppMonitor) Scan(ctx context.Context) ([]AppObservation, error) {
	if m.processProvider == nil {
		return nil, errors.New("process provider is required")
	}

	processes, err := m.processProvider.ListProcesses(ctx)
	if err != nil {
		return nil, err
	}

	observations := make([]AppObservation, 0, len(processes))
	for _, process := range processes {
		observations = append(observations, AppObservation{Process: process})
	}

	m.logger.Printf("kidcontrol app monitor observed %d processes", len(processes))
	return observations, nil
}

// Stop stops the app monitor.
func (m *AppMonitor) Stop(ctx context.Context) error {
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

	m.logger.Printf("kidcontrol app monitor stopped")
	return nil
}

func (m *AppMonitor) run(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	m.scanAndHandle(ctx)

	ticker := time.NewTicker(m.scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.scanAndHandle(ctx)
		}
	}
}

func (m *AppMonitor) scanAndHandle(ctx context.Context) {
	observations, err := m.Scan(ctx)
	if err != nil {
		if ctx.Err() == nil {
			m.logger.Printf("kidcontrol app monitor scan failed: %v", err)
		}
		return
	}

	m.mu.Lock()
	handler := m.handler
	m.mu.Unlock()
	if handler == nil {
		return
	}

	if err := handler(ctx, observations); err != nil && ctx.Err() == nil {
		m.logger.Printf("kidcontrol app monitor handler failed: %v", err)
	}
}
