package kidcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

const (
	defaultBrowserAPIAddress = "127.0.0.1:17653"
	browserProfileAPIPath    = "/api/browser/profile"
	browserCheckAPIPath      = "/api/browser/check"
	browserEndVisitAPIPath   = "/api/browser/end-visit"
)

type browserVisit struct {
	url       string
	profile   string
	startedAt time.Time
}

// BrowserMonitor tracks browser URL activity and applies browser profile rules.
type BrowserMonitor struct {
	module     *Module
	logger     *log.Logger
	apiAddress string

	mu      sync.Mutex
	running bool
	current *browserVisit
	server  *http.Server
}

// NewBrowserMonitor creates a browser monitor.
func NewBrowserMonitor(module *Module, logger *log.Logger) *BrowserMonitor {
	if logger == nil {
		logger = log.Default()
	}
	return &BrowserMonitor{
		module:     module,
		logger:     logger,
		apiAddress: defaultBrowserAPIAddress,
	}
}

// Start starts browser monitoring.
func (m *BrowserMonitor) Start(ctx context.Context) error {
	if m.module == nil {
		return errors.New("kidcontrol module is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return errors.New("browser monitor already started")
	}

	listener, err := net.Listen("tcp", m.apiAddress)
	if err != nil {
		return fmt.Errorf("start browser API on %s: %w", m.apiAddress, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+browserProfileAPIPath, m.handleProfile)
	mux.HandleFunc("POST "+browserCheckAPIPath, m.handleCheckURL)
	mux.HandleFunc("POST "+browserEndVisitAPIPath, m.handleEndVisit)

	server := &http.Server{Handler: mux}
	m.server = server
	m.running = true

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			m.logger.Printf("kidcontrol browser API stopped unexpectedly: %v", err)
		}
	}()

	m.logger.Printf("kidcontrol browser monitor started")
	return nil
}

// Stop stops browser monitoring and closes the current visit.
func (m *BrowserMonitor) Stop(ctx context.Context) error {
	return m.stopAt(ctx, time.Now())
}

func (m *BrowserMonitor) stopAt(ctx context.Context, now time.Time) error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	visit := m.current
	m.current = nil
	server := m.server
	m.server = nil
	m.mu.Unlock()

	if server != nil {
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("stop browser API: %w", err)
		}
	}
	if err := m.auditVisit(visit, now); err != nil {
		return err
	}
	m.logger.Printf("kidcontrol browser monitor stopped")
	return nil
}

func (m *BrowserMonitor) handleProfile(w http.ResponseWriter, r *http.Request) {
	name, profile, err := m.module.ResolveBrowserProfile(time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Name string `json:"name"`
		policy.BrowserProfile
	}{
		Name:           name,
		BrowserProfile: profile,
	})
}

func (m *BrowserMonitor) handleCheckURL(w http.ResponseWriter, r *http.Request) {
	var request struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(request.URL) == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	profileName, decision, err := m.handleURLAtWithProfile(r.Context(), request.URL, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Profile string `json:"profile"`
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}{
		Profile: profileName,
		Allowed: decision.Allowed,
		Reason:  decision.Reason,
	})
}

func (m *BrowserMonitor) handleEndVisit(w http.ResponseWriter, r *http.Request) {
	if err := m.EndVisit(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleURL evaluates an active browser URL and tracks allowed visit duration.
func (m *BrowserMonitor) HandleURL(ctx context.Context, rawURL string) (BrowserDecision, error) {
	return m.handleURLAt(ctx, rawURL, time.Now())
}

func (m *BrowserMonitor) handleURLAt(ctx context.Context, rawURL string, now time.Time) (BrowserDecision, error) {
	_, decision, err := m.handleURLAtWithProfile(ctx, rawURL, now)
	return decision, err
}

func (m *BrowserMonitor) handleURLAtWithProfile(ctx context.Context, rawURL string, now time.Time) (string, BrowserDecision, error) {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return "", BrowserDecision{}, errors.New("browser monitor is not started")
	}
	m.mu.Unlock()

	profileName, decision, err := m.module.CheckBrowserURL(rawURL, now)
	if err != nil {
		return "", BrowserDecision{}, err
	}

	if !decision.Allowed {
		if err := m.module.audit(fmt.Sprintf("browser blocked: %s profile=%s reason=%s", rawURL, profileName, decision.Reason)); err != nil {
			return "", BrowserDecision{}, err
		}
		return profileName, decision, nil
	}

	m.mu.Lock()
	current := m.current
	if current != nil && current.url == rawURL && current.profile == profileName {
		m.mu.Unlock()
		return profileName, decision, nil
	}
	m.current = &browserVisit{
		url:       rawURL,
		profile:   profileName,
		startedAt: now,
	}
	m.mu.Unlock()

	if err := m.auditVisit(current, now); err != nil {
		return "", BrowserDecision{}, err
	}
	return profileName, decision, nil
}

// EndVisit finishes the current allowed browser visit.
func (m *BrowserMonitor) EndVisit(ctx context.Context) error {
	return m.endVisitAt(ctx, time.Now())
}

func (m *BrowserMonitor) endVisitAt(ctx context.Context, now time.Time) error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return errors.New("browser monitor is not started")
	}
	visit := m.current
	m.current = nil
	m.mu.Unlock()

	return m.auditVisit(visit, now)
}

func (m *BrowserMonitor) auditVisit(visit *browserVisit, endedAt time.Time) error {
	if visit == nil {
		return nil
	}

	duration := endedAt.Sub(visit.startedAt)
	if duration < 0 {
		duration = 0
	}
	return m.module.audit(fmt.Sprintf(
		"browser visited: %s profile=%s duration=%s",
		visit.url,
		visit.profile,
		duration.Round(time.Second),
	))
}

// CheckBrowserURL resolves the current profile and evaluates one URL.
func (m *Module) CheckBrowserURL(rawURL string, now time.Time) (string, BrowserDecision, error) {
	profileName, profile, err := m.ResolveBrowserProfile(now)
	if err != nil {
		return "", BrowserDecision{}, err
	}
	return profileName, EvaluateBrowserURLAt(profile, rawURL, now), nil
}
