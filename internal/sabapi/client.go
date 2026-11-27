// Package sabapi is a client for SABnzbd's HTTP API, which takes every call
// as a mode on /api.
package sabapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/drzippit/arr-cli/internal/release"
)

// Client talks to one SABnzbd Instance.
type Client struct {
	baseURL string
	apiKey  string
	httpc   *http.Client
}

// New returns a Client for the SABnzbd Instance at baseURL.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpc:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Queue is what SABnzbd reports about its queue as a whole.
type Queue struct {
	Version string `json:"version"`
	// Status is SABnzbd's state: Downloading, Paused or Idle.
	Status string `json:"status"`
	// KBPerSec is the current speed in KiB per second.
	KBPerSec number `json:"kbpersec"`
	// DiskSpace1 and DiskSpace2 are the free space in GiB of the folder
	// Downloads are fetched into and of the one they end up in.
	DiskSpace1 number `json:"diskspace1"`
	DiskSpace2 number `json:"diskspace2"`
}

// SpeedBytes is the current speed in bytes per second.
func (q Queue) SpeedBytes() int64 { return int64(float64(q.KBPerSec) * 1024) }

// FreeBytes is the free space of whichever of SABnzbd's two folders has the
// least, since that one fills first.
func (q Queue) FreeBytes() int64 {
	return int64(min(float64(q.DiskSpace1), float64(q.DiskSpace2)) * (1 << 30))
}

// Warning is one warning SABnzbd reports about itself. Type is WARNING or
// ERROR.
type Warning struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Time int64  `json:"time"`
}

// Queue fetches the state of SABnzbd's queue.
func (c *Client) Queue(ctx context.Context) (Queue, error) {
	var resp struct {
		Queue Queue `json:"queue"`
	}
	err := c.get(ctx, "queue", nil, &resp)
	return resp.Queue, err
}

// Warnings fetches the warnings SABnzbd currently shows.
func (c *Client) Warnings(ctx context.Context) ([]Warning, error) {
	var resp struct {
		Warnings []Warning `json:"warnings"`
	}
	err := c.get(ctx, "warnings", nil, &resp)
	return resp.Warnings, err
}

// get calls mode with the extra query parameters and decodes the answer into
// into. SABnzbd answers some failures with 200 and {"status": false,
// "error": "..."}, which get reports as an error too.
func (c *Client) get(ctx context.Context, mode string, extra url.Values, into any) error {
	q := url.Values{"mode": {mode}, "output": {"json"}, "apikey": {c.apiKey}}
	for k, v := range extra {
		q[k] = v
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "arr-cli/"+release.Current().Version)
	resp, err := c.httpc.Do(req)
	if err != nil {
		// The URL holds the API key, so name only the mode.
		return fmt.Errorf("mode=%s: %w", mode, unwrapURLError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return &StatusError{Mode: mode, Code: resp.StatusCode, Reason: strings.TrimSpace(string(body[:min(len(body), 200)]))}
	}
	var failed struct {
		Status *bool  `json:"status"`
		Error  string `json:"error"`
	}
	if json.Unmarshal(body, &failed) == nil && failed.Status != nil && !*failed.Status && failed.Error != "" {
		return fmt.Errorf("mode=%s: %s", mode, failed.Error)
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("mode=%s: decode response: %w", mode, err)
	}
	return nil
}

// unwrapURLError drops the *url.Error wrapper, whose message repeats the URL
// and with it the API key.
func unwrapURLError(err error) error {
	if uerr, ok := err.(*url.Error); ok {
		return uerr.Err
	}
	return err
}

// StatusError means SABnzbd answered, but not with 200 OK. Reason is the
// text it gave, such as "API Key Incorrect".
type StatusError struct {
	Mode   string
	Code   int
	Reason string
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("mode=%s: %d %s", e.Mode, e.Code, http.StatusText(e.Code))
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	if e.Code == http.StatusForbidden || e.Code == http.StatusUnauthorized {
		msg += " (check the API key)"
	}
	return msg
}

// number is a number SABnzbd sends as a JSON string, such as "2048.00".
type number float64

func (n *number) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("number %s: %w", b, err)
	}
	*n = number(f)
	return nil
}
