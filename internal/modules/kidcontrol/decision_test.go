package kidcontrol

import (
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

func TestEvaluatorDetectsBlockedApp(t *testing.T) {
	evaluator := NewEvaluator([]string{"msedge.exe"})

	result := evaluator.EvaluateProcess(platform.ProcessInfo{PID: 42, Name: "msedge.exe"})

	if !result.Matched {
		t.Fatal("Matched = false, want true")
	}
	if result.Action != ActionCloseProcess {
		t.Fatalf("Action = %q, want %q", result.Action, ActionCloseProcess)
	}
	if result.ProcessID != 42 {
		t.Fatalf("ProcessID = %d, want 42", result.ProcessID)
	}
	if got, want := result.Reason, (DecisionReason{Type: ReasonBlockedApp, Value: "msedge.exe"}); got != want {
		t.Fatalf("Reason = %#v, want %#v", got, want)
	}
}

func TestEvaluatorIgnoresUnblockedApp(t *testing.T) {
	evaluator := NewEvaluator([]string{"msedge.exe"})

	result := evaluator.EvaluateProcess(platform.ProcessInfo{PID: 7, Name: "notepad.exe"})

	if result.Matched {
		t.Fatal("Matched = true, want false")
	}
	if result.Action != ActionNone {
		t.Fatalf("Action = %q, want %q", result.Action, ActionNone)
	}
}

func TestEvaluatorDetectsBlockedWindowTitleCaseInsensitive(t *testing.T) {
	evaluator := NewEvaluator(nil, []string{"inprivate"})

	result := evaluator.EvaluateWindow(platform.WindowInfo{
		ID:          "window-1",
		Title:       "InPrivate browsing - Microsoft Edge",
		ProcessID:   42,
		ProcessName: "notepad.exe",
	})

	if !result.Matched {
		t.Fatal("Matched = false, want true")
	}
	if result.Action != ActionCloseWindow {
		t.Fatalf("Action = %q, want %q", result.Action, ActionCloseWindow)
	}
	if result.WindowID != "window-1" {
		t.Fatalf("WindowID = %q, want window-1", result.WindowID)
	}
	if got, want := result.Reason, (DecisionReason{Type: ReasonBlockedWindowTitleKeyword, Value: "inprivate"}); got != want {
		t.Fatalf("Reason = %#v, want %#v", got, want)
	}
}

func TestEvaluatorDetectsBlockedWindowTitleContainsKeyword(t *testing.T) {
	evaluator := NewEvaluator(nil, []string{"private browsing"})

	result := evaluator.EvaluateWindow(platform.WindowInfo{
		ID:          "window-1",
		Title:       "Private browsing session - Microsoft Edge",
		ProcessID:   42,
		ProcessName: "notepad.exe",
	})

	if !result.Matched {
		t.Fatal("Matched = false, want true")
	}
	if got, want := result.Reason, (DecisionReason{Type: ReasonBlockedWindowTitleKeyword, Value: "private browsing"}); got != want {
		t.Fatalf("Reason = %#v, want %#v", got, want)
	}
}

func TestEvaluatorIgnoresWindowTitleWhenKeywordAbsent(t *testing.T) {
	evaluator := NewEvaluator(nil, []string{"inprivate"})

	result := evaluator.EvaluateWindow(platform.WindowInfo{
		ID:          "window-1",
		Title:       "YouTube - Microsoft Edge",
		ProcessID:   42,
		ProcessName: "notepad.exe",
	})

	if result.Matched {
		t.Fatal("Matched = true, want false")
	}
}

func TestEvaluatorWindowTitleDoesNotRequireExactFullTitle(t *testing.T) {
	evaluator := NewEvaluator(nil, []string{"restricted"})

	result := evaluator.EvaluateWindow(platform.WindowInfo{
		ID:          "window-1",
		Title:       "Restricted Content - Microsoft Edge",
		ProcessID:   42,
		ProcessName: "notepad.exe",
	})

	if !result.Matched {
		t.Fatal("Matched = false, want true")
	}
}

func TestEvaluatorDetectsBlockedWindowProcess(t *testing.T) {
	evaluator := NewEvaluator([]string{"msedge.exe"}, []string{"restricted"})

	result := evaluator.EvaluateActiveWindow(platform.WindowInfo{
		ID:          "window-1",
		Title:       "Example Search",
		ProcessID:   42,
		ProcessName: "msedge.exe",
	}, true)

	if !result.Matched {
		t.Fatal("Matched = false, want true")
	}
	if result.Action != ActionCloseWindow {
		t.Fatalf("Action = %q, want %q", result.Action, ActionCloseWindow)
	}
	if got, want := result.Reason, (DecisionReason{Type: ReasonBlockedApp, Value: "msedge.exe"}); got != want {
		t.Fatalf("Reason = %#v, want %#v", got, want)
	}
}

func TestEvaluatorActiveWindowBlockedAppIsCaseInsensitive(t *testing.T) {
	evaluator := NewEvaluator([]string{"msedge.exe"}, []string{"restricted"})

	result := evaluator.EvaluateActiveWindow(platform.WindowInfo{
		ID:          "window-1",
		Title:       "Example Search",
		ProcessID:   42,
		ProcessName: "MSEdge.exe",
	}, true)

	if !result.Matched {
		t.Fatal("Matched = false, want true")
	}
	if got, want := result.Reason, (DecisionReason{Type: ReasonBlockedApp, Value: "MSEdge.exe"}); got != want {
		t.Fatalf("Reason = %#v, want %#v", got, want)
	}
}

func TestEvaluatorSkipsTitleCheckWhenActiveWindowAppBlocked(t *testing.T) {
	evaluator := NewEvaluator([]string{"msedge.exe"}, []string{"restricted"})

	result := evaluator.EvaluateActiveWindow(platform.WindowInfo{
		ID:          "window-1",
		Title:       "Restricted Content - Microsoft Edge",
		ProcessID:   42,
		ProcessName: "msedge.exe",
	}, true)

	if got, want := result.Reason, (DecisionReason{Type: ReasonBlockedApp, Value: "msedge.exe"}); got != want {
		t.Fatalf("Reason = %#v, want %#v", got, want)
	}
}

func TestEvaluatorChecksTitleWhenActiveWindowAppCheckDisabled(t *testing.T) {
	evaluator := NewEvaluator([]string{"msedge.exe"}, []string{"restricted"})

	result := evaluator.EvaluateActiveWindow(platform.WindowInfo{
		ID:          "window-1",
		Title:       "Restricted Content - Microsoft Edge",
		ProcessID:   42,
		ProcessName: "msedge.exe",
	}, false)

	if got, want := result.Reason, (DecisionReason{Type: ReasonBlockedWindowTitleKeyword, Value: "restricted"}); got != want {
		t.Fatalf("Reason = %#v, want %#v", got, want)
	}
}

func TestEvaluatorBlockedActiveWindowAppFallsBackToCloseProcessWithoutWindowID(t *testing.T) {
	evaluator := NewEvaluator([]string{"msedge.exe"}, []string{"restricted"})

	result := evaluator.EvaluateActiveWindow(platform.WindowInfo{
		Title:       "Example Search",
		ProcessID:   42,
		ProcessName: "msedge.exe",
	}, true)

	if result.Action != ActionCloseProcess {
		t.Fatalf("Action = %q, want %q", result.Action, ActionCloseProcess)
	}
}
