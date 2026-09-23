package kidcontrol

import (
	"strings"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

const (
	// ActionNone means no action is needed.
	ActionNone = ""
	// ActionCloseProcess closes a blocked process.
	ActionCloseProcess = "close_process"
	// ActionCloseWindow closes a blocked window.
	ActionCloseWindow = "close_window"
)

const (
	// ReasonBlockedApp means a process matched blocked_apps.
	ReasonBlockedApp = "blocked_app"
	// ReasonBlockedWindowTitleKeyword means an active window title matched a blocked keyword.
	ReasonBlockedWindowTitleKeyword = "blocked_window_title_keyword"
)

// DecisionReason describes the specific value that caused a block decision.
type DecisionReason struct {
	Type  string
	Value string
}

// Decision describes the result of evaluating a process.
type Decision struct {
	Matched   bool
	Action    string
	Reason    DecisionReason
	ProcessID int
	AppName   string
	WindowID  string
	Title     string
}

type titleKeyword struct {
	value      string
	normalized string
}

// Evaluator checks processes against kidcontrol policy settings.
type Evaluator struct {
	blockedApps          map[string]struct{}
	blockedTitleKeywords []titleKeyword
}

// NewEvaluator creates an evaluator for blocked app names.
func NewEvaluator(blockedApps []string, blockedTitleKeywords ...[]string) Evaluator {
	blocked := make(map[string]struct{}, len(blockedApps))
	for _, app := range blockedApps {
		name := strings.ToLower(strings.TrimSpace(app))
		if name != "" {
			blocked[name] = struct{}{}
		}
	}

	var keywords []titleKeyword
	if len(blockedTitleKeywords) > 0 {
		for _, keyword := range blockedTitleKeywords[0] {
			value := strings.TrimSpace(keyword)
			normalized := strings.ToLower(value)
			if normalized != "" {
				keywords = append(keywords, titleKeyword{
					value:      value,
					normalized: normalized,
				})
			}
		}
	}

	return Evaluator{
		blockedApps:          blocked,
		blockedTitleKeywords: keywords,
	}
}

// EvaluateProcess checks whether process matches a blocked app.
func (e Evaluator) EvaluateProcess(process platform.ProcessInfo) Decision {
	name := strings.ToLower(strings.TrimSpace(process.Name))
	if _, ok := e.blockedApps[name]; !ok {
		return Decision{
			Matched:   false,
			Action:    ActionNone,
			ProcessID: process.PID,
			AppName:   process.Name,
		}
	}

	return Decision{
		Matched:   true,
		Action:    ActionCloseProcess,
		Reason:    DecisionReason{Type: ReasonBlockedApp, Value: process.Name},
		ProcessID: process.PID,
		AppName:   process.Name,
	}
}

// EvaluateWindow checks whether active window matches blocked apps or title keywords.
func (e Evaluator) EvaluateWindow(window platform.WindowInfo) Decision {
	return e.EvaluateActiveWindow(window, true)
}

// EvaluateActiveWindow checks active window with app blocks taking priority over title blocks.
func (e Evaluator) EvaluateActiveWindow(window platform.WindowInfo, checkBlockedApps bool) Decision {
	processName := strings.ToLower(strings.TrimSpace(window.ProcessName))
	if checkBlockedApps {
		if _, ok := e.blockedApps[processName]; ok {
			action := ActionCloseWindow
			if strings.TrimSpace(window.ID) == "" && window.ProcessID > 0 {
				action = ActionCloseProcess
			}
			return Decision{
				Matched:   true,
				Action:    action,
				Reason:    DecisionReason{Type: ReasonBlockedApp, Value: window.ProcessName},
				ProcessID: window.ProcessID,
				AppName:   window.ProcessName,
				WindowID:  window.ID,
				Title:     window.Title,
			}
		}
	}

	title := strings.ToLower(window.Title)
	for _, keyword := range e.blockedTitleKeywords {
		if strings.Contains(title, keyword.normalized) {
			return Decision{
				Matched:   true,
				Action:    ActionCloseWindow,
				Reason:    DecisionReason{Type: ReasonBlockedWindowTitleKeyword, Value: keyword.value},
				ProcessID: window.ProcessID,
				AppName:   window.ProcessName,
				WindowID:  window.ID,
				Title:     window.Title,
			}
		}
	}

	return Decision{
		Matched:   false,
		Action:    ActionNone,
		ProcessID: window.ProcessID,
		AppName:   window.ProcessName,
		WindowID:  window.ID,
		Title:     window.Title,
	}
}
