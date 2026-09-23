package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	deviceBrowserAccessRequestsRoute = APIV1Path + "/browser/access-requests"
	deviceBrowserAccessRequestRoute  = deviceBrowserAccessRequestsRoute + "/{requestID}"

	managementBrowserAccessRequestsRoute = APIV1Path + "/management/browser/access-requests"
	managementBrowserAccessRequestRoute  = managementBrowserAccessRequestsRoute + "/{requestID}"
	managementBrowserAccessApproveRoute  = managementBrowserAccessRequestRoute + "/approve"
	managementBrowserAccessRejectRoute   = managementBrowserAccessRequestRoute + "/reject"
)

type browserAccessRequestHTTPHandler struct {
	store *Store
}

type createBrowserAccessRequestHTTPBody struct {
	ProfileID            ProfileID `json:"profile_id"`
	URL                  string    `json:"url"`
	BrowserPolicyProfile string    `json:"browser_policy_profile"`
}

type browserAccessDecisionHTTPBody struct {
	Action          BrowserApprovalAction `json:"action"`
	DurationSeconds int64                 `json:"duration_seconds,omitempty"`
}

type browserAccessRequestStatusResponse struct {
	RequestID       string                    `json:"request_id"`
	State           BrowserAccessRequestState `json:"state"`
	ExpiresAt       time.Time                 `json:"expires_at"`
	Action          BrowserApprovalAction     `json:"action,omitempty"`
	DurationSeconds int64                     `json:"duration_seconds,omitempty"`
}

type managementBrowserAccessRequestResponse struct {
	RequestID            string                    `json:"request_id"`
	ProfileID            ProfileID                 `json:"profile_id"`
	DeviceID             DeviceID                  `json:"device_id"`
	URL                  string                    `json:"url"`
	Site                 string                    `json:"site"`
	BrowserPolicyProfile string                    `json:"browser_policy_profile"`
	State                BrowserAccessRequestState `json:"state"`
	Action               BrowserApprovalAction     `json:"action,omitempty"`
	DurationSeconds      int64                     `json:"duration_seconds,omitempty"`
	CreatedAt            time.Time                 `json:"created_at"`
	ExpiresAt            time.Time                 `json:"expires_at"`
}

func registerBrowserAccessRequestRoutes(mux *http.ServeMux, store *Store) {
	handler := &browserAccessRequestHTTPHandler{store: store}
	mux.HandleFunc("POST "+deviceBrowserAccessRequestsRoute, handler.create)
	mux.HandleFunc("GET "+deviceBrowserAccessRequestRoute, handler.deviceStatus)
	mux.HandleFunc("GET "+managementBrowserAccessRequestsRoute, handler.managementList)
	mux.HandleFunc("GET "+managementBrowserAccessRequestRoute, handler.managementGet)
	mux.HandleFunc("POST "+managementBrowserAccessApproveRoute, handler.managementApprove)
	mux.HandleFunc("POST "+managementBrowserAccessRejectRoute, handler.managementReject)
}

func (h *browserAccessRequestHTTPHandler) create(w http.ResponseWriter, r *http.Request) {
	caller, err := DeviceCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "device caller is required")
		return
	}

	var body createBrowserAccessRequestHTTPBody
	if err := decodeRequestJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	if err := body.ProfileID.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}
	body.BrowserPolicyProfile = strings.TrimSpace(body.BrowserPolicyProfile)
	if err := h.validateCreateRequest(r.Context(), caller, body); err != nil {
		writeBrowserAccessRequestError(w, err)
		return
	}

	request, err := h.store.CreateBrowserAccessRequest(
		r.Context(),
		caller.CustomerID,
		body.ProfileID,
		caller.DeviceID,
		body.URL,
		body.BrowserPolicyProfile,
	)
	if err != nil {
		writeBrowserAccessRequestError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newBrowserAccessRequestStatusResponse(request))
}

func (h *browserAccessRequestHTTPHandler) validateCreateRequest(
	ctx context.Context,
	caller DeviceCaller,
	body createBrowserAccessRequestHTTPBody,
) error {
	if body.BrowserPolicyProfile == "" {
		return ErrBrowserProfileNotFound
	}
	if _, err := normalizeBrowserApprovalURL(body.URL); err != nil {
		return err
	}
	if err := h.store.ValidateDeviceProfileAccess(
		ctx,
		caller.CustomerID,
		caller.DeviceID,
		body.ProfileID,
	); err != nil {
		return err
	}
	configuration, err := h.store.ProfileConfiguration(
		ctx,
		caller.CustomerID,
		body.ProfileID,
	)
	if err != nil {
		return err
	}
	_, _, err = browserProfileSettings(configuration.Policy, body.BrowserPolicyProfile)
	return err
}

func (h *browserAccessRequestHTTPHandler) deviceStatus(w http.ResponseWriter, r *http.Request) {
	caller, err := DeviceCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "device caller is required")
		return
	}

	request, err := h.store.browserAccessRequestForDevice(
		r.Context(),
		caller,
		r.PathValue("requestID"),
	)
	if err != nil {
		writeBrowserAccessRequestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newBrowserAccessRequestStatusResponse(request))
}

func (h *browserAccessRequestHTTPHandler) managementList(w http.ResponseWriter, r *http.Request) {
	caller, err := ManagementCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "management caller is required")
		return
	}

	requests, err := h.store.PendingBrowserAccessRequests(r.Context(), caller.CustomerID)
	if err != nil {
		writeBrowserAccessRequestError(w, err)
		return
	}
	responses := make([]managementBrowserAccessRequestResponse, 0, len(requests))
	for _, request := range requests {
		responses = append(responses, newManagementBrowserAccessRequestResponse(request))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *browserAccessRequestHTTPHandler) managementGet(w http.ResponseWriter, r *http.Request) {
	caller, err := ManagementCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "management caller is required")
		return
	}

	request, err := h.store.browserAccessRequestForParent(
		r.Context(),
		caller.CustomerID,
		r.PathValue("requestID"),
	)
	if err != nil {
		writeBrowserAccessRequestError(w, err)
		return
	}
	request, err = h.store.expireBrowserAccessRequest(r.Context(), request)
	if err != nil {
		writeBrowserAccessRequestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newManagementBrowserAccessRequestResponse(request))
}

func (h *browserAccessRequestHTTPHandler) managementApprove(w http.ResponseWriter, r *http.Request) {
	caller, err := ManagementCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "management caller is required")
		return
	}

	var body browserAccessDecisionHTTPBody
	if err := decodeRequestJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	decision := BrowserAccessDecision{
		Action:          body.Action,
		DurationSeconds: body.DurationSeconds,
	}
	if err := validateBrowserAccessHTTPDecision(decision); err != nil {
		writeBrowserAccessRequestError(w, err)
		return
	}

	request, err := h.store.ApproveBrowserAccessRequest(
		r.Context(),
		caller.CustomerID,
		r.PathValue("requestID"),
		decision,
	)
	if err != nil {
		writeBrowserAccessRequestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newManagementBrowserAccessRequestResponse(request))
}

func (h *browserAccessRequestHTTPHandler) managementReject(w http.ResponseWriter, r *http.Request) {
	caller, err := ManagementCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "management caller is required")
		return
	}

	request, err := h.store.RejectBrowserAccessRequest(
		r.Context(),
		caller.CustomerID,
		r.PathValue("requestID"),
	)
	if err != nil {
		writeBrowserAccessRequestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newManagementBrowserAccessRequestResponse(request))
}

func validateBrowserAccessHTTPDecision(decision BrowserAccessDecision) error {
	if err := validateBrowserAccessDecision(decision); err != nil {
		return err
	}
	if decision.Action == BrowserApprovalTemporary &&
		decision.DurationSeconds > maxTemporaryBrowserApprovalDurationSeconds {
		return ErrInvalidApprovalDuration
	}
	return nil
}

func newBrowserAccessRequestStatusResponse(
	request BrowserAccessRequest,
) browserAccessRequestStatusResponse {
	return browserAccessRequestStatusResponse{
		RequestID:       request.ID,
		State:           request.State,
		ExpiresAt:       request.ExpiresAt,
		Action:          request.DecisionAction,
		DurationSeconds: request.DurationSeconds,
	}
}

func newManagementBrowserAccessRequestResponse(
	request BrowserAccessRequest,
) managementBrowserAccessRequestResponse {
	return managementBrowserAccessRequestResponse{
		RequestID:            request.ID,
		ProfileID:            request.ProfileID,
		DeviceID:             request.SourceDeviceID,
		URL:                  request.URL,
		Site:                 request.Site,
		BrowserPolicyProfile: request.BrowserPolicyProfile,
		State:                request.State,
		Action:               request.DecisionAction,
		DurationSeconds:      request.DurationSeconds,
		CreatedAt:            request.CreatedAt,
		ExpiresAt:            request.ExpiresAt,
	}
}

func writeBrowserAccessRequestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "access request not found")
	case errors.Is(err, ErrDeviceProfileNotLinked):
		writeError(w, http.StatusForbidden, "device is not linked to profile")
	case errors.Is(err, ErrBrowserProfileNotFound):
		writeError(w, http.StatusNotFound, "browser policy profile not found")
	case errors.Is(err, ErrInvalidBrowserURL):
		writeError(w, http.StatusBadRequest, "invalid browser URL")
	case errors.Is(err, ErrInvalidApprovalAction):
		writeError(w, http.StatusBadRequest, "invalid approval action")
	case errors.Is(err, ErrInvalidApprovalDuration):
		writeError(w, http.StatusBadRequest, "invalid approval duration")
	case errors.Is(err, ErrBrowserAccessRequestState):
		writeError(w, http.StatusConflict, "access request state conflict")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
