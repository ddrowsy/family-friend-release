package service

import (
	"errors"
	"net/http"
	"strings"
)

const (
	devicePairingStatusRoute = APIV1Path + "/pairing/{sessionID}/status"

	pairingBootstrapAuthorizationScheme = "Pairing"
)

// PairingCompletionState is the installer-visible state of a claimed pairing session.
type PairingCompletionState string

const (
	PairingCompletionPendingConfirmation PairingCompletionState = "pending_confirmation"
	PairingCompletionConfirmed           PairingCompletionState = "confirmed"
	PairingCompletionCancelled           PairingCompletionState = "cancelled"
	PairingCompletionExpired             PairingCompletionState = "expired"
)

// DevicePairingStatusResponse describes the result of one bootstrap pairing attempt.
// A future permanent device credential can be added to the confirmed result separately
// from the short-lived bootstrap proof used to authorize this endpoint.
type DevicePairingStatusResponse struct {
	State PairingCompletionState `json:"state"`
}

func registerDevicePairingStatusRoutes(mux *http.ServeMux, store *Store) {
	handler := &devicePairingStatusHTTPHandler{store: store}
	mux.HandleFunc("GET "+devicePairingStatusRoute, handler.status)
}

type devicePairingStatusHTTPHandler struct {
	store *Store
}

func (h *devicePairingStatusHTTPHandler) status(
	w http.ResponseWriter,
	r *http.Request,
) {
	proof, err := pairingBootstrapProofFromAuthorization(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "pairing session not found")
		return
	}

	session, err := h.store.PairingSessionByBootstrapProof(
		r.Context(),
		r.PathValue("sessionID"),
		proof,
	)
	if errors.Is(err, ErrPairingSessionExpired) {
		writeJSON(
			w,
			http.StatusOK,
			DevicePairingStatusResponse{State: PairingCompletionExpired},
		)
		return
	}
	if err != nil {
		writeDevicePairingStatusError(w, err)
		return
	}

	state, ok := pairingCompletionState(session.State)
	if !ok {
		writeError(w, http.StatusConflict, "pairing session state conflict")
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		DevicePairingStatusResponse{State: state},
	)
}

func pairingBootstrapProofFromAuthorization(r *http.Request) (string, error) {
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 {
		return "", ErrNotFound
	}
	if !strings.EqualFold(fields[0], pairingBootstrapAuthorizationScheme) {
		return "", ErrNotFound
	}
	if strings.TrimSpace(fields[1]) == "" {
		return "", ErrNotFound
	}
	return fields[1], nil
}

func pairingCompletionState(
	state PairingSessionState,
) (PairingCompletionState, bool) {
	switch state {
	case PairingStatePendingConfirmation:
		return PairingCompletionPendingConfirmation, true
	case PairingStateConfirmed:
		return PairingCompletionConfirmed, true
	case PairingStateCancelled:
		return PairingCompletionCancelled, true
	default:
		return "", false
	}
}

func writeDevicePairingStatusError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "pairing session not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal server error")
}
