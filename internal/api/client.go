package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ErrorKind string

const (
	ErrAuth            ErrorKind = "auth"
	ErrConnect         ErrorKind = "connect"
	ErrTimeout         ErrorKind = "timeout"
	ErrMissingEndpoint ErrorKind = "missing_endpoint"
	ErrBadResponse     ErrorKind = "bad_response"
	ErrServer          ErrorKind = "server"
)

type Error struct {
	Kind       ErrorKind
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

type Client struct {
	baseURL    string
	secret     string
	httpClient *http.Client
}

type DelayResult struct {
	Delay int `json:"delay"`
}

func New(baseURL, unixSocket, secret string, tlsSkipVerify bool) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	resolvedBaseURL := strings.TrimRight(baseURL, "/")
	if unixSocket != "" {
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", unixSocket)
		}
		resolvedBaseURL = "http://unix"
	} else if strings.HasPrefix(baseURL, "https://") {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: tlsSkipVerify} //nolint:gosec
	}

	return &Client{
		baseURL: resolvedBaseURL,
		secret:  secret,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) GetVersion(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/version", nil, &out)
	return out, err
}

func (c *Client) GetConfigs(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/configs", nil, &out)
	return out, err
}

func (c *Client) PatchMode(ctx context.Context, mode string) error {
	return c.doJSON(ctx, http.MethodPatch, "/configs", map[string]string{"mode": mode}, nil)
}

func (c *Client) PutMode(ctx context.Context, mode string) error {
	return c.doJSON(ctx, http.MethodPut, "/configs", map[string]string{"mode": mode}, nil)
}

func (c *Client) PatchTUN(ctx context.Context, enabled bool) error {
	return c.doJSON(ctx, http.MethodPatch, "/configs", map[string]any{
		"tun": map[string]bool{"enable": enabled},
	}, nil)
}

func (c *Client) GetProxies(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/proxies", nil, &out)
	return out, err
}

func (c *Client) GetProxy(ctx context.Context, name string) (map[string]any, error) {
	var out map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/proxies/"+url.PathEscape(name), nil, &out)
	return out, err
}

func (c *Client) UpdateProxy(ctx context.Context, group, name string) error {
	return c.doJSON(ctx, http.MethodPut, "/proxies/"+url.PathEscape(group), map[string]string{"name": name}, nil)
}

func (c *Client) GetDelay(ctx context.Context, name, testURL string, timeout time.Duration) (DelayResult, error) {
	query := url.Values{}
	query.Set("url", testURL)
	query.Set("timeout", fmt.Sprintf("%d", timeout.Milliseconds()))
	query.Set("expected", "200-299")

	var out DelayResult
	err := c.doJSON(ctx, http.MethodGet, "/proxies/"+url.PathEscape(name)+"/delay?"+query.Encode(), nil, &out)
	return out, err
}

func (c *Client) ProbeDelayEndpoint(ctx context.Context, name string) error {
	return c.doJSON(ctx, http.MethodGet, "/proxies/"+url.PathEscape(name)+"/delay", nil, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return mapError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeHTTPError(resp)
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func decodeHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(body))
	var payload struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Message != "" {
		msg = payload.Message
	}
	if msg == "" {
		msg = resp.Status
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &Error{Kind: ErrAuth, StatusCode: resp.StatusCode, Message: msg}
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return &Error{Kind: ErrMissingEndpoint, StatusCode: resp.StatusCode, Message: msg}
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return &Error{Kind: ErrTimeout, StatusCode: resp.StatusCode, Message: msg}
	case http.StatusBadRequest:
		return &Error{Kind: ErrBadResponse, StatusCode: resp.StatusCode, Message: msg}
	default:
		return &Error{Kind: ErrServer, StatusCode: resp.StatusCode, Message: msg}
	}
}

func mapError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Kind: ErrTimeout, Message: err.Error()}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Error{Kind: ErrTimeout, Message: err.Error()}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return &Error{Kind: ErrConnect, Message: err.Error()}
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return &Error{Kind: ErrTimeout, Message: err.Error()}
		}
		return &Error{Kind: ErrConnect, Message: err.Error()}
	}

	return err
}
