package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/service"
	serviceclient "github.com/ddrowsy/family-friend-release/internal/service/client"
)

const (
	BrowserAccessRequestsPath = "/api/browser/access-requests"
	BrowserAccessRequestPath  = BrowserAccessRequestsPath + "/{requestID}"
	BrowserDirectApprovalPath = "/api/browser/approval"

	maxBrowserApprovalRequestBytes = 64 << 10
)

// BrowserApprovalClient is the remote-service boundary used by the blocked-page bridge.
type BrowserApprovalClient interface {
	CreateBrowserAccessRequest(
		ctx context.Context,
		request service.DeviceBrowserAccessRequest,
	) (service.DeviceBrowserAccessStatus, error)
	BrowserAccessRequestStatus(
		ctx context.Context,
		requestID string,
	) (service.DeviceBrowserAccessStatus, error)
	ApproveBrowserTemporary(
		ctx context.Context,
		customerID service.CustomerID,
		profileID service.ProfileID,
		request service.BrowserApprovalRequest,
	) (service.BrowserApprovalResult, error)
	ApproveBrowserPermanent(
		ctx context.Context,
		customerID service.CustomerID,
		profileID service.ProfileID,
		request service.BrowserApprovalRequest,
	) (service.BrowserApprovalResult, error)
}

// BrowserApprovalIdentity is the local account/profile/device scope for one blocked page.
type BrowserApprovalIdentity struct {
	CustomerID service.CustomerID
	ProfileID  service.ProfileID
	DeviceID   service.DeviceID
}

// BrowserApprovalIdentityResolver resolves the current local approval scope.
type BrowserApprovalIdentityResolver interface {
	BrowserApprovalIdentity(ctx context.Context) (BrowserApprovalIdentity, error)
}

// BrowserApprovalIdentityResolverFunc adapts a function to BrowserApprovalIdentityResolver.
type BrowserApprovalIdentityResolverFunc func(context.Context) (BrowserApprovalIdentity, error)

// BrowserApprovalIdentity resolves the current local approval scope.
func (f BrowserApprovalIdentityResolverFunc) BrowserApprovalIdentity(
	ctx context.Context,
) (BrowserApprovalIdentity, error) {
	return f(ctx)
}

// BrowserApprovalAPI forwards blocked-page approval operations to the control service.
type BrowserApprovalAPI struct {
	client   BrowserApprovalClient
	identity BrowserApprovalIdentityResolver
}

// NewBrowserApprovalAPI creates the localhost blocked-page approval bridge.
func NewBrowserApprovalAPI(
	client BrowserApprovalClient,
	identity BrowserApprovalIdentityResolver,
) (*BrowserApprovalAPI, error) {
	if client == nil {
		return nil, errors.New("browser approval client is required")
	}
	if identity == nil {
		return nil, errors.New("browser approval identity resolver is required")
	}
	return &BrowserApprovalAPI{
		client:   client,
		identity: identity,
	}, nil
}

func (a *BrowserApprovalAPI) register(mux *http.ServeMux) {
	mux.HandleFunc("POST "+BrowserAccessRequestsPath, a.handleCreateRequest)
	mux.HandleFunc("GET "+BrowserAccessRequestPath, a.handleRequestStatus)
	mux.HandleFunc("POST "+BrowserDirectApprovalPath, a.handleDirectApproval)
}

type browserAccessCreateRequest struct {
	URL                  string `json:"url"`
	BrowserPolicyProfile string `json:"browser_profile"`
}

type browserAccessStatusResponse struct {
	RequestID string                            `json:"request_id"`
	State     service.BrowserAccessRequestState `json:"state"`
	ExpiresAt time.Time                         `json:"expires_at"`
}

type browserDirectApprovalRequest struct {
	URL                  string                        `json:"url"`
	BrowserPolicyProfile string                        `json:"browser_profile"`
	ApprovalCode         string                        `json:"approval_code"`
	Action               service.BrowserApprovalAction `json:"action"`
	DurationSeconds      int64                         `json:"duration_seconds,omitempty"`
}

type browserDirectApprovalResponse struct {
	Approved bool `json:"approved"`
}

func (a *BrowserApprovalAPI) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	var request browserAccessCreateRequest
	if err := decodeBrowserApprovalJSON(w, r, &request); err != nil {
		writeUIError(w, http.StatusBadRequest, "malformed request")
		return
	}
	request.URL = strings.TrimSpace(request.URL)
	request.BrowserPolicyProfile = strings.TrimSpace(request.BrowserPolicyProfile)
	if request.URL == "" || request.BrowserPolicyProfile == "" {
		writeUIError(w, http.StatusBadRequest, "url and browser profile are required")
		return
	}

	identity, err := a.resolveIdentity(r.Context())
	if err != nil {
		writeUIError(w, http.StatusServiceUnavailable, "local approval identity unavailable")
		return
	}
	status, err := a.client.CreateBrowserAccessRequest(
		r.Context(),
		service.DeviceBrowserAccessRequest{
			ProfileID:            identity.ProfileID,
			URL:                  request.URL,
			BrowserPolicyProfile: request.BrowserPolicyProfile,
		},
	)
	if err != nil {
		writeBrowserApprovalRemoteError(w, err, false)
		return
	}
	writeUIJSON(w, http.StatusCreated, newBrowserAccessStatusResponse(status))
}

func (a *BrowserApprovalAPI) handleRequestStatus(w http.ResponseWriter, r *http.Request) {
	if _, err := a.resolveIdentity(r.Context()); err != nil {
		writeUIError(w, http.StatusServiceUnavailable, "local approval identity unavailable")
		return
	}

	requestID := strings.TrimSpace(r.PathValue("requestID"))
	if requestID == "" {
		writeUIError(w, http.StatusBadRequest, "request ID is required")
		return
	}
	status, err := a.client.BrowserAccessRequestStatus(r.Context(), requestID)
	if err != nil {
		writeBrowserApprovalRemoteError(w, err, false)
		return
	}
	writeUIJSON(w, http.StatusOK, newBrowserAccessStatusResponse(status))
}

func (a *BrowserApprovalAPI) handleDirectApproval(w http.ResponseWriter, r *http.Request) {
	var request browserDirectApprovalRequest
	if err := decodeBrowserApprovalJSON(w, r, &request); err != nil {
		writeUIError(w, http.StatusBadRequest, "malformed request")
		return
	}
	request.URL = strings.TrimSpace(request.URL)
	request.BrowserPolicyProfile = strings.TrimSpace(request.BrowserPolicyProfile)
	request.ApprovalCode = strings.TrimSpace(request.ApprovalCode)
	if err := validateDirectApprovalRequest(request); err != nil {
		writeUIError(w, http.StatusBadRequest, "invalid approval request")
		return
	}

	identity, err := a.resolveIdentity(r.Context())
	if err != nil {
		writeUIError(w, http.StatusServiceUnavailable, "local approval identity unavailable")
		return
	}
	approval := service.BrowserApprovalRequest{
		DeviceID:             identity.DeviceID,
		URL:                  request.URL,
		BrowserPolicyProfile: request.BrowserPolicyProfile,
		ApprovalCode:         request.ApprovalCode,
		DurationSeconds:      request.DurationSeconds,
	}

	switch request.Action {
	case service.BrowserApprovalTemporary:
		_, err = a.client.ApproveBrowserTemporary(
			r.Context(),
			identity.CustomerID,
			identity.ProfileID,
			approval,
		)
	case service.BrowserApprovalPermanent:
		_, err = a.client.ApproveBrowserPermanent(
			r.Context(),
			identity.CustomerID,
			identity.ProfileID,
			approval,
		)
	}
	if err != nil {
		writeBrowserApprovalRemoteError(w, err, true)
		return
	}
	writeUIJSON(w, http.StatusOK, browserDirectApprovalResponse{Approved: true})
}

func (a *BrowserApprovalAPI) resolveIdentity(
	ctx context.Context,
) (BrowserApprovalIdentity, error) {
	identity, err := a.identity.BrowserApprovalIdentity(ctx)
	if err != nil {
		return BrowserApprovalIdentity{}, err
	}
	if err := identity.CustomerID.Validate(); err != nil {
		return BrowserApprovalIdentity{}, err
	}
	if err := identity.ProfileID.Validate(); err != nil {
		return BrowserApprovalIdentity{}, err
	}
	if err := identity.DeviceID.Validate(); err != nil {
		return BrowserApprovalIdentity{}, err
	}
	return identity, nil
}

func validateDirectApprovalRequest(request browserDirectApprovalRequest) error {
	hasURL := request.URL != ""
	hasBrowserProfile := request.BrowserPolicyProfile != ""
	hasApprovalCode := request.ApprovalCode != ""
	if !hasURL || !hasBrowserProfile || !hasApprovalCode {
		return errors.New("required approval field is empty")
	}
	switch request.Action {
	case service.BrowserApprovalTemporary:
		if request.DurationSeconds <= 0 {
			return errors.New("temporary approval duration must be positive")
		}
	case service.BrowserApprovalPermanent:
		if request.DurationSeconds != 0 {
			return errors.New("permanent approval cannot have a duration")
		}
	default:
		return errors.New("unsupported approval action")
	}
	return nil
}

func newBrowserAccessStatusResponse(
	status service.DeviceBrowserAccessStatus,
) browserAccessStatusResponse {
	return browserAccessStatusResponse{
		RequestID: status.RequestID,
		State:     status.State,
		ExpiresAt: status.ExpiresAt,
	}
}

func decodeBrowserApprovalJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBrowserApprovalRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeBrowserApprovalRemoteError(w http.ResponseWriter, err error, direct bool) {
	var responseErr *serviceclient.ResponseError
	if !errors.As(err, &responseErr) {
		writeUIError(w, http.StatusBadGateway, "control service unavailable")
		return
	}

	switch responseErr.StatusCode {
	case http.StatusBadRequest:
		writeUIError(w, http.StatusBadRequest, "invalid approval request")
	case http.StatusUnauthorized:
		if direct {
			writeUIError(w, http.StatusUnauthorized, "invalid parent code")
			return
		}
		writeUIError(w, http.StatusBadGateway, "control service unavailable")
	case http.StatusForbidden:
		writeUIError(w, http.StatusForbidden, "approval is not available for this profile")
	case http.StatusNotFound:
		writeUIError(w, http.StatusNotFound, "access request not found")
	case http.StatusConflict:
		writeUIError(w, http.StatusConflict, "access request state conflict")
	default:
		writeUIError(w, http.StatusBadGateway, "control service unavailable")
	}
}
