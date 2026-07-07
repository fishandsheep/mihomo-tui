package view

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
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

const (
	footerHeight   = 1
	mainChromeRows = 4
)

var (
	colorGreen = lipgloss.Color("2")
	colorGray  = lipgloss.Color("245")
	colorWarn  = lipgloss.Color("11")
	colorErr   = lipgloss.Color("9")

	borderFocused   = lipgloss.NewStyle().Foreground(colorGreen)
	borderInactive  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleFocused    = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	titleInactive   = lipgloss.NewStyle().Foreground(colorGray)
	textStyle       = lipgloss.NewStyle()
	mutedText       = lipgloss.NewStyle().Foreground(colorGray)
	metaText        = lipgloss.NewStyle().Foreground(colorGray)
	currentText     = lipgloss.NewStyle().Foreground(colorGreen)
	tabActive       = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	tabInactive     = lipgloss.NewStyle().Foreground(colorGray)
	mainInfoLabel   = lipgloss.NewStyle().Foreground(colorGray)
	mainToastNormal = lipgloss.NewStyle().Foreground(colorWarn)
	mainToastErr    = lipgloss.NewStyle().Foreground(colorErr)
)

type Item struct {
	Primary        string
	Secondary      string
	Current        bool
	PrimaryColor   string
	SecondaryColor string
}

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
	IPRefreshText  string
	NoProxyMode    bool

	ActivePane   Pane
	SessionItems []Item
	TUNItems     []Item
	ModeItems    []Item
	GroupItems   []Item
	NodeItems    []Item

	SessionCursor  int
	TUNCursor      int
	ModeCursor     int
	GroupCursor    int
	NodeCursor     int
	SessionOffset  int
	TUNOffset      int
	ModeOffset     int
	GroupOffset    int
	NodeOffset     int
	MainOffset     int
	CurrentSession int
	CurrentMode    int
	CurrentNode    int

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
	Sessions Rect
	TUN      Rect
	Modes    Rect
	Groups   Rect
	Nodes    Rect
	Main     Rect
	Footer   Rect
	MainTabs []TabRect
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
	cols := [][]string{
		vjoin(
			renderListPane("1-Session", layout.Sessions, m.ActivePane == PaneSessions, m.SessionItems, m.SessionCursor, m.SessionOffset),
			renderListPane("TUN", layout.TUN, m.ActivePane == PaneTUN, m.TUNItems, m.TUNCursor, m.TUNOffset),
			renderListPane("2-Modes", layout.Modes, m.ActivePane == PaneModes, m.ModeItems, m.ModeCursor, m.ModeOffset),
			renderListPane("3-Groups", layout.Groups, m.ActivePane == PaneGroups, m.GroupItems, m.GroupCursor, m.GroupOffset),
		),
		renderListPane("4-Nodes", layout.Nodes, m.ActivePane == PaneNodes, m.NodeItems, m.NodeCursor, m.NodeOffset),
		renderMainPane("0-Main", layout.Main, m),
	}

	lines := hjoin(cols, " ")
	lines = append(lines, mutedText.Render(padPlain(m.Footer, layout.Footer.W)))
	return strings.Join(lines, "\n")
}

func ComputeLayout(m State) Layout {
	canvasWidth := m.TerminalWidth
	canvasHeight := m.TerminalHeight
	if canvasWidth <= 0 {
		canvasWidth = m.ScreenMode.Width
	}
	if canvasHeight <= 0 {
		canvasHeight = m.ScreenMode.Height
	}
	bodyHeight := canvasHeight - footerHeight

	col1 := 30
	switch {
	case canvasWidth >= 156:
		col1 = 34
	case canvasWidth >= 128:
		col1 = 32
	}
	remaining := canvasWidth - col1 - 2
	col2 := remaining / 2
	col3 := remaining - col2

	sessionsHeight := 6
	tunHeight := 4
	modesHeight := 5
	groupsHeight := bodyHeight - sessionsHeight - tunHeight - modesHeight
	if groupsHeight < 7 {
		groupsHeight = 7
		sessionsHeight = max(5, bodyHeight-groupsHeight-tunHeight-modesHeight)
	}

	layout := Layout{
		Sessions: Rect{X: 0, Y: 0, W: col1, H: sessionsHeight},
		TUN:      Rect{X: 0, Y: sessionsHeight, W: col1, H: tunHeight},
		Modes:    Rect{X: 0, Y: sessionsHeight + tunHeight, W: col1, H: modesHeight},
		Groups:   Rect{X: 0, Y: sessionsHeight + tunHeight + modesHeight, W: col1, H: groupsHeight},
		Nodes:    Rect{X: col1 + 1, Y: 0, W: col2, H: bodyHeight},
		Main:     Rect{X: col1 + col2 + 2, Y: 0, W: col3, H: bodyHeight},
		Footer:   Rect{X: 0, Y: bodyHeight, W: canvasWidth, H: footerHeight},
	}
	layout.MainTabs = computeMainTabRects(layout.Main, m.MainTabs)
	return layout
}

func ContentHeight(rect Rect) int {
	return max(0, rect.H-2)
}

func MainViewportHeight(rect Rect) int {
	return max(0, ContentHeight(rect)-mainChromeRows)
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

func renderListPane(title string, rect Rect, focused bool, items []Item, cursor, offset int) []string {
	visible := ContentHeight(rect)
	body := make([]string, 0, visible)
	for row := 0; row < visible; row++ {
		index := offset + row
		if index >= len(items) {
			body = append(body, mutedText.Render(strings.Repeat(" ", max(0, rect.W-2))))
			continue
		}
		body = append(body, renderItem(items[index], max(0, rect.W-2), index == cursor))
	}
	return renderPane(title, rect, focused, body)
}

func renderMainPane(title string, rect Rect, state State) []string {
	innerWidth := max(0, rect.W-2)
	ipText := fmt.Sprintf("ip %s", empty(state.IPRefreshText))
	if state.NoProxyMode {
		ipText = "ip no-proxy"
	}
	chrome := []string{
		renderMainInfoLine(innerWidth,
			fmt.Sprintf("session %s", empty(state.Instance)),
			fmt.Sprintf("status %s", empty(state.ConnectionText)),
			fmt.Sprintf("delay %s", yesNo(state.DelaySupported)),
			ipText,
		),
		renderMainInfoLine(innerWidth,
			fmt.Sprintf("ctl %s", empty(state.Controller)),
			fmt.Sprintf("mode %s", empty(state.Mode)),
			fmt.Sprintf("ver %s/%s", empty(state.Version), empty(state.Meta)),
		),
		renderToastLine(state.Toast, innerWidth),
		renderTabsLine(state.MainTabs, state.MainTabIndex, innerWidth),
	}

	detailLines := splitDetail(state.Detail)
	visible := MainViewportHeight(rect)
	body := append([]string{}, chrome...)
	for row := 0; row < visible; row++ {
		index := state.MainOffset + row
		if index >= len(detailLines) {
			body = append(body, mutedText.Render(strings.Repeat(" ", innerWidth)))
			continue
		}
		line := padPlain(detailLines[index], innerWidth)
		if strings.Contains(detailLines[index], "IP Info (no proxy mode)") || strings.Contains(detailLines[index], "mode: no proxy") {
			body = append(body, mainToastNormal.Render(line))
			continue
		}
		body = append(body, textStyle.Render(line))
	}
	return renderPane(title, rect, state.ActivePane == PaneMain, body)
}

func renderPane(title string, rect Rect, focused bool, body []string) []string {
	width := rect.W
	if width < 4 {
		width = 4
	}
	height := rect.H
	if height < 3 {
		height = 3
	}
	innerWidth := width - 2
	contentHeight := height - 2
	for len(body) < contentHeight {
		body = append(body, strings.Repeat(" ", innerWidth))
	}
	if len(body) > contentHeight {
		body = body[:contentHeight]
	}

	border := borderInactive
	titleStyle := titleInactive
	if focused {
		border = borderFocused
		titleStyle = titleFocused
	}

	titleText := " " + title + " "
	renderedTitle := trimPlain(titleText, innerWidth)
	line := border.Render("╭") + titleStyle.Render(renderedTitle)
	if remain := innerWidth - textWidth(renderedTitle); remain > 0 {
		line += border.Render(strings.Repeat("─", remain))
	}
	line += border.Render("╮")

	out := []string{line}
	for _, item := range body {
		out = append(out, border.Render("│")+padStyled(item, innerWidth)+border.Render("│"))
	}
	out = append(out, border.Render("╰"+strings.Repeat("─", innerWidth)+"╯"))
	return out
}

func renderItem(item Item, width int, selected bool) string {
	prefixPlain := "  "
	prefixStyled := "  "
	primaryStyle := textStyle
	secondaryStyle := metaText
	if item.PrimaryColor != "" {
		primaryStyle = primaryStyle.Foreground(lipgloss.Color(item.PrimaryColor))
	}
	if item.SecondaryColor != "" {
		secondaryStyle = secondaryStyle.Foreground(lipgloss.Color(item.SecondaryColor))
	}
	if item.Current {
		primaryStyle = currentText
	}
	if selected {
		prefixPlain = "* "
		prefixStyled = "* "
	}

	remaining := max(0, width-textWidth(prefixPlain))
	var line string
	switch {
	case item.Secondary == "":
		line = prefixStyled + primaryStyle.Render(trimPlain(item.Primary, remaining))
	case textWidth(item.Primary)+2+textWidth(item.Secondary) <= remaining:
		line = prefixStyled +
			primaryStyle.Render(item.Primary) +
			textStyle.Render("  ") +
			secondaryStyle.Render(item.Secondary)
	default:
		line = prefixStyled + primaryStyle.Render(trimPlain(item.Primary+"  "+item.Secondary, remaining))
	}

	return padStyled(line, width)
}

func renderMainInfoLine(width int, parts ...string) string {
	return mainInfoLabel.Render(padPlain(strings.Join(parts, "  "), width))
}

func renderToastLine(toast string, width int) string {
	if toast == "" {
		return mutedText.Render(strings.Repeat(" ", width))
	}
	style := mainToastNormal
	lower := strings.ToLower(toast)
	if strings.Contains(lower, "fail") || strings.Contains(lower, "error") || strings.Contains(lower, "unreachable") || strings.Contains(lower, "timeout") {
		style = mainToastErr
	}
	return padStyled(style.Render(trimPlain(toast, width)), width)
}

func renderTabsLine(tabs []string, active, width int) string {
	parts := make([]string, 0, len(tabs))
	for i, tab := range tabs {
		label := " " + tab + " "
		if i == active {
			parts = append(parts, tabActive.Render(label))
			continue
		}
		parts = append(parts, tabInactive.Render(label))
	}
	return padStyled(strings.Join(parts, ""), width)
}

func computeMainTabRects(main Rect, tabs []string) []TabRect {
	if len(tabs) == 0 {
		return nil
	}
	x := main.X + 1
	y := main.Y + 1 + (mainChromeRows - 1)
	out := make([]TabRect, 0, len(tabs))
	for i, tab := range tabs {
		label := " " + tab + " "
		out = append(out, TabRect{
			Index: i,
			Rect:  Rect{X: x, Y: y, W: textWidth(label), H: 1},
		})
		x += textWidth(label)
	}
	return out
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

func splitDetail(detail string) []string {
	trimmed := strings.TrimSpace(detail)
	if trimmed == "" {
		return []string{""}
	}
	return strings.Split(trimmed, "\n")
}

func trimPlain(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if textWidth(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	return runewidth.Truncate(text, width-1, "") + "…"
}

func padPlain(text string, width int) string {
	text = trimPlain(text, width)
	if pad := width - textWidth(text); pad > 0 {
		return text + strings.Repeat(" ", pad)
	}
	return text
}

func padStyled(text string, width int) string {
	if pad := width - lipgloss.Width(text); pad > 0 {
		return text + strings.Repeat(" ", pad)
	}
	return text
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func PaneAt(layout Layout, x, y int) Pane {
	switch {
	case layout.Sessions.Contains(x, y):
		return PaneSessions
	case layout.TUN.Contains(x, y):
		return PaneTUN
	case layout.Modes.Contains(x, y):
		return PaneModes
	case layout.Groups.Contains(x, y):
		return PaneGroups
	case layout.Nodes.Contains(x, y):
		return PaneNodes
	case layout.Main.Contains(x, y):
		return PaneMain
	default:
		return PaneMain
	}
}

func ListIndexAt(rect Rect, x, y, itemCount, offset int) (int, bool) {
	if !rect.Contains(x, y) || itemCount == 0 {
		return 0, false
	}
	if y <= rect.Y || y >= rect.Y+rect.H-1 {
		return 0, false
	}
	index := offset + y - rect.Y - 1
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
