package client

import (
	"context"
	"errors"
	"net/http"

	"github.com/ddrowsy/family-friend-release/internal/service"
)

// AgentClient calls control-service operations available to an installed agent.
type AgentClient struct {
	transport *transport
}

// NewAgent creates an installed-agent control-service client.
func NewAgent(baseURL string, httpClient *http.Client) (*AgentClient, error) {
	transport, err := newTransport(baseURL, httpClient)
	if err != nil {
		return nil, err
	}
	return &AgentClient{transport: transport}, nil
}

// ProfileConfiguration fetches the latest configuration for a child profile.
func (c *AgentClient) ProfileConfiguration(
	ctx context.Context,
	customerID service.CustomerID,
	profileID service.ProfileID,
) (service.ProfileConfiguration, error) {
	path, err := service.ProfileConfigPath(customerID, profileID)
	if err != nil {
		return service.ProfileConfiguration{}, err
	}

	var configuration service.ProfileConfiguration
	if err := c.transport.doJSON(
		ctx,
		http.MethodGet,
		path,
		nil,
		&configuration,
	); err != nil {
		return service.ProfileConfiguration{}, err
	}
	return configuration, nil
}

// RegisterDevice registers an agent installation under a customer.
func (c *AgentClient) RegisterDevice(
	ctx context.Context,
	customerID service.CustomerID,
	deviceID service.DeviceID,
) error {
	path, err := service.DevicePath(customerID, deviceID)
	if err != nil {
		return err
	}
	return c.transport.doJSON(
		ctx,
		http.MethodPut,
		path,
		nil,
		nil,
	)
}

// DeviceProfiles returns the child profiles linked to an agent installation.
func (c *AgentClient) DeviceProfiles(
	ctx context.Context,
	customerID service.CustomerID,
	deviceID service.DeviceID,
) (service.DeviceProfileAssignment, error) {
	path, err := service.DeviceProfilesPath(customerID, deviceID)
	if err != nil {
		return service.DeviceProfileAssignment{}, err
	}

	var assignment service.DeviceProfileAssignment
	if err := c.transport.doJSON(
		ctx,
		http.MethodGet,
		path,
		nil,
		&assignment,
	); err != nil {
		return service.DeviceProfileAssignment{}, err
	}
	return assignment, nil
}

// DeviceProfileLinked reports whether a device is linked to a child profile.
func (c *AgentClient) DeviceProfileLinked(
	ctx context.Context,
	customerID service.CustomerID,
	deviceID service.DeviceID,
	profileID service.ProfileID,
) (bool, error) {
	path, err := service.DeviceProfilePath(customerID, deviceID, profileID)
	if err != nil {
		return false, err
	}

	err = c.transport.doJSON(
		ctx,
		http.MethodGet,
		path,
		nil,
		nil,
	)
	if err == nil {
		return true, nil
	}
	var responseErr *ResponseError
	if errors.As(err, &responseErr) && responseErr.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, err
}

// ApproveBrowserTemporary requests a time-limited browser URL approval.
func (c *AgentClient) ApproveBrowserTemporary(
	ctx context.Context,
	customerID service.CustomerID,
	profileID service.ProfileID,
	request service.BrowserApprovalRequest,
) (service.BrowserApprovalResult, error) {
	request.Action = service.BrowserApprovalTemporary
	return c.approveBrowser(ctx, customerID, profileID, request)
}

// ApproveBrowserPermanent requests a permanent browser URL approval.
func (c *AgentClient) ApproveBrowserPermanent(
	ctx context.Context,
	customerID service.CustomerID,
	profileID service.ProfileID,
	request service.BrowserApprovalRequest,
) (service.BrowserApprovalResult, error) {
	request.Action = service.BrowserApprovalPermanent
	request.DurationSeconds = 0
	return c.approveBrowser(ctx, customerID, profileID, request)
}

func (c *AgentClient) approveBrowser(
	ctx context.Context,
	customerID service.CustomerID,
	profileID service.ProfileID,
	request service.BrowserApprovalRequest,
) (service.BrowserApprovalResult, error) {
	path, err := service.ProfileBrowserApprovalsPath(customerID, profileID)
	if err != nil {
		return service.BrowserApprovalResult{}, err
	}
	if err := request.DeviceID.Validate(); err != nil {
		return service.BrowserApprovalResult{}, err
	}

	var result service.BrowserApprovalResult
	if err := c.transport.doJSON(
		ctx,
		http.MethodPost,
		path,
		request,
		&result,
	); err != nil {
		return service.BrowserApprovalResult{}, err
	}
	return result, nil
}
