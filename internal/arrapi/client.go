// Package arrapi is a client for the /api/v3 HTTP API shared by Sonarr and
// Radarr.
package arrapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to one Instance.
type Client struct {
	baseURL string
	apiKey  string
	httpc   *http.Client
}

// New returns a Client for the Instance at baseURL.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpc:   &http.Client{Timeout: 10 * time.Second},
	}
}

// SystemStatus is the subset of /api/v3/system/status arr-cli uses.
type SystemStatus struct {
	AppName string `json:"appName"`
	Version string `json:"version"`
}

// HealthCheck is one entry from /api/v3/health.
type HealthCheck struct {
	Source  string `json:"source"`
	Type    string `json:"type"`
	Message string `json:"message"`
	WikiURL string `json:"wikiUrl"`
}

// SystemStatus fetches the Instance's App name and version.
func (c *Client) SystemStatus(ctx context.Context) (SystemStatus, error) {
	var s SystemStatus
	err := c.get(ctx, "system/status", &s)
	return s, err
}

// Health fetches the Instance's current health checks.
func (c *Client) Health(ctx context.Context) ([]HealthCheck, error) {
	var h []HealthCheck
	err := c.get(ctx, "health", &h)
	return h, err
}

func (c *Client) get(ctx context.Context, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpoint(path), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return &StatusError{Path: path, Code: resp.StatusCode}
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("GET %s: decode response: %w", endpoint(path), err)
	}
	return nil
}

// StatusError means the Instance answered, but not with 200 OK.
type StatusError struct {
	Path string
	Code int
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("GET %s: %d %s", endpoint(e.Path), e.Code, http.StatusText(e.Code))
	if e.Code == http.StatusUnauthorized {
		msg += " (check the API key)"
	}
	return msg
}

func endpoint(path string) string { return "/api/v3/" + path }
