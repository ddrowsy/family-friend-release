package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxResponseBodyBytes = 1 << 20

type transport struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func newTransport(baseURL string, httpClient *http.Client) (*transport, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing control service base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("control service base URL must include scheme and host")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &transport{
		baseURL:    parsed,
		httpClient: httpClient,
	}, nil
}

// ResponseError describes a non-successful control-service response without
// including the response body, which may contain sensitive request data.
type ResponseError struct {
	Method     string
	Path       string
	StatusCode int
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf(
		"control service %s %s returned HTTP %d",
		e.Method,
		e.Path,
		e.StatusCode,
	)
}

func (t *transport) doJSON(
	ctx context.Context,
	method string,
	path string,
	requestBody any,
	responseBody any,
) error {
	var body io.Reader
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encoding control service request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	requestURL := *t.baseURL
	requestURL.Path = strings.TrimRight(t.baseURL.Path, "/") + path
	requestURL.RawPath = ""
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return fmt.Errorf("creating control service request: %w", err)
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := t.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("calling control service: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBodyBytes+1))
		return &ResponseError{
			Method:     method,
			Path:       path,
			StatusCode: response.StatusCode,
		}
	}
	if responseBody == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBodyBytes+1))
		return nil
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
