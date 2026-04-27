package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/auth"
)

const defaultBaseURL = "https://graph.microsoft.com/v1.0"

// Client is a Microsoft Graph API client
type Client struct {
	httpClient *http.Client
	account    string
	baseURL    string
	// tokenFn is a test seam so tests can bypass auth.GetValidToken.
	tokenFn func(ctx context.Context, account string) (string, error)
}

// NewClient creates a new Graph API client for the specified account
func NewClient(account string) *Client {
	return &Client{
		httpClient: &http.Client{},
		account:    account,
		baseURL:    defaultBaseURL,
	}
}

// NewTestClient returns a Client configured for httptest. Intended for tests only;
// do not use in production code paths.
func NewTestClient(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{},
		account:    "test",
		baseURL:    baseURL,
		tokenFn:    func(ctx context.Context, account string) (string, error) { return "test-token", nil },
	}
}

func (c *Client) token(ctx context.Context) (string, error) {
	if c.tokenFn != nil {
		return c.tokenFn(ctx, c.account)
	}
	return auth.GetValidToken(ctx, c.account)
}

// GraphError represents an error response from the Graph API
type GraphError struct {
	Error struct {
		Code       string `json:"code"`
		Message    string `json:"message"`
		InnerError struct {
			RequestID string `json:"request-id"`
			Date      string `json:"date"`
		} `json:"innerError"`
	} `json:"error"`
}

func (e *GraphError) String() string {
	return fmt.Sprintf("%s: %s", e.Error.Code, e.Error.Message)
}

// request makes an authenticated request to the Graph API
func (c *Client) request(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	token, err := c.token(ctx)
	if err != nil {
		return fmt.Errorf("auth error: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	reqURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("build %s request to %s: %w", method, reqURL, err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http %s %s: %w", method, reqURL, err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	// Check for errors
	if resp.StatusCode >= 400 {
		var graphErr GraphError
		if err := json.Unmarshal(respBody, &graphErr); err == nil && graphErr.Error.Code != "" {
			return fmt.Errorf("Graph API error: %s", graphErr.String())
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse successful response
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// Get performs a GET request
func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	return c.request(ctx, "GET", path, nil, result)
}

// Post performs a POST request
func (c *Client) Post(ctx context.Context, path string, body interface{}, result interface{}) error {
	return c.request(ctx, "POST", path, body, result)
}

// Patch performs a PATCH request
func (c *Client) Patch(ctx context.Context, path string, body interface{}, result interface{}) error {
	return c.request(ctx, "PATCH", path, body, result)
}

// Delete performs a DELETE request
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.request(ctx, "DELETE", path, nil, nil)
}

// ListResponse is a generic paginated response from Graph API
type ListResponse[T any] struct {
	Value    []T    `json:"value"`
	NextLink string `json:"@odata.nextLink,omitempty"`
	Count    int    `json:"@odata.count,omitempty"`
}

// QueryParams helps build OData query parameters
type QueryParams struct {
	Select  []string
	Filter  string
	OrderBy string
	Top     int
	Skip    int
	Search  string
}

// ToQuery converts QueryParams to a URL query string
func (q *QueryParams) ToQuery() string {
	params := url.Values{}

	if len(q.Select) > 0 {
		params.Set("$select", strings.Join(q.Select, ","))
	}
	if q.Filter != "" {
		params.Set("$filter", q.Filter)
	}
	if q.OrderBy != "" {
		params.Set("$orderby", q.OrderBy)
	}
	if q.Top > 0 {
		params.Set("$top", fmt.Sprintf("%d", q.Top))
	}
	if q.Skip > 0 {
		params.Set("$skip", fmt.Sprintf("%d", q.Skip))
	}
	if q.Search != "" {
		params.Set("$search", fmt.Sprintf("\"%s\"", q.Search))
	}

	encoded := params.Encode()
	if encoded != "" {
		return "?" + encoded
	}
	return ""
}
