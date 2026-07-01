package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		tun, ok := body["tun"].(map[string]any)
		if !ok || tun["enable"] != true {
			t.Fatalf("unexpected tun body: %#v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(server.URL, "", "", false)
	if err := client.PatchTUN(context.Background(), true); err != nil {
		t.Fatalf("PatchTUN failed: %v", err)
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

	info, err := FetchIPInfo(context.Background(), server.URL)
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

	info, err := FetchIPInfoViaHTTPProxy(context.Background(), "http://ipinfo.test/json", proxy.URL)
	if err != nil {
		t.Fatalf("FetchIPInfoViaHTTPProxy failed: %v", err)
	}
	if info.IP != "198.51.100.9" || info.Country != "GB" {
		t.Fatalf("unexpected proxied ip info: %#v", info)
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
