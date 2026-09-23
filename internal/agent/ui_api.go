package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/service"
	serviceclient "github.com/ddrowsy/family-friend-release/internal/service/client"
)

const (
	DefaultUIAPIAddress = "127.0.0.1:17654"
	UIStatusPath        = "/api/ui/status"
	UIPairingClaimPath  = "/api/ui/pairing/claim"
	UIPairingStatusPath = "/api/ui/pairing/status"
	UIPairingResetPath  = "/api/ui/pairing"
)

type UIServiceState string

type UIPairingState string

const (
	UIServiceUnknown      UIServiceState = "unknown"
	UIServiceConnected    UIServiceState = "connected"
	UIServiceDisconnected UIServiceState = "disconnected"

	UIPairingUnpaired            UIPairingState = "unpaired"
	UIPairingPendingConfirmation UIPairingState = "pending_confirmation"
	UIPairingConfirmed           UIPairingState = "confirmed"
	UIPairingCancelled           UIPairingState = "cancelled"
	UIPairingExpired             UIPairingState = "expired"
)

// PairingClient is the remote pairing boundary used by the local UI API.
type PairingClient interface {
	ClaimPairing(
		ctx context.Context,
		request service.DevicePairingClaimRequest,
	) (service.DevicePairingClaimResponse, error)
	PairingStatus(
		ctx context.Context,
		sessionID string,
		claimProof string,
	) (service.DevicePairingStatusResponse, error)
}

// DeviceIdentity contains display-safe local device metadata supplied by the agent.
type DeviceIdentity struct {
	DeviceID   service.DeviceID
	DeviceName string
	Platform   string
}

// UIStatus is the secret-free status snapshot returned to the desktop UI.
type UIStatus struct {
	AgentRunning      bool                `json:"agent_running"`
	ProtectionRunning bool                `json:"protection_running"`
	Service           UIServiceState      `json:"service"`
	Paired            bool                `json:"paired"`
	Pairing           UIPairingState      `json:"pairing"`
	DeviceID          service.DeviceID    `json:"device_id,omitempty"`
	DeviceName        string              `json:"device_name,omitempty"`
	Platform          string              `json:"platform,omitempty"`
	ProfileIDs        []service.ProfileID `json:"profile_ids"`
}

// UIPairingClaimRequest is the only pairing input accepted from the desktop UI.
type UIPairingClaimRequest struct {
	Code string `json:"code"`
}

// UIPairingStatus is the secret-free pairing state returned to the desktop UI.
type UIPairingStatus struct {
	State     UIPairingState `json:"state"`
	ExpiresAt time.Time      `json:"expires_at,omitempty"`
}

type pairingAttempt struct {
	sessionID  string
	claimProof string
	state      UIPairingState
	expiresAt  time.Time
}

// UIAPI exposes local status and pairing operations without exposing remote credentials.
type UIAPI struct {
	client   PairingClient
	identity DeviceIdentity

	mu           sync.Mutex
	serviceState UIServiceState
	paired       bool
	pairing      pairingAttempt
}

// NewUIAPI creates the local desktop UI HTTP API.
func NewUIAPI(client PairingClient, identity DeviceIdentity) (*UIAPI, error) {
	if err := identity.DeviceID.Validate(); err != nil {
		return nil, fmt.Errorf("validating UI API device identity: %w", err)
	}
	identity.DeviceName = strings.TrimSpace(identity.DeviceName)
	if identity.DeviceName == "" {
		return nil, errors.New("UI API device name is required")
	}
	identity.Platform = strings.TrimSpace(identity.Platform)
	if identity.Platform == "" {
		return nil, errors.New("UI API platform is required")
	}

	state := UIServiceDisconnected
	if client != nil {
		state = UIServiceUnknown
	}
	return &UIAPI{
		client:       client,
		identity:     identity,
		serviceState: state,
		pairing: pairingAttempt{
			state: UIPairingUnpaired,
		},
	}, nil
}

// Handler returns the loopback server handler used by the desktop UI.
func (a *UIAPI) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+UIStatusPath, a.handleStatus)
	mux.HandleFunc("POST "+UIPairingClaimPath, a.handlePairingClaim)
	mux.HandleFunc("GET "+UIPairingStatusPath, a.handlePairingStatus)
	mux.HandleFunc("DELETE "+UIPairingResetPath, a.handlePairingReset)
	return mux
}

func (a *UIAPI) handleStatus(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	status := UIStatus{
		AgentRunning:      true,
		ProtectionRunning: true,
		Service:           a.serviceState,
		Paired:            a.paired,
		Pairing:           a.pairing.state,
		DeviceID:          a.identity.DeviceID,
		DeviceName:        a.identity.DeviceName,
		Platform:          a.identity.Platform,
		ProfileIDs:        []service.ProfileID{},
	}
	a.mu.Unlock()
	writeUIJSON(w, http.StatusOK, status)
}

func (a *UIAPI) handlePairingClaim(w http.ResponseWriter, r *http.Request) {
	if a.client == nil {
		writeUIError(w, http.StatusServiceUnavailable, "control service unavailable")
		return
	}

	var request UIPairingClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeUIError(w, http.StatusBadRequest, "malformed request")
		return
	}
	request.Code = strings.TrimSpace(request.Code)
	if request.Code == "" {
		writeUIError(w, http.StatusBadRequest, "pairing code is required")
		return
	}

	response, err := a.client.ClaimPairing(r.Context(), service.DevicePairingClaimRequest{
		Code:       request.Code,
		DeviceID:   a.identity.DeviceID,
		DeviceName: a.identity.DeviceName,
		Platform:   a.identity.Platform,
	})
	if err != nil {
		a.setServiceState(UIServiceDisconnected)
		writeUIPairingRemoteError(w, err)
		return
	}

	a.mu.Lock()
	a.serviceState = UIServiceConnected
	a.paired = false
	a.pairing = pairingAttempt{
		sessionID:  response.SessionID,
		claimProof: response.ClaimProof,
		state:      UIPairingPendingConfirmation,
		expiresAt:  response.ExpiresAt,
	}
	result := UIPairingStatus{
		State:     a.pairing.state,
		ExpiresAt: a.pairing.expiresAt,
	}
	a.mu.Unlock()
	writeUIJSON(w, http.StatusOK, result)
}

func (a *UIAPI) handlePairingStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	attempt := a.pairing
	a.mu.Unlock()

	if attempt.claimProof == "" || attempt.sessionID == "" {
		writeUIJSON(w, http.StatusOK, UIPairingStatus{
			State:     attempt.state,
			ExpiresAt: attempt.expiresAt,
		})
		return
	}
	if a.client == nil {
		writeUIError(w, http.StatusServiceUnavailable, "control service unavailable")
		return
	}

	response, err := a.client.PairingStatus(r.Context(), attempt.sessionID, attempt.claimProof)
	if err != nil {
		a.setServiceState(UIServiceDisconnected)
		writeUIPairingRemoteError(w, err)
		return
	}

	state, err := uiPairingState(response.State)
	if err != nil {
		writeUIError(w, http.StatusBadGateway, "invalid control service pairing state")
		return
	}

	a.mu.Lock()
	a.serviceState = UIServiceConnected
	a.pairing.state = state
	if state == UIPairingConfirmed {
		a.paired = true
		a.pairing.claimProof = ""
	}
	if state == UIPairingCancelled || state == UIPairingExpired {
		a.paired = false
		a.pairing.claimProof = ""
	}
	result := UIPairingStatus{
		State:     a.pairing.state,
		ExpiresAt: a.pairing.expiresAt,
	}
	a.mu.Unlock()
	writeUIJSON(w, http.StatusOK, result)
}

func (a *UIAPI) handlePairingReset(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.paired {
		writeUIError(w, http.StatusConflict, "paired device cannot reset pairing locally")
		return
	}
	a.pairing = pairingAttempt{state: UIPairingUnpaired}
	w.WriteHeader(http.StatusNoContent)
}

func (a *UIAPI) setServiceState(state UIServiceState) {
	a.mu.Lock()
	a.serviceState = state
	a.mu.Unlock()
}

func uiPairingState(state service.PairingCompletionState) (UIPairingState, error) {
	switch state {
	case service.PairingCompletionPendingConfirmation:
		return UIPairingPendingConfirmation, nil
	case service.PairingCompletionConfirmed:
		return UIPairingConfirmed, nil
	case service.PairingCompletionCancelled:
		return UIPairingCancelled, nil
	case service.PairingCompletionExpired:
		return UIPairingExpired, nil
	default:
		return "", fmt.Errorf("unsupported pairing state %q", state)
	}
}

func writeUIPairingRemoteError(w http.ResponseWriter, err error) {
	var responseErr *serviceclient.ResponseError
	if errors.As(err, &responseErr) {
		switch responseErr.StatusCode {
		case http.StatusNotFound, http.StatusGone:
			writeUIError(w, http.StatusBadRequest, "pairing code or session is invalid or expired")
			return
		case http.StatusConflict:
			writeUIError(w, http.StatusConflict, "pairing state conflict")
			return
		}
	}
	writeUIError(w, http.StatusBadGateway, "control service unavailable")
}

func writeUIJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeUIError(w http.ResponseWriter, status int, message string) {
	writeUIJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

// UIAPIServer binds the desktop UI API to a loopback-only address.
type UIAPIServer struct {
	api     *UIAPI
	address string

	mu     sync.Mutex
	server *http.Server
}

// NewUIAPIServer creates a loopback-only server for the desktop UI API.
func NewUIAPIServer(api *UIAPI, address string) (*UIAPIServer, error) {
	if api == nil {
		return nil, errors.New("UI API is required")
	}
	if strings.TrimSpace(address) == "" {
		address = DefaultUIAPIAddress
	}
	if err := validateLoopbackAddress(address); err != nil {
		return nil, err
	}
	return &UIAPIServer{api: api, address: address}, nil
}

// Start starts the loopback UI API server.
func (s *UIAPIServer) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("starting UI API on %s: %w", s.address, err)
	}

	s.mu.Lock()
	if s.server != nil {
		s.mu.Unlock()
		_ = listener.Close()
		return errors.New("UI API server already started")
	}
	server := &http.Server{Handler: s.api.Handler()}
	s.server = server
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		_ = s.Stop(context.Background())
	}()
	go func() {
		_ = server.Serve(listener)
	}()
	return nil
}

// Stop stops the loopback UI API server.
func (s *UIAPIServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("stopping UI API: %w", err)
	}
	return nil
}

func validateLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid UI API address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("UI API address %q must use a loopback IP", address)
	}
	return nil
}
