package app

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/metacubex/mihomo-tui/internal/api"
	"github.com/metacubex/mihomo-tui/internal/compat"
	"github.com/metacubex/mihomo-tui/internal/profile"
)

type fakeService struct {
	snapshot compat.Snapshot
	caps     compat.Capabilities
	loadErr  error
	modeErr  error
	proxyErr error
	delayErr error

	setModeCalls []string
	switchCalls  [][2]string
}

func (f *fakeService) LoadSnapshot(context.Context, profile.Profile) (compat.Snapshot, compat.Capabilities, error) {
	return f.snapshot, f.caps, f.loadErr
}

func (f *fakeService) SetMode(_ context.Context, _ profile.Profile, mode string) (compat.Config, error) {
	f.setModeCalls = append(f.setModeCalls, mode)
	if f.modeErr != nil {
		return compat.Config{}, f.modeErr
	}
	return compat.Config{Mode: mode}, nil
}

func (f *fakeService) SwitchProxy(_ context.Context, _ profile.Profile, group, node string) (compat.Proxy, error) {
	f.switchCalls = append(f.switchCalls, [2]string{group, node})
	if f.proxyErr != nil {
		return compat.Proxy{}, f.proxyErr
	}
	return compat.Proxy{Name: group, Now: node}, nil
}

func (f *fakeService) RunDelay(context.Context, profile.Profile, string) (api.DelayResult, error) {
	if f.delayErr != nil {
		return api.DelayResult{}, f.delayErr
	}
	return api.DelayResult{Delay: 25}, nil
}

func TestPaneSwitch(t *testing.T) {
	t.Parallel()

	model := newTestModel(&fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := next.(Model)
	if got.activePane != PaneNodes {
		t.Fatalf("expected pane nodes, got %v", got.activePane)
	}
}

func TestModeSwitchSuccessAndFailure(t *testing.T) {
	t.Parallel()

	svc := &fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}}
	model := newTestModel(svc)
	model.activePane = PaneModes

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = next.(Model)
	model, _ = runCmd(t, model, cmd)
	if model.snapshot.Config.Mode != "rule" {
		t.Fatalf("expected rule mode, got %s", model.snapshot.Config.Mode)
	}

	svc.modeErr = errors.New("boom")
	model.modeCursor = 1
	next, cmd = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = next.(Model)
	model, _ = runCmd(t, model, cmd)
	if model.toast != "boom" {
		t.Fatalf("expected error toast, got %q", model.toast)
	}
}

func TestGroupEnterFocusesNodesAndSpaceSwitchesNode(t *testing.T) {
	t.Parallel()

	svc := &fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}}
	model := newTestModel(svc)
	model.activePane = PaneGroups

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.activePane != PaneNodes {
		t.Fatalf("expected nodes focus after enter, got %v", model.activePane)
	}

	model.nodeCursor = 1
	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = next.(Model)
	model, _ = runCmd(t, model, cmd)
	if len(svc.switchCalls) != 1 {
		t.Fatalf("expected switch call")
	}
	if svc.switchCalls[0] != [2]string{"Auto", "NodeB"} {
		t.Fatalf("unexpected switch call: %#v", svc.switchCalls[0])
	}
}

func TestReconnectStateRecovery(t *testing.T) {
	t.Parallel()

	svc := &fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}, loadErr: &api.Error{Kind: api.ErrConnect, Message: "dial tcp"}}
	model := newTestModel(svc)

	msg := model.loadSnapshotCmd()()
	next, _ := model.Update(msg)
	model = next.(Model)
	if model.connected {
		t.Fatalf("expected disconnected state")
	}

	svc.loadErr = nil
	msg = model.loadSnapshotCmd()()
	next, _ = model.Update(msg)
	model = next.(Model)
	if !model.connected {
		t.Fatalf("expected reconnect success")
	}
}

func TestResizeSelectsScreenModeAndTooSmallState(t *testing.T) {
	t.Parallel()

	model := newTestModel(&fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}})
	next, _ := model.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	model = next.(Model)
	state := model.renderState()
	if !state.TooSmall {
		t.Fatalf("expected too small state")
	}

	next, _ = model.Update(tea.WindowSizeMsg{Width: 156, Height: 44})
	model = next.(Model)
	state = model.renderState()
	if state.TooSmall {
		t.Fatalf("expected fitting screen mode")
	}
	if state.ScreenMode.Name != "Wide" {
		t.Fatalf("expected wide mode, got %s", state.ScreenMode.Name)
	}
}

func newTestModel(svc Service) Model {
	model := NewModel(Options{
		Store: &profile.Store{},
		DirectProfile: profile.Profile{
			Name:          "test",
			ControllerURL: "http://127.0.0.1:9090",
		},
		Service: svc,
	})
	model.snapshot = fixtureSnapshot()
	model.capabilities = compat.Capabilities{Delay: true}
	model.connected = true
	model.syncCursors()
	return model
}

func runCmd(t *testing.T, model Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return model, nil
	}
	msg := cmd()
	next, nextCmd := model.Update(msg)
	return next.(Model), nextCmd
}

func fixtureSnapshot() compat.Snapshot {
	return compat.Snapshot{
		Version: compat.Version{Core: "1.0.0", Meta: "mihomo"},
		Config:  compat.Config{Mode: "rule"},
		Proxies: map[string]compat.Proxy{
			"NodeA": {Name: "NodeA", Alive: true, History: []compat.DelayHistory{{Delay: 10}}},
			"NodeB": {Name: "NodeB", Alive: true, History: []compat.DelayHistory{{Delay: 20}}},
		},
		Groups: []compat.ProxyGroup{
			{Name: "Auto", Type: "selector", Now: "NodeA", Options: []string{"NodeA", "NodeB"}, TestURL: compat.DefaultTestURL},
		},
	}
}
