package client

import (
	"context"
	"net/http"

	"github.com/ddrowsy/family-friend-release/internal/policy"
	"github.com/ddrowsy/family-friend-release/internal/service"
)

const managementApprovalCodePath = service.APIV1Path + "/management/approval-code"

type managementApprovalCodeRequest struct {
	ApprovalCode string `json:"approval_code"`
}

// ManagementClient calls parent/admin control-service operations.
type ManagementClient struct {
	transport *transport
}

// NewManagement creates a parent/admin control-service client.
func NewManagement(baseURL string, httpClient *http.Client) (*ManagementClient, error) {
	transport, err := newTransport(baseURL, httpClient)
	if err != nil {
		return nil, err
	}
	return &ManagementClient{transport: transport}, nil
}

// SetApprovalCode sets or replaces the customer's long-term parent approval code.
func (c *ManagementClient) SetApprovalCode(ctx context.Context, approvalCode string) error {
	return c.transport.doJSON(
		ctx,
		http.MethodPut,
		managementApprovalCodePath,
		managementApprovalCodeRequest{ApprovalCode: approvalCode},
		nil,
	)
}

// PutProfileConfiguration replaces a child profile's policy.
func (c *ManagementClient) PutProfileConfiguration(
	ctx context.Context,
	customerID service.CustomerID,
	profileID service.ProfileID,
	profilePolicy policy.ProfilePolicy,
) (service.ProfileConfiguration, error) {
	path, err := service.ProfileConfigPath(customerID, profileID)
	if err != nil {
		return service.ProfileConfiguration{}, err
	}

	var configuration service.ProfileConfiguration
	if err := c.transport.doJSON(
		ctx,
		http.MethodPut,
		path,
		profilePolicy,
		&configuration,
	); err != nil {
		return service.ProfileConfiguration{}, err
	}
	return configuration, nil
}

// SetActiveBrowserProfile changes or clears the active browser policy profile.
func (c *ManagementClient) SetActiveBrowserProfile(
	ctx context.Context,
	customerID service.CustomerID,
	profileID service.ProfileID,
	request service.ActiveBrowserProfileRequest,
) (service.ProfileConfiguration, error) {
	path, err := service.ProfileActiveBrowserPath(customerID, profileID)
	if err != nil {
		return service.ProfileConfiguration{}, err
	}

	var configuration service.ProfileConfiguration
	if err := c.transport.doJSON(
		ctx,
		http.MethodPut,
		path,
		request,
		&configuration,
	); err != nil {
		return service.ProfileConfiguration{}, err
	}
	return configuration, nil
}
