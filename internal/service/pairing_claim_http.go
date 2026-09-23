package service

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

const devicePairingClaimRoute = APIV1Path + "/pairing/claim"

// DevicePairingClaimRequest identifies an unpaired device claiming a pairing code.
type DevicePairingClaimRequest struct {
	Code       string   `json:"code"`
	DeviceID   DeviceID `json:"device_id"`
	DeviceName string   `json:"device_name"`
	Platform   string   `json:"platform"`
}

// DevicePairingClaimResponse describes a pairing claim awaiting parent confirmation.
type DevicePairingClaimResponse struct {
	SessionID  string              `json:"session_id"`
	State      PairingSessionState `json:"state"`
	ExpiresAt  time.Time           `json:"expires_at"`
	ClaimProof string              `json:"claim_proof"`
}

func registerDevicePairingClaimRoutes(mux *http.ServeMux, store *Store) {
	handler := &devicePairingClaimHTTPHandler{store: store}
	mux.HandleFunc("POST "+devicePairingClaimRoute, handler.claim)
}

type devicePairingClaimHTTPHandler struct {
	store *Store
}

func (h *devicePairingClaimHTTPHandler) claim(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request DevicePairingClaimRequest
	if err := decodeRequestJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	if err := request.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	proof, proofHash, err := newPairingBootstrapProof()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	session, err := h.store.ClaimPairingSession(
		r.Context(),
		request.Code,
		PairingDeviceClaim{
			DeviceID:   request.DeviceID,
			DeviceName: request.DeviceName,
			Platform:   request.Platform,
		},
		proofHash,
	)
	if err != nil {
		writeDevicePairingClaimError(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		DevicePairingClaimResponse{
			SessionID:  session.ID,
			State:      session.State,
			ExpiresAt:  session.ExpiresAt,
			ClaimProof: proof,
		},
	)
}

func (r DevicePairingClaimRequest) validate() error {
	if strings.TrimSpace(r.Code) == "" {
		return errors.New("pairing code is required")
	}
	if err := r.DeviceID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.DeviceName) == "" {
		return errors.New("device name is required")
	}
	if strings.TrimSpace(r.Platform) == "" {
		return errors.New("platform is required")
	}
	return nil
}

func writeDevicePairingClaimError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "pairing code not found")
	case errors.Is(err, ErrPairingSessionExpired):
		writeError(w, http.StatusGone, "pairing code expired")
	case errors.Is(err, ErrPairingSessionState):
		writeError(w, http.StatusConflict, "pairing session state conflict")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
