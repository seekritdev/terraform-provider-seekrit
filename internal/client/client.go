// Package client is a small typed HTTP client over the seekrit REST API
// (apps/api, the /v1/orgs/... routes). It mirrors the shapes in
// packages/api-client and does no crypto — callers pass already-wrapped DEKs
// and pre-computed token material (see internal/crypto).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to one seekrit API deployment as one bearer principal.
type Client struct {
	baseURL   string
	token     string
	userAgent string
	http      *http.Client
}

// New returns a client for baseURL authenticating with the given service token.
// version is the provider build version, surfaced in the User-Agent so API logs
// can tell which provider release made a call.
func New(baseURL, token, version string, httpClient *http.Client) *Client {
	if httpClient == nil {
		// Not http.DefaultClient: a shared client has no timeout, so a hung API
		// would hang `terraform apply` indefinitely with no way to interrupt it
		// cleanly. Terraform's own context cancellation still applies on top.
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	if version == "" {
		version = "dev"
	}
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		token:     token,
		userAgent: "terraform-provider-seekrit/" + version,
		http:      httpClient,
	}
}

// APIError is a structured error from the API ({error:{code,message}}).
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("seekrit API error %d (%s): %s", e.Status, e.Code, e.Message)
}

// IsNotFound reports whether err is a 404 from the API — the signal a resource
// no longer exists (or is no longer visible) and should be dropped from state.
func IsNotFound(err error) bool {
	return hasStatus(err, http.StatusNotFound)
}

// IsForbidden reports whether err is a 403. For key-grant reads this is how the
// API says "you hold no grant on this environment", which is a different
// diagnostic from "the environment is gone".
func IsForbidden(err error) bool {
	return hasStatus(err, http.StatusForbidden)
}

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status == status
}

// do issues a request and decodes a JSON response into out (may be nil).
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("authorization", "Bearer "+c.token)
	req.Header.Set("user-agent", c.userAgent)
	// Attributes API usage to Terraform in the product analytics (lib/events.ts),
	// the same way the CLI and the SDKs identify themselves.
	req.Header.Set("x-seekrit-client", "terraform")
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer res.Body.Close()

	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return parseAPIError(res.StatusCode, payload)
	}
	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// parseAPIError decodes the {error:{code,message}} envelope, mirroring the
// fallback behavior of SeekritApiError in packages/api-client.
func parseAPIError(status int, payload []byte) *APIError {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Error.Code == "" {
		return &APIError{Status: status, Code: "internal", Message: fmt.Sprintf("HTTP %d", status)}
	}
	return &APIError{Status: status, Code: envelope.Error.Code, Message: envelope.Error.Message}
}
