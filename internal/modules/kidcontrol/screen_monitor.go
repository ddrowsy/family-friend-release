package kidcontrol

import (
	"context"
	"errors"
	"log"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

// ScreenMonitor is a placeholder for screen monitoring.
type ScreenMonitor struct {
	screenProvider        platform.ScreenProvider
	sampleIntervalSeconds int
	logger                *log.Logger
}

// NewScreenMonitor creates a screen monitor placeholder.
func NewScreenMonitor(screenProvider platform.ScreenProvider, sampleIntervalSeconds int, logger *log.Logger) *ScreenMonitor {
	if logger == nil {
		logger = log.Default()
	}

	return &ScreenMonitor{
		screenProvider:        screenProvider,
		sampleIntervalSeconds: sampleIntervalSeconds,
		logger:                logger,
	}
}

// Start starts the screen monitor placeholder.
func (m *ScreenMonitor) Start(ctx context.Context) error {
	if m.screenProvider == nil {
		return errors.New("screen provider is required")
	}

	m.logger.Printf("kidcontrol screen monitor started; sample_interval_seconds=%d", m.sampleIntervalSeconds)
	frame, err := m.screenProvider.CaptureScreen(ctx)
	if err != nil {
		return err
	}
	m.logger.Printf("kidcontrol screen monitor captured frame width=%d height=%d source=%s", frame.Width, frame.Height, frame.Source)
	return nil
}

// Stop stops the screen monitor placeholder.
func (m *ScreenMonitor) Stop(ctx context.Context) error {
	m.logger.Printf("kidcontrol screen monitor stopped")
	return nil
}
