package view

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

const (
	ansiReset      = "\033[0m"
	ansiBorder     = "\033[38;5;245m"
	ansiMuted      = "\033[38;5;250m"
	ansiStatus     = "\033[38;5;81m"
	ansiGreen      = "\033[38;5;114m"
	ansiRed        = "\033[38;5;203m"
	ansiYellow     = "\033[38;5;221m"
	ansiPaneFocus  = "\033[38;5;229m"
	ansiPaneAccent = "\033[38;5;186m"
	ansiTabActive  = "\033[38;5;81m"
	ansiSelectFg   = "\033[38;5;255m"
	ansiSelectBg   = "\033[48;5;67m"
	ansiLog        = "\033[38;5;222m"
)

type Pane int

const (
	PaneSessions Pane = iota
	PaneModes
	PaneGroups
	PaneNodes
	PaneMain
)

type ScreenMode struct {
	Name   string
	Width  int
	Height int
}

var ScreenModes = []ScreenMode{
	{Name: "Compact", Width: 100, Height: 30},
	{Name: "Standard", Width: 128, Height: 36},
	{Name: "Wide", Width: 156, Height: 44},
}

type State struct {
	TerminalWidth  int
	TerminalHeight int
	ScreenMode     ScreenMode
	TooSmall       bool
	MinWidth       int
	MinHeight      int

	Instance       string
	Controller     string
	Connected      bool
	ConnectionText string
	Mode           string
	Version        string
	Meta           string
	DelaySupported bool

	ActivePane   Pane
	SessionItems []string
	ModeItems    []string
	GroupItems   []string
	NodeItems    []string

	SessionCursor int
	ModeCursor    int
	GroupCursor   int
	NodeCursor    int

	MainTab      string
	MainTabIndex int
	MainTabs     []string
	Detail       string
	Footer       string
	Toast        string
}

type Rect struct {
	X int
	Y int
	W int
	H int
}

func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

type TabRect struct {
	Index int
	Rect  Rect
}

type Layout struct {
	Status     Rect
	Sessions   Rect
	Groups     Rect
	Nodes      Rect
	Modes      Rect
	Main       Rect
	CommandLog Rect
	Footer     Rect
	MainTabs   []TabRect
}

func BestScreenModeIndex(width, height int) int {
	for i := len(ScreenModes) - 1; i >= 0; i-- {
		if width >= ScreenModes[i].Width && height >= ScreenModes[i].Height {
			return i
		}
	}
	return -1
}

func Render(m State) string {
	if m.TooSmall {
		return renderTooSmall(m)
	}

	layout := ComputeLayout(m)

	status := box("Status", []string{
		fmt.Sprintf("%s  %s", statusGlyph(m.Connected), empty(m.Instance)),
		fmt.Sprintf("ctl: %s", empty(m.Controller)),
		fmt.Sprintf("mode:%s  ver:%s  delay:%s", empty(m.Mode), empty(m.Version), yesNo(m.DelaySupported)),
	}, layout.Status.W, layout.Status.H, false, -1)
	sessions := box("[1] Sessions  click/enter", m.SessionItems, layout.Sessions.W, layout.Sessions.H, m.ActivePane == PaneSessions, m.SessionCursor)
	groups := box("[3] Groups  click/enter", m.GroupItems, layout.Groups.W, layout.Groups.H, m.ActivePane == PaneGroups, m.GroupCursor)
	nodes := box("[4] Nodes  click/space", m.NodeItems, layout.Nodes.W, layout.Nodes.H, m.ActivePane == PaneNodes, m.NodeCursor)
	modes := box("[2] Modes  click/space", m.ModeItems, layout.Modes.W, layout.Modes.H, m.ActivePane == PaneModes, m.ModeCursor)
	left := vjoin(status, sessions, groups, nodes, modes)

	mainTitle := "[0] Main  " + tabsLabel(m.MainTabs, m.MainTabIndex)
	main := box(mainTitle, strings.Split(strings.TrimSpace(m.Detail), "\n"), layout.Main.W, layout.Main.H, m.ActivePane == PaneMain, -1)
	logLines := []string{"ready"}
	if m.Toast != "" {
		logLines = []string{m.Toast}
	}
	commandLog := box("Command log", logLines, layout.CommandLog.W, layout.CommandLog.H, false, -1)
	right := vjoin(main, commandLog)

	footer := colorize(ansiMuted, pad(m.Footer, layout.Footer.W))
	lines := hjoin([][]string{left, right}, " ")
	lines = append(lines, footer)
	return strings.Join(lines, "\n")
}

func ComputeLayout(m State) Layout {
	canvasWidth := m.ScreenMode.Width
	canvasHeight := m.ScreenMode.Height

	footerHeight := 1
	bodyHeight := canvasHeight - footerHeight

	leftWidth := 42
	if canvasWidth >= 156 {
		leftWidth = 46
	}
	rightWidth := canvasWidth - leftWidth - 1

	statusHeight := 4
	sessionsHeight := 6
	groupsHeight := 8
	modesHeight := 5
	commandLogHeight := 3
	mainHeight := bodyHeight - commandLogHeight
	nodesHeight := bodyHeight - statusHeight - sessionsHeight - groupsHeight - modesHeight
	if nodesHeight < 6 {
		nodesHeight = 6
		groupsHeight = max(6, bodyHeight-statusHeight-sessionsHeight-modesHeight-nodesHeight)
	}

	layout := Layout{
		Status:     Rect{X: 0, Y: 0, W: leftWidth, H: statusHeight},
		Sessions:   Rect{X: 0, Y: statusHeight, W: leftWidth, H: sessionsHeight},
		Groups:     Rect{X: 0, Y: statusHeight + sessionsHeight, W: leftWidth, H: groupsHeight},
		Nodes:      Rect{X: 0, Y: statusHeight + sessionsHeight + groupsHeight, W: leftWidth, H: nodesHeight},
		Modes:      Rect{X: 0, Y: statusHeight + sessionsHeight + groupsHeight + nodesHeight, W: leftWidth, H: modesHeight},
		Main:       Rect{X: leftWidth + 1, Y: 0, W: rightWidth, H: mainHeight},
		CommandLog: Rect{X: leftWidth + 1, Y: mainHeight, W: rightWidth, H: commandLogHeight},
		Footer:     Rect{X: 0, Y: bodyHeight, W: canvasWidth, H: footerHeight},
	}
	layout.MainTabs = computeMainTabRects(layout.Main, m.MainTabs)
	return layout
}

func computeMainTabRects(main Rect, tabs []string) []TabRect {
	if len(tabs) == 0 {
		return nil
	}
	prefix := "[0] Main  "
	x := main.X + 1 + textWidth(prefix)
	out := make([]TabRect, 0, len(tabs))
	for i, tab := range tabs {
		label := tab
		w := textWidth(label)
		out = append(out, TabRect{
			Index: i,
			Rect:  Rect{X: x, Y: main.Y, W: w, H: 1},
		})
		x += w
		if i < len(tabs)-1 {
			x += textWidth(" | ")
		}
	}
	return out
}

func renderTooSmall(m State) string {
	lines := []string{
		"mihomo-tui",
		"",
		fmt.Sprintf("terminal too small: need at least %dx%d", m.MinWidth, m.MinHeight),
		fmt.Sprintf("current terminal: %dx%d", m.TerminalWidth, m.TerminalHeight),
		"",
		"Resize terminal and reopen tui.",
	}
	return strings.Join(lines, "\n")
}

func box(title string, items []string, width, height int, focused bool, cursor int) []string {
	if height < 3 {
		height = 3
	}
	innerWidth := width - 2
	contentHeight := height - 2

	top := colorize(borderColor(focused), "┌"+fillTitle(title, innerWidth, focused)+"┐")
	lines := []string{top}
	for i := 0; i < contentHeight; i++ {
		text := pad("", innerWidth)
		if i < len(items) {
			text = pad(items[i], innerWidth)
			if cursor >= 0 && i == cursor {
				text = selectedText(pad("> "+trim(items[i], innerWidth-2), innerWidth))
			} else {
				text = contentText(text)
			}
		} else {
			text = contentText(text)
		}
		lines = append(lines, colorize(borderColor(focused), "│")+text+colorize(borderColor(focused), "│"))
	}
	lines = append(lines, colorize(borderColor(focused), "└"+strings.Repeat("─", innerWidth)+"┘"))
	return lines
}

func fillTitle(title string, width int, focused bool) string {
	title = " " + title + " "
	if width <= textWidth(title) {
		return titleText(trim(title, width), focused)
	}
	return titleText(title, focused) + colorize(borderColor(focused), strings.Repeat("─", width-textWidth(title)))
}

func hjoin(cols [][]string, gap string) []string {
	height := 0
	for _, col := range cols {
		if len(col) > height {
			height = len(col)
		}
	}
	out := make([]string, 0, height)
	for row := 0; row < height; row++ {
		parts := make([]string, 0, len(cols))
		for _, col := range cols {
			if row < len(col) {
				parts = append(parts, col[row])
			}
		}
		out = append(out, strings.Join(parts, gap))
	}
	return out
}

func vjoin(blocks ...[]string) []string {
	height := 0
	for _, block := range blocks {
		height += len(block)
	}
	out := make([]string, 0, height)
	for _, block := range blocks {
		out = append(out, block...)
	}
	return out
}

func tabsLabel(tabs []string, active int) string {
	parts := make([]string, 0, len(tabs))
	for i, tab := range tabs {
		if i == active {
			parts = append(parts, colorize(ansiTabActive, tab))
			continue
		}
		parts = append(parts, colorize(ansiMuted, tab))
	}
	return strings.Join(parts, colorize(ansiBorder, " | "))
}

func pad(text string, width int) string {
	text = trim(text, width)
	if width <= textWidth(text) {
		return text
	}
	return text + strings.Repeat(" ", width-textWidth(text))
}

func trim(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if textWidth(text) <= width {
		return text
	}
	if width <= 1 {
		return "…"
	}
	return runewidth.Truncate(text, width-1, "") + "…"
}

func textWidth(text string) int {
	return runewidth.StringWidth(text)
}

func empty(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func statusWord(value bool) string {
	if value {
		return "up"
	}
	return "down"
}

func statusGlyph(value bool) string {
	if value {
		return colorize(ansiGreen, "✓")
	}
	return colorize(ansiRed, "×")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func colorize(style, text string) string {
	return style + text + ansiReset
}

func borderColor(focused bool) string {
	if focused {
		return ansiPaneFocus
	}
	return ansiBorder
}

func titleText(text string, focused bool) string {
	if focused {
		return colorize(ansiPaneAccent, text)
	}
	return colorize(ansiStatus, text)
}

func selectedText(text string) string {
	return ansiSelectBg + ansiSelectFg + text + ansiReset
}

func contentText(text string) string {
	return colorize(ansiMuted, text)
}

func PaneAt(layout Layout, x, y int) Pane {
	switch {
	case layout.Sessions.Contains(x, y):
		return PaneSessions
	case layout.Groups.Contains(x, y):
		return PaneGroups
	case layout.Nodes.Contains(x, y):
		return PaneNodes
	case layout.Modes.Contains(x, y):
		return PaneModes
	case layout.Main.Contains(x, y):
		return PaneMain
	default:
		return PaneMain
	}
}

func ListIndexAt(rect Rect, x, y, itemCount int) (int, bool) {
	if !rect.Contains(x, y) || itemCount == 0 {
		return 0, false
	}
	if y <= rect.Y || y >= rect.Y+rect.H-1 {
		return 0, false
	}
	index := y - rect.Y - 1
	if index < 0 || index >= itemCount {
		return 0, false
	}
	return index, true
}

func MainTabAt(layout Layout, x, y int) (int, bool) {
	for _, tab := range layout.MainTabs {
		if tab.Rect.Contains(x, y) {
			return tab.Index, true
		}
	}
	return 0, false
}
