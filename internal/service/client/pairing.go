package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ddrowsy/family-friend-release/internal/service"
)

const pairingAuthorizationScheme = "Pairing"

// ClaimPairing submits a bootstrap pairing code for this installation.
func (c *AgentClient) ClaimPairing(
	ctx context.Context,
	request service.DevicePairingClaimRequest,
) (service.DevicePairingClaimResponse, error) {
	var response service.DevicePairingClaimResponse
	if err := c.transport.doJSON(
		ctx,
		http.MethodPost,
		service.APIV1Path+"/pairing/claim",
		request,
		&response,
	); err != nil {
		return service.DevicePairingClaimResponse{}, err
	}
	return response, nil
}

// PairingStatus returns the bootstrap pairing result for one claimed session.
func (c *AgentClient) PairingStatus(
	ctx context.Context,
	sessionID string,
	claimProof string,
) (service.DevicePairingStatusResponse, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return service.DevicePairingStatusResponse{}, errors.New("pairing session ID is required")
	}
	claimProof = strings.TrimSpace(claimProof)
	if claimProof == "" {
		return service.DevicePairingStatusResponse{}, errors.New("pairing claim proof is required")
	}

	path := service.APIV1Path + "/pairing/" + url.PathEscape(sessionID) + "/status"
	var response service.DevicePairingStatusResponse
	if err := c.transport.getAuthorizedJSON(
		ctx,
		path,
		pairingAuthorizationScheme+" "+claimProof,
		&response,
	); err != nil {
		return service.DevicePairingStatusResponse{}, err
	}
	return response, nil
}

func (t *transport) getAuthorizedJSON(
	ctx context.Context,
	path string,
	authorization string,
	responseBody any,
) error {
	requestURL := *t.baseURL
	requestURL.Path = strings.TrimRight(t.baseURL.Path, "/") + path
	requestURL.RawPath = ""
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		requestURL.String(),
		nil,
	)
	if err != nil {
		return fmt.Errorf("creating control service request: %w", err)
	}
	request.Header.Set("Authorization", authorization)

	response, err := t.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("calling control service: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBodyBytes+1))
		return &ResponseError{
			Method:     http.MethodGet,
			Path:       path,
			StatusCode: response.StatusCode,
		}
	}

	limited := io.LimitReader(response.Body, maxResponseBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("reading control service response: %w", err)
	}
	if len(data) > maxResponseBodyBytes {
		return errors.New("control service response exceeds maximum size")
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("decoding control service response: %w", err)
	}
	return nil
}
