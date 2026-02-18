package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Color Palette
// ---------------------------------------------------------------------------

// ColorPrimary is the main brand/accent color used for titles and highlights.
var ColorPrimary = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7B78FF"}

// ColorSecondary is used for secondary interactive elements and tab indicators.
var ColorSecondary = lipgloss.AdaptiveColor{Light: "#3B82F6", Dark: "#60A5FA"}

// ColorAccent is a green-teal accent for positive indicators and active states.
var ColorAccent = lipgloss.AdaptiveColor{Light: "#10B981", Dark: "#34D399"}

// ColorSuccess represents successful operations (green).
var ColorSuccess = lipgloss.AdaptiveColor{Light: "#16A34A", Dark: "#4ADE80"}

// ColorWarning represents cautionary states such as rate limits (amber/yellow).
var ColorWarning = lipgloss.AdaptiveColor{Light: "#D97706", Dark: "#FBBF24"}

// ColorError represents failures and error states (red).
var ColorError = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"}

// ColorInfo represents informational messages (blue).
var ColorInfo = lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#60A5FA"}

// ColorMuted is a subdued foreground color for secondary text.
var ColorMuted = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}

// ColorSubtle provides very low-contrast borders and dividers.
var ColorSubtle = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#4B5563"}

// ColorBorder is the standard panel border color.
var ColorBorder = lipgloss.AdaptiveColor{Light: "#E5E7EB", Dark: "#374151"}

// ColorHighlight is a background highlight for selected or hovered items.
var ColorHighlight = lipgloss.AdaptiveColor{Light: "#F3F4F6", Dark: "#1F2937"}

// ---------------------------------------------------------------------------
// Theme
// ---------------------------------------------------------------------------

// Theme holds all Lipgloss styles for the Raven TUI components. Every field
// is a pre-built lipgloss.Style value. Width and Height are NOT set on any
// theme style -- those are applied dynamically by the layout manager (T-069).
type Theme struct {
	// Title bar
	TitleBar     lipgloss.Style
	TitleText    lipgloss.Style
	TitleVersion lipgloss.Style
	TitleHint    lipgloss.Style

	// Sidebar
	SidebarContainer lipgloss.Style
	SidebarTitle     lipgloss.Style
	SidebarItem      lipgloss.Style
	SidebarActive    lipgloss.Style
	SidebarInactive  lipgloss.Style

	// Agent panel
	AgentContainer lipgloss.Style
	AgentHeader    lipgloss.Style
	AgentTab       lipgloss.Style
	AgentTabActive lipgloss.Style
	AgentOutput    lipgloss.Style

	// Event log
	EventContainer lipgloss.Style
	EventTimestamp lipgloss.Style
	EventMessage   lipgloss.Style

	// Status bar
	StatusBar       lipgloss.Style
	StatusKey       lipgloss.Style
	StatusValue     lipgloss.Style
	StatusSeparator lipgloss.Style

	// Progress bars
	ProgressFilled  lipgloss.Style
	ProgressEmpty   lipgloss.Style
	ProgressLabel   lipgloss.Style
	ProgressPercent lipgloss.Style

	// Status indicators
	StatusRunning   lipgloss.Style
	StatusCompleted lipgloss.Style
	StatusFailed    lipgloss.Style
	StatusWaiting   lipgloss.Style
	StatusBlocked   lipgloss.Style

	// General
	Border    lipgloss.Style
	HelpKey   lipgloss.Style
	HelpDesc  lipgloss.Style
	ErrorText lipgloss.Style

	// Dividers
	VerticalDivider   lipgloss.Style
	HorizontalDivider lipgloss.Style
}

// DefaultTheme returns the default Raven TUI theme with adaptive colors.
// All colors use lipgloss.AdaptiveColor for automatic light/dark terminal
// support. No Width or Height values are set -- those are applied at render
// time by the layout manager.
func DefaultTheme() Theme {
	return Theme{
		// --- Title bar ---
		TitleBar: lipgloss.NewStyle().
			Bold(true).
			Background(ColorPrimary).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1),

		TitleText: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")),

		TitleVersion: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#E0DFFF", Dark: "#C4C2FF"}),

		TitleHint: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#C7C5FF", Dark: "#A8A5FF"}),

		// --- Sidebar ---
		SidebarContainer: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderRight(true).
			BorderForeground(ColorBorder).
			PaddingLeft(1),

		SidebarTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1),

		SidebarItem: lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(1),

		SidebarActive: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent).
			Background(ColorHighlight).
			PaddingLeft(1),

		SidebarInactive: lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(1),

		// --- Agent panel ---
		AgentContainer: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1),

		AgentHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1),

		AgentTab: lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 1),

		AgentTabActive: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent).
			Underline(true).
			Padding(0, 1),

		AgentOutput: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#E5E7EB"}),

		// --- Event log ---
		EventContainer: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1),

		EventTimestamp: lipgloss.NewStyle().
			Foreground(ColorMuted),

		EventMessage: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#111827", Dark: "#F9FAFB"}),

		// --- Status bar ---
		StatusBar: lipgloss.NewStyle().
			Background(ColorHighlight).
			Foreground(ColorMuted).
			Padding(0, 1),

		StatusKey: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary),

		StatusValue: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#374151", Dark: "#D1D5DB"}),

		StatusSeparator: lipgloss.NewStyle().
			Foreground(ColorSubtle),

		// --- Progress bars ---
		ProgressFilled: lipgloss.NewStyle().
			Foreground(ColorAccent),

		ProgressEmpty: lipgloss.NewStyle().
			Foreground(ColorSubtle),

		ProgressLabel: lipgloss.NewStyle().
			Foreground(ColorMuted),

		ProgressPercent: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent),

		// --- Status indicators ---
		StatusRunning: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent),

		StatusCompleted: lipgloss.NewStyle().
			Foreground(ColorSuccess),

		StatusFailed: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorError),

		StatusWaiting: lipgloss.NewStyle().
			Foreground(ColorWarning),

		StatusBlocked: lipgloss.NewStyle().
			Foreground(ColorMuted),

		// --- General ---
		Border: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorBorder),

		HelpKey: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary),

		HelpDesc: lipgloss.NewStyle().
			Foreground(ColorMuted),

		ErrorText: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorError),

		// --- Dividers ---
		VerticalDivider: lipgloss.NewStyle().
			Foreground(ColorBorder),

		HorizontalDivider: lipgloss.NewStyle().
			Foreground(ColorBorder),
	}
}

// StatusIndicator returns a styled Unicode symbol string for the given
// AgentStatus. The returned string is ready to embed in a view.
//
// Symbol mapping:
//   - AgentIdle        → "○" (open circle, muted)
//   - AgentRunning     → "●" (filled circle, green/accent)
//   - AgentCompleted   → "✓" (check mark, success green)
//   - AgentFailed      → "!" (exclamation, red)
//   - AgentRateLimited → "×" (times/cross, warning)
//   - AgentWaiting     → "◌" (dashed circle, yellow/warning)
func (t Theme) StatusIndicator(status AgentStatus) string {
	switch status {
	case AgentRunning:
		return t.StatusRunning.Render("●")
	case AgentCompleted:
		return t.StatusCompleted.Render("✓")
	case AgentFailed:
		return t.StatusFailed.Render("!")
	case AgentRateLimited:
		return t.StatusWaiting.Render("×")
	case AgentWaiting:
		return t.StatusWaiting.Render("◌")
	default: // AgentIdle and any unknown value
		return t.StatusBlocked.Render("○")
	}
}

// ProgressBar renders a text-based progress bar of the given total width.
// filled is clamped to [0.0, 1.0]; width <= 0 returns an empty string.
// Uses U+2588 (FULL BLOCK) for filled cells and U+2591 (LIGHT SHADE) for
// empty cells. The filled and empty portions are styled independently using
// the theme's ProgressFilled and ProgressEmpty styles.
func (t Theme) ProgressBar(filled float64, width int) string {
	if width <= 0 {
		return ""
	}

	// Clamp filled to [0.0, 1.0].
	if filled < 0.0 {
		filled = 0.0
	}
	if filled > 1.0 {
		filled = 1.0
	}

	filledCount := int(filled * float64(width))
	emptyCount := width - filledCount

	var sb strings.Builder
	if filledCount > 0 {
		sb.WriteString(t.ProgressFilled.Render(strings.Repeat("\u2588", filledCount)))
	}
	if emptyCount > 0 {
		sb.WriteString(t.ProgressEmpty.Render(strings.Repeat("\u2591", emptyCount)))
	}
	return sb.String()
}
