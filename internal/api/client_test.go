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
