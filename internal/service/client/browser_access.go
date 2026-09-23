package client

import (
	"context"
	"net/http"

	"github.com/ddrowsy/family-friend-release/internal/service"
)

// CreateBrowserAccessRequest asks the parent side to decide a blocked-site request.
func (c *AgentClient) CreateBrowserAccessRequest(
	ctx context.Context,
	request service.DeviceBrowserAccessRequest,
) (service.DeviceBrowserAccessStatus, error) {
	var status service.DeviceBrowserAccessStatus
	if err := c.transport.doJSON(
		ctx,
		http.MethodPost,
		service.DeviceBrowserAccessRequestsPath(),
		request,
		&status,
	); err != nil {
		return service.DeviceBrowserAccessStatus{}, err
	}
	return status, nil
}

// BrowserAccessRequestStatus fetches one request owned by the authenticated device.
func (c *AgentClient) BrowserAccessRequestStatus(
	ctx context.Context,
	requestID string,
) (service.DeviceBrowserAccessStatus, error) {
	path, err := service.DeviceBrowserAccessRequestPath(requestID)
	if err != nil {
		return service.DeviceBrowserAccessStatus{}, err
	}

	var status service.DeviceBrowserAccessStatus
	if err := c.transport.doJSON(
		ctx,
		http.MethodGet,
		path,
		nil,
		&status,
	); err != nil {
		return service.DeviceBrowserAccessStatus{}, err
	}
	return status, nil
}
