package sailpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client handles IdentityNow API requests
type Client struct {
	baseURL   string
	auth      *Auth
	httpClient *http.Client
}

// NewClient creates a new IdentityNow API client
func NewClient(tenant, baseURL string, auth *Auth) *Client {
	return &Client{
		baseURL:    fmt.Sprintf("https://%s.api.%s", tenant, baseURL),
		auth:       auth,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Identity represents a SailPoint identity
type Identity struct {
	ID          string      `json:"id"`
	Name        interface{} `json:"name"`       // Can be string or map
	Email       string      `json:"email"`
	EmployeeID  string      `json:"employeeId"`
	Status      string      `json:"status"`
	Attributes  interface{} `json:"attributes"`  // Can be array, map, or null
}

// AccessProfile represents a SailPoint access profile
type AccessProfile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Role represents a SailPoint role
type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Entitlement represents a SailPoint entitlement
type Entitlement struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ListIdentitiesResponse represents paginated identities response
// IdentityNow API returns an array directly, not an object with items
type ListIdentitiesResponse struct {
	Items []Identity `json:"-"` // Not used - API returns array directly
	Total int        `json:"-"` // Not used - need to get from headers or count
}

// ListAccessProfilesResponse represents paginated access profiles response
type ListAccessProfilesResponse struct {
	Items []AccessProfile `json:"items"`
	Total int             `json:"total"`
}

// ListRolesResponse represents paginated roles response
type ListRolesResponse struct {
	Items []Role `json:"items"`
	Total int    `json:"total"`
}

// ListEntitlementsResponse represents paginated entitlements response
type ListEntitlementsResponse struct {
	Items []Entitlement `json:"items"`
	Total int           `json:"total"`
}

// doRequest performs an authenticated HTTP request
func (c *Client) doRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	token, err := c.auth.GetAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	url := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// ListIdentities lists all identities with pagination
func (c *Client) ListIdentities(ctx context.Context, limit, offset int) (*ListIdentitiesResponse, error) {
	endpoint := fmt.Sprintf("/v3/public-identities?limit=%d&offset=%d", limit, offset)
	resp, err := c.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list identities failed: %d %s", resp.StatusCode, string(body))
	}

	// IdentityNow API returns an array directly, not an object
	var identities []Identity
	if err := json.NewDecoder(resp.Body).Decode(&identities); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Get total count from X-Total-Count header if available
	total := len(identities)
	if totalCount := resp.Header.Get("X-Total-Count"); totalCount != "" {
		if parsed, err := strconv.Atoi(totalCount); err == nil {
			total = parsed
		}
	}

	return &ListIdentitiesResponse{
		Items: identities,
		Total: total,
	}, nil
}

// GetIdentityAccess gets access for a specific identity
func (c *Client) GetIdentityAccess(ctx context.Context, identityID string) ([]map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/v3/identities/%s/access", identityID)
	resp, err := c.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get identity access failed: %d %s", resp.StatusCode, string(body))
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result, nil
}

// ListAccessProfiles lists all access profiles with pagination
func (c *Client) ListAccessProfiles(ctx context.Context, limit, offset int) (*ListAccessProfilesResponse, error) {
	endpoint := fmt.Sprintf("/v3/access-profiles?limit=%d&offset=%d", limit, offset)
	resp, err := c.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list access profiles failed: %d %s", resp.StatusCode, string(body))
	}

	// IdentityNow API returns an array directly
	var profiles []AccessProfile
	if err := json.NewDecoder(resp.Body).Decode(&profiles); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	total := len(profiles)
	if totalCount := resp.Header.Get("X-Total-Count"); totalCount != "" {
		if parsed, err := strconv.Atoi(totalCount); err == nil {
			total = parsed
		}
	}

	return &ListAccessProfilesResponse{
		Items: profiles,
		Total: total,
	}, nil
}

// ListRoles lists all roles with pagination
func (c *Client) ListRoles(ctx context.Context, limit, offset int) (*ListRolesResponse, error) {
	endpoint := fmt.Sprintf("/v3/roles?limit=%d&offset=%d", limit, offset)
	resp, err := c.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list roles failed: %d %s", resp.StatusCode, string(body))
	}

	// IdentityNow API returns an array directly
	var roles []Role
	if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	total := len(roles)
	if totalCount := resp.Header.Get("X-Total-Count"); totalCount != "" {
		if parsed, err := strconv.Atoi(totalCount); err == nil {
			total = parsed
		}
	}

	return &ListRolesResponse{
		Items: roles,
		Total: total,
	}, nil
}

// ListEntitlements lists all entitlements with pagination
func (c *Client) ListEntitlements(ctx context.Context, limit, offset int) (*ListEntitlementsResponse, error) {
	endpoint := fmt.Sprintf("/v3/entitlements?limit=%d&offset=%d", limit, offset)
	resp, err := c.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// Return a special error for 404 so caller can handle it gracefully
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("endpoint_not_found: %d %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("list entitlements failed: %d %s", resp.StatusCode, string(body))
	}

	// IdentityNow API returns an array directly
	var entitlements []Entitlement
	if err := json.NewDecoder(resp.Body).Decode(&entitlements); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	total := len(entitlements)
	if totalCount := resp.Header.Get("X-Total-Count"); totalCount != "" {
		if parsed, err := strconv.Atoi(totalCount); err == nil {
			total = parsed
		}
	}

	return &ListEntitlementsResponse{
		Items: entitlements,
		Total: total,
	}, nil
}
