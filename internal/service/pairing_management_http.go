package service

import (
	"errors"
	"net/http"
	"time"
)

const (
	managementPairingRoute          = APIV1Path + "/pairing"
	managementPairingHeartbeatRoute = managementPairingRoute +
		"/{sessionID}/heartbeat"
	managementPairingConfirmRoute = managementPairingRoute +
		"/{sessionID}/confirm"
	managementPairingCancelRoute = managementPairingRoute +
		"/{sessionID}/cancel"
)

type managementPairingHTTPHandler struct {
	service *managementPairingService
}

// ManagementPairingResponse describes management pairing create and heartbeat state.
type ManagementPairingResponse struct {
	SessionID string                `json:"session_id"`
	State     PairingSessionState   `json:"state"`
	Code      string                `json:"code,omitempty"`
	ExpiresAt time.Time             `json:"expires_at"`
	Device    *PairingPendingDevice `json:"device,omitempty"`
}

// PairingPendingDevice is the device waiting for parent confirmation.
type PairingPendingDevice struct {
	DeviceID   DeviceID `json:"device_id"`
	DeviceName string   `json:"device_name"`
	Platform   string   `json:"platform"`
}

func registerManagementPairingRoutes(mux *http.ServeMux, store *Store) {
	handler := &managementPairingHTTPHandler{
		service: newManagementPairingService(store),
	}
	mux.HandleFunc("POST "+managementPairingRoute, handler.create)
	mux.HandleFunc(
		"POST "+managementPairingHeartbeatRoute,
		handler.heartbeat,
	)
	mux.HandleFunc("POST "+managementPairingConfirmRoute, handler.confirm)
	mux.HandleFunc("POST "+managementPairingCancelRoute, handler.cancel)
}

func (h *managementPairingHTTPHandler) create(
	w http.ResponseWriter,
	r *http.Request,
) {
	caller, err := ManagementCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "management caller is required")
		return
	}

	session, err := h.service.create(r.Context(), caller.CustomerID)
	if err != nil {
		writeManagementPairingError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newManagementPairingResponse(session))
}

func (h *managementPairingHTTPHandler) heartbeat(
	w http.ResponseWriter,
	r *http.Request,
) {
	caller, err := ManagementCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "management caller is required")
		return
	}

	session, err := h.service.heartbeat(
		r.Context(),
		caller.CustomerID,
		r.PathValue("sessionID"),
	)
	if err != nil {
		writeManagementPairingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newManagementPairingResponse(session))
}

func (h *managementPairingHTTPHandler) confirm(
	w http.ResponseWriter,
	r *http.Request,
) {
	caller, err := ManagementCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "management caller is required")
		return
	}

	_, err = h.service.confirm(
		r.Context(),
		caller.CustomerID,
		r.PathValue("sessionID"),
	)
	if err != nil {
		writeManagementPairingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *managementPairingHTTPHandler) cancel(
	w http.ResponseWriter,
	r *http.Request,
) {
	caller, err := ManagementCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "management caller is required")
		return
	}

	_, err = h.service.cancel(
		r.Context(),
		caller.CustomerID,
		r.PathValue("sessionID"),
	)
	if err != nil {
		writeManagementPairingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func newManagementPairingResponse(
	session PairingSession,
) ManagementPairingResponse {
	response := ManagementPairingResponse{
		SessionID: session.ID,
		State:     session.State,
		Code:      session.Code,
		ExpiresAt: session.ExpiresAt,
	}
	if session.State == PairingStatePendingConfirmation {
		response.Device = &PairingPendingDevice{
			DeviceID:   session.DeviceID,
			DeviceName: session.DeviceName,
			Platform:   session.Platform,
		}
	}
	return response
}

func writeManagementPairingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "pairing session not found")
	case errors.Is(err, ErrPairingSessionExpired):
		writeError(w, http.StatusGone, "pairing session expired")
	case errors.Is(err, ErrPairingSessionState):
		writeError(w, http.StatusConflict, "pairing session state conflict")
	case errors.Is(err, errPairingCodeUnavailable):
		writeError(w, http.StatusServiceUnavailable, "pairing code unavailable")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
