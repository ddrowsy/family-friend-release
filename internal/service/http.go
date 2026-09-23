package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/ddrowsy/family-friend-release/internal/policy"
)

const (
	maxRequestBodyBytes                        = 1 << 20
	maxTemporaryBrowserApprovalDurationSeconds = 24 * 60 * 60

	profileConfigurationRoute = APIV1Path +
		"/customers/{customerID}/profiles/{profileID}/config"
	profileBrowserApprovalsRoute = APIV1Path +
		"/customers/{customerID}/profiles/{profileID}/browser/approvals"
	profileActiveBrowserRoute = APIV1Path +
		"/customers/{customerID}/profiles/{profileID}/browser/active-profile"
	deviceRoute = APIV1Path +
		"/customers/{customerID}/devices/{deviceID}"
	deviceProfilesRoute = deviceRoute + "/profiles"
	deviceProfileRoute  = deviceProfilesRoute + "/{profileID}"
)

// NewHTTPHandler creates the versioned control-service HTTP API.
func NewHTTPHandler(store *Store) http.Handler {
	handler := &httpHandler{store: store}
	mux := http.NewServeMux()

	registerManagementPairingRoutes(mux, store)
	registerManagementApprovalCodeRoutes(mux, store)
	registerDevicePairingClaimRoutes(mux, store)
	registerDevicePairingStatusRoutes(mux, store)
	registerBrowserAccessRequestRoutes(mux, store)

	mux.HandleFunc("GET "+profileConfigurationRoute, handler.getProfileConfiguration)
	mux.HandleFunc("PUT "+profileConfigurationRoute, handler.putProfileConfiguration)
	mux.HandleFunc("POST "+profileBrowserApprovalsRoute, handler.approveBrowserURL)
	mux.HandleFunc("PUT "+profileActiveBrowserRoute, handler.putActiveBrowserProfile)
	mux.HandleFunc("PUT "+deviceRoute, handler.registerDevice)
	mux.HandleFunc("GET "+deviceProfilesRoute, handler.getDeviceProfiles)
	mux.HandleFunc("GET "+deviceProfileRoute, handler.getDeviceProfile)
	mux.HandleFunc("PUT "+deviceProfileRoute, handler.linkDeviceProfile)
	mux.HandleFunc("DELETE "+deviceProfileRoute, handler.unlinkDeviceProfile)

	return mux
}

type httpHandler struct {
	store *Store
}

func (h *httpHandler) getProfileConfiguration(w http.ResponseWriter, r *http.Request) {
	customerID, profileID, err := profileIdentity(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	configuration, err := h.store.ProfileConfiguration(
		r.Context(),
		customerID,
		profileID,
	)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configuration)
}

func (h *httpHandler) putProfileConfiguration(w http.ResponseWriter, r *http.Request) {
	customerID, profileID, err := profileIdentity(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var profilePolicy policy.ProfilePolicy
	if err := decodeRequestJSON(w, r, &profilePolicy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	configuration := ProfileConfiguration{
		CustomerID: customerID,
		ProfileID:  profileID,
		Policy:     profilePolicy,
	}
	if err := configuration.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	configuration, err = h.store.PutProfileConfiguration(r.Context(), configuration)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configuration)
}

func (h *httpHandler) putActiveBrowserProfile(w http.ResponseWriter, r *http.Request) {
	customerID, profileID, err := profileIdentity(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var request ActiveBrowserProfileRequest
	if err := decodeRequestJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}

	configuration, err := h.store.SetActiveBrowserProfile(
		r.Context(),
		customerID,
		profileID,
		request,
	)
	if err != nil {
		writeActiveBrowserProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configuration)
}

func (h *httpHandler) approveBrowserURL(w http.ResponseWriter, r *http.Request) {
	customerID, profileID, err := profileIdentity(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var request BrowserApprovalRequest
	if err := decodeRequestJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	if err := validateBrowserApprovalHTTPRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.store.ApproveBrowserURL(
		r.Context(),
		customerID,
		profileID,
		request,
	)
	if err != nil {
		writeBrowserApprovalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *httpHandler) registerDevice(w http.ResponseWriter, r *http.Request) {
	customerID, deviceID, err := deviceIdentity(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	registration := DeviceRegistration{
		CustomerID: customerID,
		DeviceID:   deviceID,
	}
	if err := h.store.RegisterDevice(r.Context(), registration); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpHandler) getDeviceProfiles(w http.ResponseWriter, r *http.Request) {
	customerID, deviceID, err := deviceIdentity(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	profileIDs, err := h.store.DeviceProfiles(r.Context(), customerID, deviceID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		DeviceProfileAssignment{
			CustomerID: customerID,
			DeviceID:   deviceID,
			ProfileIDs: profileIDs,
		},
	)
}

func (h *httpHandler) getDeviceProfile(w http.ResponseWriter, r *http.Request) {
	customerID, deviceID, profileID, err := deviceProfileIdentity(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	linked, err := h.store.DeviceProfileLinked(
		r.Context(),
		customerID,
		deviceID,
		profileID,
	)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !linked {
		writeError(w, http.StatusNotFound, "device profile link not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpHandler) linkDeviceProfile(w http.ResponseWriter, r *http.Request) {
	customerID, deviceID, profileID, err := deviceProfileIdentity(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.LinkDeviceProfile(
		r.Context(),
		customerID,
		deviceID,
		profileID,
	); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpHandler) unlinkDeviceProfile(w http.ResponseWriter, r *http.Request) {
	customerID, deviceID, profileID, err := deviceProfileIdentity(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.UnlinkDeviceProfile(
		r.Context(),
		customerID,
		deviceID,
		profileID,
	); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func profileIdentity(r *http.Request) (CustomerID, ProfileID, error) {
	customerID := CustomerID(r.PathValue("customerID"))
	if err := customerID.Validate(); err != nil {
		return "", "", err
	}
	profileID := ProfileID(r.PathValue("profileID"))
	if err := profileID.Validate(); err != nil {
		return "", "", err
	}
	return customerID, profileID, nil
}

func deviceIdentity(r *http.Request) (CustomerID, DeviceID, error) {
	customerID := CustomerID(r.PathValue("customerID"))
	if err := customerID.Validate(); err != nil {
		return "", "", err
	}
	deviceID := DeviceID(r.PathValue("deviceID"))
	if err := deviceID.Validate(); err != nil {
		return "", "", err
	}
	return customerID, deviceID, nil
}

func deviceProfileIdentity(
	r *http.Request,
) (CustomerID, DeviceID, ProfileID, error) {
	customerID, deviceID, err := deviceIdentity(r)
	if err != nil {
		return "", "", "", err
	}
	profileID := ProfileID(r.PathValue("profileID"))
	if err := profileID.Validate(); err != nil {
		return "", "", "", err
	}
	return customerID, deviceID, profileID, nil
}

func validateBrowserApprovalHTTPRequest(request BrowserApprovalRequest) error {
	if err := validateBrowserApprovalRequest(request); err != nil {
		return err
	}
	if request.Action != BrowserApprovalTemporary {
		return nil
	}
	if request.DurationSeconds > maxTemporaryBrowserApprovalDurationSeconds {
		return errors.New("temporary approval duration exceeds server maximum")
	}
	return nil
}

func decodeRequestJSON(
	w http.ResponseWriter,
	r *http.Request,
	target any,
) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("request body must contain one JSON value")
	}
	return err
}

func writeActiveBrowserProfileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrBrowserProfileNotFound):
		writeError(w, http.StatusNotFound, "browser policy profile not found")
	case errors.Is(err, ErrInvalidActiveBrowserProfile):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeBrowserApprovalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "resource not found")
	case errors.Is(err, ErrDeviceProfileNotLinked):
		writeError(w, http.StatusForbidden, "device is not linked to profile")
	case errors.Is(err, ErrInvalidApprovalCode):
		writeError(w, http.StatusUnauthorized, "invalid approval code")
	case errors.Is(err, ErrApprovalCodeThrottled):
		writeError(w, http.StatusTooManyRequests, "approval code verification throttled")
	case errors.Is(err, ErrBrowserProfileNotFound):
		writeError(w, http.StatusNotFound, "browser policy profile not found")
	case errors.Is(err, ErrInvalidBrowserURL):
		writeError(w, http.StatusBadRequest, "invalid browser URL")
	case errors.Is(err, ErrInvalidApprovalDuration):
		writeError(w, http.StatusBadRequest, "invalid approval duration")
	case errors.Is(err, ErrInvalidApprovalAction):
		writeError(w, http.StatusBadRequest, "invalid approval action")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(
		w,
		status,
		struct {
			Error string `json:"error"`
		}{
			Error: message,
		},
	)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}
