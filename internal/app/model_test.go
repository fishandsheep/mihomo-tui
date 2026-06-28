package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/metacubex/mihomo-tui/internal/api"
	"github.com/metacubex/mihomo-tui/internal/compat"
	"github.com/metacubex/mihomo-tui/internal/profile"
	"github.com/metacubex/mihomo-tui/internal/view"
)

type fakeService struct {
	snapshot compat.Snapshot
	caps     compat.Capabilities
	loadErr  error
	modeErr  error
	tunErr   error
	proxyErr error
	delayErr error

	setModeCalls []string
	setTUNCalls  []bool
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
	return compat.Config{Mode: mode, TunEnabled: f.snapshot.Config.TunEnabled, TunSupported: f.snapshot.Config.TunSupported}, nil
}

func (f *fakeService) SetTUN(_ context.Context, _ profile.Profile, enabled bool) (compat.Config, error) {
	f.setTUNCalls = append(f.setTUNCalls, enabled)
	if f.tunErr != nil {
		return compat.Config{}, f.tunErr
	}
	return compat.Config{Mode: f.snapshot.Config.Mode, TunEnabled: enabled, TunSupported: true}, nil
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

func TestTUNToggleBySpaceAndMouse(t *testing.T) {
	t.Parallel()

	svc := &fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}}
	model := newTestModel(svc)
	model.activePane = PaneTUN

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = next.(Model)
	model, _ = runCmd(t, model, cmd)
	if len(svc.setTUNCalls) != 1 || !svc.setTUNCalls[0] {
		t.Fatalf("unexpected tun calls: %#v", svc.setTUNCalls)
	}

	now := time.Unix(100, 0)
	model.now = func() time.Time { return now }
	layout := view.ComputeLayout(model.renderState())
	pos := mouseClick(layout.TUN.X+2, layout.TUN.Y+1)

	next, cmd = model.Update(pos)
	model = next.(Model)
	if cmd != nil {
		t.Fatalf("first tun click should not trigger command")
	}

	now = now.Add(200 * time.Millisecond)
	next, cmd = model.Update(pos)
	model = next.(Model)
	if cmd == nil {
		t.Fatalf("double click should trigger tun command")
	}
}

func TestSnapshotLoadPreservesTUNWhenConfigOmitsTunBlock(t *testing.T) {
	t.Parallel()

	model := newTestModel(&fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}})
	model.snapshot.Config.TunEnabled = true
	model.snapshot.Config.TunSupported = true

	msg := snapshotLoadedMsg{
		snapshot: compat.Snapshot{
			Version: model.snapshot.Version,
			Config:  compat.Config{Mode: "rule"},
			Proxies: model.snapshot.Proxies,
			Groups:  model.snapshot.Groups,
		},
		caps: compat.Capabilities{Delay: true},
	}
	next, _ := model.Update(msg)
	model = next.(Model)
	if !model.snapshot.Config.TunEnabled || !model.snapshot.Config.TunSupported {
		t.Fatalf("expected tun state preserved, got %#v", model.snapshot.Config)
	}
}

func TestEnterDoesNothingAndSpaceSwitchesNode(t *testing.T) {
	t.Parallel()

	svc := &fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}}
	model := newTestModel(svc)
	model.activePane = PaneGroups

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.activePane != PaneGroups {
		t.Fatalf("expected enter no-op, got %v", model.activePane)
	}

	model.activePane = PaneNodes
	model.nodeCursor = 1
	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = next.(Model)
	model, _ = runCmd(t, model, cmd)
	if len(svc.switchCalls) != 1 {
		t.Fatalf("expected switch call")
	}
	if svc.switchCalls[0] != [2]string{"Halsh Cloud", "NodeB"} {
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

func TestMouseSingleClickSelectsWithoutApplying(t *testing.T) {
	t.Parallel()

	svc := &fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}}
	model := newTestModel(svc)
	model.sessions = []sessionEntry{
		{Label: "alpha", Profile: profile.Profile{Name: "alpha", ControllerURL: "http://a"}},
		{Label: "beta", Profile: profile.Profile{Name: "beta", ControllerURL: "http://b"}},
	}
	model.activeProfile = model.sessions[0].Profile
	model.syncCursors()

	layout := view.ComputeLayout(model.renderState())
	next, cmd := model.Update(mouseClick(layout.Sessions.X+2, layout.Sessions.Y+2))
	model = next.(Model)
	if cmd != nil {
		t.Fatalf("single click should not trigger command")
	}
	if model.sessionCursor != 1 {
		t.Fatalf("expected session cursor 1, got %d", model.sessionCursor)
	}
	if model.activeProfile.Name != "alpha" {
		t.Fatalf("expected active profile unchanged, got %s", model.activeProfile.Name)
	}
	if len(svc.setModeCalls) != 0 || len(svc.switchCalls) != 0 {
		t.Fatalf("unexpected action on single click")
	}
}

func TestMouseDoubleClickTriggersModeApply(t *testing.T) {
	t.Parallel()

	svc := &fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}}
	model := newTestModel(svc)
	model.activePane = PaneModes
	now := time.Unix(100, 0)
	model.now = func() time.Time { return now }

	layout := view.ComputeLayout(model.renderState())
	pos := mouseClick(layout.Modes.X+2, layout.Modes.Y+2)

	next, cmd := model.Update(pos)
	model = next.(Model)
	if cmd != nil {
		t.Fatalf("first click should not apply mode")
	}

	now = now.Add(200 * time.Millisecond)
	next, cmd = model.Update(pos)
	model = next.(Model)
	if cmd == nil {
		t.Fatalf("second click should trigger apply")
	}
	model, _ = runCmd(t, model, cmd)
	if len(svc.setModeCalls) != 1 || svc.setModeCalls[0] != "global" {
		t.Fatalf("unexpected mode calls: %#v", svc.setModeCalls)
	}
}

func TestMouseDoubleClickResetsOnDifferentItem(t *testing.T) {
	t.Parallel()

	svc := &fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}}
	model := newTestModel(svc)
	model.sessions = []sessionEntry{
		{Label: "alpha", Profile: profile.Profile{Name: "alpha", ControllerURL: "http://a"}},
		{Label: "beta", Profile: profile.Profile{Name: "beta", ControllerURL: "http://b"}},
	}
	model.activeProfile = model.sessions[0].Profile
	now := time.Unix(100, 0)
	model.now = func() time.Time { return now }
	model.syncCursors()

	layout := view.ComputeLayout(model.renderState())
	first := mouseClick(layout.Sessions.X+2, layout.Sessions.Y+1)
	second := mouseClick(layout.Sessions.X+2, layout.Sessions.Y+2)

	next, cmd := model.Update(first)
	model = next.(Model)
	if cmd != nil {
		t.Fatalf("first click should not switch")
	}
	now = now.Add(100 * time.Millisecond)
	next, cmd = model.Update(second)
	model = next.(Model)
	if cmd != nil {
		t.Fatalf("click on different item should reset double click")
	}
	if model.activeProfile.Name != "alpha" {
		t.Fatalf("expected active profile unchanged, got %s", model.activeProfile.Name)
	}
}

func TestMouseWheelScrollsHoveredPane(t *testing.T) {
	t.Parallel()

	model := newTestModel(&fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}})
	model.snapshot.Groups = []compat.ProxyGroup{{Name: "Halsh Cloud", Type: "selector", Now: "Node00", Options: manyNodes(120), TestURL: compat.DefaultTestURL}}
	model.snapshot.Config.Mode = "rule"
	model.groupCursor = 0
	model.activePane = PaneNodes
	model.ensureOffsets()

	layout := view.ComputeLayout(model.renderState())
	next, _ := model.Update(mouseWheel(layout.Nodes.X+2, layout.Nodes.Y+2, tea.MouseButtonWheelDown))
	model = next.(Model)
	if model.nodeOffset == 0 {
		t.Fatalf("expected node offset to change")
	}
	if model.groupOffset != 0 || model.sessionOffset != 0 || model.modeOffset != 0 {
		t.Fatalf("unexpected other offsets: sessions=%d modes=%d groups=%d", model.sessionOffset, model.modeOffset, model.groupOffset)
	}
}

func TestKeyboardCursorMovementAutoScrollsLists(t *testing.T) {
	t.Parallel()

	model := newTestModel(&fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}})
	model.snapshot.Groups = []compat.ProxyGroup{{Name: "Halsh Cloud", Type: "selector", Now: "Node00", Options: manyNodes(120), TestURL: compat.DefaultTestURL}}
	model.snapshot.Config.Mode = "rule"
	model.activePane = PaneNodes
	model.ensureOffsets()

	for i := 0; i < 70; i++ {
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = next.(Model)
	}
	if model.nodeCursor != 70 {
		t.Fatalf("expected cursor 70, got %d", model.nodeCursor)
	}
	if model.nodeOffset == 0 {
		t.Fatalf("expected node offset to auto-scroll")
	}
}

func TestMainPaneKeyboardScrollDoesNotChangeSelection(t *testing.T) {
	t.Parallel()

	model := newTestModel(&fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}})
	model.activePane = PaneMain
	model.activeMainTab = MainTabEvents
	model.events = make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		model.events = append(model.events, "event line")
	}
	model.ensureOffsets()
	groupBefore := model.groupCursor
	nodeBefore := model.nodeCursor

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = next.(Model)
	if model.mainOffset == 0 {
		t.Fatalf("expected main offset to scroll")
	}
	if model.groupCursor != groupBefore || model.nodeCursor != nodeBefore {
		t.Fatalf("main scroll should not change selection")
	}
}

func TestVisibleGroupsFollowMode(t *testing.T) {
	t.Parallel()

	model := newTestModel(&fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}})
	model.snapshot.Groups = []compat.ProxyGroup{
		{Name: "Halsh Cloud", Type: "selector", Now: "NodeA", Options: []string{"NodeA", "NodeB"}},
		{Name: "Auto", Type: "selector", Now: "NodeA", Options: []string{"NodeA", "NodeB"}},
		{Name: "GLOBAL", Type: "selector", Now: "NodeA", Options: []string{"NodeA", "NodeB"}},
	}

	model.snapshot.Config.Mode = "rule"
	groups := model.visibleGroups()
	if len(groups) != 1 || groups[0].Name != "Halsh Cloud" {
		t.Fatalf("rule mode should show only Halsh Cloud, got %#v", groups)
	}

	model.snapshot.Config.Mode = "global"
	groups = model.visibleGroups()
	if len(groups) != 1 || groups[0].Name != "GLOBAL" {
		t.Fatalf("global mode should show only GLOBAL, got %#v", groups)
	}

	model.snapshot.Config.Mode = "direct"
	if got := len(model.visibleGroups()); got != 0 {
		t.Fatalf("direct mode should hide groups, got %d", got)
	}
}

func TestGroupSelectionPersistsPerMode(t *testing.T) {
	t.Parallel()

	model := newTestModel(&fakeService{snapshot: fixtureSnapshot(), caps: compat.Capabilities{Delay: true}})
	model.snapshot.Groups = []compat.ProxyGroup{
		{Name: "Halsh Cloud", Type: "selector", Now: "NodeA", Options: []string{"NodeA", "NodeB"}},
		{Name: "GLOBAL", Type: "selector", Now: "NodeA", Options: []string{"NodeA", "NodeB"}},
	}
	model.snapshot.Config.Mode = "global"
	model.syncCursors()
	if got := model.currentGroup().Name; got != "GLOBAL" {
		t.Fatalf("expected global group selected, got %q", got)
	}

	model.snapshot.Config.Mode = "rule"
	model.syncCursors()
	if got := model.currentGroup().Name; got != "Halsh Cloud" {
		t.Fatalf("expected rule group selected, got %q", got)
	}

	model.preferredGroupByMode["rule"] = "Halsh Cloud"
	model.preferredGroupByMode["global"] = "GLOBAL"
	model.snapshot.Config.Mode = "global"
	model.groupCursor = 0
	model.syncCursors()
	if got := model.currentGroup().Name; got != "GLOBAL" {
		t.Fatalf("expected remembered global group, got %q", got)
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
	model.width = 156
	model.height = 44
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
		Config:  compat.Config{Mode: "rule", TunSupported: true},
		Proxies: map[string]compat.Proxy{
			"NodeA": {Name: "NodeA", Alive: true, History: []compat.DelayHistory{{Delay: 10}}},
			"NodeB": {Name: "NodeB", Alive: true, History: []compat.DelayHistory{{Delay: 20}}},
		},
		Groups: []compat.ProxyGroup{
			{Name: "Halsh Cloud", Type: "selector", Now: "NodeA", Options: []string{"NodeA", "NodeB"}, TestURL: compat.DefaultTestURL},
		},
	}
}

func mouseClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func mouseWheel(x, y int, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: button, Action: tea.MouseActionPress}
}

func manyNodes(n int) []string {
	nodes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		nodes = append(nodes, fmt.Sprintf("Node%02d", i))
	}
	return nodes
}

func makeManyGroups(n int) []compat.ProxyGroup {
	groups := make([]compat.ProxyGroup, 0, n)
	for i := 0; i < n; i++ {
		now := "NodeA"
		if i%2 == 1 {
			now = "NodeB"
		}
		groups = append(groups, compat.ProxyGroup{
			Name:    fmt.Sprintf("Group%02d", i),
			Type:    "selector",
			Now:     now,
			Options: []string{"NodeA", "NodeB"},
			TestURL: compat.DefaultTestURL,
		})
	}
	return groups
}
