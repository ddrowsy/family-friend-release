package kidcontrol

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/audit"
	"github.com/ddrowsy/family-friend-release/internal/platform"
)

type lifecycle interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type auditor interface {
	Log(event audit.Event) error
}

func (m *Module) configureRuntime(config Config) {
	m.config = config
	m.evaluator = NewEvaluator(config.Settings.AppControl.BlockedApps, config.Settings.WindowMonitoring.BlockedTitleKeywords)
	m.executor = NewExecutor(m.platform)
	m.auditLogger = audit.NewLogger(m.logger)
	m.handledPIDs = make(map[int]struct{})
	m.handledWindows = make(map[string]struct{})
	m.started = nil

	appMonitor := NewAppMonitor(
		m.platform,
		time.Duration(config.Settings.AppControl.ScanIntervalSeconds)*time.Second,
		config.Settings.AppControl.BlockedApps,
		m.logger,
	)
	appMonitor.SetHandler(m.handleAppObservations)
	m.appMonitor = appMonitor

	windowMonitor := NewWindowMonitor(
		m.platform,
		time.Duration(config.Settings.WindowMonitoring.ScanIntervalSeconds)*time.Second,
		m.logger,
	)
	windowMonitor.SetHandler(m.handleWindowObservation)
	m.windowMonitor = windowMonitor
	m.screenMonitor = NewScreenMonitor(m.platform, config.Settings.ScreenMonitoring.SampleIntervalSeconds, m.logger)
	m.browserMonitor = NewBrowserMonitor(m, m.logger)
}

func (m *Module) resetRuntime() {
	m.config = Config{}
	m.appMonitor = nil
	m.windowMonitor = nil
	m.screenMonitor = nil
	m.browserMonitor = nil
	m.evaluator = Evaluator{}
	m.executor = Executor{}
	m.auditLogger = nil
	m.handledPIDs = nil
	m.handledWindows = nil
	m.started = nil
}

// startRuntime starts configured kidcontrol runtime components.
func (s *Module) startRuntime(ctx context.Context) error {
	s.logger.Printf("kidcontrol started")
	s.logger.Printf("kidcontrol mode: %s", s.config.Mode)

	if s.config.Settings.AppControl.Enabled {
		if err := s.startComponent(ctx, "app monitor", s.appMonitor); err != nil {
			return err
		}
	}
	if s.config.Settings.WindowMonitoring.Enabled {
		if err := s.startComponent(ctx, "window monitor", s.windowMonitor); err != nil {
			return err
		}
	}
	if s.config.Settings.ScreenMonitoring.Enabled {
		if err := s.startComponent(ctx, "screen monitor", s.screenMonitor); err != nil {
			return err
		}
	}
	if s.config.Settings.BrowserMonitoring.Enabled {
		if err := s.startComponent(ctx, "browser monitor", s.browserMonitor); err != nil {
			return err
		}
	}

	return nil
}

// stopRuntime stops configured kidcontrol runtime components.
func (s *Module) stopRuntime(ctx context.Context) error {
	var err error

	for i := len(s.started) - 1; i >= 0; i-- {
		if stopErr := s.started[i].Stop(ctx); stopErr != nil {
			err = errors.Join(err, stopErr)
		}
	}

	s.started = nil
	s.logger.Printf("kidcontrol stopped")
	return err
}

func (s *Module) handleAppObservations(ctx context.Context, observations []AppObservation) error {
	for _, observation := range observations {
		result := s.evaluator.EvaluateProcess(observation.Process)
		if !result.Matched {
			continue
		}
		if s.alreadyHandled(result.ProcessID) {
			continue
		}

		reason := reasonLogFields(result)
		s.logger.Printf("blocked app detected: %s %s", result.AppName, reason)
		if err := s.audit("blocked app detected: " + result.AppName + " " + reason); err != nil {
			return err
		}

		if err := s.handleDecisionAction(ctx, result); err != nil {
			return err
		}
	}

	return nil
}

func (s *Module) handleWindowObservation(ctx context.Context, observation WindowObservation) error {
	window := observation.Window
	if window.ID == "" && window.Title == "" && window.ProcessName == "" {
		return nil
	}

	s.logger.Printf("active window detected: %s", window.ProcessName)
	result := s.evaluator.EvaluateActiveWindow(window, s.config.Settings.AppControl.Enabled)
	if !result.Matched {
		return nil
	}
	if s.windowAlreadyHandled(window) {
		return nil
	}

	reason := reasonLogFields(result)
	if result.Reason.Type == ReasonBlockedApp {
		s.logger.Printf("blocked app detected from active window: %s %s", result.AppName, reason)
		if err := s.audit("blocked app detected from active window: " + result.AppName + " " + reason); err != nil {
			return err
		}
	} else {
		name := actionDisplayName(result, "window")
		s.logger.Printf("blocked window title detected: %s %s", name, reason)
		if err := s.audit("blocked window title detected: " + name + " " + reason); err != nil {
			return err
		}
	}

	return s.handleDecisionAction(ctx, result)
}

func (s *Module) handleDecisionAction(ctx context.Context, result Decision) error {
	target := actionTarget(result)
	reason := reasonLogFields(result)
	s.logger.Printf("action selected: %s for %s %s", result.Action, result.AppName, reason)
	if err := s.audit("action selected: " + result.Action + " for " + result.AppName + " " + reason); err != nil {
		return err
	}

	if s.config.Mode != "enforce" {
		s.logger.Printf("action skipped in monitor mode: %s", result.AppName)
		if err := s.audit("action skipped in monitor mode: " + result.AppName); err != nil {
			return err
		}
		return nil
	}

	name := actionDisplayName(result, target)
	if s.config.Settings.DryRun {
		s.logger.Printf("dry-run close %s: %s", target, name)
		if err := s.audit("dry-run close " + target + ": " + name); err != nil {
			return err
		}
		return nil
	}

	s.logger.Printf("closing %s: %s", target, name)
	if err := s.executor.Execute(ctx, result); err != nil {
		if auditErr := s.audit("action failed: " + result.Action + " for " + name + ": " + err.Error()); auditErr != nil {
			return auditErr
		}
		return fmt.Errorf("execute action %q: %w", result.Action, err)
	}
	if err := s.audit("action executed: " + result.Action + " for " + name); err != nil {
		return err
	}

	return nil
}

func actionDisplayName(result Decision, target string) string {
	if target == "window" && result.Reason.Type == ReasonBlockedWindowTitleKeyword && result.Title != "" {
		return result.Title
	}
	return result.AppName
}

func actionTarget(result Decision) string {
	if result.Action == ActionCloseWindow {
		return "window"
	}
	return "process"
}

func reasonLogFields(result Decision) string {
	if result.Reason.Type == "" && result.Reason.Value == "" {
		return "reason_type=unknown reason_value=unknown"
	}
	return fmt.Sprintf("reason_type=%s reason_value=%q", result.Reason.Type, result.Reason.Value)
}

func (s *Module) windowAlreadyHandled(window platform.WindowInfo) bool {
	s.handledMu.Lock()
	defer s.handledMu.Unlock()

	if s.handledWindows == nil {
		s.handledWindows = make(map[string]struct{})
	}
	key := fmt.Sprintf("%s|%d|%s", window.ID, window.ProcessID, window.Title)
	if _, ok := s.handledWindows[key]; ok {
		return true
	}
	s.handledWindows[key] = struct{}{}
	return false
}

func (s *Module) alreadyHandled(pid int) bool {
	s.handledMu.Lock()
	defer s.handledMu.Unlock()

	if s.handledPIDs == nil {
		s.handledPIDs = make(map[int]struct{})
	}
	if _, ok := s.handledPIDs[pid]; ok {
		return true
	}
	s.handledPIDs[pid] = struct{}{}
	return false
}

func (s *Module) audit(message string) error {
	if s.auditLogger == nil {
		return nil
	}

	return s.auditLogger.Log(audit.Event{
		Time:    time.Now(),
		Message: message,
	})
}

func (s *Module) startComponent(ctx context.Context, name string, component lifecycle) error {
	if err := component.Start(ctx); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}

	s.started = append(s.started, component)
	return nil
}
