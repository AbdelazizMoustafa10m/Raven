package tui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// benchWidth and benchHeight are the terminal dimensions used for all TUI
// rendering benchmarks. 120x40 exceeds the minimum required dimensions
// (80x24) and matches the performance target from the PRD.
const benchWidth = 120
const benchHeight = 40

// buildReadyApp constructs an App and initialises it with a WindowSizeMsg so
// that View() renders the full layout instead of "Initializing Raven...".
// The resulting App is ready for benchmarking.
func buildReadyApp(b *testing.B) App {
	b.Helper()
	app := NewApp(AppConfig{
		Version:     "1.0.0",
		ProjectName: "bench-project",
	})
	model, _ := app.Update(tea.WindowSizeMsg{Width: benchWidth, Height: benchHeight})
	ready, ok := model.(App)
	if !ok {
		b.Fatal("Update(WindowSizeMsg) did not return an App")
	}
	return ready
}

// BenchmarkAppView measures App.View() rendering at 120x40 — the reference
// terminal size for the <100ms per-frame target from the PRD.
func BenchmarkAppView(b *testing.B) {
	app := buildReadyApp(b)
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = app.View()
	}
}

// BenchmarkAppViewWithEvents measures App.View() after 50 event log entries
// have been added, which adds scrollable content to the event log panel.
func BenchmarkAppViewWithEvents(b *testing.B) {
	app := buildReadyApp(b)
	for i := 0; i < 50; i++ {
		cat := EventCategory(i % 5)
		app.eventLog.AddEntry(cat, fmt.Sprintf("benchmark event log entry number %d", i))
	}
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = app.View()
	}
}

// BenchmarkAppUpdateWindowSize measures the cost of processing a WindowSizeMsg,
// which triggers layout recalculation and sub-model dimension updates.
func BenchmarkAppUpdateWindowSize(b *testing.B) {
	app := NewApp(AppConfig{Version: "1.0.0", ProjectName: "bench-project"})
	msg := tea.WindowSizeMsg{Width: benchWidth, Height: benchHeight}
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = app.Update(msg)
	}
}

// BenchmarkAppUpdateAgentOutput measures the throughput of dispatching
// AgentOutputMsg messages to the App's Update method.
func BenchmarkAppUpdateAgentOutput(b *testing.B) {
	app := buildReadyApp(b)
	msg := AgentOutputMsg{
		Agent:     "claude",
		Line:      "Implementing T-083: performance benchmarking suite",
		Stream:    "stdout",
		Timestamp: time.Now(),
	}
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = app.Update(msg)
	}
}

// BenchmarkAppUpdateWorkflowEvent measures the throughput of dispatching
// WorkflowEventMsg messages, which update the sidebar and event log.
func BenchmarkAppUpdateWorkflowEvent(b *testing.B) {
	app := buildReadyApp(b)
	msg := WorkflowEventMsg{
		WorkflowID:   "wf-bench-001",
		WorkflowName: "implement-review-pr",
		Step:         "review",
		PrevStep:     "implement",
		Event:        "success",
		Detail:       "implementation complete",
		Timestamp:    time.Now(),
	}
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = app.Update(msg)
	}
}

// BenchmarkEventLogAddEntry measures the throughput of adding event entries to
// the EventLogModel ring buffer.
func BenchmarkEventLogAddEntry(b *testing.B) {
	theme := DefaultTheme()
	el := NewEventLogModel(theme)
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		el.AddEntry(EventInfo, "benchmark event entry")
	}
}

// BenchmarkEventLogAddEntryRingBuffer measures AddEntry throughput when the
// ring buffer is full (500 entries), exercising the eviction path.
func BenchmarkEventLogAddEntryRingBuffer(b *testing.B) {
	theme := DefaultTheme()
	el := NewEventLogModel(theme)
	// Fill the buffer to capacity.
	for i := 0; i < MaxEventLogEntries; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("entry %d", i))
	}
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		el.AddEntry(EventWarning, "overflow entry")
	}
}

// BenchmarkLayoutResize measures the cost of Layout.Resize at 120x40,
// which recalculates panel dimensions on every terminal resize event.
func BenchmarkLayoutResize(b *testing.B) {
	layout := NewLayout()
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		layout.Resize(benchWidth, benchHeight)
	}
}

// BenchmarkNewApp measures the allocation cost of constructing a new App
// including all sub-models.
func BenchmarkNewApp(b *testing.B) {
	cfg := AppConfig{Version: "1.0.0", ProjectName: "bench-project"}
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = NewApp(cfg)
	}
}
