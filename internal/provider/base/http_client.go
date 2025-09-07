package base

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// HTTPClient provides common HTTP functionality for providers
type HTTPClient struct {
	client  *http.Client
	baseURL string
	name    string // provider name for logging
}

// NewHTTPClient creates a new HTTP client with default settings
func NewHTTPClient(providerName string, timeoutSec int) *HTTPClient {
	if timeoutSec == 0 {
		timeoutSec = 30 // default timeout
	}

	return &HTTPClient{
		client: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
		name: providerName,
	}
}

// SetBaseURL sets the base URL for all requests
func (c *HTTPClient) SetBaseURL(baseURL string) {
	c.baseURL = baseURL
}

// PostJSON makes a POST request with JSON payload
func (c *HTTPClient) PostJSON(ctx context.Context, endpoint string, payload interface{}, headers map[string]string) (*HTTPResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	url := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("PayMatch/%s", c.name))

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Log the request (without sensitive data)
	log.Debug().
		Str("provider", c.name).
		Str("method", "POST").
		Str("url", url).
		Msg("making HTTP request")

	resp, err := c.client.Do(req)
	if err != nil {
		log.Error().
			Str("provider", c.name).
			Str("url", url).
			Err(err).
			Msg("HTTP request failed")
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	return c.handleResponse(resp)
}

// Get makes a GET request
func (c *HTTPClient) Get(ctx context.Context, endpoint string, headers map[string]string) (*HTTPResponse, error) {
	url := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers
	req.Header.Set("User-Agent", fmt.Sprintf("PayMatch/%s", c.name))

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	log.Debug().
		Str("provider", c.name).
		Str("method", "GET").
		Str("url", url).
		Msg("making HTTP request")

	resp, err := c.client.Do(req)
	if err != nil {
		log.Error().
			Str("provider", c.name).
			Str("url", url).
			Err(err).
			Msg("HTTP request failed")
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	return c.handleResponse(resp)
}

// handleResponse processes the HTTP response
func (c *HTTPClient) handleResponse(resp *http.Response) (*HTTPResponse, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	httpResp := &HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
	}

	// Log response (without sensitive data in body)
	log.Debug().
		Str("provider", c.name).
		Int("status_code", resp.StatusCode).
		Int("body_length", len(body)).
		Msg("received HTTP response")

	return httpResp, nil
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// IsSuccess checks if the response indicates success (2xx status code)
func (r *HTTPResponse) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// UnmarshalJSON unmarshals the response body into the provided struct
func (r *HTTPResponse) UnmarshalJSON(v interface{}) error {
	return json.Unmarshal(r.Body, v)
}

// String returns the response body as a string
func (r *HTTPResponse) String() string {
	return string(r.Body)
}