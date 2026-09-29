package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/metacubex/mihomo-tui/internal/compat"
	"github.com/metacubex/mihomo-tui/internal/profile"
	"github.com/metacubex/mihomo-tui/internal/view"
)

func TestControllerServiceEndToEndMultiProfileIsolation(t *testing.T) {
	t.Parallel()

	serverA := newMockController("alpha")
	defer serverA.Close()
	serverB := newMockController("beta")
	defer serverB.Close()

	store, err := profile.NewStore(filepath.Join(t.TempDir(), "profiles.json"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	if err := store.Upsert(profile.Profile{Name: "alpha", ControllerURL: serverA.URL, Default: true}); err != nil {
		t.Fatalf("store alpha: %v", err)
	}
	if err := store.Upsert(profile.Profile{Name: "beta", ControllerURL: serverB.URL}); err != nil {
		t.Fatalf("store beta: %v", err)
	}

	model := NewModel(Options{
		Store:          store,
		InitialProfile: "alpha",
		Service:        controllerService{},
	})
	model.width = 156
	model.height = 44
	now := time.Unix(100, 0)
	model.now = func() time.Time { return now }

	msg := model.loadSnapshotCmd()()
	next, _ := model.Update(msg)
	model = next.(Model)
	if model.snapshot.Config.Mode != "rule" {
		t.Fatalf("expected alpha rule mode, got %s", model.snapshot.Config.Mode)
	}

	model.activePane = PaneSessions
	model.sessionCursor = 1
	layout := view.ComputeLayout(model.renderState())
	click := tea.MouseMsg{X: layout.Sessions.X + 2, Y: layout.Sessions.Y + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, cmd := model.Update(click)
	model = next.(Model)
	now = now.Add(200 * time.Millisecond)
	next, cmd = model.Update(click)
	model = next.(Model)
	msg = cmd()
	next, _ = model.Update(msg)
	model = next.(Model)
	if model.activeProfile.Name != "beta" {
		t.Fatalf("expected beta active profile, got %s", model.activeProfile.Name)
	}

	modeMsg := model.setModeCmd("global")()
	next, _ = model.Update(modeMsg)
	model = next.(Model)
	if serverB.state.mode != "global" {
		t.Fatalf("expected beta mode update")
	}
	if serverA.state.mode != "rule" {
		t.Fatalf("alpha state polluted: %s", serverA.state.mode)
	}

	tunMsg := model.setTUNCmd(true)()
	next, _ = model.Update(tunMsg)
	model = next.(Model)
	if !serverB.state.tun {
		t.Fatalf("expected beta tun enabled")
	}
	if serverA.state.tun {
		t.Fatalf("alpha tun polluted")
	}

	modeMsg = model.setModeCmd("rule")()
	next, _ = model.Update(modeMsg)
	model = next.(Model)

	model.activePane = PaneNodes
	model.nodeCursor = 1
	next, cmd = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = next.(Model)
	msg = cmd()
	next, _ = model.Update(msg)
	model = next.(Model)
	if serverB.state.groupNow != "NodeB" {
		t.Fatalf("expected beta group switch, got %s", serverB.state.groupNow)
	}
	if serverA.state.groupNow != "NodeA" {
		t.Fatalf("alpha selector polluted: %s", serverA.state.groupNow)
	}

	delayMsg := model.delayCmd("NodeB")()
	next, _ = model.Update(delayMsg)
	model = next.(Model)
	if !strings.Contains(model.toast, "25ms") {
		t.Fatalf("expected delay toast, got %q", model.toast)
	}
}

type mockController struct {
	*httptest.Server
	state *mockControllerState
}

type mockControllerState struct {
	mu       sync.Mutex
	name     string
	mode     string
	groupNow string
	tun      bool
}

func newMockController(name string) *mockController {
	state := &mockControllerState{name: name, mode: "rule", groupNow: "NodeA"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/version":
			json.NewEncoder(w).Encode(map[string]string{"version": "1.0.0", "meta": name})
		case r.Method == http.MethodGet && r.URL.Path == "/configs":
			json.NewEncoder(w).Encode(map[string]any{"mode": state.mode, "tun": map[string]bool{"enable": state.tun}})
		case r.Method == http.MethodPatch && r.URL.Path == "/configs":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if mode, ok := body["mode"].(string); ok {
				state.mode = mode
			}
			if tun, ok := body["tun"].(map[string]any); ok {
				if enabled, ok := tun["enable"].(bool); ok {
					state.tun = enabled
				}
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/proxies":
			json.NewEncoder(w).Encode(map[string]any{
				"proxies": map[string]any{
					"Halsh Cloud": map[string]any{
						"name":    "Halsh Cloud",
						"type":    "Selector",
						"now":     state.groupNow,
						"all":     []string{"NodeA", "NodeB"},
						"alive":   true,
						"testUrl": compat.DefaultTestURL,
					},
					"GLOBAL": map[string]any{
						"name":  "GLOBAL",
						"type":  "Selector",
						"now":   state.groupNow,
						"all":   []string{"NodeA", "NodeB", "Halsh Cloud"},
						"alive": true,
					},
					"NodeA": map[string]any{
						"name":    "NodeA",
						"type":    "Trojan",
						"alive":   true,
						"history": []map[string]any{{"delay": 10}},
					},
					"NodeB": map[string]any{
						"name":    "NodeB",
						"type":    "Trojan",
						"alive":   true,
						"history": []map[string]any{{"delay": 25}},
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/proxies/Halsh Cloud":
			json.NewEncoder(w).Encode(map[string]any{
				"name": "Halsh Cloud",
				"type": "Selector",
				"now":  state.groupNow,
				"all":  []string{"NodeA", "NodeB"},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/proxies/Halsh Cloud":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			state.groupNow = body["name"]
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/proxies/GLOBAL/delay":
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"message": "Body invalid"})
		case r.Method == http.MethodGet && r.URL.Path == "/proxies/Halsh Cloud/delay":
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"message": "Body invalid"})
		case r.Method == http.MethodGet && r.URL.Path == "/proxies/NodeB/delay":
			json.NewEncoder(w).Encode(map[string]int{"delay": 25})
		default:
			http.NotFound(w, r)
		}
	}))
	return &mockController{Server: server, state: state}
}

func TestMain(m *testing.M) {
	_ = os.Setenv("MIHOMO_TUI_CONFIG", "")
	os.Exit(m.Run())
}

func TestControllerServiceDelayCapabilityFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			json.NewEncoder(w).Encode(map[string]string{"version": "1.0.0"})
		case "/configs":
			json.NewEncoder(w).Encode(map[string]any{"mode": "rule", "tun": map[string]bool{"enable": false}})
		case "/proxies":
			json.NewEncoder(w).Encode(map[string]any{
				"proxies": map[string]any{
					"Halsh Cloud": map[string]any{
						"name": "Halsh Cloud",
						"type": "Selector",
						"now":  "NodeA",
						"all":  []string{"NodeA"},
					},
					"GLOBAL": map[string]any{
						"name": "GLOBAL",
						"type": "Selector",
						"now":  "NodeA",
						"all":  []string{"NodeA", "Halsh Cloud"},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	snapshot, caps, err := controllerService{}.LoadSnapshot(context.Background(), profile.Profile{
		Name:          "one",
		ControllerURL: server.URL,
	})
	if err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}
	if caps.Delay {
		t.Fatalf("expected delay unsupported")
	}
	if len(snapshot.Groups) != 2 {
		t.Fatalf("unexpected snapshot groups: %#v", snapshot.Groups)
	}
}

func TestControllerServiceSetTUNReturnsErrorWhenBackendDoesNotConfirm(t *testing.T) {
	t.Parallel()

	patches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/configs":
			patches++
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/configs":
			json.NewEncoder(w).Encode(map[string]any{"mode": "rule", "tun": map[string]bool{"enable": false}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := controllerService{}.SetTUN(context.Background(), profile.Profile{
		Name:          "one",
		ControllerURL: server.URL,
	}, true)
	if err == nil || !strings.Contains(err.Error(), "did not apply TUN=true") {
		t.Fatalf("expected unconfirmed TUN error, got %v", err)
	}
	if want := 1 + tunMaxDeviceCandidates; patches != want {
		t.Fatalf("expected %d patch attempts (default + device candidates), got %d", want, patches)
	}
}

func TestControllerServiceSetTUNRetriesWithAlternateDevice(t *testing.T) {
	t.Parallel()

	type patchCall struct {
		enable bool
		device string
	}
	mu := sync.Mutex{}
	var patches []patchCall
	applied := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/configs":
			var body struct {
				Tun struct {
					Enable bool   `json:"enable"`
					Device string `json:"device"`
				} `json:"tun"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode patch body: %v", err)
			}
			mu.Lock()
			patches = append(patches, patchCall{enable: body.Tun.Enable, device: body.Tun.Device})
			mu.Unlock()
			// Simulate the stale-device failure: only a patch with a fresh
			// device name is applied, the default name stays unconfirmed.
			if body.Tun.Enable && body.Tun.Device != "" {
				applied = body.Tun.Device
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/configs":
			mu.Lock()
			device := applied
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{
				"mode": "rule",
				"tun":  map[string]any{"enable": device != "", "device": device},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	config, err := controllerService{}.SetTUN(context.Background(), profile.Profile{
		Name:          "one",
		ControllerURL: server.URL,
	}, true)
	if err != nil {
		t.Fatalf("SetTUN should recover via alternate device, got %v", err)
	}
	if !config.TunEnabled {
		t.Fatalf("expected confirmed tun state, got %#v", config)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(patches) != 2 {
		t.Fatalf("expected default + first free device patch, got %#v", patches)
	}
	if patches[0].device != "" || patches[1].device == "" {
		t.Fatalf("unexpected patch sequence: %#v", patches)
	}
	if applied != patches[1].device {
		t.Fatalf("expected applied device %q to match final patch, got %q", patches[1].device, applied)
	}
}

func TestNextTUNDeviceCandidatesSkipUsedNames(t *testing.T) {
	t.Parallel()

	// Simulate three leaked devices, as left behind by mihomo disable bugs.
	candidates := nextTUNDeviceCandidates(map[string]bool{
		"Meta": true, "mihomo-tui": true, "mihomo-tui-2": true, "lo": true, "wlp5s0": true,
	})
	if len(candidates) != tunMaxDeviceCandidates {
		t.Fatalf("expected %d candidates, got %#v", tunMaxDeviceCandidates, candidates)
	}
	if candidates[0] != "mihomo-tui-3" {
		t.Fatalf("expected first free candidate mihomo-tui-3, got %q", candidates[0])
	}
	seen := map[string]bool{}
	for _, name := range candidates {
		if seen[name] || name == "mihomo-tui" || name == "mihomo-tui-2" {
			t.Fatalf("unexpected duplicate or used candidate: %#v", candidates)
		}
		seen[name] = true
	}

	candidates = nextTUNDeviceCandidates(map[string]bool{})
	if len(candidates) != tunMaxDeviceCandidates || candidates[0] != "mihomo-tui" {
		t.Fatalf("expected clean slate to start at mihomo-tui, got %#v", candidates)
	}
}

func TestControllerServiceSetTUNWaitsForBackendState(t *testing.T) {
	t.Parallel()

	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/configs":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/configs":
			gets++
			enabled := gets > 1
			json.NewEncoder(w).Encode(map[string]any{"mode": "rule", "tun": map[string]bool{"enable": enabled}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	config, err := controllerService{}.SetTUN(context.Background(), profile.Profile{
		Name:          "one",
		ControllerURL: server.URL,
	}, true)
	if err != nil {
		t.Fatalf("SetTUN failed: %v", err)
	}
	if !config.TunEnabled {
		t.Fatalf("expected backend tun state true, got %#v", config)
	}
	if gets < 2 {
		t.Fatalf("expected backend state polling, got %d GETs", gets)
	}
}

func TestControllerServiceLoadIPInfoRejectsDirectFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/configs" {
			http.Error(w, "controller unavailable", http.StatusServiceUnavailable)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := controllerService{}.LoadIPInfo(context.Background(), profile.Profile{
		Name:          "one",
		ControllerURL: server.URL,
	})
	if err == nil || !strings.Contains(err.Error(), "load proxy config") {
		t.Fatalf("expected proxied IP configuration error, got %v", err)
	}
}

func TestProxyEndpointPrefersMixedPort(t *testing.T) {
	endpoint := proxyEndpoint(profile.Profile{ControllerURL: "http://127.0.0.1:9090"}, compat.Config{
		MixedPort: 7890,
		Port:      7891,
	})
	if endpoint != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected proxy endpoint: %q", endpoint)
	}
}

func TestProxyEndpointUsesLocalhostForUnixSocket(t *testing.T) {
	endpoint := proxyEndpoint(profile.Profile{UnixSocket: "/tmp/verge/verge-mihomo.sock"}, compat.Config{
		MixedPort: 7897,
	})
	if endpoint != "http://127.0.0.1:7897" {
		t.Fatalf("unexpected unix-socket proxy endpoint: %q", endpoint)
	}
}

func TestProxyEndpointWithoutPortsReturnsEmpty(t *testing.T) {
	if endpoint := proxyEndpoint(profile.Profile{UnixSocket: "/tmp/verge/verge-mihomo.sock"}, compat.Config{}); endpoint != "" {
		t.Fatalf("expected empty endpoint without ports, got %q", endpoint)
	}
	if endpoint := proxyEndpoint(profile.Profile{ControllerURL: "http://192.168.1.4:9090"}, compat.Config{Port: 7891}); endpoint != "http://192.168.1.4:7891" {
		t.Fatalf("expected remote controller host fallback to http port, got %q", endpoint)
	}
}
