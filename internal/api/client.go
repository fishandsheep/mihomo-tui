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

type IPInfo struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Loc      string `json:"loc"`
	Org      string `json:"org"`
	Postal   string `json:"postal"`
	Timezone string `json:"timezone"`
	Anycast  bool   `json:"anycast"`
	Readme   string `json:"readme"`
}

const IPInfoURL = "https://ipinfo.io/json"

type ipProvider struct {
	endpoint string
	decode   func([]byte) (IPInfo, error)
}

// ipInfoProviders are tried in order; the first successful response wins.
// ipinfo.io is richest but rate-limits shared exit IPs, so fall back to other
// free sources and finally to Cloudflare's trace endpoint (IP only).
var ipInfoProviders = []ipProvider{
	{endpoint: IPInfoURL, decode: decodeIPInfoBody},
	{endpoint: "https://ipwho.is/", decode: decodeIPWhoIsBody},
	{endpoint: "http://ip-api.com/json/", decode: decodeIPAPIBody},
	{endpoint: "https://www.cloudflare.com/cdn-cgi/trace", decode: decodeCloudflareTraceBody},
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

func (c *Client) PatchTUN(ctx context.Context, enabled bool, device string) error {
	tun := map[string]any{"enable": enabled}
	if device != "" {
		tun["device"] = device
	}
	return c.doJSON(ctx, http.MethodPatch, "/configs", map[string]any{
		"tun": tun,
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

func GetIPInfo(ctx context.Context) (IPInfo, error) {
	return FetchIPInfo(ctx)
}

func FetchIPInfo(ctx context.Context) (IPInfo, error) {
	return fetchIPInfoChain(ctx, ipInfoProviders, nil)
}

func FetchIPInfoViaHTTPProxy(ctx context.Context, proxyEndpoint string) (IPInfo, error) {
	proxyURL, err := url.Parse(proxyEndpoint)
	if err != nil {
		return IPInfo{}, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	return fetchIPInfoChain(ctx, ipInfoProviders, transport)
}

func fetchIPInfoChain(ctx context.Context, providers []ipProvider, transport http.RoundTripper) (IPInfo, error) {
	failures := make([]string, 0, len(providers))
	for _, provider := range providers {
		info, err := fetchIPInfoFrom(ctx, provider, transport)
		if err == nil {
			return info, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", provider.host(), err))
		if ctx.Err() != nil {
			break
		}
	}
	return IPInfo{}, fmt.Errorf("all IP info providers failed: %s", strings.Join(failures, "; "))
}

func (p ipProvider) host() string {
	parsed, err := url.Parse(p.endpoint)
	if err != nil || parsed.Host == "" {
		return p.endpoint
	}
	return parsed.Host
}

func fetchIPInfoFrom(ctx context.Context, provider ipProvider, transport http.RoundTripper) (IPInfo, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, provider.endpoint, nil)
	if err != nil {
		return IPInfo{}, err
	}
	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return IPInfo{}, mapError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return IPInfo{}, decodeHTTPError(resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return IPInfo{}, err
	}
	return provider.decode(body)
}

func decodeIPInfoBody(body []byte) (IPInfo, error) {
	var out IPInfo
	if err := json.Unmarshal(body, &out); err != nil {
		return IPInfo{}, err
	}
	if out.IP == "" {
		return IPInfo{}, errors.New("missing ip field")
	}
	return out, nil
}

func decodeIPWhoIsBody(body []byte) (IPInfo, error) {
	var raw struct {
		IP          string  `json:"ip"`
		Message     string  `json:"message"`
		City        string  `json:"city"`
		Region      string  `json:"region"`
		CountryCode string  `json:"country_code"`
		Postal      string  `json:"postal"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Connection  struct {
			ISP string `json:"isp"`
			Org string `json:"org"`
		} `json:"connection"`
		Timezone struct {
			ID string `json:"id"`
		} `json:"timezone"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return IPInfo{}, err
	}
	if raw.Message != "" {
		return IPInfo{}, errors.New(raw.Message)
	}
	if raw.IP == "" {
		return IPInfo{}, errors.New("missing ip field")
	}
	org := raw.Connection.Org
	if org == "" {
		org = raw.Connection.ISP
	}
	return IPInfo{
		IP:       raw.IP,
		City:     raw.City,
		Region:   raw.Region,
		Country:  raw.CountryCode,
		Postal:   raw.Postal,
		Loc:      latLon(raw.Latitude, raw.Longitude),
		Org:      org,
		Timezone: raw.Timezone.ID,
	}, nil
}

func decodeIPAPIBody(body []byte) (IPInfo, error) {
	var raw struct {
		Status      string  `json:"status"`
		Message     string  `json:"message"`
		Query       string  `json:"query"`
		City        string  `json:"city"`
		RegionName  string  `json:"regionName"`
		CountryCode string  `json:"countryCode"`
		Zip         string  `json:"zip"`
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		Timezone    string  `json:"timezone"`
		ISP         string  `json:"isp"`
		Org         string  `json:"org"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return IPInfo{}, err
	}
	if raw.Status != "success" {
		if raw.Message != "" {
			return IPInfo{}, errors.New(raw.Message)
		}
		return IPInfo{}, fmt.Errorf("status %q", raw.Status)
	}
	if raw.Query == "" {
		return IPInfo{}, errors.New("missing query field")
	}
	org := raw.Org
	if org == "" {
		org = raw.ISP
	}
	return IPInfo{
		IP:       raw.Query,
		City:     raw.City,
		Region:   raw.RegionName,
		Country:  raw.CountryCode,
		Postal:   raw.Zip,
		Loc:      latLon(raw.Lat, raw.Lon),
		Org:      org,
		Timezone: raw.Timezone,
	}, nil
}

func decodeCloudflareTraceBody(body []byte) (IPInfo, error) {
	var out IPInfo
	for _, line := range strings.Split(string(body), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "ip":
			out.IP = value
		case "loc":
			out.Country = value
		}
	}
	if out.IP == "" {
		return IPInfo{}, errors.New("missing ip field in trace body")
	}
	return out, nil
}

func latLon(lat, lon float64) string {
	if lat == 0 && lon == 0 {
		return ""
	}
	return fmt.Sprintf("%.4f,%.4f", lat, lon)
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
