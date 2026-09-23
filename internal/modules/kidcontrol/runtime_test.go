package kidcontrol

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/audit"
	"github.com/ddrowsy/family-friend-release/internal/platform"
	"github.com/ddrowsy/family-friend-release/internal/platform/windows"
	"github.com/ddrowsy/family-friend-release/internal/policy"
)

type fakePlatform struct {
	processes        []platform.ProcessInfo
	closedPID        int
	closedWindowID   string
	closeCount       int
	closeWindowCount int
	closeErr         error
	closeWindowErr   error
	mu               sync.Mutex
}

func (f *fakePlatform) ListProcesses(ctx context.Context) ([]platform.ProcessInfo, error) {
	return f.processes, nil
}

func (f *fakePlatform) ActiveWindow(ctx context.Context) (platform.WindowInfo, error) {
	return platform.WindowInfo{}, nil
}

func (f *fakePlatform) CaptureScreen(ctx context.Context) (platform.ScreenFrame, error) {
	return platform.ScreenFrame{}, nil
}

func (f *fakePlatform) CloseProcess(ctx context.Context, pid int) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closedPID = pid
	f.closeCount++
	return f.closeErr
}

func (f *fakePlatform) CloseWindow(ctx context.Context, windowID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closedWindowID = windowID
	f.closeWindowCount++
	return f.closeWindowErr
}

func (f *fakePlatform) ClosedPID() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.closedPID
}

func (f *fakePlatform) CloseCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.closeCount
}

func (f *fakePlatform) ClosedWindowID() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.closedWindowID
}

func (f *fakePlatform) CloseWindowCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.closeWindowCount
}

type fakeAuditLogger struct {
	events []audit.Event
}

func (f *fakeAuditLogger) Log(event audit.Event) error {
	f.events = append(f.events, event)
	return nil
}

func TestModuleStartsAppMonitorOnlyWhenEnabled(t *testing.T) {
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			AppControl: policy.AppControlSettings{
				Enabled:             true,
				ScanIntervalSeconds: DefaultAppControlScanIntervalSeconds,
			},
		},
	}, &fakePlatform{})

	if err := module.startRuntime(context.Background()); err != nil {
		t.Fatalf("startRuntime returned error: %v", err)
	}
	if len(module.started) != 1 || module.started[0] != module.appMonitor {
		t.Fatalf("started = %#v, want app monitor only", module.started)
	}
	if err := module.stopRuntime(context.Background()); err != nil {
		t.Fatalf("stopRuntime returned error: %v", err)
	}

	module = testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			AppControl: policy.AppControlSettings{Enabled: false},
		},
	}, &fakePlatform{})

	if err := module.startRuntime(context.Background()); err != nil {
		t.Fatalf("startRuntime returned error: %v", err)
	}
	if len(module.started) != 0 {
		t.Fatalf("started count = %d, want 0", len(module.started))
	}
}

func TestModuleStartsWindowMonitorOnlyWhenEnabled(t *testing.T) {
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			WindowMonitoring: policy.WindowMonitoringSettings{
				Enabled:             true,
				ScanIntervalSeconds: DefaultWindowMonitoringScanIntervalSeconds,
			},
		},
	}, &fakePlatform{})

	if err := module.startRuntime(context.Background()); err != nil {
		t.Fatalf("startRuntime returned error: %v", err)
	}
	if len(module.started) != 1 || module.started[0] != module.windowMonitor {
		t.Fatalf("started = %#v, want window monitor only", module.started)
	}
	if err := module.stopRuntime(context.Background()); err != nil {
		t.Fatalf("stopRuntime returned error: %v", err)
	}

	module = testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			WindowMonitoring: policy.WindowMonitoringSettings{Enabled: false},
		},
	}, &fakePlatform{})

	if err := module.startRuntime(context.Background()); err != nil {
		t.Fatalf("startRuntime returned error: %v", err)
	}
	if len(module.started) != 0 {
		t.Fatalf("started count = %d, want 0", len(module.started))
	}
}

func TestModuleStartsScreenMonitorOnlyWhenEnabled(t *testing.T) {
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			ScreenMonitoring: policy.ScreenMonitoringSettings{Enabled: true, SampleIntervalSeconds: 3},
		},
	}, &fakePlatform{})

	if err := module.startRuntime(context.Background()); err != nil {
		t.Fatalf("startRuntime returned error: %v", err)
	}
	if len(module.started) != 1 || module.started[0] != module.screenMonitor {
		t.Fatalf("started = %#v, want screen monitor only", module.started)
	}
	if err := module.stopRuntime(context.Background()); err != nil {
		t.Fatalf("stopRuntime returned error: %v", err)
	}

	module = testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			ScreenMonitoring: policy.ScreenMonitoringSettings{Enabled: false, SampleIntervalSeconds: 3},
		},
	}, &fakePlatform{})

	if err := module.startRuntime(context.Background()); err != nil {
		t.Fatalf("startRuntime returned error: %v", err)
	}
	if len(module.started) != 0 {
		t.Fatalf("started count = %d, want 0", len(module.started))
	}
}

func TestModuleStopsStartedComponents(t *testing.T) {
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			AppControl: policy.AppControlSettings{
				Enabled:             true,
				ScanIntervalSeconds: DefaultAppControlScanIntervalSeconds,
			},
			WindowMonitoring: policy.WindowMonitoringSettings{Enabled: false},
			ScreenMonitoring: policy.ScreenMonitoringSettings{Enabled: true, SampleIntervalSeconds: 3},
		},
	}, &fakePlatform{})

	if err := module.startRuntime(context.Background()); err != nil {
		t.Fatalf("startRuntime returned error: %v", err)
	}
	if len(module.started) != 2 {
		t.Fatalf("started count = %d, want 2", len(module.started))
	}

	if err := module.stopRuntime(context.Background()); err != nil {
		t.Fatalf("stopRuntime returned error: %v", err)
	}
	if len(module.started) != 0 {
		t.Fatalf("started count after stop = %d, want 0", len(module.started))
	}
}

func TestModuleMonitorModeDetectsBlockedAppWithoutClosingProcess(t *testing.T) {
	platformLayer := &fakePlatform{}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "monitor",
		Settings: policy.KidControlSettings{
			AppControl: policy.AppControlSettings{Enabled: true, BlockedApps: []string{"msedge.exe"}},
		},
	}, platformLayer)
	module.auditLogger = auditLogger
	if err := module.handleAppObservations(context.Background(), []AppObservation{{Process: platform.ProcessInfo{PID: 123, Name: "msedge.exe"}}}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if platformLayer.ClosedPID() != 0 {
		t.Fatalf("closedPID = %d, want 0", platformLayer.ClosedPID())
	}
	if len(auditLogger.events) != 3 {
		t.Fatalf("len(audit events) = %d, want 3", len(auditLogger.events))
	}
}

func TestModuleEnforceModeWithDryRunSkipsRealClose(t *testing.T) {
	platformLayer := &fakePlatform{}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			DryRun:     true,
			AppControl: policy.AppControlSettings{Enabled: true, BlockedApps: []string{"msedge.exe"}},
		},
	}, platformLayer)
	module.auditLogger = auditLogger
	if err := module.handleAppObservations(context.Background(), []AppObservation{{Process: platform.ProcessInfo{PID: 456, Name: "msedge.exe"}}}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if platformLayer.ClosedPID() != 0 {
		t.Fatalf("closedPID = %d, want 0", platformLayer.ClosedPID())
	}
	if len(auditLogger.events) != 3 {
		t.Fatalf("len(audit events) = %d, want 3", len(auditLogger.events))
	}
	if !auditContains(auditLogger.events, "dry-run close process: msedge.exe") {
		t.Fatalf("audit events missing dry-run close: %#v", auditLogger.events)
	}
}

func TestModuleEnforceModeWithDryRunFalseClosesBlockedProcess(t *testing.T) {
	platformLayer := &fakePlatform{}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			DryRun:     false,
			AppControl: policy.AppControlSettings{Enabled: true, BlockedApps: []string{"msedge.exe"}},
		},
	}, platformLayer)
	module.auditLogger = auditLogger
	if err := module.handleAppObservations(context.Background(), []AppObservation{{Process: platform.ProcessInfo{PID: 456, Name: "msedge.exe"}}}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if platformLayer.ClosedPID() != 456 {
		t.Fatalf("closedPID = %d, want 456", platformLayer.ClosedPID())
	}
	if len(auditLogger.events) != 3 {
		t.Fatalf("len(audit events) = %d, want 3", len(auditLogger.events))
	}
	if !auditContains(auditLogger.events, "action executed: close_process for msedge.exe") {
		t.Fatalf("audit events missing action executed: %#v", auditLogger.events)
	}
}

func TestModuleAuditsCloseFailure(t *testing.T) {
	platformLayer := &fakePlatform{closeErr: errors.New("close denied")}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			DryRun:     false,
			AppControl: policy.AppControlSettings{Enabled: true, BlockedApps: []string{"msedge.exe"}},
		},
	}, platformLayer)
	module.auditLogger = auditLogger

	err := module.handleAppObservations(context.Background(), []AppObservation{{
		Process: platform.ProcessInfo{PID: 456, Name: "msedge.exe"},
	}})
	if err == nil {
		t.Fatal("Start returned nil, want close error")
	}

	if platformLayer.CloseCount() != 1 {
		t.Fatalf("CloseCount = %d, want 1", platformLayer.CloseCount())
	}
	if !auditContains(auditLogger.events, "action failed: close_process for msedge.exe") {
		t.Fatalf("audit events missing action failed: %#v", auditLogger.events)
	}
}

func TestModuleAppControlFlowIgnoresUnblockedProcess(t *testing.T) {
	platformLayer := &fakePlatform{}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			DryRun:     false,
			AppControl: policy.AppControlSettings{Enabled: true, BlockedApps: []string{"msedge.exe"}},
		},
	}, platformLayer)
	module.auditLogger = auditLogger
	if err := module.handleAppObservations(context.Background(), []AppObservation{{Process: platform.ProcessInfo{PID: 789, Name: "notepad.exe"}}}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if platformLayer.ClosedPID() != 0 {
		t.Fatalf("closedPID = %d, want 0", platformLayer.ClosedPID())
	}
	if len(auditLogger.events) != 0 {
		t.Fatalf("len(audit events) = %d, want 0", len(auditLogger.events))
	}
}

func TestModuleDoesNotRepeatActionForSameProcessID(t *testing.T) {
	platformLayer := &fakePlatform{}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			DryRun:     false,
			AppControl: policy.AppControlSettings{Enabled: true, BlockedApps: []string{"msedge.exe"}},
		},
	}, platformLayer)
	module.auditLogger = auditLogger
	observations := []AppObservation{{Process: platform.ProcessInfo{PID: 456, Name: "msedge.exe"}}}
	if err := module.handleAppObservations(context.Background(), observations); err != nil {
		t.Fatalf("handleAppObservations returned error: %v", err)
	}
	if err := module.handleAppObservations(context.Background(), observations); err != nil {
		t.Fatalf("handleAppObservations returned error: %v", err)
	}

	if platformLayer.CloseCount() != 1 {
		t.Fatalf("CloseCount = %d, want 1", platformLayer.CloseCount())
	}
	if len(auditLogger.events) != 3 {
		t.Fatalf("len(audit events) = %d, want 3", len(auditLogger.events))
	}
}

func TestModuleMonitorModeSkipsWindowClose(t *testing.T) {
	platformLayer := &fakePlatform{}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "monitor",
		Settings: policy.KidControlSettings{
			AppControl: policy.AppControlSettings{Enabled: false, BlockedApps: []string{"msedge.exe"}},
			WindowMonitoring: policy.WindowMonitoringSettings{
				Enabled:              true,
				BlockedTitleKeywords: []string{"restricted"},
			},
		},
	}, platformLayer)
	module.auditLogger = auditLogger
	if err := module.handleWindowObservation(context.Background(), WindowObservation{Window: platform.WindowInfo{
		ID:          "window-1",
		Title:       "Restricted",
		ProcessID:   456,
		ProcessName: "msedge.exe",
	}}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if platformLayer.ClosedWindowID() != "" {
		t.Fatalf("ClosedWindowID = %q, want empty", platformLayer.ClosedWindowID())
	}
	if !auditContains(auditLogger.events, "action skipped in monitor mode: msedge.exe") {
		t.Fatalf("audit events missing monitor skip: %#v", auditLogger.events)
	}
}

func TestModuleEnforceDryRunSkipsWindowClose(t *testing.T) {
	platformLayer := &fakePlatform{}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			DryRun:     true,
			AppControl: policy.AppControlSettings{Enabled: false, BlockedApps: []string{"msedge.exe"}},
			WindowMonitoring: policy.WindowMonitoringSettings{
				Enabled:              true,
				BlockedTitleKeywords: []string{"restricted"},
			},
		},
	}, platformLayer)
	module.auditLogger = auditLogger
	if err := module.handleWindowObservation(context.Background(), WindowObservation{Window: platform.WindowInfo{
		ID:          "window-1",
		Title:       "Restricted",
		ProcessID:   456,
		ProcessName: "msedge.exe",
	}}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if platformLayer.ClosedWindowID() != "" {
		t.Fatalf("ClosedWindowID = %q, want empty", platformLayer.ClosedWindowID())
	}
	if !auditContains(auditLogger.events, "dry-run close window: Restricted") {
		t.Fatalf("audit events missing dry-run close window: %#v", auditLogger.events)
	}
}

func TestModuleActiveWindowBlockedAppTakesPriorityOverTitle(t *testing.T) {
	platformLayer := &fakePlatform{}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			DryRun:     true,
			AppControl: policy.AppControlSettings{Enabled: true, BlockedApps: []string{"msedge.exe"}},
			WindowMonitoring: policy.WindowMonitoringSettings{
				Enabled:              true,
				BlockedTitleKeywords: []string{"restricted"},
			},
		},
	}, platformLayer)
	module.auditLogger = auditLogger
	if err := module.handleWindowObservation(context.Background(), WindowObservation{Window: platform.WindowInfo{
		ID:          "window-1",
		Title:       "Restricted Content - Microsoft Edge",
		ProcessID:   456,
		ProcessName: "msedge.exe",
	}}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if !auditContains(auditLogger.events, "blocked app detected from active window: msedge.exe") {
		t.Fatalf("audit events missing active-window app block: %#v", auditLogger.events)
	}
	if auditContains(auditLogger.events, "blocked window title detected") {
		t.Fatalf("audit events should not include title block when app is blocked: %#v", auditLogger.events)
	}
	if !auditContains(auditLogger.events, "dry-run close window: msedge.exe") {
		t.Fatalf("audit events missing app-priority close-window dry-run: %#v", auditLogger.events)
	}
}

func TestModuleActiveWindowBlockedAppFallsBackToCloseProcess(t *testing.T) {
	platformLayer := &fakePlatform{}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			DryRun:     false,
			AppControl: policy.AppControlSettings{Enabled: true, BlockedApps: []string{"msedge.exe"}},
			WindowMonitoring: policy.WindowMonitoringSettings{
				Enabled:              true,
				BlockedTitleKeywords: []string{"restricted"},
			},
		},
	}, platformLayer)
	module.auditLogger = auditLogger
	if err := module.handleWindowObservation(context.Background(), WindowObservation{Window: platform.WindowInfo{
		Title:       "Example Search",
		ProcessID:   456,
		ProcessName: "msedge.exe",
	}}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if platformLayer.ClosedPID() != 456 {
		t.Fatalf("ClosedPID = %d, want 456", platformLayer.ClosedPID())
	}
	if !auditContains(auditLogger.events, "action executed: close_process for msedge.exe") {
		t.Fatalf("audit events missing close-process fallback: %#v", auditLogger.events)
	}
}

func TestModuleEnforceDryRunFalseClosesWindow(t *testing.T) {
	platformLayer := &fakePlatform{}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			DryRun:     false,
			AppControl: policy.AppControlSettings{Enabled: false, BlockedApps: []string{"msedge.exe"}},
			WindowMonitoring: policy.WindowMonitoringSettings{
				Enabled:              true,
				BlockedTitleKeywords: []string{"restricted"},
			},
		},
	}, platformLayer)
	module.auditLogger = auditLogger
	if err := module.handleWindowObservation(context.Background(), WindowObservation{Window: platform.WindowInfo{
		ID:          "window-1",
		Title:       "Restricted",
		ProcessID:   456,
		ProcessName: "msedge.exe",
	}}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if platformLayer.ClosedWindowID() != "window-1" {
		t.Fatalf("ClosedWindowID = %q, want window-1", platformLayer.ClosedWindowID())
	}
	if !auditContains(auditLogger.events, "action executed: close_window for Restricted") {
		t.Fatalf("audit events missing close-window success: %#v", auditLogger.events)
	}
}

func TestModuleAuditsCloseWindowFailure(t *testing.T) {
	platformLayer := &fakePlatform{closeWindowErr: errors.New("close window denied")}
	auditLogger := &fakeAuditLogger{}
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			DryRun:     false,
			AppControl: policy.AppControlSettings{Enabled: false, BlockedApps: []string{"msedge.exe"}},
			WindowMonitoring: policy.WindowMonitoringSettings{
				Enabled:              true,
				BlockedTitleKeywords: []string{"restricted"},
			},
		},
	}, platformLayer)
	module.auditLogger = auditLogger

	err := module.handleWindowObservation(context.Background(), WindowObservation{Window: platform.WindowInfo{
		ID:          "window-1",
		Title:       "Restricted",
		ProcessID:   456,
		ProcessName: "msedge.exe",
	}})
	if err == nil {
		t.Fatal("Start returned nil, want close window error")
	}
	if platformLayer.CloseWindowCount() != 1 {
		t.Fatalf("CloseWindowCount = %d, want 1", platformLayer.CloseWindowCount())
	}
	if !auditContains(auditLogger.events, "action failed: close_window for Restricted") {
		t.Fatalf("audit events missing close-window failure: %#v", auditLogger.events)
	}
}

func testModule(settings policy.KidControlSettings) *Module {
	logger := log.New(io.Discard, "", 0)
	module := New(windows.NewPlatform(logger), logger)
	module.configureRuntime(Config{Mode: "enforce", Settings: settings})
	return module
}

func testModuleWithPlatform(config Config, platformLayer platform.Platform) *Module {
	module := New(platformLayer, log.New(io.Discard, "", 0))
	module.configureRuntime(config)
	return module
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timed out waiting for condition")
}

func auditContains(events []audit.Event, text string) bool {
	for _, event := range events {
		if strings.Contains(event.Message, text) {
			return true
		}
	}
	return false
}
