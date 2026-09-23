package kidcontrol

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/ddrowsy/family-friend-release/internal/platform"
	"github.com/ddrowsy/family-friend-release/internal/policy"
)

// Module is the kidcontrol module.
type Module struct {
	platform       platform.Platform
	logger         *log.Logger
	config         Config
	appMonitor     *AppMonitor
	windowMonitor  *WindowMonitor
	screenMonitor  *ScreenMonitor
	browserMonitor *BrowserMonitor
	evaluator      Evaluator
	executor       Executor
	auditLogger    auditor
	handledPIDs    map[int]struct{}
	handledWindows map[string]struct{}
	handledMu      sync.Mutex
	browserMu      sync.RWMutex
	started        []lifecycle
	running        bool
}

// New creates a kidcontrol module.
func New(platformLayer platform.Platform, logger *log.Logger) *Module {
	if logger == nil {
		logger = log.Default()
	}

	return &Module{platform: platformLayer, logger: logger}
}

// Name returns the JSON and package module name.
func (m *Module) Name() string {
	return policy.KidControlModuleName
}

// Start starts the module.
func (m *Module) Start(ctx context.Context, modulePolicy policy.ModulePolicy) error {
	if m.platform == nil {
		return fmt.Errorf("kidcontrol platform is required")
	}

	settings, err := ParseSettings(modulePolicy.Settings)
	if err != nil {
		return fmt.Errorf("parse kidcontrol settings: %w", err)
	}
	if err := validateSettings(settings); err != nil {
		return fmt.Errorf("validate kidcontrol settings: %w", err)
	}

	m.configureRuntime(Config{
		Mode:     modulePolicy.Mode,
		Settings: settings,
	})
	if err := m.startRuntime(ctx); err != nil {
		return err
	}

	m.running = true
	return nil
}

// Stop stops the module.
func (m *Module) Stop(ctx context.Context) error {
	if !m.running {
		return nil
	}

	err := m.stopRuntime(ctx)
	m.running = false
	m.resetRuntime()
	return err
}
