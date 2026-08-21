// Package authz calls auth-api's POST /authz/check to verify a caller's bearer token and
// resolve their roles, and provides Gin middleware that gates a route on holding one of a set
// of allowed roles. Copied verbatim from catalog-api/authz - see platform's
// user-authorization spec (openspec/specs/user-authorization/spec.md in sweetrpg/platform).
package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sweetrpg/common.go/logging"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Platform role names, matching auth-api's fixed role model exactly.
const (
	RoleUser      = "user"
	RoleSubmitter = "submitter"
	RoleEditor    = "editor"
	RoleModerator = "moderator"
	RoleApprover  = "approver"
	RoleAdmin     = "admin"
)

// CheckResponse is the union of auth-api's allowed/denied /authz/check response shapes.
type CheckResponse struct {
	Allowed bool     `json:"allowed"`
	Roles   []string `json:"roles"`
	Sub     string   `json:"sub"`
	Reason  string   `json:"reason"`
}

// InvalidTokenError means auth-api rejected the bearer token itself (missing, expired,
// unverifiable) - distinct from a service-level deny (Allowed: false with a Reason) or a
// transport/backend failure.
type InvalidTokenError struct{}

func (InvalidTokenError) Error() string { return "authz: invalid or missing token" }

// Client calls auth-api's /authz/check endpoint.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a Client against auth-api's base URL. An empty baseURL is accepted so the
// service can still start when AUTH_API_URL isn't configured; every Check call will then fail
// with a transport error, which RequireAnyRole surfaces as a 503.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// Check verifies token against auth-api and returns the caller's allowed/roles/subject for the
// given service name. Returns InvalidTokenError if auth-api rejects the token itself.
func (c *Client) Check(ctx context.Context, token, service string) (*CheckResponse, error) {
	body, err := json.Marshal(map[string]string{"service": service})
	if err != nil {
		return nil, fmt.Errorf("authz: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/authz/check", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("authz: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("authz: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, InvalidTokenError{}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authz: unexpected status %d from auth-api", resp.StatusCode)
	}

	var out CheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("authz: decode response: %w", err)
	}

	logging.Logger.Debug("authz check", "service", service, "allowed", out.Allowed)
	return &out, nil
}
