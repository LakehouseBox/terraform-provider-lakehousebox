package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client speaks the LakehouseBox API with one key (an API key or agent key, LHBOX_API_KEY): the routes a person's
// `lhbox` uses, with the same checks.
type Client struct {
	Endpoint string
	APIKey   string
	HTTP     *http.Client
	UA       string
}

// APIError is the API's error envelope: {"error": {code, message, remedy, ...}}.
type APIError struct {
	Status  int
	Code    string
	Message string
	Remedy  string
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("%s (HTTP %d): %s", e.Code, e.Status, e.Message)
	if e.Remedy != "" {
		s += " Remedy: " + e.Remedy
	}
	return s
}

// goneCodes: the API's 404 codes that mean "this resource is not there" (deleted outside OpenTofu). A bare `not_found` is
// a route the deployment does not serve (sinks or scheduled SQL off, a wrong endpoint): never read as gone, or every
// resource would drop out of state on a refresh (high review of #3032).
var goneCodes = map[string]bool{"sink_not_found": true, "connection_not_found": true, "warehouse_not_found": true,
	"warehouse_deleted": true, "table_not_found": true, "namespace_not_found": true, "post_prefix_not_found": true}

// NotFound is a 404 of the API naming the resource as gone.
func NotFound(err error) bool {
	e, ok := err.(*APIError)
	return ok && e.Status == 404 && goneCodes[e.Code]
}

// Do sends one request; a body is JSON; out (a pointer) receives the JSON answer. Errors are *APIError when the API
// answered with its envelope.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := strings.TrimRight(c.Endpoint, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("User-Agent", c.UA)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var env struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Remedy  string `json:"remedy"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &env)
		if env.Error.Code == "" {
			env.Error.Code, env.Error.Message = "http_error", strings.TrimSpace(string(raw))
		}
		return &APIError{Status: resp.StatusCode, Code: env.Error.Code, Message: env.Error.Message, Remedy: env.Error.Remedy}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func newClient(endpoint, key, version string) *Client {
	return &Client{Endpoint: endpoint, APIKey: key, HTTP: &http.Client{Timeout: 5 * time.Minute},
		UA: "terraform-provider-lakehousebox/" + version}
}
