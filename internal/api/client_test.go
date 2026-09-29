package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientInjectsBearerToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer top-secret" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		json.NewEncoder(w).Encode(map[string]string{"version": "1.0.0"})
	}))
	defer server.Close()

	client := New(server.URL, "", "top-secret", false)
	if _, err := client.GetVersion(context.Background()); err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
}

func TestPatchModeBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/configs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["mode"] != "global" {
			t.Fatalf("unexpected mode body: %#v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(server.URL, "", "", false)
	if err := client.PatchMode(context.Background(), "global"); err != nil {
		t.Fatalf("PatchMode failed: %v", err)
	}
}

func TestUpdateProxyBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/proxies/Proxy" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["name"] != "NodeA" {
			t.Fatalf("unexpected request body: %#v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(server.URL, "", "", false)
	if err := client.UpdateProxy(context.Background(), "Proxy", "NodeA"); err != nil {
		t.Fatalf("UpdateProxy failed: %v", err)
	}
}

func TestPatchTUNBody(t *testing.T) {
	t.Parallel()

	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/configs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		bodies = append(bodies, body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(server.URL, "", "", false)
	if err := client.PatchTUN(context.Background(), true, ""); err != nil {
		t.Fatalf("PatchTUN failed: %v", err)
	}
	if err := client.PatchTUN(context.Background(), true, "mihomo-tui"); err != nil {
		t.Fatalf("PatchTUN with device failed: %v", err)
	}

	tun, ok := bodies[0]["tun"].(map[string]any)
	if !ok || tun["enable"] != true {
		t.Fatalf("unexpected tun body: %#v", bodies[0])
	}
	if _, hasDevice := tun["device"]; hasDevice {
		t.Fatalf("unexpected device field in default patch: %#v", bodies[0])
	}
	tun, ok = bodies[1]["tun"].(map[string]any)
	if !ok || tun["enable"] != true || tun["device"] != "mihomo-tui" {
		t.Fatalf("unexpected fallback tun body: %#v", bodies[1])
	}
}

func TestDelayMissingEndpointMapsToCapabilityFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := New(server.URL, "", "", false)
	err := client.ProbeDelayEndpoint(context.Background(), "Proxy")
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected api error, got %T", err)
	}
	if apiErr.Kind != ErrMissingEndpoint {
		t.Fatalf("unexpected error kind: %s", apiErr.Kind)
	}
}

func TestDelayRequestEncodesQuery(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("url") != "https://www.gstatic.com/generate_204" {
			t.Fatalf("unexpected delay url: %s", r.URL.Query().Get("url"))
		}
		if r.URL.Query().Get("timeout") != "5000" {
			t.Fatalf("unexpected timeout: %s", r.URL.Query().Get("timeout"))
		}
		json.NewEncoder(w).Encode(DelayResult{Delay: 42})
	}))
	defer server.Close()

	client := New(server.URL, "", "", false)
	result, err := client.GetDelay(context.Background(), "NodeA", "https://www.gstatic.com/generate_204", 5*time.Second)
	if err != nil {
		t.Fatalf("GetDelay failed: %v", err)
	}
	if result.Delay != 42 {
		t.Fatalf("unexpected delay result: %#v", result)
	}
}

func TestFetchIPInfoDecodesResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		json.NewEncoder(w).Encode(IPInfo{
			IP:       "203.0.113.8",
			Hostname: "example.net",
			City:     "Tokyo",
			Region:   "Tokyo",
			Country:  "JP",
			Loc:      "35.6895,139.6917",
			Org:      "AS64500 Example",
			Postal:   "100-0001",
			Timezone: "Asia/Tokyo",
			Anycast:  true,
			Readme:   "https://ipinfo.io/missingauth",
		})
	}))
	defer server.Close()

	info, err := fetchIPInfoChain(context.Background(), []ipProvider{{endpoint: server.URL, decode: decodeIPInfoBody}}, nil)
	if err != nil {
		t.Fatalf("FetchIPInfo failed: %v", err)
	}
	if info.IP != "203.0.113.8" || info.Country != "JP" || !info.Anycast {
		t.Fatalf("unexpected ip info: %#v", info)
	}
}

func TestFetchIPInfoViaHTTPProxy(t *testing.T) {
	t.Parallel()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.String() != "http://ipinfo.test/json" {
			t.Fatalf("unexpected proxy request URL: %s", r.URL.String())
		}
		json.NewEncoder(w).Encode(IPInfo{IP: "198.51.100.9", Country: "GB"})
	}))
	defer proxy.Close()

	info, err := fetchIPInfoChain(context.Background(), []ipProvider{{endpoint: "http://ipinfo.test/json", decode: decodeIPInfoBody}}, proxyTransport(t, proxy.URL))
	if err != nil {
		t.Fatalf("FetchIPInfoViaHTTPProxy failed: %v", err)
	}
	if info.IP != "198.51.100.9" || info.Country != "GB" {
		t.Fatalf("unexpected proxied ip info: %#v", info)
	}
}

func proxyTransport(t *testing.T, proxyEndpoint string) http.RoundTripper {
	t.Helper()
	proxyURL, err := url.Parse(proxyEndpoint)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	return transport
}

func TestFetchIPInfoChainFallsBackOnProviderFailure(t *testing.T) {
	t.Parallel()

	rateLimited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "rate limited"})
	}))
	defer rateLimited.Close()
	working := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"query": "198.51.100.10", "status": "success", "countryCode": "SG", "city": "Singapore", "timezone": "Asia/Singapore", "isp": "Example ISP", "lat": 1.29, "lon": 103.85})
	}))
	defer working.Close()

	providers := []ipProvider{
		{endpoint: rateLimited.URL, decode: decodeIPInfoBody},
		{endpoint: working.URL, decode: decodeIPAPIBody},
	}
	info, err := fetchIPInfoChain(context.Background(), providers, nil)
	if err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if info.IP != "198.51.100.10" || info.Country != "SG" || info.City != "Singapore" {
		t.Fatalf("unexpected fallback ip info: %#v", info)
	}
}

func TestFetchIPInfoChainReportsAllFailures(t *testing.T) {
	t.Parallel()

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer down.Close()

	providers := []ipProvider{
		{endpoint: down.URL, decode: decodeIPInfoBody},
		{endpoint: "http://definitely.invalid/json", decode: decodeIPInfoBody},
	}
	_, err := fetchIPInfoChain(context.Background(), providers, nil)
	if err == nil {
		t.Fatalf("expected chain failure error")
	}
	if !strings.Contains(err.Error(), "all IP info providers failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIPProviderDecoders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		decode  func([]byte) (IPInfo, error)
		body    string
		want    IPInfo
		wantErr bool
	}{
		{
			name:   "ipwho.is",
			decode: decodeIPWhoIsBody,
			body:   `{"ip":"203.0.113.9","city":"Singapore","region":"Central Singapore","country":"Singapore","country_code":"SG","postal":"018989","latitude":1.289,"longitude":103.85,"connection":{"org":"Amazon.com, Inc.","isp":"AWS EC2"},"timezone":{"id":"Asia/Singapore"}}`,
			want:   IPInfo{IP: "203.0.113.9", City: "Singapore", Region: "Central Singapore", Country: "SG", Postal: "018989", Loc: "1.2890,103.8500", Org: "Amazon.com, Inc.", Timezone: "Asia/Singapore"},
		},
		{
			name:    "ipwho.is error",
			decode:  decodeIPWhoIsBody,
			body:    `{"ip":"","success":false,"message":"quota exceeded"}`,
			wantErr: true,
		},
		{
			name:   "ip-api.com",
			decode: decodeIPAPIBody,
			body:   `{"status":"success","query":"203.0.113.10","country":"Singapore","countryCode":"SG","region":"01","regionName":"Central Singapore","city":"Singapore","zip":"018989","lat":1.289,"lon":103.85,"timezone":"Asia/Singapore","isp":"Example ISP","org":"AS64500 Example"}`,
			want:   IPInfo{IP: "203.0.113.10", City: "Singapore", Region: "Central Singapore", Country: "SG", Postal: "018989", Loc: "1.2890,103.8500", Org: "AS64500 Example", Timezone: "Asia/Singapore"},
		},
		{
			name:    "ip-api.com failure status",
			decode:  decodeIPAPIBody,
			body:    `{"status":"fail","message":"reserved range","query":"10.0.0.1"}`,
			wantErr: true,
		},
		{
			name:   "cloudflare trace",
			decode: decodeCloudflareTraceBody,
			body:   "fl=abc\nh=www.cloudflare.com\nip=203.0.113.11\nts=1700000000\nvisit_scheme=https\ncolo=SIN\nloc=SG\ntls=TLSv1.3\nwarp=off\n",
			want:   IPInfo{IP: "203.0.113.11", Country: "SG"},
		},
		{
			name:    "cloudflare trace missing ip",
			decode:  decodeCloudflareTraceBody,
			body:    "fl=abc\nloc=SG\n",
			wantErr: true,
		},
		{
			name:   "ipinfo.io",
			decode: decodeIPInfoBody,
			body:   `{"ip":"203.0.113.12","city":"Tokyo","country":"JP","org":"AS64500 Example"}`,
			want:   IPInfo{IP: "203.0.113.12", City: "Tokyo", Country: "JP", Org: "AS64500 Example"},
		},
		{
			name:    "ipinfo.io missing ip",
			decode:  decodeIPInfoBody,
			body:    `{"city":"Tokyo"}`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.decode([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected ip info: %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestClientSupportsUnixSocket(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), "mihomo.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer os.Remove(socketPath)

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sock-secret" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if r.URL.Path != "/version" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{"version": "1.0.0"})
	})}
	defer server.Close()
	defer listener.Close()
	go server.Serve(listener)

	client := New("", socketPath, "sock-secret", false)
	if _, err := client.GetVersion(context.Background()); err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
}
