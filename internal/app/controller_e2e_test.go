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
	if len(snapshot.Groups) != 1 {
		t.Fatalf("unexpected snapshot groups: %#v", snapshot.Groups)
	}
}
