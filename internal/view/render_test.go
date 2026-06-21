package view

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderMainPaneTabsDoNotCorruptBorder(t *testing.T) {
	t.Parallel()

	state := State{
		ScreenMode:     ScreenModes[len(ScreenModes)-1],
		TerminalWidth:  156,
		TerminalHeight: 44,
		Connected:      true,
		ConnectionText: "connected",
		DelaySupported: true,
		Instance:       "local",
		Controller:     "http://127.0.0.1:9090",
		Mode:           "rule",
		Version:        "1.0.0",
		Meta:           "mihomo",
		ActivePane:     PaneMain,
		SessionItems:   []Item{{Primary: "local", Secondary: "http://127.0.0.1:9090", Current: true}},
		ModeItems:      []Item{{Primary: "rule", Current: true}},
		GroupItems:     []Item{{Primary: "Auto", Secondary: "-> NodeA  [selector]"}},
		NodeItems:      []Item{{Primary: "NodeA", Secondary: "[up] 10ms", Current: true}},
		MainTabs:       []string{"Inspector", "Delay History", "Events"},
		MainTabIndex:   0,
		Detail:         strings.Repeat("line\n", 40),
		Footer:         "q quit",
		CurrentSession: 0,
		CurrentMode:    0,
		CurrentNode:    0,
	}

	rendered := Render(state)
	if strings.Contains(rendered, "�") {
		t.Fatalf("unexpected replacement glyph in render:\n%s", rendered)
	}
	if strings.Contains(rendered, "Command log") || strings.Contains(rendered, "Status") {
		t.Fatalf("legacy panes still rendered:\n%s", rendered)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if lipgloss.Width(line) != state.ScreenMode.Width {
			t.Fatalf("line width mismatch: got %d want %d\n%s", lipgloss.Width(line), state.ScreenMode.Width, line)
		}
	}
}

func TestComputeLayoutUsesThreeColumnOrder(t *testing.T) {
	t.Parallel()

	layout := ComputeLayout(State{ScreenMode: ScreenModes[1], TerminalWidth: 140, TerminalHeight: 40, MainTabs: []string{"Inspector"}})
	if layout.Sessions.X != 0 || layout.Modes.X != 0 || layout.Groups.X != 0 {
		t.Fatalf("left column misaligned: %#v", layout)
	}
	if layout.Nodes.X <= layout.Sessions.X+layout.Sessions.W {
		t.Fatalf("nodes column should be right of left column: %#v", layout)
	}
	if layout.Main.X <= layout.Nodes.X+layout.Nodes.W {
		t.Fatalf("main column should be right of nodes column: %#v", layout)
	}
	if diff := layout.Nodes.W - layout.Main.W; diff < -1 || diff > 1 {
		t.Fatalf("nodes/main should be near half split: nodes=%d main=%d", layout.Nodes.W, layout.Main.W)
	}
	if layout.Sessions.Y >= layout.Modes.Y || layout.Modes.Y >= layout.Groups.Y {
		t.Fatalf("left column vertical order wrong: %#v", layout)
	}
	if got := MainViewportHeight(layout.Main); got <= 0 {
		t.Fatalf("expected positive main viewport height, got %d", got)
	}
}

func TestRenderUsesTerminalWidthNotScreenPreset(t *testing.T) {
	t.Parallel()

	state := State{
		ScreenMode:     ScreenModes[1],
		TerminalWidth:  140,
		TerminalHeight: 40,
		Connected:      true,
		ConnectionText: "connected",
		DelaySupported: true,
		Instance:       "local",
		Controller:     "http://127.0.0.1:9090",
		Mode:           "rule",
		Version:        "1.0.0",
		Meta:           "mihomo",
		SessionItems:   []Item{{Primary: "local", Current: true}},
		ModeItems:      []Item{{Primary: "rule", Current: true}},
		GroupItems:     []Item{{Primary: "Auto"}},
		NodeItems:      []Item{{Primary: "NodeA", Secondary: "[up] 10ms", SecondaryColor: "2"}},
		MainTabs:       []string{"Inspector"},
		Detail:         "line",
		Footer:         "q quit",
	}

	rendered := Render(state)
	for _, line := range strings.Split(rendered, "\n") {
		if lipgloss.Width(line) != 140 {
			t.Fatalf("line width mismatch: got %d want %d\n%s", lipgloss.Width(line), 140, line)
		}
	}
}

func TestListIndexAtUsesViewportOffset(t *testing.T) {
	t.Parallel()

	rect := Rect{X: 0, Y: 0, W: 20, H: 6}
	index, ok := ListIndexAt(rect, 1, 2, 20, 5)
	if !ok || index != 6 {
		t.Fatalf("expected index 6, got %d ok=%v", index, ok)
	}
}
