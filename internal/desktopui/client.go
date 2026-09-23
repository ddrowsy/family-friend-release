package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/agent"
)

const (
	defaultHTTPTimeout     = 3 * time.Second
	maxStatusResponseBytes = 1 << 20
)

// AgentClient is the minimal localhost agent contract needed by the desktop shell.
type AgentClient interface {
	Status(ctx context.Context) (agent.UIStatus, error)
}

// HTTPAgentClient reads the secret-free desktop status from the local agent.
type HTTPAgentClient struct {
	baseURL    *url.URL
	httpClient *http.Client
}

// NewHTTPAgentClient creates a localhost-only agent status client.
func NewHTTPAgentClient(baseURL string, httpClient *http.Client) (*HTTPAgentClient, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parsing local agent URL: %w", err)
	}
	if parsedURL.Scheme != "http" {
		return nil, errors.New("local agent URL must use http")
	}
	if err := validateLoopbackHost(parsedURL.Hostname()); err != nil {
		return nil, err
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &HTTPAgentClient{
		baseURL:    parsedURL,
		httpClient: httpClient,
	}, nil
}

// Status returns the current secret-free agent status.
func (c *HTTPAgentClient) Status(ctx context.Context) (agent.UIStatus, error) {
	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(c.baseURL.Path, "/") + agent.UIStatusPath
	requestURL.RawPath = ""

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		requestURL.String(),
		nil,
	)
	if err != nil {
		return agent.UIStatus{}, fmt.Errorf("creating local agent status request: %w", err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return agent.UIStatus{}, fmt.Errorf("calling local agent status API: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(
			io.Discard,
			io.LimitReader(response.Body, maxStatusResponseBytes+1),
		)
		return agent.UIStatus{}, fmt.Errorf(
			"local agent status API returned HTTP %d",
			response.StatusCode,
		)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, maxStatusResponseBytes+1))
	if err != nil {
		return agent.UIStatus{}, fmt.Errorf("reading local agent status: %w", err)
	}
	if len(data) > maxStatusResponseBytes {
		return agent.UIStatus{}, errors.New("local agent status response exceeds maximum size")
	}

	var status agent.UIStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return agent.UIStatus{}, fmt.Errorf("decoding local agent status: %w", err)
	}
	return status, nil
}

func validateLoopbackHost(host string) error {
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("local agent URL host %q must be loopback", host)
	}
	return nil
}
