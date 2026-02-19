package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// DefaultKeyMap
// ---------------------------------------------------------------------------

func TestDefaultKeyMap_AllBindingsNonEmpty(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()

	bindings := []struct {
		name    string
		binding key.Binding
	}{
		{"Quit", km.Quit},
		{"Help", km.Help},
		{"Pause", km.Pause},
		{"Skip", km.Skip},
		{"ToggleLog", km.ToggleLog},
		{"FocusNext", km.FocusNext},
		{"FocusPrev", km.FocusPrev},
		{"Up", km.Up},
		{"Down", km.Down},
		{"PageUp", km.PageUp},
		{"PageDown", km.PageDown},
		{"Home", km.Home},
		{"End", km.End},
		{"NextAgent", km.NextAgent},
		{"PrevAgent", km.PrevAgent},
	}

	for _, tt := range bindings {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// A binding is non-empty when it has at least one key and a help key.
			assert.NotEmpty(t, tt.binding.Help().Key,
				"%s binding must have a non-empty help key", tt.name)
			assert.NotEmpty(t, tt.binding.Help().Desc,
				"%s binding must have a non-empty help description", tt.name)
		})
	}
}

func TestDefaultKeyMap_QuitMatchesExpectedKeys(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()

	tests := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{
			name: "q rune",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
		},
		{
			name: "ctrl+c",
			msg:  tea.KeyMsg{Type: tea.KeyCtrlC},
		},
		{
			name: "ctrl+q",
			msg:  tea.KeyMsg{Type: tea.KeyCtrlQ},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.True(t, key.Matches(tt.msg, km.Quit),
				"Quit binding must match %s", tt.name)
		})
	}
}

func TestDefaultKeyMap_HelpMatchesQuestionMark(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
	assert.True(t, key.Matches(msg, km.Help), "Help binding must match '?'")
}

func TestDefaultKeyMap_TabMatchesFocusNext(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()

	msg := tea.KeyMsg{Type: tea.KeyTab}
	assert.True(t, key.Matches(msg, km.FocusNext), "FocusNext must match Tab")
}

func TestDefaultKeyMap_ShiftTabMatchesFocusPrev(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()

	msg := tea.KeyMsg{Type: tea.KeyShiftTab}
	assert.True(t, key.Matches(msg, km.FocusPrev), "FocusPrev must match Shift+Tab")
}

func TestDefaultKeyMap_PauseMatchesP(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	assert.True(t, key.Matches(msg, km.Pause), "Pause binding must match 'p'")
}

func TestDefaultKeyMap_SkipMatchesS(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	assert.True(t, key.Matches(msg, km.Skip), "Skip binding must match 's'")
}

func TestDefaultKeyMap_ToggleLogMatchesL(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}
	assert.True(t, key.Matches(msg, km.ToggleLog), "ToggleLog binding must match 'l'")
}

func TestDefaultKeyMap_ScrollingKeys(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()

	tests := []struct {
		name    string
		msg     tea.KeyMsg
		binding key.Binding
	}{
		{
			name:    "up arrow",
			msg:     tea.KeyMsg{Type: tea.KeyUp},
			binding: km.Up,
		},
		{
			name:    "k rune (up)",
			msg:     tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}},
			binding: km.Up,
		},
		{
			name:    "down arrow",
			msg:     tea.KeyMsg{Type: tea.KeyDown},
			binding: km.Down,
		},
		{
			name:    "j rune (down)",
			msg:     tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
			binding: km.Down,
		},
		{
			name:    "pgup",
			msg:     tea.KeyMsg{Type: tea.KeyPgUp},
			binding: km.PageUp,
		},
		{
			name:    "pgdown",
			msg:     tea.KeyMsg{Type: tea.KeyPgDown},
			binding: km.PageDown,
		},
		{
			name:    "home",
			msg:     tea.KeyMsg{Type: tea.KeyHome},
			binding: km.Home,
		},
		{
			name:    "end",
			msg:     tea.KeyMsg{Type: tea.KeyEnd},
			binding: km.End,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.True(t, key.Matches(tt.msg, tt.binding),
				"%s must match its binding", tt.name)
		})
	}
}

// ---------------------------------------------------------------------------
// NextFocus / PrevFocus
// ---------------------------------------------------------------------------

func TestNextFocus_Cycle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current FocusPanel
		want    FocusPanel
	}{
		{FocusSidebar, FocusAgentPanel},
		{FocusAgentPanel, FocusEventLog},
		{FocusEventLog, FocusSidebar},
	}
	for _, tt := range tests {
		t.Run(tt.current.String(), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, NextFocus(tt.current))
		})
	}
}

func TestPrevFocus_Cycle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current FocusPanel
		want    FocusPanel
	}{
		{FocusSidebar, FocusEventLog},
		{FocusEventLog, FocusAgentPanel},
		{FocusAgentPanel, FocusSidebar},
	}
	for _, tt := range tests {
		t.Run(tt.current.String(), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, PrevFocus(tt.current))
		})
	}
}

func TestNextFocus_FullCycle_ReturnsToCurrent(t *testing.T) {
	t.Parallel()
	start := FocusSidebar
	cur := start
	for i := 0; i < focusPanelCount; i++ {
		cur = NextFocus(cur)
	}
	assert.Equal(t, start, cur, "cycling NextFocus N times must return to the starting panel")
}

func TestPrevFocus_FullCycle_ReturnsToCurrent(t *testing.T) {
	t.Parallel()
	start := FocusAgentPanel
	cur := start
	for i := 0; i < focusPanelCount; i++ {
		cur = PrevFocus(cur)
	}
	assert.Equal(t, start, cur, "cycling PrevFocus N times must return to the starting panel")
}

func TestNextPrevFocus_AreInverses(t *testing.T) {
	t.Parallel()
	panels := []FocusPanel{FocusSidebar, FocusAgentPanel, FocusEventLog}
	for _, p := range panels {
		assert.Equal(t, p, PrevFocus(NextFocus(p)),
			"PrevFocus(NextFocus(p)) must equal p for panel %v", p)
		assert.Equal(t, p, NextFocus(PrevFocus(p)),
			"NextFocus(PrevFocus(p)) must equal p for panel %v", p)
	}
}

// ---------------------------------------------------------------------------
// HelpOverlay
// ---------------------------------------------------------------------------

func TestHelpOverlay_InitiallyHidden(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	assert.False(t, h.IsVisible(), "help overlay must start hidden")
}

func TestHelpOverlay_Toggle_FlipsVisibility(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())

	h.Toggle()
	assert.True(t, h.IsVisible(), "Toggle() once should make the overlay visible")

	h.Toggle()
	assert.False(t, h.IsVisible(), "Toggle() twice should hide the overlay again")
}

func TestHelpOverlay_Toggle_MultipleFlips(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())

	for i := 0; i < 6; i++ {
		prev := h.IsVisible()
		h.Toggle()
		assert.Equal(t, !prev, h.IsVisible(),
			"Toggle() must flip visibility at iteration %d", i)
	}
}

func TestHelpOverlay_SetDimensions(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(120, 40)

	assert.Equal(t, 120, h.width, "SetDimensions must update width")
	assert.Equal(t, 40, h.height, "SetDimensions must update height")
}

func TestHelpOverlay_View_HiddenReturnsEmpty(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(120, 40)

	assert.Empty(t, h.View(), "View() must return empty string when overlay is hidden")
}

func TestHelpOverlay_View_NoDimensionsReturnsEmpty(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.Toggle() // visible but no dimensions

	assert.Empty(t, h.View(), "View() must return empty string when dimensions are not set")
}

func TestHelpOverlay_View_ContainsKeyBindings(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(160, 50)
	h.Toggle()

	view := h.View()
	require.NotEmpty(t, view, "View() must not be empty when visible with dimensions set")

	// Check that all major keybinding descriptions appear in the view.
	descriptions := []string{
		"quit",
		"help",
		"pause",
		"skip",
		"toggle log",
		"next panel",
		"prev panel",
		"scroll up",
		"scroll down",
		"page up",
		"page down",
		"go to top",
		"go to bottom",
	}
	lower := strings.ToLower(view)
	for _, desc := range descriptions {
		assert.Contains(t, lower, strings.ToLower(desc),
			"View() must contain keybinding description: %q", desc)
	}
}

func TestHelpOverlay_View_ContainsCategories(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(160, 50)
	h.Toggle()

	view := h.View()
	lower := strings.ToLower(view)

	categories := []string{"navigation", "actions", "scrolling"}
	for _, cat := range categories {
		assert.Contains(t, lower, cat,
			"View() must contain category heading: %q", cat)
	}
}

func TestHelpOverlay_View_ContainsDismissHint(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(160, 50)
	h.Toggle()

	view := h.View()
	lower := strings.ToLower(view)
	assert.Contains(t, lower, "esc", "View() must contain the Esc dismiss hint")
}

func TestHelpOverlay_Update_EscHidesOverlay(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(120, 40)
	h.Toggle() // visible

	updated, cmd := h.Update(tea.KeyMsg{Type: tea.KeyEsc})

	assert.False(t, updated.IsVisible(), "Esc must hide the overlay")
	assert.Nil(t, cmd, "Esc must not return a command")
}

func TestHelpOverlay_Update_QuestionMarkHidesOverlay(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(120, 40)
	h.Toggle() // visible

	updated, cmd := h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	assert.False(t, updated.IsVisible(), "? must hide the overlay")
	assert.Nil(t, cmd, "? must not return a command")
}

func TestHelpOverlay_Update_OtherKeyDoesNotChangeVisibility(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(120, 40)
	h.Toggle() // visible

	updated, cmd := h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	assert.True(t, updated.IsVisible(), "unrelated key must not change overlay visibility")
	assert.Nil(t, cmd)
}

func TestHelpOverlay_Update_NonKeyMsgNoOp(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(120, 40)
	h.Toggle() // visible

	type randomMsg struct{}
	updated, cmd := h.Update(randomMsg{})

	assert.True(t, updated.IsVisible(), "non-key message must not change overlay visibility")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// FocusPanel.String helper (used by test names above)
// ---------------------------------------------------------------------------

// String returns a human-readable label for the FocusPanel constant.
// This is a test-local method; the production type does not expose String().
func (f FocusPanel) String() string {
	switch f {
	case FocusSidebar:
		return "FocusSidebar"
	case FocusAgentPanel:
		return "FocusAgentPanel"
	case FocusEventLog:
		return "FocusEventLog"
	default:
		return "FocusPanel(unknown)"
	}
}

// ---------------------------------------------------------------------------
// App integration: keyboard navigation
// ---------------------------------------------------------------------------

func TestApp_Tab_CyclesFocusForward(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)
	require.Equal(t, FocusSidebar, a.focus, "initial focus should be sidebar")

	// First Tab: sidebar -> agent panel
	a, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyTab})
	require.NotNil(t, cmd, "Tab must return a FocusChangedMsg command")
	assert.Equal(t, FocusAgentPanel, a.focus, "Tab from sidebar should focus agent panel")

	// Second Tab: agent panel -> event log
	a, cmd = applyMsg(a, tea.KeyMsg{Type: tea.KeyTab})
	require.NotNil(t, cmd)
	assert.Equal(t, FocusEventLog, a.focus, "Tab from agent panel should focus event log")

	// Third Tab: event log -> sidebar (wraps around)
	a, cmd = applyMsg(a, tea.KeyMsg{Type: tea.KeyTab})
	require.NotNil(t, cmd)
	assert.Equal(t, FocusSidebar, a.focus, "Tab from event log should wrap to sidebar")
}

func TestApp_ShiftTab_CyclesFocusBackward(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)
	require.Equal(t, FocusSidebar, a.focus)

	// Shift+Tab from sidebar wraps to event log.
	a, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyShiftTab})
	require.NotNil(t, cmd)
	assert.Equal(t, FocusEventLog, a.focus, "Shift+Tab from sidebar should wrap to event log")

	// Shift+Tab from event log -> agent panel.
	a, cmd = applyMsg(a, tea.KeyMsg{Type: tea.KeyShiftTab})
	require.NotNil(t, cmd)
	assert.Equal(t, FocusAgentPanel, a.focus, "Shift+Tab from event log should focus agent panel")

	// Shift+Tab from agent panel -> sidebar.
	a, cmd = applyMsg(a, tea.KeyMsg{Type: tea.KeyShiftTab})
	require.NotNil(t, cmd)
	assert.Equal(t, FocusSidebar, a.focus, "Shift+Tab from agent panel should focus sidebar")
}

func TestApp_QuestionMark_TogglesHelpOverlay(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)
	assert.False(t, a.helpOverlay.IsVisible(), "overlay should be hidden initially")

	a, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	assert.Nil(t, cmd, "toggling help overlay must not return a command")
	assert.True(t, a.helpOverlay.IsVisible(), "? must show the help overlay")

	a, cmd = applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	assert.Nil(t, cmd)
	assert.False(t, a.helpOverlay.IsVisible(), "? again must hide the overlay")
}

func TestApp_HelpOverlayVisible_SuppressesOtherKeys(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)

	// Show the overlay.
	a, _ = applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	require.True(t, a.helpOverlay.IsVisible())

	// Tab should NOT change focus while overlay is visible.
	before := a.focus
	a, _ = applyMsg(a, tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, before, a.focus, "Tab must not change focus when help overlay is visible")

	// 'p' should NOT quit or send PauseRequestMsg.
	a2, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	assert.False(t, a2.quitting, "'p' must not quit while overlay is visible")
	assert.Nil(t, cmd, "'p' must not return a command while overlay is visible")
}

func TestApp_HelpOverlay_EscHidesOverlay(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)

	// Show overlay.
	a, _ = applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	require.True(t, a.helpOverlay.IsVisible())

	// Esc dismisses it.
	a, _ = applyMsg(a, tea.KeyMsg{Type: tea.KeyEsc})
	assert.False(t, a.helpOverlay.IsVisible(), "Esc must dismiss the help overlay")
}

func TestApp_PKey_SendsPauseRequestMsg(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)

	_, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	require.NotNil(t, cmd, "'p' must return a command")

	msg := cmd()
	_, isPause := msg.(PauseRequestMsg)
	assert.True(t, isPause, "'p' command must return a PauseRequestMsg")
}

func TestApp_SKey_SendsSkipRequestMsg(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)

	_, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	require.NotNil(t, cmd, "'s' must return a command")

	msg := cmd()
	_, isSkip := msg.(SkipRequestMsg)
	assert.True(t, isSkip, "'s' command must return a SkipRequestMsg")
}

func TestApp_LKey_NoOpForNow(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)

	_, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Nil(t, cmd, "'l' must return nil command (event log not wired yet)")
}

func TestApp_WindowSizeMsg_UpdatesHelpOverlayDimensions(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})

	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 200, Height: 60})

	assert.Equal(t, 200, a.helpOverlay.width, "WindowSizeMsg must update help overlay width")
	assert.Equal(t, 60, a.helpOverlay.height, "WindowSizeMsg must update help overlay height")
}

func TestApp_View_HelpOverlayVisible_RendersOverlay(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 160, 50)

	// Show the help overlay.
	a, _ = applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	require.True(t, a.helpOverlay.IsVisible())

	view := a.View()
	require.NotEmpty(t, view, "View() must not be empty when help overlay is visible")

	lower := strings.ToLower(view)
	assert.Contains(t, lower, "quit", "help overlay view must contain 'quit' keybinding")
	assert.Contains(t, lower, "navigation", "help overlay view must contain 'Navigation' section")
}

func TestApp_Tab_RapidPresses_NoPanic(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)

	assert.NotPanics(t, func() {
		for i := 0; i < 20; i++ {
			a, _ = applyMsg(a, tea.KeyMsg{Type: tea.KeyTab})
		}
	}, "rapid Tab presses must not panic")
}

func TestApp_FocusCycleWith_Tab_FullCircle(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)
	start := a.focus

	for i := 0; i < focusPanelCount; i++ {
		a, _ = applyMsg(a, tea.KeyMsg{Type: tea.KeyTab})
	}

	assert.Equal(t, start, a.focus, "N Tab presses must return focus to the starting panel")
}

// ---------------------------------------------------------------------------
// DefaultKeyMap – agent panel bindings
// ---------------------------------------------------------------------------

func TestDefaultKeyMap_NextAgentMatchesTab(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()
	msg := tea.KeyMsg{Type: tea.KeyTab}
	assert.True(t, key.Matches(msg, km.NextAgent), "NextAgent binding must match Tab")
}

func TestDefaultKeyMap_PrevAgentMatchesShiftTab(t *testing.T) {
	t.Parallel()
	km := DefaultKeyMap()
	msg := tea.KeyMsg{Type: tea.KeyShiftTab}
	assert.True(t, key.Matches(msg, km.PrevAgent), "PrevAgent binding must match Shift+Tab")
}

// ---------------------------------------------------------------------------
// PauseRequestMsg / SkipRequestMsg – struct type assertions
// ---------------------------------------------------------------------------

func TestPauseRequestMsg_IsZeroStruct(t *testing.T) {
	t.Parallel()
	// PauseRequestMsg is an empty struct; ensure it can be constructed and
	// matched against its type without panicking.
	var msg PauseRequestMsg
	_, ok := (tea.Msg(msg)).(PauseRequestMsg)
	assert.True(t, ok, "PauseRequestMsg must be type-assertable as PauseRequestMsg")
}

func TestSkipRequestMsg_IsZeroStruct(t *testing.T) {
	t.Parallel()
	var msg SkipRequestMsg
	_, ok := (tea.Msg(msg)).(SkipRequestMsg)
	assert.True(t, ok, "SkipRequestMsg must be type-assertable as SkipRequestMsg")
}

func TestPauseRequestMsg_NotSkipRequestMsg(t *testing.T) {
	t.Parallel()
	var pause PauseRequestMsg
	_, isSkip := (tea.Msg(pause)).(SkipRequestMsg)
	assert.False(t, isSkip, "PauseRequestMsg must not match SkipRequestMsg")
}

func TestSkipRequestMsg_NotPauseRequestMsg(t *testing.T) {
	t.Parallel()
	var skip SkipRequestMsg
	_, isPause := (tea.Msg(skip)).(PauseRequestMsg)
	assert.False(t, isPause, "SkipRequestMsg must not match PauseRequestMsg")
}

// ---------------------------------------------------------------------------
// HelpOverlay – partial dimensions
// ---------------------------------------------------------------------------

func TestHelpOverlay_View_ZeroWidthOnly_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(0, 40) // zero width only
	h.Toggle()

	assert.Empty(t, h.View(),
		"View() must return empty when width is zero even if height is set")
}

func TestHelpOverlay_View_ZeroHeightOnly_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(120, 0) // zero height only
	h.Toggle()

	assert.Empty(t, h.View(),
		"View() must return empty when height is zero even if width is set")
}

// ---------------------------------------------------------------------------
// HelpOverlay – Update when overlay is not visible
// ---------------------------------------------------------------------------

func TestHelpOverlay_Update_WhenHidden_EscIsNoOp(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	// Overlay starts hidden; Esc should keep it hidden (visible stays false).
	updated, cmd := h.Update(tea.KeyMsg{Type: tea.KeyEsc})

	assert.False(t, updated.IsVisible(), "Esc on a hidden overlay must leave it hidden")
	assert.Nil(t, cmd)
}

func TestHelpOverlay_Update_WhenHidden_QuestionMarkIsNoOp(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	// Overlay starts hidden; '?' via Update sets visible=false (no-op since already false).
	updated, cmd := h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	assert.False(t, updated.IsVisible(),
		"? on a hidden overlay must not change visibility through Update")
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// HelpOverlay – View contains "next agent" binding
// ---------------------------------------------------------------------------

func TestHelpOverlay_View_ContainsNextAgentBinding(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(160, 50)
	h.Toggle()

	view := h.View()
	require.NotEmpty(t, view)

	// "next agent" appears in the Navigation section.
	lower := strings.ToLower(view)
	assert.Contains(t, lower, "next agent",
		"View() must list the 'next agent' binding under Navigation")
}

// ---------------------------------------------------------------------------
// HelpOverlay – View content is stable (idempotent)
// ---------------------------------------------------------------------------

func TestHelpOverlay_View_Idempotent(t *testing.T) {
	t.Parallel()
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(160, 50)
	h.Toggle()

	first := h.View()
	second := h.View()
	assert.Equal(t, first, second,
		"HelpOverlay.View() must return identical output on repeated calls")
}

// ---------------------------------------------------------------------------
// FocusPanel.String – unknown value falls through to default branch
// ---------------------------------------------------------------------------

func TestFocusPanel_String_UnknownValue(t *testing.T) {
	t.Parallel()
	unknown := FocusPanel(99)
	assert.Equal(t, "FocusPanel(unknown)", unknown.String(),
		"String() for an out-of-range FocusPanel must return the unknown sentinel")
}

// ---------------------------------------------------------------------------
// focusPanelCount – constant integrity
// ---------------------------------------------------------------------------

func TestFocusPanelCount_MatchesDefinedPanels(t *testing.T) {
	t.Parallel()
	// Verify that focusPanelCount == 3, matching the three FocusPanel constants.
	assert.Equal(t, 3, focusPanelCount,
		"focusPanelCount must equal the number of FocusPanel constants")
}

// ---------------------------------------------------------------------------
// Benchmarks – keybindings hot paths
// ---------------------------------------------------------------------------

// BenchmarkDefaultKeyMap measures allocation cost of building the default key map.
func BenchmarkDefaultKeyMap(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DefaultKeyMap()
	}
}

// BenchmarkNextFocus measures the focus cycling function; it runs in O(1).
func BenchmarkNextFocus(b *testing.B) {
	cur := FocusSidebar
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur = NextFocus(cur)
	}
}

// BenchmarkPrevFocus mirrors BenchmarkNextFocus for the reverse direction.
func BenchmarkPrevFocus(b *testing.B) {
	cur := FocusEventLog
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur = PrevFocus(cur)
	}
}

// BenchmarkHelpOverlay_View measures rendering cost with dimensions set.
func BenchmarkHelpOverlay_View(b *testing.B) {
	h := NewHelpOverlay(DefaultTheme(), DefaultKeyMap())
	h.SetDimensions(160, 50)
	h.Toggle()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.View()
	}
}

// BenchmarkKeyMatches measures the key.Matches hot path used in Update dispatch.
func BenchmarkKeyMatches(b *testing.B) {
	km := DefaultKeyMap()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = key.Matches(msg, km.Quit)
	}
}
