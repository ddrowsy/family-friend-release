package service

import (
	"errors"
	"net/http"
	"strings"
)

const (
	managementApprovalCodeRoute = APIV1Path + "/management/approval-code"
	maxApprovalCodeLength       = 256
)

type managementApprovalCodeHTTPBody struct {
	ApprovalCode string `json:"approval_code"`
}

func registerManagementApprovalCodeRoutes(mux *http.ServeMux, store *Store) {
	handler := &managementApprovalCodeHTTPHandler{store: store}
	mux.HandleFunc("PUT "+managementApprovalCodeRoute, handler.put)
}

type managementApprovalCodeHTTPHandler struct {
	store *Store
}

func (h *managementApprovalCodeHTTPHandler) put(w http.ResponseWriter, r *http.Request) {
	caller, err := ManagementCallerFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "management caller identity required")
		return
	}

	var body managementApprovalCodeHTTPBody
	if err := decodeRequestJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	if err := validateManagementApprovalCode(body.ApprovalCode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.SetApprovalCode(r.Context(), caller.CustomerID, body.ApprovalCode); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateManagementApprovalCode(code string) error {
	if strings.TrimSpace(code) == "" {
		return errors.New("approval code is required")
	}
	if len(code) > maxApprovalCodeLength {
		return errors.New("approval code is too long")
	}
	return nil
}
