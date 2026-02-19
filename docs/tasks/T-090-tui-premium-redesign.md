# T-090: TUI Premium Visual Redesign & UX Enhancement

## Metadata
| Field | Value |
|-------|-------|
| Priority | Should Have |
| Estimated Effort | Large: 20-30hrs |
| Dependencies | T-068 (styles), T-069 (layout), T-070 (sidebar), T-071 (agent panel), T-072 (event log), T-074 (status bar) |
| Blocked By | None (all Phase 6 TUI tasks complete) |
| Blocks | None |

## Goal
Redesign the Raven TUI from a functional dashboard into a premium, visually distinctive command center. This task covers: a new multi-theme color system with curated palettes (moving beyond generic Tailwind grays), advanced styling with refined borders/typography/spacing, enhanced interactivity with a command palette and contextual overlays, and modernized progress/status visualization. The result should look and feel like a best-in-class terminal application on par with lazygit, k9s, and btop.

## Background

### Current State
The existing TUI (Phase 6, tasks T-066 through T-078) is fully functional with:
- 11 `AdaptiveColor` variables using Tailwind CSS-derived hex codes
- 37-field `Theme` struct with pre-built `lipgloss.Style` values
- Split-pane layout: sidebar (22 cols) + agent panel (65%) + event log (35%) + status bar
- 15 keybindings, focus cycling, help overlay, pipeline wizard
- `NormalBorder()` throughout, basic `U+2588`/`U+2591` progress bars
- ~4,200 lines of production TUI code, ~9,000 lines of tests

### Why Redesign
1. **Generic appearance**: Current colors are Tailwind utility grays/blues -- functional but indistinguishable from any other dashboard
2. **Flat hierarchy**: All panels use the same `NormalBorder()` with identical border colors, making the layout feel monotone
3. **Limited interactivity**: No command palette, no search/filter, no contextual menus, no toast notifications
4. **Basic visualization**: Block-character progress bars and simple Unicode status dots lack visual depth
5. **Single theme**: No user-selectable themes; no way to match user's terminal aesthetic
6. **Missing polish**: No gradient effects, no subtle animations (spinner), no visual breathing room

---

## Research Findings

### 1. State-of-the-Art Terminal Color Schemes

The best TUI applications use curated, intentional color palettes rather than generic utility colors. Here are the top palettes from the terminal ecosystem, each with complete hex codes and design rationale:

#### 1.1 Catppuccin Mocha (Recommended as Default Dark Theme)
Community-driven pastel palette. Warm, cozy, high-readability. 26 named colors. Used by 1000+ applications.

| Role | Name | Hex | Usage in Raven |
|------|------|-----|----------------|
| Background | Base | `#1E1E2E` | Main background |
| Surface | Surface0 | `#313244` | Panel backgrounds |
| Surface | Surface1 | `#45475A` | Elevated surfaces, hover |
| Surface | Surface2 | `#585B70` | Active borders |
| Overlay | Overlay0 | `#6C7086` | Comments, muted text |
| Overlay | Overlay1 | `#7F849C` | Secondary text |
| Overlay | Overlay2 | `#9399B2` | Tertiary text |
| Text | Subtext0 | `#A6ADC8` | Dimmed foreground |
| Text | Subtext1 | `#BAC2DE` | Soft foreground |
| Text | Text | `#CDD6F4` | Primary foreground |
| Accent | Rosewater | `#F5E0DC` | Subtle highlights |
| Accent | Flamingo | `#F2CDCD` | |
| Accent | Pink | `#F5C2E7` | |
| Accent | Mauve | `#CBA6F7` | Keywords, primary accent |
| Accent | Red | `#F38BA8` | Errors |
| Accent | Maroon | `#EBA0AC` | |
| Accent | Peach | `#FAB387` | Warnings |
| Accent | Yellow | `#F9E2AF` | Cautions |
| Accent | Green | `#A6E3A1` | Success |
| Accent | Teal | `#94E2D5` | Active states |
| Accent | Sky | `#89DCEB` | Info |
| Accent | Sapphire | `#74C7EC` | Links |
| Accent | Blue | `#89B4FA` | Functions, secondary |
| Accent | Lavender | `#B4BEFE` | Selections, highlights |

**Why Catppuccin**: Richest named-color palette (26 colors vs. typical 8-16). Warm pastels reduce eye strain. Distinct surface hierarchy (Base/Surface0/Surface1/Surface2) enables visual depth. Massive community adoption means users recognize it.

#### 1.2 Tokyo Night (Storm)
Cool, blue-shifted palette inspired by Tokyo's nighttime cityscape. Popular in Neovim/VS Code.

| Role | Hex | Usage |
|------|-----|-------|
| Background | `#24283B` | Main background (storm variant) |
| Background alt | `#1F2335` | Darker panels |
| Foreground | `#C0CAF5` | Primary text |
| Comment | `#565F89` | Muted/secondary |
| Terminal Black | `#414868` | Borders, surfaces |
| Red | `#F7768E` | Errors |
| Green | `#9ECE6A` | Success |
| Yellow | `#E0AF68` | Warnings |
| Blue | `#7AA2F7` | Primary accent |
| Magenta | `#BB9AF7` | Keywords |
| Cyan | `#7DCFFF` | Info, links |
| Orange | `#FF9E64` | Constants |
| Teal | `#73DACA` | Active states |

**Why Tokyo Night**: Clean, professional aesthetic. Cool tones feel futuristic/technical. High contrast without harshness.

#### 1.3 Kanagawa (Wave)
Inspired by Hokusai's "Great Wave" painting. Warm, muted, uniquely artistic.

| Role | Name | Hex | Usage |
|------|------|-----|-------|
| Background | sumiInk3 | `#1F1F28` | Main background |
| Surface | sumiInk4 | `#2A2A37` | Panel backgrounds |
| Border | sumiInk5 | `#363646` | Subtle borders |
| Muted | fujiGray | `#727169` | Comments, secondary text |
| Foreground | fujiWhite | `#DCD7BA` | Primary text |
| Purple | oniViolet | `#957FB8` | Keywords, primary accent |
| Blue | crystalBlue | `#7E9CD8` | Functions |
| Green | springGreen | `#98BB6C` | Success, strings |
| Yellow | carpYellow | `#E6C384` | Identifiers |
| Red | waveRed | `#E46876` | Errors |
| Orange | surimiOrange | `#FFA066` | Warnings, constants |
| Teal | waveAqua2 | `#7AA89F` | Types, active states |
| Pink | sakuraPink | `#D27E99` | Numbers |

**Why Kanagawa**: Utterly unique aesthetic. Warm, slightly desaturated tones feel refined. Strong cultural identity differentiates from generic themes.

#### 1.4 Rose Pine
"Soho vibes" -- elegant dark theme with muted pinks and golds.

| Role | Hex | Usage |
|------|-----|-------|
| Base | `#191724` | Main background |
| Surface | `#1F1D2E` | Panel backgrounds |
| Overlay | `#26233A` | Elevated surfaces |
| Muted | `#6E6A86` | Secondary text |
| Subtle | `#908CAA` | Tertiary text |
| Text | `#E0DEF4` | Primary text |
| Love (red) | `#EB6F92` | Errors |
| Gold (yellow) | `#F6C177` | Warnings |
| Rose (pink) | `#EBBCBA` | Primary accent |
| Pine (teal) | `#31748F` | Active states |
| Foam (cyan) | `#9CCFD8` | Info |
| Iris (purple) | `#C4A7E7` | Highlights |

**Why Rose Pine**: Sophisticated, understated palette. Gold + rose as accents create a luxury feel. Low saturation reduces eye fatigue.

#### 1.5 Nord
Arctic-inspired, blue-tinted, minimal palette.

| Role | Name | Hex | Usage |
|------|------|-----|-------|
| Background | nord0 | `#2E3440` | Main background |
| Surface | nord1 | `#3B4252` | Panel backgrounds |
| Border | nord2 | `#434C5E` | Borders |
| Muted | nord3 | `#4C566A` | Secondary text |
| Text | nord4 | `#D8DEE9` | Primary text |
| Bright text | nord6 | `#ECEFF4` | Emphasis |
| Teal | nord7 | `#8FBCBB` | Active states |
| Cyan | nord8 | `#88C0D0` | Info, primary accent |
| Blue | nord9 | `#81A1C1` | Functions |
| Dark blue | nord10 | `#5E81AC` | Deep accent |
| Red | nord11 | `#BF616A` | Errors |
| Orange | nord12 | `#D08770` | Warnings |
| Yellow | nord13 | `#EBCB8B` | Cautions |
| Green | nord14 | `#A3BE8C` | Success |
| Purple | nord15 | `#B48EAD` | Keywords |

**Why Nord**: Minimal, calming palette. Professional and understated. Works well with low-contrast workflows.

#### 1.6 Everforest
Green-based, nature-inspired, eye-protective.

| Role | Hex | Usage |
|------|-----|-------|
| Background (medium) | `#2F383E` | Main background |
| Surface | `#374247` | Panel backgrounds |
| Border | `#4A555B` | Borders |
| Muted | `#859289` | Secondary text |
| Text | `#D3C6AA` | Primary text |
| Red | `#E67E80` | Errors |
| Orange | `#E69875` | Warnings |
| Yellow | `#DBBC7F` | Cautions |
| Green | `#A7C080` | Success, primary accent |
| Aqua | `#83C092` | Active states |
| Blue | `#7FBBB3` | Info |
| Purple | `#D699B6` | Keywords |

**Why Everforest**: Unique green-tinted aesthetic. Extremely easy on the eyes. Nature theme stands out from blue/purple dominated schemes.

#### 1.7 Gruvbox
Retro, earthy palette with high contrast.

| Role | Hex (Dark) | Usage |
|------|-----------|-------|
| Background | `#282828` | Main background |
| Surface | `#3C3836` | Panel backgrounds |
| Border | `#504945` | Borders |
| Muted | `#928374` | Secondary text |
| Text | `#EBDBB2` | Primary text |
| Red | `#FB4934` | Errors |
| Orange | `#FE8019` | Warnings |
| Yellow | `#FABD2F` | Cautions |
| Green | `#B8BB26` | Success |
| Aqua | `#8EC07C` | Active states |
| Blue | `#83A598` | Info |
| Purple | `#D3869B` | Keywords |

**Why Gruvbox**: Bold, retro personality. High saturation creates energetic feel. Strong community following.

### 2. Color Design Principles for Premium TUIs

#### 2.1 Surface Hierarchy (Depth Through Background Variation)
The key differentiator between "flat" and "premium" TUIs is **surface depth**. Instead of uniform backgrounds, use 3-4 levels:

```
Level 0 (deepest):  Base background        e.g., #1E1E2E
Level 1 (panels):   Surface background     e.g., #313244  (+1 shade lighter)
Level 2 (elevated): Hover/selected state   e.g., #45475A  (+2 shades lighter)
Level 3 (overlay):  Modal/popup/tooltip    e.g., #585B70  (+3 shades lighter)
```

This creates the illusion of layered surfaces -- panels appear to "float" above the base, modals appear above panels.

**Implementation**: Apply different background colors to:
- App background (Level 0)
- Sidebar container, Agent panel, Event log (Level 1)
- Selected sidebar item, active tab (Level 2)
- Help overlay, command palette, wizard (Level 3)

#### 2.2 Text Hierarchy (4 Levels of Emphasis)
Premium TUIs use 4+ text color levels, not just "text" and "muted":

| Level | Purpose | Example Colors (Catppuccin) |
|-------|---------|---------------------------|
| Primary | Headings, active items | Text `#CDD6F4` |
| Secondary | Body text, descriptions | Subtext1 `#BAC2DE` |
| Tertiary | Timestamps, metadata | Subtext0 `#A6ADC8` or Overlay2 `#9399B2` |
| Muted | Comments, disabled | Overlay0 `#6C7086` |

#### 2.3 Accent Color Strategy
Best-in-class TUIs use one dominant accent + semantic colors:

- **Primary accent**: Used for active borders, focused panel highlights, selected items. Should be distinctive (Mauve, Teal, or Rose -- NOT blue, which blends with too many terminals)
- **Semantic colors**: Red/Green/Yellow/Blue for status only
- **Avoid**: Using the same blue for both "primary" and "info" (current Raven issue with `ColorSecondary` and `ColorInfo` sharing `#60A5FA`)

#### 2.4 Border as Visual Storytelling
Borders should communicate focus state, not just divide space:

| State | Border Style | Color |
|-------|-------------|-------|
| Unfocused panel | `NormalBorder()` | Surface2 (subtle, almost invisible) |
| Focused panel | `RoundedBorder()` | Primary accent color |
| Active/running | `RoundedBorder()` | Accent/teal (pulsing if possible) |
| Error state | `NormalBorder()` or `ThickBorder()` | Red accent |

### 3. Premium Styling Techniques

#### 3.1 Border Styles
Lipgloss v1.0+ provides these border styles:

| Style | Look | Use For |
|-------|------|---------|
| `NormalBorder()` | `│ ─ ┌ ┐ └ ┘` | Standard panels |
| `RoundedBorder()` | `│ ─ ╭ ╮ ╰ ╯` | Focused panels, modals, overlays |
| `ThickBorder()` | `┃ ━ ┏ ┓ ┗ ┛` | Error states, critical alerts |
| `DoubleBorder()` | `║ ═ ╔ ╗ ╚ ╝` | Title bars, primary containers |
| `HiddenBorder()` | (blank space) | Padding without visible borders |
| Custom `lipgloss.Border{}` | Any Unicode | Brand-specific borders |

**Premium technique**: Use `RoundedBorder()` as default (softer, more modern feel) and `NormalBorder()` for unfocused panels. The rounded corners alone make a significant visual difference.

#### 3.2 Custom Border Characters
Create unique borders using Unicode box-drawing extensions:

```go
// Example: dotted border for "thinking" states
DottedBorder := lipgloss.Border{
    Top:         "┄",
    Bottom:      "┄",
    Left:        "┊",
    Right:       "┊",
    TopLeft:     "╭",
    TopRight:    "╮",
    BottomLeft:  "╰",
    BottomRight: "╯",
}
```

#### 3.3 Typography Combinations
Terminal styling supports: **Bold**, *Italic*, ~~Strikethrough~~, Underline, Dim, Reverse, Blink.

Premium combinations:
- **Title**: Bold + Primary accent foreground
- **Section header**: Bold + Secondary foreground + Underline
- **Metadata/timestamp**: Dim + Muted foreground
- **De-emphasized**: Italic + Muted (use sparingly; not all terminals support italic)
- **Error emphasis**: Bold + Red + (optionally Reverse for critical)
- **Keyboard shortcuts**: `Reverse` or `Background(accent)` for keycap appearance

#### 3.4 Spacing & Whitespace
The biggest "free" improvement. Current Raven uses minimal padding.

Recommended spacing changes:
- **Panel padding**: `Padding(1, 2)` instead of `Padding(0, 1)` -- more breathing room
- **Section separators**: Use styled horizontal dividers between sidebar sections (not just newlines)
- **Title bar**: `Padding(0, 2)` with `MarginBottom(0)` -- wider padding feels more premium
- **Status bar**: `Padding(0, 2)` to match title bar
- **Between items**: Use `MarginBottom(1)` between list items for clarity

#### 3.5 Status Indicators (Enhanced)
Replace simple Unicode dots with richer indicators:

| Status | Current | Premium Option | Unicode |
|--------|---------|---------------|---------|
| Running | `●` | Spinner animation (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`) | Braille dots |
| Completed | `✓` | `✔` or ` ` (Nerd Font) | U+2714 |
| Failed | `!` | `✘` or `󰅙` | U+2718 |
| Waiting | `◌` | `⏳` or animated dots `...` | U+23F3 |
| Paused | (none) | `⏸` or `▐▐` | U+23F8 |
| Idle | `○` | `◯` or `─` | U+25EF |

**Spinner animation**: Bubble Tea's `spinner` component from `charmbracelet/bubbles` provides beautiful animated spinners. Use `spinner.Dot`, `spinner.MiniDot`, `spinner.Line`, or `spinner.Pulse`.

#### 3.6 Progress Bar Enhancements
Replace basic block/shade with richer options:

**Option A: Gradient Progress Bar**
```
Progress: ████████░░░░░░░░░░░░ 42%
          ^green   ^dimmed
```
Use different foreground colors for the filled portion based on percentage:
- 0-33%: Red/Orange
- 34-66%: Yellow
- 67-100%: Green

**Option B: Segmented Progress Bar**
```
Progress: [■■■■■□□□□□] 5/10
```
Each segment represents one task. Completed segments are filled, remaining are empty.

**Option C: Braille Sparkline**
```
Activity: ⣿⣶⣤⣀⡀⢀⠠⠐⠈
```
Braille patterns (U+2800-U+28FF) can create high-resolution mini-graphs showing activity over time in just one row of characters.

**Option D: Unicode Block Elements**
```
Progress: ▏▎▍▌▋▊▉█░░░░ 58%
```
Use the 8 sub-block characters (U+2589-U+258F) for sub-character precision.

#### 3.7 Styled Badges & Tags
Use background + foreground + padding for inline badges:

```go
// Mode badge
modeBadge := lipgloss.NewStyle().
    Bold(true).
    Foreground(lipgloss.Color("#1E1E2E")).  // dark text
    Background(lipgloss.Color("#A6E3A1")).  // green bg
    Padding(0, 1).
    Render("IMPLEMENT")

// Agent tag
agentTag := lipgloss.NewStyle().
    Foreground(lipgloss.Color("#89B4FA")).
    Background(lipgloss.Color("#313244")).
    Padding(0, 1).
    Render("claude")
```

### 4. UX & Interactivity Enhancements

#### 4.1 Command Palette (Ctrl+P / Ctrl+K)
A searchable overlay listing all available actions. This is the single most impactful UX addition.

**Design (reference: OpenCode, VS Code)**:
```
╭─────────────────────────────────────╮
│ > search commands...                │
├─────────────────────────────────────┤
│   Pause Workflow              p     │
│   Skip Current Task           s     │
│   Toggle Event Log            l     │
│   Switch Agent Tab          Tab     │
│   Filter Events...                  │
│   Export Logs...                    │
│   Change Theme...                   │
│   Resize Panels...                  │
╰─────────────────────────────────────╯
```

**Implementation approach**:
- New `CommandPaletteModel` sub-model
- Registered commands with title, description, keybinding, callback
- Fuzzy search via `textinput` from `charmbracelet/bubbles`
- Overlay rendered with `lipgloss.Place()` (same pattern as HelpOverlay)
- Categories: Navigation, Actions, View, Settings

#### 4.2 Search/Filter in Panels
Allow `/` to enter search mode within focused panel:
- **Event Log**: Filter events by text, category (info/warn/error)
- **Agent Panel**: Search output text, highlight matches
- **Sidebar**: Filter workflows by name/status

**Implementation**: Add `filterMode bool` and `filterInput textinput.Model` to each panel model. When active, filter the viewport content.

#### 4.3 Toast Notifications
Temporary messages that appear and auto-dismiss (2-3 seconds):
- "Workflow paused" / "Workflow resumed"
- "Rate limit cleared for Claude"
- "Task T-042 completed"

**Implementation**:
```go
type ToastModel struct {
    message   string
    style     lipgloss.Style  // success/warning/error/info variant
    timer     time.Time
    duration  time.Duration
    visible   bool
}
```
Rendered as a floating box in the bottom-right corner using `lipgloss.Place()`.

#### 4.4 Contextual Status Line
Enhance the status bar to show context-aware information:
- When hovering a workflow: show its step count, duration, last error
- When in agent panel: show output line count, scroll position
- When rate limited: show countdown prominently with color coding

#### 4.5 Panel Resize (Mouse/Keyboard)
Allow users to resize the sidebar width and agent/event log split:
- `Ctrl+Left/Right`: Adjust sidebar width (18-40 cols)
- `Ctrl+Up/Down`: Adjust agent/event log split (30-80%)
- Persist last sizes in `.raven/tui-prefs.json`

#### 4.6 Mini-Map / Breadcrumb Navigation
Show current position in workflow hierarchy:
```
Pipeline > Phase 2/5 > implement > Task T-042 > Iteration 3/5
```
Rendered as a breadcrumb trail in the title bar or just below it. Each segment is clickable (with mouse) or navigable.

#### 4.7 Keyboard Shortcut Discoverability
Replace the static help overlay with a contextual approach:
- Show relevant shortcuts inline at the bottom of each panel
- Context-sensitive: different shortcuts shown based on focused panel
- Dim/muted styling so they don't distract

```
─────────────────────────────────────
 j/k navigate  Enter expand  p pause  ? all shortcuts
```

### 5. Theme System Architecture

#### 5.1 Multi-Theme Support
Replace the single `DefaultTheme()` with a registry:

```go
// ThemeID identifies a built-in or custom theme.
type ThemeID string

const (
    ThemeCatppuccinMocha ThemeID = "catppuccin-mocha"
    ThemeTokyoNight      ThemeID = "tokyo-night"
    ThemeKanagawa        ThemeID = "kanagawa"
    ThemeRosePine        ThemeID = "rose-pine"
    ThemeNord            ThemeID = "nord"
    ThemeEverforest      ThemeID = "everforest"
    ThemeGruvbox         ThemeID = "gruvbox"
)

// ThemeRegistry maps ThemeID to Theme constructors.
var ThemeRegistry = map[ThemeID]func() Theme{
    ThemeCatppuccinMocha: NewCatppuccinMochaTheme,
    ThemeTokyoNight:      NewTokyoNightTheme,
    ThemeKanagawa:        NewKanagawaTheme,
    ThemeRosePine:        NewRosePineTheme,
    ThemeNord:            NewNordTheme,
    ThemeEverforest:      NewEverforestTheme,
    ThemeGruvbox:         NewGruvboxTheme,
}
```

#### 5.2 Theme Structure (Enhanced)
Extend the `Theme` struct with surface hierarchy and semantic slots:

```go
type Theme struct {
    // Identity
    ID          ThemeID
    Name        string
    Description string

    // Surface hierarchy (the key to "premium" feel)
    Base     lipgloss.Color  // Deepest background
    Surface0 lipgloss.Color  // Panel backgrounds
    Surface1 lipgloss.Color  // Elevated surfaces (hover, selected)
    Surface2 lipgloss.Color  // Highest elevation (modals, overlays)

    // Text hierarchy
    TextPrimary   lipgloss.Color  // Headings, active items
    TextSecondary lipgloss.Color  // Body text
    TextTertiary  lipgloss.Color  // Metadata, timestamps
    TextMuted     lipgloss.Color  // Disabled, comments

    // Accent colors
    AccentPrimary   lipgloss.Color  // Main brand/accent (for borders, selections)
    AccentSecondary lipgloss.Color  // Secondary interactive elements

    // Semantic colors
    Success lipgloss.Color
    Warning lipgloss.Color
    Error   lipgloss.Color
    Info    lipgloss.Color

    // All existing component styles remain but are built from above colors
    // ...existing 37 style fields...
}
```

#### 5.3 Configuration
Add theme selection to `raven.toml`:

```toml
[tui]
theme = "catppuccin-mocha"  # or "tokyo-night", "kanagawa", etc.
```

And to the `--theme` CLI flag on `raven dashboard`:
```
raven dashboard --theme=tokyo-night
```

#### 5.4 Light Theme Support
Each dark theme should have a light counterpart using `lipgloss.AdaptiveColor`:
- Catppuccin Mocha (dark) / Catppuccin Latte (light)
- Tokyo Night (dark) / Tokyo Night Day (light)
- Rose Pine (dark) / Rose Pine Dawn (light)
- Nord (dark / light handled by same palette)
- Everforest Dark / Everforest Light

The existing `AdaptiveColor` pattern makes this straightforward -- each color slot gets both light and dark variants.

### 6. Reference TUI Applications Analysis

#### 6.1 lazygit
- **Colors**: Uses a custom theme system with configurable colors per element
- **Borders**: `RoundedBorder()` for all panels; focused panel border turns bright
- **Status**: Colored dots and text badges
- **UX**: Excellent keyboard navigation, contextual help at bottom, popup confirmations
- **Key takeaway**: Focus-aware border coloring is crucial for panel navigation

#### 6.2 k9s (Kubernetes TUI)
- **Colors**: Themed "skins" system with many community themes
- **Layout**: Header bar + tabbed main content + footer with shortcuts
- **Status**: Rich colored badges for pod status (Running=green, Pending=yellow, etc.)
- **UX**: Command mode (`:` prefix), search (`/`), filter, sort by column
- **Key takeaway**: Command mode + search is essential for power users

#### 6.3 btop / bottom (System Monitors)
- **Colors**: Gradient color ramps for CPU/memory bars (green -> yellow -> red)
- **Visualization**: Braille-dot sparkline graphs, colored segmented bars
- **Layout**: Dense multi-panel with thin borders
- **Key takeaway**: Gradient progress bars and sparklines add visual richness

#### 6.4 superfile (File Manager)
- **Colors**: Full Catppuccin theme with surface hierarchy
- **Borders**: Rounded everywhere, accent-colored on focus
- **Styling**: Heavy use of padding, clean typography hierarchy
- **Key takeaway**: Generous padding + surface hierarchy = premium feel

#### 6.5 OpenCode (AI Coding Agent)
- **Colors**: JSON-based custom theme system with dark/light adaptive support
- **Command palette**: `Ctrl+P` opens searchable command list
- **Theming**: System theme auto-detection, custom theme via config
- **Key takeaway**: Command palette is a must-have for AI tool TUIs

### 7. Implementation Recommendations (Prioritized)

#### Phase A: Visual Foundation (High Impact, Moderate Effort)
These changes dramatically improve appearance with relatively contained code changes:

1. **Switch to `RoundedBorder()` globally** -- single line change per panel, huge visual improvement
2. **Implement surface hierarchy** -- add 4 background levels to Theme struct
3. **Focus-aware border coloring** -- focused panel gets accent-colored rounded border
4. **Add Catppuccin Mocha as default theme** -- replace Tailwind grays with curated palette
5. **Increase padding** -- `Padding(1, 2)` on panels for breathing room
6. **4-level text hierarchy** -- replace 2-level (text/muted) with 4 levels
7. **Styled badges** -- mode, agent name, status as background-colored badges in status bar

#### Phase B: Enhanced Visualization (Medium Impact, Low-Medium Effort)
8. **Spinner for running status** -- integrate `bubbles/spinner` for active agents
9. **Gradient progress bars** -- color changes based on completion percentage
10. **Braille sparkline** -- activity graph in sidebar showing agent output rate
11. **Better section dividers** -- styled horizontal rules between sidebar sections
12. **Breadcrumb navigation** -- show pipeline > phase > task > iteration path

#### Phase C: Interactivity (High Impact, Higher Effort)
13. **Command palette** -- new `CommandPaletteModel` with fuzzy search
14. **Panel search/filter** -- `/` to search within focused panel
15. **Toast notifications** -- auto-dismissing status messages
16. **Contextual shortcut hints** -- panel-aware shortcut bar at bottom
17. **Panel resize** -- keyboard-driven sidebar/split adjustment

#### Phase D: Theme System (Medium Impact, Medium Effort)
18. **Theme registry** -- `ThemeRegistry` map with all 7 themes
19. **Theme config** -- `[tui] theme = "..."` in raven.toml
20. **CLI flag** -- `--theme` on dashboard command
21. **Theme switcher** -- runtime theme switching (command palette action)
22. **Light theme variants** -- Catppuccin Latte, Tokyo Night Day, Rose Pine Dawn

---

## Technical Specifications

### Files to Modify
| File | Changes |
|------|---------|
| `internal/tui/styles.go` | New Theme struct with surface/text hierarchy, theme constructors for all 7 palettes, ThemeRegistry, enhanced StatusIndicator/ProgressBar |
| `internal/tui/themes/` | New directory: one file per theme (catppuccin.go, tokyo_night.go, kanagawa.go, etc.) |
| `internal/tui/app.go` | Theme selection from config, pass to sub-models, command palette integration |
| `internal/tui/layout.go` | Surface-level backgrounds, focus-aware border rendering |
| `internal/tui/sidebar.go` | Surface hierarchy, section dividers, sparkline, spinner integration |
| `internal/tui/agent_panel.go` | Focus-aware borders, styled tabs with backgrounds, output search |
| `internal/tui/event_log.go` | Surface background, search/filter mode, styled category badges |
| `internal/tui/status_bar.go` | Styled mode badges, breadcrumb, contextual hints |
| `internal/tui/keybindings.go` | New keybindings for command palette, search, panel resize |
| `internal/tui/command_palette.go` | New file: CommandPaletteModel |
| `internal/tui/toast.go` | New file: ToastModel |
| `internal/config/config.go` | Add `[tui] theme` config field |
| `internal/cli/dashboard.go` | Add `--theme` flag |

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/lipgloss | v1.0+ | Styling (already present) |
| charmbracelet/bubbles | latest | spinner, textinput (already present) |
| charmbracelet/bubbletea | v1.2+ | TUI framework (already present) |
| sahilm/fuzzy | latest | Fuzzy search for command palette (new) |

### API/Interface Contracts

```go
// Theme selection
func ThemeForID(id ThemeID) (Theme, error)
func ListThemes() []ThemeID

// Command palette
type Command struct {
    ID          string
    Title       string
    Description string
    Category    string
    Keybinding  string
    Action      func() tea.Cmd
}

type CommandPaletteModel struct { /* ... */ }
func NewCommandPalette(commands []Command, theme Theme) CommandPaletteModel
func (m CommandPaletteModel) Update(msg tea.Msg) (CommandPaletteModel, tea.Cmd)
func (m CommandPaletteModel) View() string

// Toast
type ToastModel struct { /* ... */ }
func NewToast(message string, level ToastLevel, duration time.Duration) ToastModel

// Enhanced progress
func (t Theme) GradientProgressBar(filled float64, width int) string
func (t Theme) SparkLine(data []float64, width int) string
```

## Acceptance Criteria

### Phase A (Visual Foundation)
- [ ] All panels use `RoundedBorder()` with focus-aware accent coloring
- [ ] Theme struct includes surface hierarchy (Base, Surface0, Surface1, Surface2)
- [ ] Theme struct includes 4-level text hierarchy (Primary, Secondary, Tertiary, Muted)
- [ ] Catppuccin Mocha theme is the default dark theme
- [ ] Status bar uses styled background-colored badges for mode/agent
- [ ] Panel padding increased to `Padding(1, 2)` for breathing room
- [ ] Visual spot-check: TUI looks distinctly premium compared to current version

### Phase B (Enhanced Visualization)
- [ ] Running agents show animated spinner instead of static `●`
- [ ] Progress bars use gradient coloring (red -> yellow -> green)
- [ ] Sidebar sections separated by styled horizontal dividers
- [ ] Breadcrumb navigation shows current pipeline position

### Phase C (Interactivity)
- [ ] `Ctrl+K` or `Ctrl+P` opens command palette with fuzzy search
- [ ] `/` enters search mode within focused panel
- [ ] Toast notifications appear for key state changes
- [ ] Bottom of each panel shows context-aware keyboard hints

### Phase D (Theme System)
- [ ] All 7 themes (Catppuccin, Tokyo Night, Kanagawa, Rose Pine, Nord, Everforest, Gruvbox) implemented
- [ ] `[tui] theme = "..."` config option works
- [ ] `--theme` flag on `raven dashboard` works
- [ ] Each theme passes visual spot-check in both dark and light terminals

## Testing Requirements

### Unit Tests
- Each theme constructor returns a Theme with no zero-value style fields
- ThemeRegistry contains all expected ThemeIDs
- `ThemeForID` returns error for unknown theme
- `GradientProgressBar` returns correct colors at 0%, 33%, 66%, 100%
- `SparkLine` handles empty data, single point, full width
- `CommandPaletteModel` filters commands correctly with fuzzy search
- `ToastModel` auto-dismisses after duration
- Focus-aware border color changes on `FocusChangedMsg`

### Integration Tests
- Render each theme's full TUI to string; verify non-empty, valid ANSI output
- Command palette: open, search, select, dismiss sequence
- Toast: show, wait, auto-dismiss sequence
- Theme switching: verify all components re-render with new colors

### Edge Cases
- Terminal with no TrueColor support (degrade gracefully to ANSI256)
- Very narrow terminals (< 80 cols): badges truncate cleanly
- Rapid theme switching doesn't cause flickering
- Command palette with 0 matching results shows "No results" message
- Sparkline with all-zero data renders flat line

## Implementation Notes

### Recommended Approach
1. Start with Phase A (visual foundation) -- this gives the biggest bang for effort
2. Implement one theme fully (Catppuccin Mocha), then template the rest
3. Build command palette before panel search (it's more impactful)
4. Add toast notifications alongside command palette (they share overlay rendering)
5. Theme registry last (requires all themes to be implemented)

### Potential Pitfalls
- **lipgloss.Color vs lipgloss.AdaptiveColor**: Theme colors for light/dark need AdaptiveColor when both terminal backgrounds must be supported. If only targeting dark terminals initially, can use plain `lipgloss.Color` and add adaptive later.
- **Spinner goroutine**: bubbles/spinner sends tick messages. Must be properly initialized and cleaned up.
- **Fuzzy search dependency**: `sahilm/fuzzy` is a small dependency. Alternatively, implement simple substring matching to avoid the dep.
- **Test stability**: Tests that depend on exact ANSI output will break with theme changes. Use semantic assertions (non-empty, contains expected text) rather than golden file matching for theme-dependent output.
- **Performance**: Rendering 7 themes' worth of styles at startup is negligible. Command palette filtering should be < 1ms for < 100 commands.

### Security Considerations
- No security considerations for pure TUI styling and theming code
- Custom themes from config files should validate hex color format to prevent injection via malformed TOML values

## References
- [Catppuccin Palette](https://catppuccin.com/palette/) -- Full 26-color palette with design rationale
- [Dracula Spec](https://draculatheme.com/spec) -- Color specification standard
- [Tokyo Night VSCode Theme](https://github.com/tokyo-night/tokyo-night-vscode-theme) -- Color definitions
- [Kanagawa.nvim colors](https://github.com/rebelot/kanagawa.nvim/blob/master/lua/kanagawa/colors.lua) -- Full palette source
- [Rose Pine Palette](https://rosepinetheme.com/palette/) -- Official color definitions
- [Nord Colors](https://www.nordtheme.com/docs/colors-and-palettes/) -- Official palette documentation
- [Everforest Palette](https://github.com/sainnhe/everforest/blob/master/palette.md) -- Full palette with contrast levels
- [Lipgloss GitHub](https://github.com/charmbracelet/lipgloss) -- Styling library
- [Bubbles Spinner](https://github.com/charmbracelet/bubbles/tree/master/spinner) -- Animated spinners
- [OpenCode TUI Theming](https://deepwiki.com/sst/opencode/6.4-tui-theming-keybinds-and-commands) -- Theme system reference
- [Awesome TUIs](https://github.com/rothgar/awesome-tuis) -- Curated list of terminal UIs
- [Nerd Fonts](https://www.nerdfonts.com) -- Icon font for terminal applications
