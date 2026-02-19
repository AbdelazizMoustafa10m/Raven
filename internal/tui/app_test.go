package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time verification that App implements tea.Model.
var _ tea.Model = App{}

// applyMsg applies a single message to App.Update and returns the updated App
// and any command. It panics if Update returns a non-App model, which would
// indicate a bug in the implementation.
func applyMsg(a App, msg tea.Msg) (App, tea.Cmd) {
	model, cmd := a.Update(msg)
	updated, ok := model.(App)
	if !ok {
		panic("Update returned a non-App tea.Model")
	}
	return updated, cmd
}

// applyMsgs applies a sequence of messages in order and returns the final App.
func applyMsgs(t *testing.T, a App, msgs ...tea.Msg) App {
	t.Helper()
	var cmd tea.Cmd
	for _, msg := range msgs {
		a, cmd = applyMsg(a, msg)
		_ = cmd
	}
	return a
}

// makeReadyApp returns an App that has received a WindowSizeMsg with the given
// dimensions, placing it in the "ready" state.
func makeReadyApp(t *testing.T, cfg AppConfig, width, height int) App {
	t.Helper()
	a := NewApp(cfg)
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: width, Height: height})
	require.True(t, a.ready, "makeReadyApp: app should be ready after WindowSizeMsg")
	return a
}

// ---- FocusPanel constants ----

func TestFocusPanel_Constants(t *testing.T) {
	t.Parallel()
	// Verify that the three panel constants are distinct and that FocusSidebar
	// is the zero value (iota starts at 0).
	assert.Equal(t, FocusPanel(0), FocusSidebar, "FocusSidebar should be zero value")
	assert.Equal(t, FocusPanel(1), FocusAgentPanel, "FocusAgentPanel should be 1")
	assert.Equal(t, FocusPanel(2), FocusEventLog, "FocusEventLog should be 2")

	// All three must be distinct.
	assert.NotEqual(t, FocusSidebar, FocusAgentPanel)
	assert.NotEqual(t, FocusSidebar, FocusEventLog)
	assert.NotEqual(t, FocusAgentPanel, FocusEventLog)
}

// ---- AppConfig ----

func TestAppConfig_ZeroValue(t *testing.T) {
	t.Parallel()
	// A zero-value AppConfig must not cause NewApp to panic.
	assert.NotPanics(t, func() {
		a := NewApp(AppConfig{})
		_ = a.View()
	})
}

func TestAppConfig_EmptyProjectName(t *testing.T) {
	t.Parallel()
	cfg := AppConfig{Version: "1.0.0", ProjectName: ""}
	a := makeReadyApp(t, cfg, 120, 40)

	view := a.View()
	assert.Contains(t, view, "Raven v1.0.0")
	// The title bar project-name separator "  |  " should not appear in the
	// title bar line when ProjectName is empty. Check only the first line.
	titleBar := strings.SplitN(view, "\n", 2)[0]
	assert.NotContains(t, titleBar, "  |  ")
}

func TestAppConfig_WithProjectName(t *testing.T) {
	t.Parallel()
	cfg := AppConfig{Version: "1.0.0", ProjectName: "acme"}
	a := makeReadyApp(t, cfg, 120, 40)

	view := a.View()
	assert.Contains(t, view, "acme", "project name must appear in title bar")
	assert.Contains(t, view, "|", "separator must appear when project name is set")
}

// ---- NewApp defaults ----

func TestNewApp_Defaults(t *testing.T) {
	t.Parallel()
	cfg := AppConfig{Version: "1.2.3", ProjectName: "my-project"}
	a := NewApp(cfg)

	assert.Equal(t, FocusSidebar, a.focus, "default focus should be sidebar")
	assert.False(t, a.ready, "ready should be false before first WindowSizeMsg")
	assert.False(t, a.quitting, "quitting should be false on construction")
	assert.Equal(t, 0, a.width, "width should be zero before WindowSizeMsg")
	assert.Equal(t, 0, a.height, "height should be zero before WindowSizeMsg")
	assert.Equal(t, cfg, a.config, "config should be stored as-is")
}

func TestNewApp_DefaultFocusIsSidebar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  AppConfig
	}{
		{name: "with version and project", cfg: AppConfig{Version: "1.0.0", ProjectName: "proj"}},
		{name: "version only", cfg: AppConfig{Version: "2.0.0"}},
		{name: "zero value config", cfg: AppConfig{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := NewApp(tt.cfg)
			assert.Equal(t, FocusSidebar, a.focus)
			assert.False(t, a.ready)
			assert.False(t, a.quitting)
		})
	}
}

// ---- Init ----

func TestApp_Init_ReturnsNil(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "0.0.1"})
	cmd := a.Init()
	assert.Nil(t, cmd, "Init should return nil (bubbletea sends WindowSizeMsg automatically)")
}

// Init must not mutate the App.
func TestApp_Init_DoesNotMutateState(t *testing.T) {
	t.Parallel()
	cfg := AppConfig{Version: "1.0.0", ProjectName: "test"}
	a := NewApp(cfg)
	before := a

	_ = a.Init()

	assert.Equal(t, before, a, "Init must not modify App state")
}

// ---- Update: WindowSizeMsg ----

func TestApp_Update_WindowSizeMsg_SetsReadyAndDimensions(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	updated, cmd := applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})

	assert.True(t, updated.ready, "ready should be true after WindowSizeMsg")
	assert.Equal(t, 120, updated.width)
	assert.Equal(t, 40, updated.height)
	assert.Nil(t, cmd, "no follow-up command expected for WindowSizeMsg")
}

func TestApp_Update_WindowSizeMsg_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		width       int
		height      int
		wantReady   bool
		wantWidth   int
		wantHeight  int
	}{
		{
			name: "standard terminal 80x24",
			width: 80, height: 24,
			wantReady: true, wantWidth: 80, wantHeight: 24,
		},
		{
			name: "large terminal 200x60",
			width: 200, height: 60,
			wantReady: true, wantWidth: 200, wantHeight: 60,
		},
		{
			name: "very small terminal 10x5",
			width: 10, height: 5,
			wantReady: true, wantWidth: 10, wantHeight: 5,
		},
		{
			name: "zero dimensions",
			width: 0, height: 0,
			wantReady: true, wantWidth: 0, wantHeight: 0,
		},
		{
			name: "just below minimum width",
			width: 79, height: 30,
			wantReady: true, wantWidth: 79, wantHeight: 30,
		},
		{
			name: "just below minimum height",
			width: 120, height: 23,
			wantReady: true, wantWidth: 120, wantHeight: 23,
		},
		{
			name: "ultra wide terminal",
			width: 500, height: 200,
			wantReady: true, wantWidth: 500, wantHeight: 200,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := NewApp(AppConfig{Version: "1.0.0"})
			updated, cmd := applyMsg(a, tea.WindowSizeMsg{Width: tt.width, Height: tt.height})

			assert.Equal(t, tt.wantReady, updated.ready)
			assert.Equal(t, tt.wantWidth, updated.width)
			assert.Equal(t, tt.wantHeight, updated.height)
			assert.Nil(t, cmd)
		})
	}
}

func TestApp_Update_MultipleWindowSizeMsgs_TracksLatest(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})

	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 100, Height: 30})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 200, Height: 50})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 80, Height: 24})

	assert.Equal(t, 80, a.width, "width should reflect most recent WindowSizeMsg")
	assert.Equal(t, 24, a.height, "height should reflect most recent WindowSizeMsg")
	assert.True(t, a.ready)
}

func TestApp_Update_ZeroDimensionWindowSizeMsg(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	updated, cmd := applyMsg(a, tea.WindowSizeMsg{Width: 0, Height: 0})

	// Should not panic; just store the values.
	assert.Equal(t, 0, updated.width)
	assert.Equal(t, 0, updated.height)
	assert.True(t, updated.ready, "ready is set even for zero-size window")
	assert.Nil(t, cmd)
}

// WindowSizeMsg must not change the quitting flag.
func TestApp_Update_WindowSizeMsg_DoesNotAffectQuitting(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	updated, _ := applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})

	assert.False(t, updated.quitting, "WindowSizeMsg must not affect quitting flag")
}

// WindowSizeMsg must not change the focus.
func TestApp_Update_WindowSizeMsg_DoesNotAffectFocus(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	updated, _ := applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})

	assert.Equal(t, FocusSidebar, updated.focus, "WindowSizeMsg must not change focus")
}

// ---- Update: KeyMsg quit keys ----

func TestApp_Update_QuitKeys_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		msg     tea.KeyMsg
		wantQuit bool
	}{
		{
			name:     "ctrl+c",
			msg:      tea.KeyMsg{Type: tea.KeyCtrlC},
			wantQuit: true,
		},
		{
			name:     "ctrl+q",
			msg:      tea.KeyMsg{Type: tea.KeyCtrlQ},
			wantQuit: true,
		},
		{
			name:     "q rune",
			msg:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
			wantQuit: true,
		},
		{
			name:     "other rune x",
			msg:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}},
			wantQuit: false,
		},
		{
			name:     "other rune Q uppercase",
			msg:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}},
			wantQuit: false,
		},
		{
			name:     "enter key",
			msg:      tea.KeyMsg{Type: tea.KeyEnter},
			wantQuit: false,
		},
		{
			name:     "space rune",
			msg:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}},
			wantQuit: false,
		},
		{
			name:     "escape key",
			msg:      tea.KeyMsg{Type: tea.KeyEsc},
			wantQuit: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)
			updated, cmd := applyMsg(a, tt.msg)

			assert.Equal(t, tt.wantQuit, updated.quitting,
				"quitting flag mismatch for key %v", tt.msg)
			if tt.wantQuit {
				require.NotNil(t, cmd, "quit key must return a command")
			} else {
				assert.Nil(t, cmd, "non-quit key must return nil command")
			}
		})
	}
}

func TestApp_Update_CtrlC_SetsQuitting(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})

	updated, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyCtrlC})

	assert.True(t, updated.quitting, "quitting should be true after ctrl+c")
	require.NotNil(t, cmd, "should return a tea.Quit command")
}

func TestApp_Update_CtrlQ_SetsQuitting(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})

	updated, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyCtrlQ})

	assert.True(t, updated.quitting, "quitting should be true after ctrl+q")
	require.NotNil(t, cmd, "should return a tea.Quit command")
}

func TestApp_Update_QRune_SetsQuitting(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})

	updated, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	assert.True(t, updated.quitting, "quitting should be true after 'q' key")
	require.NotNil(t, cmd, "should return a tea.Quit command")
}

func TestApp_Update_OtherRune_DoesNotQuit(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})

	updated, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	assert.False(t, updated.quitting, "non-quit key should not set quitting")
	assert.Nil(t, cmd)
}

func TestApp_Update_QuitBeforeReady(t *testing.T) {
	t.Parallel()
	// Quitting before the first WindowSizeMsg should still work cleanly.
	a := NewApp(AppConfig{Version: "1.0.0"})
	assert.False(t, a.ready)

	updated, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyCtrlC})

	assert.True(t, updated.quitting)
	require.NotNil(t, cmd)
	// The app must not become ready just because it quit.
	assert.False(t, updated.ready)
}

// Quitting should not affect dimensions stored in the App.
func TestApp_Update_QuitPreservesDimensions(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)
	updated, _ := applyMsg(a, tea.KeyMsg{Type: tea.KeyCtrlC})

	assert.Equal(t, 120, updated.width)
	assert.Equal(t, 40, updated.height)
}

// ---- Update: unhandled message ----

func TestApp_Update_UnknownMsg_NoOp(t *testing.T) {
	t.Parallel()
	type customMsg struct{ val int }

	a := NewApp(AppConfig{Version: "1.0.0"})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})

	updated, cmd := applyMsg(a, customMsg{val: 42})

	assert.Equal(t, a, updated, "unhandled message should not change state")
	assert.Nil(t, cmd)
}

func TestApp_Update_UnknownMsg_TableDriven(t *testing.T) {
	t.Parallel()
	type customMsg struct{ payload string }
	type anotherMsg int

	tests := []struct {
		name string
		msg  tea.Msg
	}{
		{name: "custom struct message", msg: customMsg{payload: "hello"}},
		{name: "integer message", msg: anotherMsg(99)},
		{name: "nil message", msg: nil},
		{name: "string message", msg: tea.Msg("some-string")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)
			snapshot := a

			updated, cmd := applyMsg(a, tt.msg)

			assert.Equal(t, snapshot, updated, "unknown message must not change state")
			assert.Nil(t, cmd, "unknown message must return nil command")
		})
	}
}

// ---- View ----

func TestApp_View_NotReady_ReturnsInitializingMessage(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	// Do not send WindowSizeMsg — app is not ready.
	view := a.View()
	assert.Contains(t, view, "Initializing", "not-ready view should contain initialization message")
}

func TestApp_View_NotReady_DoesNotContainVersionOrPanels(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "9.9.9", ProjectName: "secret"})
	view := a.View()

	// Before ready, the full layout should not be rendered.
	assert.NotContains(t, view, "9.9.9",
		"version must not appear in the loading view")
	assert.NotContains(t, view, "panels",
		"panel stubs must not appear in the loading view")
}

func TestApp_View_Quitting_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})
	a, _ = applyMsg(a, tea.KeyMsg{Type: tea.KeyCtrlC})

	view := a.View()
	assert.Empty(t, view, "quitting view should be empty string")
}

// Quitting before ready should also produce an empty view.
func TestApp_View_QuittingBeforeReady_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	a, _ = applyMsg(a, tea.KeyMsg{Type: tea.KeyCtrlC})

	view := a.View()
	assert.Empty(t, view, "quitting before ready should still produce an empty view")
}

func TestApp_View_TerminalTooSmall_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		width     int
		height    int
		wantSmall bool
	}{
		{name: "width 79 height 30 — width too small", width: 79, height: 30, wantSmall: true},
		{name: "width 120 height 23 — height too small", width: 120, height: 23, wantSmall: true},
		{name: "width 79 height 23 — both too small", width: 79, height: 23, wantSmall: true},
		{name: "width 0 height 0 — zero dimensions", width: 0, height: 0, wantSmall: true},
		{name: "width 1 height 1 — extremely small", width: 1, height: 1, wantSmall: true},
		{name: "width 80 height 24 — exact minimum", width: 80, height: 24, wantSmall: false},
		{name: "width 81 height 25 — just above minimum", width: 81, height: 25, wantSmall: false},
		{name: "width 120 height 40 — normal terminal", width: 120, height: 40, wantSmall: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, tt.width, tt.height)
			view := a.View()
			lower := strings.ToLower(view)

			if tt.wantSmall {
				assert.Contains(t, lower, "too small",
					"view should show resize warning for %dx%d", tt.width, tt.height)
			} else {
				assert.NotContains(t, lower, "too small",
					"view should not show resize warning for %dx%d", tt.width, tt.height)
			}
		})
	}
}

func TestApp_View_TerminalTooSmall_Width(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 79, Height: 30})

	view := a.View()
	assert.Contains(t, strings.ToLower(view), "too small", "view should warn when width < 80")
}

func TestApp_View_TerminalTooSmall_Height(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 23})

	view := a.View()
	assert.Contains(t, strings.ToLower(view), "too small", "view should warn when height < 24")
}

func TestApp_View_TerminalTooSmall_ContainsMinimumInfo(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 50, 10)
	view := a.View()

	// The warning should mention the minimum dimensions.
	assert.Contains(t, view, "80", "warning should mention minimum width 80")
	assert.Contains(t, view, "24", "warning should mention minimum height 24")
}

func TestApp_View_TerminalExactMinimum_ShowsFullView(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "2.0.0", ProjectName: "raven"})
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 80, Height: 24})

	view := a.View()
	assert.NotEmpty(t, view, "minimum-size terminal should render a full view")
	assert.NotContains(t, strings.ToLower(view), "too small")
}

func TestApp_View_Ready_ContainsTitleBar(t *testing.T) {
	t.Parallel()
	cfg := AppConfig{Version: "2.0.0", ProjectName: "my-project"}
	a := NewApp(cfg)
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})

	view := a.View()
	assert.Contains(t, view, "Raven v2.0.0", "title bar should contain version")
	assert.Contains(t, view, "Command Center", "title bar should contain 'Command Center'")
}

// The title bar must include the project name when it is non-empty.
func TestApp_View_TitleBar_WithProjectName(t *testing.T) {
	t.Parallel()
	cfg := AppConfig{Version: "1.5.0", ProjectName: "my-app"}
	a := makeReadyApp(t, cfg, 120, 40)
	view := a.View()

	assert.Contains(t, view, "Raven v1.5.0")
	assert.Contains(t, view, "Command Center")
	assert.Contains(t, view, "my-app")
}

// The title bar must omit the project name section when it is empty.
func TestApp_View_TitleBar_WithoutProjectName(t *testing.T) {
	t.Parallel()
	cfg := AppConfig{Version: "1.5.0", ProjectName: ""}
	a := makeReadyApp(t, cfg, 120, 40)
	view := a.View()

	assert.Contains(t, view, "Raven v1.5.0")
	assert.Contains(t, view, "Command Center")
	// The title bar separator "  |  " only appears in the title bar line when
	// a project name is present. Check only the first line.
	titleBar := strings.SplitN(view, "\n", 2)[0]
	assert.NotContains(t, titleBar, "  |  ")
}

// The full view must contain the sidebar workflow list section header, which
// is rendered by SidebarModel.View() as "WORKFLOWS".
func TestApp_View_Ready_ContainsPanelArea(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)
	view := a.View()

	assert.Contains(t, view, "WORKFLOWS", "full view must contain the sidebar WORKFLOWS section header")
}

func TestApp_View_Ready_LargeTerminal_NoPanic(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})
	// Very large terminal should not panic or produce garbled output.
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 300, Height: 100})

	assert.NotPanics(t, func() {
		_ = a.View()
	})
}

// View must not panic for height=1 (triggers terminal-too-small branch; must not panic).
func TestApp_View_HeightOne_NoPanic(t *testing.T) {
	t.Parallel()
	// height=1 is less than the minimum 24, so View takes the "too small" branch.
	// This must not panic regardless of the tiny dimensions.
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 80, 1)

	assert.NotPanics(t, func() {
		view := a.View()
		assert.Contains(t, strings.ToLower(view), "too small")
	})
}

// fullView with heights near the minimum must not panic.
func TestApp_View_FullView_PanelHeightClamp_Boundary(t *testing.T) {
	t.Parallel()
	// height=24 → fullView renders; panelHeight = 23.
	// height=25 → fullView renders; panelHeight = 24.
	// height=26 → fullView renders; panelHeight = 25.
	// None of these should panic.
	for _, h := range []int{24, 25, 26} {
		h := h
		t.Run("height="+itoa(h), func(t *testing.T) {
			t.Parallel()
			a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 80, h)
			assert.NotPanics(t, func() {
				_ = a.View()
			})
		})
	}
}

// itoa converts an int to its decimal string representation.
func itoa(n int) string {
	return strconv.Itoa(n)
}

// View must not panic when called multiple times on the same App (idempotency).
func TestApp_View_Idempotent(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0", ProjectName: "p"}, 120, 40)

	first := a.View()
	second := a.View()
	assert.Equal(t, first, second, "View must return identical output on repeated calls")
}

// ---- Rapid successive WindowSizeMsg messages ----

func TestApp_RapidWindowSizeMsgs_NoCorruption(t *testing.T) {
	t.Parallel()
	a := NewApp(AppConfig{Version: "1.0.0"})

	sizes := [][2]int{
		{100, 30}, {200, 50}, {80, 24}, {160, 45}, {90, 35}, {80, 24},
	}
	for _, s := range sizes {
		a, _ = applyMsg(a, tea.WindowSizeMsg{Width: s[0], Height: s[1]})
		require.True(t, a.ready, "app must be ready after each WindowSizeMsg")
	}

	last := sizes[len(sizes)-1]
	assert.Equal(t, last[0], a.width)
	assert.Equal(t, last[1], a.height)

	// View must not panic after rapid resizes.
	assert.NotPanics(t, func() { _ = a.View() })
}

// ---- Integration: WindowSizeMsg then View ----

func TestApp_Integration_WindowSizeThenView(t *testing.T) {
	t.Parallel()
	cfg := AppConfig{Version: "3.1.4", ProjectName: "pi-project"}
	a := NewApp(cfg)

	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 160, Height: 50})
	require.True(t, a.ready)

	view := a.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "3.1.4", "version must appear in the full view")
	assert.Contains(t, view, "pi-project", "project name must appear in the full view")
}

// Full lifecycle: init → resize → interact → quit → empty view.
func TestApp_Integration_FullLifecycle(t *testing.T) {
	t.Parallel()
	cfg := AppConfig{Version: "5.0.0", ProjectName: "lifecycle-test"}
	a := NewApp(cfg)

	// Phase 1: before ready.
	require.False(t, a.ready)
	loadingView := a.View()
	assert.Contains(t, loadingView, "Initializing")

	// Phase 2: receive window size → ready.
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})
	require.True(t, a.ready)
	readyView := a.View()
	assert.Contains(t, readyView, "5.0.0")
	assert.Contains(t, readyView, "lifecycle-test")

	// Phase 3: send a non-quit key → still running.
	a, cmd := applyMsg(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.False(t, a.quitting)
	assert.Nil(t, cmd)

	// Phase 4: resize terminal to too small → warning shown.
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 60, Height: 20})
	smallView := a.View()
	assert.Contains(t, strings.ToLower(smallView), "too small")

	// Phase 5: resize back to normal.
	a, _ = applyMsg(a, tea.WindowSizeMsg{Width: 120, Height: 40})
	assert.NotContains(t, strings.ToLower(a.View()), "too small")

	// Phase 6: quit.
	a, cmd = applyMsg(a, tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.True(t, a.quitting)
	require.NotNil(t, cmd)

	// Phase 7: view after quit → empty.
	assert.Empty(t, a.View())
}

// Multiple quit key presses after the first should remain idempotent.
func TestApp_Integration_DoubleQuit_Idempotent(t *testing.T) {
	t.Parallel()
	a := makeReadyApp(t, AppConfig{Version: "1.0.0"}, 120, 40)

	a, cmd1 := applyMsg(a, tea.KeyMsg{Type: tea.KeyCtrlC})
	require.NotNil(t, cmd1)
	assert.True(t, a.quitting)

	// Second quit key while already quitting.
	a, cmd2 := applyMsg(a, tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.True(t, a.quitting)
	require.NotNil(t, cmd2) // still returns Quit command
	assert.Empty(t, a.View())
}

// ---- Version display ----

func TestApp_View_VersionFormats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		version string
	}{
		{name: "semantic version", version: "2.0.0"},
		{name: "pre-release", version: "1.0.0-alpha.1"},
		{name: "dev build", version: "dev"},
		{name: "empty version", version: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := AppConfig{Version: tt.version}
			a := makeReadyApp(t, cfg, 120, 40)

			assert.NotPanics(t, func() {
				view := a.View()
				assert.NotEmpty(t, view)
				if tt.version != "" {
					assert.Contains(t, view, tt.version)
				}
			})
		})
	}
}
