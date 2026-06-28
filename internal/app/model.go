package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/metacubex/mihomo-tui/internal/api"
	"github.com/metacubex/mihomo-tui/internal/compat"
	"github.com/metacubex/mihomo-tui/internal/profile"
	"github.com/metacubex/mihomo-tui/internal/view"
)

type Pane int

const (
	PaneSessions Pane = iota
	PaneTUN
	PaneModes
	PaneGroups
	PaneNodes
	PaneMain
)

type MainTab int

const (
	MainTabInspector MainTab = iota
	MainTabDelay
	MainTabEvents
)

var (
	refreshInterval = 5 * time.Second
	modeOptions     = []string{"rule", "global", "direct"}
	mainTabs        = []string{"Inspector", "Delay History", "Events"}
)

const doubleClickThreshold = 350 * time.Millisecond

type Options struct {
	Store          *profile.Store
	InitialProfile string
	DirectProfile  profile.Profile
	Service        Service
}

type Service interface {
	LoadSnapshot(context.Context, profile.Profile) (compat.Snapshot, compat.Capabilities, error)
	SetMode(context.Context, profile.Profile, string) (compat.Config, error)
	SetTUN(context.Context, profile.Profile, bool) (compat.Config, error)
	SwitchProxy(context.Context, profile.Profile, string, string) (compat.Proxy, error)
	RunDelay(context.Context, profile.Profile, string) (api.DelayResult, error)
}

type controllerService struct{}

type sessionEntry struct {
	Label   string
	Profile profile.Profile
}

type clickState struct {
	pane  Pane
	index int
	at    time.Time
}

type Model struct {
	store *profile.Store
	svc   Service

	sessions      []sessionEntry
	activeProfile profile.Profile

	width  int
	height int

	activePane       Pane
	previousSidePane Pane
	activeMainTab    MainTab

	sessionCursor int
	tunCursor     int
	modeCursor    int
	groupCursor   int
	nodeCursor    int
	sessionOffset int
	tunOffset     int
	modeOffset    int
	groupOffset   int
	nodeOffset    int
	mainOffset    int

	screenModeIndex int
	manualScreen    bool

	snapshot             compat.Snapshot
	capabilities         compat.Capabilities
	connected            bool
	toast                string
	connectionErr        string
	events               []string
	preferredGroupByMode map[string]string
	lastClick            clickState
	now                  func() time.Time
}

type snapshotLoadedMsg struct {
	snapshot compat.Snapshot
	caps     compat.Capabilities
	err      error
}

type modeChangedMsg struct {
	config compat.Config
	err    error
}

type tunChangedMsg struct {
	config compat.Config
	err    error
}

type proxyChangedMsg struct {
	proxy compat.Proxy
	err   error
}

type delayResultMsg struct {
	name   string
	result api.DelayResult
	err    error
}

type tickMsg time.Time

func NewModel(opts Options) Model {
	svc := opts.Service
	if svc == nil {
		svc = controllerService{}
	}

	sessions := buildSessions(opts.Store, opts.DirectProfile)
	active := resolveActiveProfile(opts, sessions)

	model := Model{
		store:                opts.Store,
		svc:                  svc,
		sessions:             sessions,
		activeProfile:        active,
		activePane:           PaneGroups,
		previousSidePane:     PaneNodes,
		activeMainTab:        MainTabInspector,
		screenModeIndex:      len(view.ScreenModes) - 1,
		preferredGroupByMode: make(map[string]string, 2),
		now:                  time.Now,
	}
	model.sessionCursor = model.indexSession(active.Name, active.Target())
	model.modeCursor = model.indexMode(compat.NormalizeConfig(map[string]any{"mode": "rule"}).Mode)
	return model
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadSnapshotCmd(), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if !m.manualScreen {
			m.screenModeIndex = view.BestScreenModeIndex(msg.Width, msg.Height)
		}
		m.ensureOffsets()
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case snapshotLoadedMsg:
		if msg.err != nil {
			m.connected = false
			m.connectionErr = statusMessage(msg.err)
			m.toast = m.connectionErr
			m.pushEvent("connection error: " + m.connectionErr)
			return m, nil
		}
		if !msg.snapshot.Config.TunSupported {
			msg.snapshot.Config.TunEnabled = m.snapshot.Config.TunEnabled
			msg.snapshot.Config.TunSupported = m.snapshot.Config.TunSupported
		}
		m.snapshot = msg.snapshot
		m.capabilities = msg.caps
		m.connected = true
		m.connectionErr = ""
		m.modeCursor = m.indexMode(msg.snapshot.Config.Mode)
		m.syncCursors()
		m.ensureOffsets()
		return m, nil
	case modeChangedMsg:
		if msg.err != nil {
			m.toast = statusMessage(msg.err)
			m.pushEvent("mode change failed: " + m.toast)
			return m, nil
		}
		m.snapshot.Config = msg.config
		m.modeCursor = m.indexMode(msg.config.Mode)
		m.restorePreferredGroup()
		m.toast = "mode -> " + msg.config.Mode
		m.pushEvent(m.toast)
		m.ensureOffsets()
		return m, m.loadSnapshotCmd()
	case tunChangedMsg:
		if msg.err != nil {
			m.toast = statusMessage(msg.err)
			m.pushEvent("tun change failed: " + m.toast)
			return m, nil
		}
		m.snapshot.Config = msg.config
		state := "off"
		if msg.config.TunEnabled {
			state = "on"
		}
		m.toast = "tun -> " + state
		m.pushEvent(m.toast)
		m.ensureOffsets()
		return m, nil
	case proxyChangedMsg:
		if msg.err != nil {
			m.toast = statusMessage(msg.err)
			m.pushEvent("node switch failed: " + m.toast)
			return m, nil
		}
		name := msg.proxy.Name
		if name == "" {
			name = m.currentGroup().Name
		}
		m.toast = "group updated: " + name
		m.pushEvent(m.toast)
		m.ensureOffsets()
		return m, m.loadSnapshotCmd()
	case delayResultMsg:
		if msg.err != nil {
			text := delayText(msg.err)
			m.toast = fmt.Sprintf("%s delay: %s", msg.name, text)
			m.pushEvent(m.toast)
			return m, nil
		}
		m.toast = fmt.Sprintf("%s delay: %dms", msg.name, msg.result.Delay)
		m.pushEvent(m.toast)
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.loadSnapshotCmd(), tickCmd())
	}
	return m, nil
}

func (m Model) View() string {
	return view.Render(m.renderState())
}

func (m Model) renderState() view.State {
	screenMode, fits := m.currentScreenMode()
	group := m.currentGroup()
	node := m.selectedNode()
	groups := m.visibleGroups()

	return view.State{
		TerminalWidth:  m.width,
		TerminalHeight: m.height,
		ScreenMode:     screenMode,
		TooSmall:       !fits,
		MinWidth:       view.ScreenModes[0].Width,
		MinHeight:      view.ScreenModes[0].Height,
		Instance:       m.activeProfile.Name,
		Controller:     m.activeProfile.Target(),
		Connected:      m.connected,
		Mode:           m.snapshot.Config.Mode,
		Version:        m.snapshot.Version.Core,
		Meta:           m.snapshot.Version.Meta,
		ConnectionText: m.connectionStatusText(),
		DelaySupported: m.capabilities.Delay,
		ActivePane:     view.Pane(m.activePane),
		SessionItems:   m.sessionItems(),
		TUNItems:       m.tunItems(),
		ModeItems:      m.modeItems(),
		GroupItems:     m.groupItems(),
		NodeItems:      m.nodeItems(),
		SessionCursor:  clampCursor(m.sessionCursor, len(m.sessions)),
		TUNCursor:      clampCursor(m.tunCursor, len(m.tunItems())),
		ModeCursor:     clampCursor(m.modeCursor, len(modeOptions)),
		GroupCursor:    clampCursor(m.groupCursor, len(groups)),
		NodeCursor:     clampCursor(m.nodeCursor, len(group.Options)),
		SessionOffset:  m.sessionOffset,
		TUNOffset:      m.tunOffset,
		ModeOffset:     m.modeOffset,
		GroupOffset:    m.groupOffset,
		NodeOffset:     m.nodeOffset,
		MainOffset:     m.mainOffset,
		CurrentSession: m.indexSession(m.activeProfile.Name, m.activeProfile.Target()),
		CurrentMode:    m.indexMode(m.snapshot.Config.Mode),
		CurrentNode:    selectedNodeIndex(group),
		MainTabIndex:   int(m.activeMainTab),
		MainTabs:       append([]string(nil), mainTabs...),
		Detail:         m.mainDetail(group, node),
		Footer:         m.footerText(),
		Toast:          m.toast,
	}
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		m.focusNext()
		m.ensureOffsets()
		return m, nil
	case "shift+tab":
		m.focusPrev()
		m.ensureOffsets()
		return m, nil
	case "p":
		m.activePane = PaneSessions
		m.ensureOffsets()
		return m, nil
	case "m":
		m.activePane = PaneModes
		m.ensureOffsets()
		return m, nil
	case "r":
		m.toast = "refreshing"
		return m, m.loadSnapshotCmd()
	case "+":
		m.manualScreen = true
		m.screenModeIndex = clampCursor(m.screenModeIndex+1, len(view.ScreenModes))
		return m, nil
	case "_":
		m.manualScreen = true
		m.screenModeIndex = clampCursor(m.screenModeIndex-1, len(view.ScreenModes))
		return m, nil
	case "esc":
		if m.activePane == PaneMain {
			m.activePane = m.previousSidePane
		}
		m.ensureOffsets()
		return m, nil
	case "1":
		m.activePane = PaneSessions
		m.ensureOffsets()
		return m, nil
	case "2":
		m.activePane = PaneModes
		m.ensureOffsets()
		return m, nil
	case "5":
		m.activePane = PaneTUN
		m.ensureOffsets()
		return m, nil
	case "3":
		m.activePane = PaneGroups
		m.ensureOffsets()
		return m, nil
	case "4":
		m.activePane = PaneNodes
		m.ensureOffsets()
		return m, nil
	case "0":
		m.activePane = PaneMain
		m.ensureOffsets()
		return m, nil
	}

	switch msg.String() {
	case "j", "down":
		m.moveCursor(1)
		return m, nil
	case "k", "up":
		m.moveCursor(-1)
		return m, nil
	case "pgdown":
		m.pageMove(1)
		return m, nil
	case "pgup":
		m.pageMove(-1)
		return m, nil
	case "home":
		m.jumpToBoundary(false)
		return m, nil
	case "end":
		m.jumpToBoundary(true)
		return m, nil
	case "h", "left", "[":
		if m.activePane == PaneMain {
			m.activeMainTab = clampMainTab(m.activeMainTab - 1)
			m.mainOffset = 0
			m.ensureOffsets()
			return m, nil
		}
	case "l", "right", "]":
		if m.activePane == PaneMain {
			m.activeMainTab = clampMainTab(m.activeMainTab + 1)
			m.mainOffset = 0
			m.ensureOffsets()
			return m, nil
		}
	case " ":
		return m.handleSpace()
	case "d":
		if !m.capabilities.Delay {
			m.toast = "delay endpoint unavailable"
			return m, nil
		}
		name := m.delayTarget()
		if name == "" {
			return m, nil
		}
		return m, m.delayCmd(name)
	}

	return m, nil
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	state := m.renderState()
	if state.TooSmall {
		return m, nil
	}
	layout := view.ComputeLayout(state)

	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollHoveredPane(layout, msg.X, msg.Y, -1)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.scrollHoveredPane(layout, msg.X, msg.Y, 1)
			return m, nil
		}
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if index, ok := view.MainTabAt(layout, msg.X, msg.Y); ok {
		m.activePane = PaneMain
		m.activeMainTab = clampMainTab(MainTab(index))
		m.mainOffset = 0
		m.ensureOffsets()
		return m, nil
	}

	switch view.PaneAt(layout, msg.X, msg.Y) {
	case view.PaneSessions:
		m.activePane = PaneSessions
		if index, ok := view.ListIndexAt(layout.Sessions, msg.X, msg.Y, len(m.sessions), m.sessionOffset); ok {
			m.sessionCursor = index
			m.ensureOffsets()
			if m.registerDoubleClick(PaneSessions, index) {
				if session, ok := m.currentSession(); ok {
					m.activeProfile = session.Profile
					m.toast = "session -> " + session.Label
					m.pushEvent(m.toast)
					return m, m.loadSnapshotCmd()
				}
			}
		}
	case view.PaneTUN:
		m.activePane = PaneTUN
		if index, ok := view.ListIndexAt(layout.TUN, msg.X, msg.Y, len(m.tunItems()), m.tunOffset); ok {
			m.tunCursor = index
			m.ensureOffsets()
			if m.registerDoubleClick(PaneTUN, index) {
				return m.handleTUNToggle()
			}
		}
	case view.PaneGroups:
		m.activePane = PaneGroups
		if index, ok := view.ListIndexAt(layout.Groups, msg.X, msg.Y, len(m.visibleGroups()), m.groupOffset); ok {
			m.setGroupCursor(index)
			m.ensureOffsets()
			if m.registerDoubleClick(PaneGroups, index) {
				m.previousSidePane = PaneGroups
				m.activePane = PaneNodes
				m.ensureOffsets()
			}
		}
	case view.PaneNodes:
		m.activePane = PaneNodes
		if index, ok := view.ListIndexAt(layout.Nodes, msg.X, msg.Y, len(m.currentGroup().Options), m.nodeOffset); ok {
			m.nodeCursor = index
			m.ensureOffsets()
			if m.registerDoubleClick(PaneNodes, index) {
				group := m.currentGroup()
				node := m.selectedNode()
				if group.Name != "" && node != "" {
					return m, m.switchProxyCmd(group.Name, node)
				}
			}
		}
	case view.PaneModes:
		m.activePane = PaneModes
		if index, ok := view.ListIndexAt(layout.Modes, msg.X, msg.Y, len(modeOptions), m.modeOffset); ok {
			m.modeCursor = index
			m.ensureOffsets()
			if m.registerDoubleClick(PaneModes, index) {
				return m, m.setModeCmd(modeOptions[m.modeCursor])
			}
		}
	case view.PaneMain:
		m.activePane = PaneMain
		m.ensureOffsets()
	}

	return m, nil
}

func (m Model) handleSpace() (tea.Model, tea.Cmd) {
	switch m.activePane {
	case PaneTUN:
		return m.handleTUNToggle()
	case PaneModes:
		mode := modeOptions[clampCursor(m.modeCursor, len(modeOptions))]
		return m, m.setModeCmd(mode)
	case PaneNodes:
		group := m.currentGroup()
		node := m.selectedNode()
		if group.Name != "" && node != "" {
			return m, m.switchProxyCmd(group.Name, node)
		}
	}
	return m, nil
}

func (m Model) handleTUNToggle() (tea.Model, tea.Cmd) {
	return m, m.setTUNCmd(!m.snapshot.Config.TunEnabled)
}

func (m *Model) focusNext() {
	order := []Pane{PaneSessions, PaneTUN, PaneModes, PaneGroups, PaneNodes, PaneMain}
	m.activePane = order[(indexPane(order, m.activePane)+1)%len(order)]
}

func (m *Model) focusPrev() {
	order := []Pane{PaneSessions, PaneTUN, PaneModes, PaneGroups, PaneNodes, PaneMain}
	index := indexPane(order, m.activePane) - 1
	if index < 0 {
		index = len(order) - 1
	}
	m.activePane = order[index]
}

func (m *Model) moveCursor(step int) {
	switch m.activePane {
	case PaneSessions:
		m.sessionCursor = clampCursor(m.sessionCursor+step, len(m.sessions))
	case PaneTUN:
		m.tunCursor = clampCursor(m.tunCursor+step, len(m.tunItems()))
	case PaneModes:
		m.modeCursor = clampCursor(m.modeCursor+step, len(modeOptions))
	case PaneGroups:
		m.setGroupCursor(clampCursor(m.groupCursor+step, len(m.visibleGroups())))
	case PaneNodes:
		m.nodeCursor = clampCursor(m.nodeCursor+step, len(m.currentGroup().Options))
	case PaneMain:
		m.mainOffset += step
	}
	m.ensureOffsets()
}

func (m *Model) pageMove(direction int) {
	layout, ok := m.currentLayout()
	if !ok {
		return
	}
	switch m.activePane {
	case PaneSessions:
		m.sessionCursor = clampCursor(m.sessionCursor+(direction*max(1, view.ContentHeight(layout.Sessions)-1)), len(m.sessions))
	case PaneTUN:
		m.tunCursor = clampCursor(m.tunCursor+(direction*max(1, view.ContentHeight(layout.TUN)-1)), len(m.tunItems()))
	case PaneModes:
		m.modeCursor = clampCursor(m.modeCursor+(direction*max(1, view.ContentHeight(layout.Modes)-1)), len(modeOptions))
	case PaneGroups:
		m.setGroupCursor(clampCursor(m.groupCursor+(direction*max(1, view.ContentHeight(layout.Groups)-1)), len(m.visibleGroups())))
	case PaneNodes:
		m.nodeCursor = clampCursor(m.nodeCursor+(direction*max(1, view.ContentHeight(layout.Nodes)-1)), len(m.currentGroup().Options))
	case PaneMain:
		m.mainOffset += direction * max(1, view.MainViewportHeight(layout.Main)-1)
	}
	m.ensureOffsets()
}

func (m *Model) jumpToBoundary(last bool) {
	switch m.activePane {
	case PaneSessions:
		m.sessionCursor = boundaryIndex(len(m.sessions), last)
	case PaneTUN:
		m.tunCursor = boundaryIndex(len(m.tunItems()), last)
	case PaneModes:
		m.modeCursor = boundaryIndex(len(modeOptions), last)
	case PaneGroups:
		m.setGroupCursor(boundaryIndex(len(m.visibleGroups()), last))
	case PaneNodes:
		m.nodeCursor = boundaryIndex(len(m.currentGroup().Options), last)
	case PaneMain:
		if !last {
			m.mainOffset = 0
		} else {
			m.mainOffset = 1 << 30
		}
	}
	m.ensureOffsets()
}

func (m *Model) syncCursors() {
	m.sessionCursor = m.indexSession(m.activeProfile.Name, m.activeProfile.Target())
	m.tunCursor = clampCursor(m.tunCursor, len(m.tunItems()))
	m.restorePreferredGroup()
	m.nodeCursor = clampCursor(selectedNodeIndex(m.currentGroup()), len(m.currentGroup().Options))
	m.ensureOffsets()
}

func (m *Model) registerDoubleClick(pane Pane, index int) bool {
	now := time.Now()
	if m.now != nil {
		now = m.now()
	}
	double := m.lastClick.pane == pane && m.lastClick.index == index && now.Sub(m.lastClick.at) <= doubleClickThreshold
	m.lastClick = clickState{pane: pane, index: index, at: now}
	return double
}

func (m *Model) scrollHoveredPane(layout view.Layout, x, y, step int) {
	switch view.PaneAt(layout, x, y) {
	case view.PaneSessions:
		m.activePane = PaneSessions
		m.sessionOffset += step
	case view.PaneTUN:
		m.activePane = PaneTUN
		m.tunOffset += step
	case view.PaneModes:
		m.activePane = PaneModes
		m.modeOffset += step
	case view.PaneGroups:
		m.activePane = PaneGroups
		m.groupOffset += step
	case view.PaneNodes:
		m.activePane = PaneNodes
		m.nodeOffset += step
	case view.PaneMain:
		m.activePane = PaneMain
		m.mainOffset += step
	}
	m.normalizeOffsets()
}

func (m Model) currentLayout() (view.Layout, bool) {
	state := m.renderState()
	if state.TooSmall {
		return view.Layout{}, false
	}
	return view.ComputeLayout(state), true
}

func (m *Model) ensureOffsets() {
	layout, ok := m.currentLayout()
	if !ok {
		m.normalizeOffsets()
		return
	}

	m.sessionCursor = clampCursor(m.sessionCursor, len(m.sessions))
	m.tunCursor = clampCursor(m.tunCursor, len(m.tunItems()))
	m.modeCursor = clampCursor(m.modeCursor, len(modeOptions))
	m.groupCursor = clampCursor(m.groupCursor, len(m.visibleGroups()))
	m.nodeCursor = clampCursor(m.nodeCursor, len(m.currentGroup().Options))

	m.normalizeOffsets()
	m.sessionOffset = clampListOffset(m.sessionOffset, m.sessionCursor, len(m.sessions), view.ContentHeight(layout.Sessions))
	m.tunOffset = clampListOffset(m.tunOffset, m.tunCursor, len(m.tunItems()), view.ContentHeight(layout.TUN))
	m.modeOffset = clampListOffset(m.modeOffset, m.modeCursor, len(modeOptions), view.ContentHeight(layout.Modes))
	m.groupOffset = clampListOffset(m.groupOffset, m.groupCursor, len(m.visibleGroups()), view.ContentHeight(layout.Groups))
	m.nodeOffset = clampListOffset(m.nodeOffset, m.nodeCursor, len(m.currentGroup().Options), view.ContentHeight(layout.Nodes))
}

func (m Model) mainLines() []string {
	return strings.Split(strings.TrimSpace(m.mainDetail(m.currentGroup(), m.selectedNode())), "\n")
}

func (m *Model) normalizeOffsets() {
	layout, ok := m.currentLayout()
	if !ok {
		m.sessionOffset = maxInt(0, m.sessionOffset)
		m.tunOffset = maxInt(0, m.tunOffset)
		m.modeOffset = maxInt(0, m.modeOffset)
		m.groupOffset = maxInt(0, m.groupOffset)
		m.nodeOffset = maxInt(0, m.nodeOffset)
		m.mainOffset = maxInt(0, m.mainOffset)
		return
	}
	m.sessionOffset = clampScrollOffset(m.sessionOffset, len(m.sessions), view.ContentHeight(layout.Sessions))
	m.tunOffset = clampScrollOffset(m.tunOffset, len(m.tunItems()), view.ContentHeight(layout.TUN))
	m.modeOffset = clampScrollOffset(m.modeOffset, len(modeOptions), view.ContentHeight(layout.Modes))
	m.groupOffset = clampScrollOffset(m.groupOffset, len(m.visibleGroups()), view.ContentHeight(layout.Groups))
	m.nodeOffset = clampScrollOffset(m.nodeOffset, len(m.currentGroup().Options), view.ContentHeight(layout.Nodes))
	m.mainOffset = clampScrollOffset(m.mainOffset, len(m.mainLines()), view.MainViewportHeight(layout.Main))
}

func (m Model) currentSession() (sessionEntry, bool) {
	if len(m.sessions) == 0 || m.sessionCursor >= len(m.sessions) {
		return sessionEntry{}, false
	}
	return m.sessions[m.sessionCursor], true
}

func (m Model) visibleGroups() []compat.ProxyGroup {
	switch strings.ToLower(m.snapshot.Config.Mode) {
	case "global":
		return filterGroupsByName(m.snapshot.Groups, "GLOBAL")
	case "direct":
		return nil
	default:
		return filterGroupsByName(m.snapshot.Groups, "Halsh Cloud")
	}
}

func (m Model) currentGroup() compat.ProxyGroup {
	groups := m.visibleGroups()
	if len(groups) == 0 || m.groupCursor >= len(groups) {
		return compat.ProxyGroup{}
	}
	return groups[m.groupCursor]
}

func filterGroupsByName(groups []compat.ProxyGroup, name string) []compat.ProxyGroup {
	out := make([]compat.ProxyGroup, 0, 1)
	for _, group := range groups {
		if strings.EqualFold(group.Name, name) {
			out = append(out, group)
		}
	}
	return out
}

func (m *Model) setGroupCursor(index int) {
	m.groupCursor = index
	m.rememberCurrentGroup()
	m.nodeCursor = selectedNodeIndex(m.currentGroup())
}

func (m *Model) rememberCurrentGroup() {
	group := m.currentGroup()
	mode := strings.ToLower(m.snapshot.Config.Mode)
	if group.Name == "" || mode == "" {
		return
	}
	m.preferredGroupByMode[mode] = group.Name
}

func (m *Model) restorePreferredGroup() {
	groups := m.visibleGroups()
	if len(groups) == 0 {
		m.groupCursor = 0
		return
	}
	mode := strings.ToLower(m.snapshot.Config.Mode)
	if name := m.preferredGroupByMode[mode]; name != "" {
		for i, group := range groups {
			if strings.EqualFold(group.Name, name) {
				m.groupCursor = i
				return
			}
		}
	}
	m.groupCursor = clampCursor(m.groupCursor, len(groups))
	m.rememberCurrentGroup()
}

func (m Model) selectedNode() string {
	group := m.currentGroup()
	if len(group.Options) == 0 || m.nodeCursor >= len(group.Options) {
		return ""
	}
	return group.Options[m.nodeCursor]
}

func (m Model) delayTarget() string {
	if m.activePane == PaneNodes || m.activePane == PaneMain {
		if node := m.selectedNode(); node != "" {
			return node
		}
	}
	return m.currentGroup().Name
}

func (m Model) loadSnapshotCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		snapshot, caps, err := m.svc.LoadSnapshot(ctx, m.activeProfile)
		if err != nil {
			return snapshotLoadedMsg{err: err}
		}
		return snapshotLoadedMsg{snapshot: snapshot, caps: caps}
	}
}

func (m Model) setModeCmd(mode string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		config, err := m.svc.SetMode(ctx, m.activeProfile, mode)
		return modeChangedMsg{config: config, err: err}
	}
}

func (m Model) setTUNCmd(enabled bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		config, err := m.svc.SetTUN(ctx, m.activeProfile, enabled)
		return tunChangedMsg{config: config, err: err}
	}
}

func (m Model) switchProxyCmd(group, node string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		proxy, err := m.svc.SwitchProxy(ctx, m.activeProfile, group, node)
		return proxyChangedMsg{proxy: proxy, err: err}
	}
}

func (m Model) delayCmd(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		result, err := m.svc.RunDelay(ctx, m.activeProfile, name)
		return delayResultMsg{name: name, result: result, err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *Model) pushEvent(event string) {
	if event == "" {
		return
	}
	timestamp := time.Now().Format("15:04:05")
	m.events = append([]string{timestamp + "  " + event}, m.events...)
	if len(m.events) > 40 {
		m.events = m.events[:40]
	}
}

func (m Model) sessionItems() []view.Item {
	items := make([]view.Item, 0, len(m.sessions))
	for _, session := range m.sessions {
		items = append(items, view.Item{
			Primary:   session.Label,
			Secondary: session.Profile.Target(),
			Current:   sameProfileTarget(session.Profile, m.activeProfile),
		})
	}
	return items
}

func (m Model) tunItems() []view.Item {
	label := "off"
	color := "2"
	if m.snapshot.Config.TunEnabled {
		label = "on"
		color = "11"
	}
	return []view.Item{{Primary: "state", Secondary: label, SecondaryColor: color}}
}

func (m Model) modeItems() []view.Item {
	items := make([]view.Item, 0, len(modeOptions))
	for _, mode := range modeOptions {
		items = append(items, view.Item{
			Primary: mode,
			Current: mode == m.snapshot.Config.Mode,
		})
	}
	return items
}

func (m Model) groupItems() []view.Item {
	groups := m.visibleGroups()
	items := make([]view.Item, 0, len(groups))
	for _, group := range groups {
		secondary := ""
		if group.Now != "" {
			secondary = "-> " + group.Now
		}
		if group.Type != "" {
			if secondary != "" {
				secondary += "  "
			}
			secondary += "[" + group.Type + "]"
		}
		items = append(items, view.Item{Primary: group.Name, Secondary: secondary})
	}
	return items
}

func (m Model) nodeItems() []view.Item {
	group := m.currentGroup()
	items := make([]view.Item, 0, len(group.Options))
	for _, name := range group.Options {
		proxy := m.snapshot.Proxies[name]
		delay := historyText(proxy.History)
		alive := "down"
		secondaryColor := "9"
		if proxy.Alive {
			alive = "up"
			secondaryColor = "2"
		}
		items = append(items, view.Item{
			Primary:        name,
			Secondary:      fmt.Sprintf("[%s] %s", alive, delay),
			Current:        name == group.Now,
			SecondaryColor: secondaryColor,
		})
	}
	return items
}

func (m Model) mainDetail(group compat.ProxyGroup, node string) string {
	switch m.activeMainTab {
	case MainTabInspector:
		return inspectorDetail(m.snapshot, m.activeProfile, m.capabilities, group, node, m.connectionErr)
	case MainTabDelay:
		return delayHistoryDetail(m.snapshot, group, node)
	case MainTabEvents:
		if len(m.events) == 0 {
			return "No events yet."
		}
		return strings.Join(m.events, "\n")
	default:
		return ""
	}
}

func (m Model) footerText() string {
	return "tab cycle  j/k move  pgup/pgdn scroll  space apply  h/l tabs  d delay  r refresh  q quit"
}

func (m Model) currentScreenMode() (view.ScreenMode, bool) {
	if m.manualScreen {
		mode := view.ScreenModes[clampCursor(m.screenModeIndex, len(view.ScreenModes))]
		return mode, m.width >= mode.Width && m.height >= mode.Height
	}
	index := view.BestScreenModeIndex(m.width, m.height)
	if index < 0 {
		return view.ScreenModes[0], false
	}
	return view.ScreenModes[index], true
}

func (m Model) connectionStatusText() string {
	if m.connected {
		return "connected"
	}
	if m.connectionErr != "" {
		return m.connectionErr
	}
	return "disconnected"
}

func (m Model) indexMode(mode string) int {
	for i, item := range modeOptions {
		if item == mode {
			return i
		}
	}
	return 0
}

func (m Model) indexSession(name, target string) int {
	for i, item := range m.sessions {
		if item.Profile.Name == name && item.Profile.Target() == target {
			return i
		}
	}
	return 0
}

func sameProfileTarget(a, b profile.Profile) bool {
	return a.Name == b.Name && a.Target() == b.Target()
}

func buildSessions(store *profile.Store, direct profile.Profile) []sessionEntry {
	sessions := make([]sessionEntry, 0, 8)
	if direct.Target() != "" {
		label := "Current Session"
		if direct.Name != "" && direct.Name != "direct" {
			label = direct.Name
		}
		sessions = append(sessions, sessionEntry{Label: label, Profile: direct})
	}
	if store == nil {
		return sessions
	}
	for _, item := range store.List() {
		if direct.Target() != "" && item.Target() == direct.Target() && item.Secret == direct.Secret {
			continue
		}
		sessions = append(sessions, sessionEntry{Label: item.Name, Profile: item})
	}
	return sessions
}

func resolveActiveProfile(opts Options, sessions []sessionEntry) profile.Profile {
	if opts.DirectProfile.Target() != "" {
		return opts.DirectProfile
	}
	if opts.InitialProfile != "" && opts.Store != nil {
		if p, ok := opts.Store.Get(opts.InitialProfile); ok {
			return p
		}
	}
	if opts.Store != nil {
		if p, ok := opts.Store.Default(); ok {
			return p
		}
	}
	if len(sessions) > 0 {
		return sessions[0].Profile
	}
	return profile.Profile{Name: "no-session"}
}

func inspectorDetail(snapshot compat.Snapshot, active profile.Profile, caps compat.Capabilities, group compat.ProxyGroup, node, connectionErr string) string {
	lines := []string{
		"Session",
		"  name: " + valueOrDash(active.Name),
		"  controller: " + valueOrDash(active.Target()),
		"",
		"Controller",
		"  mode: " + valueOrDash(snapshot.Config.Mode),
		"  tun: " + boolWord(snapshot.Config.TunEnabled),
		"  version: " + valueOrDash(snapshot.Version.Core),
		"  meta: " + valueOrDash(snapshot.Version.Meta),
		"  delay endpoint: " + boolWord(caps.Delay),
	}
	if connectionErr != "" {
		lines = append(lines, "  error: "+connectionErr)
	}
	lines = append(lines,
		"",
		"Group",
		"  name: "+valueOrDash(group.Name),
		"  type: "+valueOrDash(group.Type),
		"  current: "+valueOrDash(group.Now),
		"  test-url: "+valueOrDash(group.TestURL),
	)
	if node != "" {
		proxy := snapshot.Proxies[node]
		lines = append(lines,
			"",
			"Node",
			"  name: "+node,
			"  alive: "+boolWord(proxy.Alive),
			"  last-delay: "+historyText(proxy.History),
		)
	}
	return strings.Join(lines, "\n")
}

func delayHistoryDetail(snapshot compat.Snapshot, group compat.ProxyGroup, node string) string {
	target := node
	if target == "" {
		target = group.Now
	}
	proxy := snapshot.Proxies[target]
	lines := []string{
		"Delay History",
		"  group: " + valueOrDash(group.Name),
		"  node: " + valueOrDash(target),
		"",
	}
	if len(proxy.History) == 0 {
		lines = append(lines, "No delay samples yet.")
		return strings.Join(lines, "\n")
	}
	for i := len(proxy.History) - 1; i >= 0; i-- {
		item := proxy.History[i]
		when := item.Time.Format("2006-01-02 15:04:05")
		if item.Time.IsZero() {
			when = "unknown"
		}
		value := "unavailable"
		if item.Delay > 0 {
			value = fmt.Sprintf("%dms", item.Delay)
		}
		lines = append(lines, fmt.Sprintf("%s  %s", when, value))
	}
	return strings.Join(lines, "\n")
}

func historyText(history []compat.DelayHistory) string {
	if len(history) == 0 {
		return "-"
	}
	last := history[len(history)-1]
	if last.Delay == 0 {
		return "unavailable"
	}
	return fmt.Sprintf("%dms", last.Delay)
}

func delayText(err error) string {
	var apiErr *api.Error
	if strings.Contains(strings.ToLower(err.Error()), "timeout") {
		return "timeout"
	}
	if ok := errorAs(err, &apiErr); ok {
		if apiErr.Kind == api.ErrTimeout {
			return "timeout"
		}
		if apiErr.Kind == api.ErrServer || apiErr.Kind == api.ErrBadResponse {
			return "unavailable"
		}
	}
	return err.Error()
}

func statusMessage(err error) string {
	var apiErr *api.Error
	if errorAs(err, &apiErr) {
		switch apiErr.Kind {
		case api.ErrAuth:
			return "auth failed"
		case api.ErrConnect:
			return "controller unreachable"
		case api.ErrTimeout:
			return "request timeout"
		case api.ErrMissingEndpoint:
			return "endpoint missing"
		default:
			if apiErr.Message != "" {
				return apiErr.Message
			}
		}
	}
	return err.Error()
}

func selectedNodeIndex(group compat.ProxyGroup) int {
	for i, name := range group.Options {
		if name == group.Now {
			return i
		}
	}
	return 0
}

func clampCursor(cursor, length int) int {
	if length <= 0 {
		return 0
	}
	if cursor < 0 {
		return 0
	}
	if cursor >= length {
		return length - 1
	}
	return cursor
}

func clampListOffset(offset, cursor, length, visible int) int {
	offset = clampScrollOffset(offset, length, visible)
	if visible <= 0 || length == 0 {
		return 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+visible {
		offset = cursor - visible + 1
	}
	return clampScrollOffset(offset, length, visible)
}

func clampScrollOffset(offset, length, visible int) int {
	if visible <= 0 || length <= visible {
		return 0
	}
	if offset < 0 {
		return 0
	}
	maxOffset := length - visible
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func boundaryIndex(length int, last bool) int {
	if length <= 0 {
		return 0
	}
	if last {
		return length - 1
	}
	return 0
}

func clampMainTab(tab MainTab) MainTab {
	if tab < 0 {
		return 0
	}
	if int(tab) >= len(mainTabs) {
		return MainTab(len(mainTabs) - 1)
	}
	return tab
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func indexPane(items []Pane, target Pane) int {
	for i, item := range items {
		if item == target {
			return i
		}
	}
	return 0
}

func boolWord(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func errorAs[T error](err error, target *T) bool {
	switch typed := any(target).(type) {
	case **api.Error:
		value, ok := err.(*api.Error)
		if !ok {
			return false
		}
		*typed = value
		return true
	default:
		return false
	}
}

func (controllerService) LoadSnapshot(ctx context.Context, p profile.Profile) (compat.Snapshot, compat.Capabilities, error) {
	client := api.New(p.ControllerURL, p.UnixSocket, p.Secret, p.TLSSkipVerify)

	versionRaw, err := client.GetVersion(ctx)
	if err != nil {
		return compat.Snapshot{}, compat.Capabilities{}, err
	}
	configRaw, err := client.GetConfigs(ctx)
	if err != nil {
		return compat.Snapshot{}, compat.Capabilities{}, err
	}
	proxiesRaw, err := client.GetProxies(ctx)
	if err != nil {
		return compat.Snapshot{}, compat.Capabilities{}, err
	}

	proxies, groups, err := compat.NormalizeProxies(proxiesRaw)
	if err != nil {
		return compat.Snapshot{}, compat.Capabilities{}, err
	}

	caps := compat.Capabilities{Version: true, Configs: true, Proxies: true}
	for _, group := range groups {
		probeErr := client.ProbeDelayEndpoint(ctx, group.Name)
		if probeErr == nil {
			caps.Delay = true
		} else if apiErr, ok := probeErr.(*api.Error); ok && apiErr.Kind == api.ErrBadResponse {
			caps.Delay = true
		}
		break
	}

	return compat.Snapshot{
		Version: compat.NormalizeVersion(versionRaw),
		Config:  compat.NormalizeConfig(configRaw),
		Proxies: proxies,
		Groups:  groups,
	}, caps, nil
}

func (controllerService) SetMode(ctx context.Context, p profile.Profile, mode string) (compat.Config, error) {
	client := api.New(p.ControllerURL, p.UnixSocket, p.Secret, p.TLSSkipVerify)
	err := client.PatchMode(ctx, mode)
	if apiErr, ok := err.(*api.Error); ok && apiErr.Kind == api.ErrMissingEndpoint {
		err = client.PutMode(ctx, mode)
	}
	if err != nil {
		return compat.Config{}, err
	}
	configRaw, err := client.GetConfigs(ctx)
	if err != nil {
		return compat.Config{}, err
	}
	return compat.NormalizeConfig(configRaw), nil
}

func (controllerService) SetTUN(ctx context.Context, p profile.Profile, enabled bool) (compat.Config, error) {
	client := api.New(p.ControllerURL, p.UnixSocket, p.Secret, p.TLSSkipVerify)
	if err := client.PatchTUN(ctx, enabled); err != nil {
		return compat.Config{}, err
	}
	configRaw, err := client.GetConfigs(ctx)
	if err != nil {
		return compat.Config{}, err
	}
	config := compat.NormalizeConfig(configRaw)
	config.TunEnabled = enabled
	config.TunSupported = true
	return config, nil
}

func (controllerService) SwitchProxy(ctx context.Context, p profile.Profile, group, node string) (compat.Proxy, error) {
	client := api.New(p.ControllerURL, p.UnixSocket, p.Secret, p.TLSSkipVerify)
	if err := client.UpdateProxy(ctx, group, node); err != nil {
		return compat.Proxy{}, err
	}
	raw, err := client.GetProxy(ctx, group)
	if err != nil {
		return compat.Proxy{}, err
	}
	return compat.NormalizeProxy(raw), nil
}

func (controllerService) RunDelay(ctx context.Context, p profile.Profile, name string) (api.DelayResult, error) {
	client := api.New(p.ControllerURL, p.UnixSocket, p.Secret, p.TLSSkipVerify)
	return client.GetDelay(ctx, name, compat.DefaultTestURL, 5*time.Second)
}
