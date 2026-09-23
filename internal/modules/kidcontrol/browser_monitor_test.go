package kidcontrol

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

func TestBrowserMonitorUsesCurrentProfileAndAuditsVisitDuration(t *testing.T) {
	module := testBrowserRuntimeModule()
	auditLogger := &fakeAuditLogger{}
	module.auditLogger = auditLogger
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))
	startTestBrowserMonitor(t, monitor)

	start := time.Unix(100, 0)
	decision, err := monitor.handleURLAt(context.Background(), "https://wikipedia.org/wiki/Go", start)
	if err != nil {
		t.Fatalf("handleURLAt returned error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("decision = %#v, want allowed", decision)
	}

	if _, err := monitor.handleURLAt(context.Background(), "https://en.wikipedia.org/wiki/Mathematics", start.Add(5*time.Minute)); err != nil {
		t.Fatalf("second handleURLAt returned error: %v", err)
	}

	if !auditContainsText(auditLogger, "browser visited: https://wikipedia.org/wiki/Go profile=study duration=5m0s") {
		t.Fatalf("audit events = %#v, want completed wikipedia visit", auditLogger.events)
	}
}

func TestBrowserMonitorBlockedURLAuditsAndKeepsCurrentVisit(t *testing.T) {
	module := testBrowserRuntimeModule()
	auditLogger := &fakeAuditLogger{}
	module.auditLogger = auditLogger
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))
	startTestBrowserMonitor(t, monitor)

	start := time.Unix(100, 0)
	if _, err := monitor.handleURLAt(context.Background(), "https://wikipedia.org", start); err != nil {
		t.Fatalf("allowed handleURLAt returned error: %v", err)
	}

	decision, err := monitor.handleURLAt(context.Background(), "https://tiktok.com", start.Add(time.Minute))
	if err != nil {
		t.Fatalf("blocked handleURLAt returned error: %v", err)
	}
	if decision.Allowed || decision.Reason != BrowserReasonNotInAllowList {
		t.Fatalf("decision = %#v, want not_in_allow_list block", decision)
	}
	if !auditContainsText(auditLogger, "browser blocked: https://tiktok.com profile=study reason=not_in_allow_list") {
		t.Fatalf("audit events = %#v, want blocked audit", auditLogger.events)
	}

	if err := monitor.endVisitAt(context.Background(), start.Add(3*time.Minute)); err != nil {
		t.Fatalf("endVisitAt returned error: %v", err)
	}
	if !auditContainsText(auditLogger, "browser visited: https://wikipedia.org profile=study duration=3m0s") {
		t.Fatalf("audit events = %#v, want wikipedia duration to continue through blocked attempt", auditLogger.events)
	}
}

func TestBrowserMonitorUsesUpdatedProfileWithoutRestart(t *testing.T) {
	module := testBrowserRuntimeModule()
	module.auditLogger = &fakeAuditLogger{}
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))
	startTestBrowserMonitor(t, monitor)

	now := time.Unix(100, 0)
	if err := module.SetBrowserProfile("study", policy.BrowserProfile{
		Policy:      policy.BrowserPolicyAllowList,
		AllowedURLs: []string{"khanacademy.org"},
	}); err != nil {
		t.Fatalf("SetBrowserProfile returned error: %v", err)
	}

	decision, err := monitor.handleURLAt(context.Background(), "https://khanacademy.org/math", now)
	if err != nil {
		t.Fatalf("handleURLAt returned error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("decision = %#v, want allowed from updated profile", decision)
	}
}

func TestBrowserMonitorBlocksInvalidURL(t *testing.T) {
	module := testBrowserRuntimeModule()
	auditLogger := &fakeAuditLogger{}
	module.auditLogger = auditLogger
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))
	startTestBrowserMonitor(t, monitor)

	decision, err := monitor.handleURLAt(context.Background(), "://bad-url", time.Unix(100, 0))
	if err != nil {
		t.Fatalf("handleURLAt returned error: %v", err)
	}
	if decision.Allowed || decision.Reason != BrowserReasonInvalidURL {
		t.Fatalf("decision = %#v, want invalid_url block", decision)
	}
}

func TestBrowserMonitorCheckAPIAllowed(t *testing.T) {
	module := testBrowserRuntimeModule()
	module.auditLogger = &fakeAuditLogger{}
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))
	startTestBrowserMonitor(t, monitor)

	request := httptest.NewRequest(http.MethodPost, browserCheckAPIPath, strings.NewReader(`{"url":"https://wikipedia.org/wiki/Go"}`))
	response := httptest.NewRecorder()
	monitor.handleCheckURL(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var got struct {
		Profile string `json:"profile"`
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Profile != "study" || !got.Allowed || got.Reason != BrowserReasonAllowedByAllowList {
		t.Fatalf("response = %#v, want allowed study decision", got)
	}
}

func TestBrowserMonitorCheckAPIBlocked(t *testing.T) {
	module := testBrowserRuntimeModule()
	auditLogger := &fakeAuditLogger{}
	module.auditLogger = auditLogger
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))
	startTestBrowserMonitor(t, monitor)

	request := httptest.NewRequest(http.MethodPost, browserCheckAPIPath, strings.NewReader(`{"url":"https://tiktok.com"}`))
	response := httptest.NewRecorder()
	monitor.handleCheckURL(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var got struct {
		Profile string `json:"profile"`
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Profile != "study" || got.Allowed || got.Reason != BrowserReasonNotInAllowList {
		t.Fatalf("response = %#v, want blocked study decision", got)
	}
	if !auditContainsText(auditLogger, "browser blocked: https://tiktok.com profile=study reason=not_in_allow_list") {
		t.Fatalf("audit events = %#v, want blocked audit", auditLogger.events)
	}
}

func TestBrowserMonitorCheckAPIRejectsBadRequest(t *testing.T) {
	module := testBrowserRuntimeModule()
	module.auditLogger = &fakeAuditLogger{}
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))
	startTestBrowserMonitor(t, monitor)

	for _, body := range []string{
		"{",
		`{}`,
		`{"url":""}`,
		`{"url":"   "}`,
	} {
		request := httptest.NewRequest(http.MethodPost, browserCheckAPIPath, strings.NewReader(body))
		response := httptest.NewRecorder()
		monitor.handleCheckURL(response, request)

		if response.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want %d", body, response.Code, http.StatusBadRequest)
		}
	}
}

func TestBrowserMonitorCheckAPIInvalidURLIsDecision(t *testing.T) {
	module := testBrowserRuntimeModule()
	module.auditLogger = &fakeAuditLogger{}
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))
	startTestBrowserMonitor(t, monitor)

	request := httptest.NewRequest(http.MethodPost, browserCheckAPIPath, strings.NewReader(`{"url":"://bad-url"}`))
	response := httptest.NewRecorder()
	monitor.handleCheckURL(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var got struct {
		Profile string `json:"profile"`
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Profile != "study" || got.Allowed || got.Reason != BrowserReasonInvalidURL {
		t.Fatalf("response = %#v, want invalid_url block decision", got)
	}
}

func TestBrowserMonitorEndVisitAPIReturnsNoContentAndAuditsVisit(t *testing.T) {
	module := testBrowserRuntimeModule()
	auditLogger := &fakeAuditLogger{}
	module.auditLogger = auditLogger
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))
	startTestBrowserMonitor(t, monitor)

	if _, err := monitor.HandleURL(context.Background(), "https://wikipedia.org"); err != nil {
		t.Fatalf("HandleURL returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, browserEndVisitAPIPath, nil)
	response := httptest.NewRecorder()
	monitor.handleEndVisit(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if !auditContainsText(auditLogger, "browser visited: https://wikipedia.org profile=study duration=") {
		t.Fatalf("audit events = %#v, want completed visit audit", auditLogger.events)
	}
}

func TestBrowserMonitorEndVisitAPIIsIdempotentWithoutCurrentVisit(t *testing.T) {
	module := testBrowserRuntimeModule()
	module.auditLogger = &fakeAuditLogger{}
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))
	startTestBrowserMonitor(t, monitor)

	for i := 0; i < 2; i++ {
		request := httptest.NewRequest(http.MethodPost, browserEndVisitAPIPath, nil)
		response := httptest.NewRecorder()
		monitor.handleEndVisit(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("call %d: status = %d, want %d", i+1, response.Code, http.StatusNoContent)
		}
	}
}

func TestBrowserMonitorEndVisitAPIReturnsServerErrorWhenMonitorStopped(t *testing.T) {
	module := testBrowserRuntimeModule()
	module.auditLogger = &fakeAuditLogger{}
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))

	request := httptest.NewRequest(http.MethodPost, browserEndVisitAPIPath, nil)
	response := httptest.NewRecorder()
	monitor.handleEndVisit(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func TestBrowserMonitorProfileAPIReturnsCurrentProfile(t *testing.T) {
	module := testBrowserModule()
	if err := module.activateBrowserProfile("study", 15*time.Minute, "default", time.Now()); err != nil {
		t.Fatalf("activateBrowserProfile returned error: %v", err)
	}
	monitor := NewBrowserMonitor(module, log.New(io.Discard, "", 0))

	request := httptest.NewRequest(http.MethodGet, browserProfileAPIPath, nil)
	response := httptest.NewRecorder()
	monitor.handleProfile(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var got struct {
		Name string `json:"name"`
		policy.BrowserProfile
	}
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "study" {
		t.Fatalf("profile name = %q, want study", got.Name)
	}
	if got.Policy != policy.BrowserPolicyAllowList {
		t.Fatalf("policy = %q, want %q", got.Policy, policy.BrowserPolicyAllowList)
	}
	if len(got.AllowedURLs) != 1 || got.AllowedURLs[0] != "wikipedia.org" {
		t.Fatalf("allowed URLs = %#v, want wikipedia.org", got.AllowedURLs)
	}
}

func TestModuleStartsBrowserMonitorWhenEnabled(t *testing.T) {
	module := testModuleWithPlatform(Config{
		Mode: "enforce",
		Settings: policy.KidControlSettings{
			BrowserMonitoring: policy.BrowserMonitoringSettings{
				Enabled:        true,
				DefaultProfile: "default",
				Profiles: map[string]policy.BrowserProfile{
					"default": {Policy: policy.BrowserPolicyBlockList},
				},
			},
		},
	}, &fakePlatform{})

	module.browserMonitor.apiAddress = "127.0.0.1:0"
	if err := module.startRuntime(context.Background()); err != nil {
		t.Fatalf("startRuntime returned error: %v", err)
	}
	if len(module.started) != 1 || module.started[0] != module.browserMonitor {
		t.Fatalf("started = %#v, want browser monitor only", module.started)
	}
	if err := module.stopRuntime(context.Background()); err != nil {
		t.Fatalf("stopRuntime returned error: %v", err)
	}
}

func startTestBrowserMonitor(t *testing.T, monitor *BrowserMonitor) {
	t.Helper()

	monitor.apiAddress = "127.0.0.1:0"
	if err := monitor.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := monitor.Stop(context.Background()); err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
	})
}

func testBrowserRuntimeModule() *Module {
	module := testBrowserModule()
	module.logger = log.New(io.Discard, "", 0)
	module.auditLogger = &fakeAuditLogger{}
	if err := module.activateBrowserProfile("study", 0, "default", time.Unix(100, 0)); err != nil {
		panic(err)
	}
	return module
}

func auditContainsText(logger *fakeAuditLogger, text string) bool {
	for _, event := range logger.events {
		if strings.Contains(event.Message, text) {
			return true
		}
	}
	return false
}
