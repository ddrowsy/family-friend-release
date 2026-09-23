package kidcontrol

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/mockdata"
	"github.com/ddrowsy/family-friend-release/internal/policy"
)

func TestMockDataBrowserAPIsUseProductionContracts(t *testing.T) {
	policyConfig, err := mockdata.Policy()
	if err != nil {
		t.Fatalf("mockdata.Policy returned error: %v", err)
	}
	modulePolicy := policyConfig.Modules[policy.KidControlModuleName]
	settings, err := ParseSettings(modulePolicy.Settings)
	if err != nil {
		t.Fatalf("ParseSettings returned error: %v", err)
	}
	if err := validateSettings(settings); err != nil {
		t.Fatalf("validateSettings returned error: %v", err)
	}

	logger := log.New(io.Discard, "", 0)
	module := New(nil, logger)
	module.config = Config{
		Mode:     modulePolicy.Mode,
		Settings: settings,
	}
	monitor := NewBrowserMonitor(module, logger)
	startTestBrowserMonitor(t, monitor)

	profileRequest := httptest.NewRequest(http.MethodGet, browserProfileAPIPath, nil)
	profileResponse := httptest.NewRecorder()
	monitor.handleProfile(profileResponse, profileRequest)

	if profileResponse.Code != http.StatusOK {
		t.Fatalf("profile status = %d, want %d", profileResponse.Code, http.StatusOK)
	}

	var profile struct {
		Name string `json:"name"`
		policy.BrowserProfile
	}
	if err := json.NewDecoder(profileResponse.Body).Decode(&profile); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}
	if profile.Name != "default" {
		t.Fatalf("profile name = %q, want default", profile.Name)
	}
	if profile.Policy != policy.BrowserPolicyBlockList {
		t.Fatalf("profile policy = %q, want %q", profile.Policy, policy.BrowserPolicyBlockList)
	}
	if len(profile.TemporaryAllowedURLs) != 1 {
		t.Fatalf("temporary approvals = %d, want 1", len(profile.TemporaryAllowedURLs))
	}

	checkRequest := httptest.NewRequest(
		http.MethodPost,
		browserCheckAPIPath,
		strings.NewReader(`{"url":"https://facebook.com/school/home"}`),
	)
	checkResponse := httptest.NewRecorder()
	monitor.handleCheckURL(checkResponse, checkRequest)

	if checkResponse.Code != http.StatusOK {
		t.Fatalf("check status = %d, want %d", checkResponse.Code, http.StatusOK)
	}

	var decision struct {
		Profile string `json:"profile"`
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(checkResponse.Body).Decode(&decision); err != nil {
		t.Fatalf("decode check response: %v", err)
	}
	if decision.Profile != "default" {
		t.Fatalf("decision profile = %q, want default", decision.Profile)
	}
	if !decision.Allowed || decision.Reason != BrowserReasonAllowedByTemporaryApproval {
		t.Fatalf("decision = %#v, want temporary approval", decision)
	}
}
