# T-072: Sidebar -- Rate-Limit Status Display with Countdown

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-8hrs |
| Dependencies | T-067, T-068, T-069, T-070 |
| Blocked By | T-070 |
| Blocks | T-078 |

## Goal
Implement the rate-limit status section of the sidebar panel, displaying per-provider rate-limit status with live countdown timers. When an agent hits a rate limit, the sidebar shows the provider name, a "WAIT" status with remaining seconds, and a countdown that decrements every second via `TickMsg`. When the rate limit expires, the status returns to "OK".

## Background
Per PRD Section 5.12, the sidebar displays:
```
Rate Limits
claude: OK
codex: WAIT 1:42
```

Rate-limit events originate from the agent adapters (Phase 2 tasks) and flow to the TUI via `RateLimitMsg` (T-067). The TUI maintains per-provider state tracking when each provider's rate limit resets. A periodic `TickMsg` (T-067) drives the countdown display, decrementing the remaining time and transitioning from "WAIT" to "OK" when the time expires.

The rate-limit display uses distinct colors: green for "OK", yellow/orange for "WAIT" with the countdown timer, and red if the agent encountered an error.

## Technical Specifications
### Implementation Approach
Add a `RateLimitSection` to the sidebar model in `internal/tui/sidebar.go` (extending the file from T-070/T-071). This section maintains a map of provider names to their rate-limit state. On `RateLimitMsg`, it stores the reset time. On `TickMsg`, it recalculates remaining time for each provider and clears expired limits. The view renders each provider with its current status.

### Key Components
- **RateLimitSection**: Manages per-provider rate-limit display state
- **ProviderRateLimit**: State for a single provider's rate limit (reset time, agent name)
- **Countdown formatting**: Converts remaining duration to "M:SS" display format

### API/Interface Contracts
```go
// internal/tui/sidebar.go (extension)

// ProviderRateLimit tracks rate-limit state for a single provider.
type ProviderRateLimit struct {
    Provider  string
    Agent     string        // Agent that triggered the limit
    ResetAt   time.Time     // When the rate limit expires
    Remaining time.Duration // Calculated on each tick
    Active    bool          // True if currently rate-limited
}

// RateLimitSection manages rate-limit status display in the sidebar.
type RateLimitSection struct {
    theme     Theme
    providers map[string]*ProviderRateLimit // keyed by provider name
    order     []string                      // stable display order
}

// NewRateLimitSection creates a rate-limit section with the given theme.
func NewRateLimitSection(theme Theme) RateLimitSection

// Update processes RateLimitMsg and TickMsg to update rate-limit state.
func (rl RateLimitSection) Update(msg tea.Msg) (RateLimitSection, tea.Cmd)

// View renders the rate-limit section as a string.
func (rl RateLimitSection) View(width int) string

// HasActiveLimit returns true if any provider is currently rate-limited.
func (rl RateLimitSection) HasActiveLimit() bool

// formatCountdown converts a duration to "M:SS" or "H:MM:SS" format.
func formatCountdown(d time.Duration) string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/bubbletea | v1.2+ | tea.Msg, tea.Cmd for tick scheduling |
| charmbracelet/lipgloss | v1.0+ | Styling OK/WAIT status |
| internal/tui (T-067) | - | RateLimitMsg, TickMsg, TickCmd |
| internal/tui (T-068) | - | Theme status styles |

## Acceptance Criteria
- [ ] "Rate Limits" section header displayed
- [ ] Each known provider shows on a separate line: `{name}: {status}`
- [ ] Providers with no active rate limit show "OK" in green
- [ ] Providers with active rate limit show "WAIT M:SS" in yellow/orange
- [ ] Countdown decrements every second via TickMsg
- [ ] When countdown reaches zero, provider status transitions to "OK"
- [ ] `RateLimitMsg` adds new providers or updates existing ones
- [ ] Provider display order is stable (sorted alphabetically or insertion order)
- [ ] Countdown format: "M:SS" for under 1 hour, "H:MM:SS" for 1+ hours
- [ ] `HasActiveLimit()` returns true when any provider is rate-limited
- [ ] The section requests a TickCmd when any countdown is active (to drive updates)
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- `NewRateLimitSection` creates section with empty provider map
- `Update` with `RateLimitMsg{Provider: "anthropic", ResetAfter: 120s}` adds provider with active limit
- `Update` with `TickMsg` decrements remaining time
- Provider transitions to "OK" when remaining reaches zero
- `formatCountdown(90 * time.Second)` returns "1:30"
- `formatCountdown(5 * time.Second)` returns "0:05"
- `formatCountdown(3661 * time.Second)` returns "1:01:01"
- `formatCountdown(0)` returns "0:00"
- `HasActiveLimit` returns false when no providers are limited
- `HasActiveLimit` returns true when at least one provider is limited
- `View(20)` with no providers shows empty section with header only
- `View(20)` with one OK and one WAIT provider renders correctly
- Second `RateLimitMsg` for same provider updates the reset time

### Integration Tests
- Simulate rate-limit lifecycle: receive RateLimitMsg, tick down to zero, verify transition to OK

### Edge Cases to Handle
- Rate limit with zero duration (immediately expires -- show "OK")
- Rate limit with very large duration (24+ hours -- still formats correctly)
- Multiple rate limits for same provider (latest one wins)
- TickMsg when no providers are rate-limited (no-op, do not request another tick)
- Negative remaining time after tick (clamp to zero and clear)
- Provider name longer than sidebar width (truncate)

## Implementation Notes
### Recommended Approach
1. Define `ProviderRateLimit` struct and `RateLimitSection` with map + order slice
2. Implement `Update`:
   - `RateLimitMsg`:
     - Look up provider in map; create new entry if not found, append to order
     - Set `ResetAt = time.Now().Add(msg.ResetAfter)`, `Active = true`
     - Return a `TickCmd(1 * time.Second)` to start the countdown
   - `TickMsg`:
     - For each active provider, recalculate `Remaining = time.Until(ResetAt)`
     - If `Remaining <= 0`, set `Active = false`
     - If any provider still active, return another `TickCmd(1 * time.Second)`
3. Implement `View(width)`:
   - Render "Rate Limits" header
   - For each provider in order:
     - If active: `theme.StatusWaiting.Render(provider + ": WAIT " + formatCountdown(remaining))`
     - If not active: `theme.StatusCompleted.Render(provider + ": OK")`
   - Truncate each line to width
4. Implement `formatCountdown`:
   - If duration < 0, return "0:00"
   - If hours > 0: `fmt.Sprintf("%d:%02d:%02d", hours, mins, secs)`
   - Otherwise: `fmt.Sprintf("%d:%02d", mins, secs)`
5. Integrate into `SidebarModel.View()` as the bottom section

### Potential Pitfalls
- The TickMsg must be requested by returning a `tea.Cmd` from `Update`. If you forget to return the tick command, the countdown will freeze. The pattern is: if any provider is active after update, return `TickCmd(1*time.Second)`.
- Multiple sections might independently request TickMsg. That is fine -- Bubble Tea deduplicates identical commands, and even if it doesn't, the Update handler for TickMsg is idempotent.
- Use `time.Until(ResetAt)` for remaining time calculation rather than decrementing a stored duration. This handles the case where tick intervals are not exactly 1 second.
- The `order` slice must be maintained to prevent map iteration randomization from causing UI flicker.
- If the parent App already has a tick loop running, coordinate to avoid multiple independent tick chains. A single application-wide tick every second is more efficient.

### Security Considerations
- Rate-limit reset times could reveal API usage patterns. This is acceptable for a local-only TUI.

## References
- [Bubble Tea Timer Example](https://github.com/charmbracelet/bubbletea/blob/main/examples/timer/main.go)
- [Bubble Tea tea.Tick Documentation](https://pkg.go.dev/github.com/charmbracelet/bubbletea#Tick)
- [PRD Section 5.12 - Rate-Limit Status Display](docs/prd/PRD-Raven.md)
- [PRD Section 5.4 - Rate-Limit Detection and Recovery](docs/prd/PRD-Raven.md)