package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleTaskMarkdown is a minimal but valid task spec markdown used as the
// hot-path input for parser benchmarks.
const sampleTaskMarkdown = `# T-001: Sample Task

| Priority | Must Have |
|----------|-----------|
| Dependencies | None |
| Estimated Effort | Small: 1-2hrs |
| Blocked By | None |
| Blocks | T-002 |

## Goal

Sample task for benchmarking.
`

// benchWriteStateFile writes n task state lines to a temp file and returns
// the file path. Even-indexed tasks are "completed"; odd-indexed are
// "not_started". The caller owns the directory via b.TempDir().
func benchWriteStateFile(b *testing.B, dir string, n int) string {
	b.Helper()
	path := filepath.Join(dir, "task-state.conf")
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		status := "not_started"
		if i%2 == 0 {
			status = "completed"
		}
		fmt.Fprintf(&sb, "T-%03d|%s|claude||\n", i, status)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		b.Fatalf("writing state file: %v", err)
	}
	return path
}

// benchWriteTaskFile writes a task spec file for the given task ID to dir and
// returns the path. The heading uses sampleTaskMarkdown's format with the ID
// substituted in, so ParseTaskFile will extract the correct ID.
func benchWriteTaskFile(b *testing.B, dir, id string) string {
	b.Helper()
	content := strings.ReplaceAll(sampleTaskMarkdown, "T-001", id)
	name := id + "-sample-task.md"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatalf("writing task file: %v", err)
	}
	return path
}

// --- Parser benchmarks -------------------------------------------------------

// BenchmarkParseTaskSpec_Minimal measures in-memory markdown parsing of a
// minimal task spec with no dependencies -- no file I/O.
func BenchmarkParseTaskSpec_Minimal(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		spec, err := ParseTaskSpec(sampleTaskMarkdown)
		if err != nil {
			b.Fatalf("ParseTaskSpec: %v", err)
		}
		_ = spec
	}
}

// BenchmarkParseTaskSpec_WithDeps measures parsing a task spec that contains
// multiple dependency references, exercising the regex extraction path.
func BenchmarkParseTaskSpec_WithDeps(b *testing.B) {
	content := `# T-050: Complex Task

| Priority | Must Have |
|----------|-----------|
| Dependencies | T-001, T-002, T-003, T-004, T-005 |
| Estimated Effort | Large: 10-20hrs |
| Blocked By | T-010, T-011 |
| Blocks | T-060, T-070 |

## Goal

A task with many dependency references for benchmarking.

## Acceptance Criteria

- Must complete after T-001 through T-005 are done.
`
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		spec, err := ParseTaskSpec(content)
		if err != nil {
			b.Fatalf("ParseTaskSpec: %v", err)
		}
		_ = spec
	}
}

// BenchmarkParseTaskFile measures the full file-read-and-parse path, including
// the OS open/read syscalls.
func BenchmarkParseTaskFile(b *testing.B) {
	dir := b.TempDir()
	path := benchWriteTaskFile(b, dir, "T-001")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		spec, err := ParseTaskFile(path)
		if err != nil {
			b.Fatalf("ParseTaskFile: %v", err)
		}
		_ = spec
	}
}

// BenchmarkDiscoverTasks_10 measures DiscoverTasks scanning a directory of 10
// task spec files: glob + parse + dedup + sort.
func BenchmarkDiscoverTasks_10(b *testing.B) {
	benchmarkDiscoverTasks(b, 10)
}

// BenchmarkDiscoverTasks_100 measures DiscoverTasks scanning a directory of
// 100 task spec files.
func BenchmarkDiscoverTasks_100(b *testing.B) {
	benchmarkDiscoverTasks(b, 100)
}

// benchmarkDiscoverTasks is the shared implementation for DiscoverTasks
// benchmarks at different scales.
func benchmarkDiscoverTasks(b *testing.B, n int) {
	b.Helper()
	dir := b.TempDir()

	// Write n task spec files to the temp directory. Each file must use its
	// own task ID in the heading so DiscoverTasks does not reject duplicates.
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("T-%03d", i)
		name := fmt.Sprintf("%s-sample-task.md", id)
		path := filepath.Join(dir, name)
		content := strings.ReplaceAll(sampleTaskMarkdown, "T-001", id)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			b.Fatalf("writing task file %s: %v", name, err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		specs, err := DiscoverTasks(dir)
		if err != nil {
			b.Fatalf("DiscoverTasks: %v", err)
		}
		_ = specs
	}
}

// --- StateManager benchmarks -------------------------------------------------

// BenchmarkStateManager_Load_10 measures StateManager.Load reading a state
// file with 10 entries: file open + line scan + parse.
func BenchmarkStateManager_Load_10(b *testing.B) {
	benchmarkStateManagerLoad(b, 10)
}

// BenchmarkStateManager_Load_100 measures StateManager.Load reading a state
// file with 100 entries.
func BenchmarkStateManager_Load_100(b *testing.B) {
	benchmarkStateManagerLoad(b, 100)
}

// benchmarkStateManagerLoad is the shared implementation for StateManager.Load
// benchmarks.
func benchmarkStateManagerLoad(b *testing.B, n int) {
	b.Helper()
	dir := b.TempDir()
	path := benchWriteStateFile(b, dir, n)
	sm := NewStateManager(path)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		states, err := sm.Load()
		if err != nil {
			b.Fatalf("StateManager.Load: %v", err)
		}
		_ = states
	}
}

// BenchmarkStateManager_LoadMap_100 measures StateManager.LoadMap, which
// additionally builds the task-ID keyed map on top of the slice parse.
func BenchmarkStateManager_LoadMap_100(b *testing.B) {
	dir := b.TempDir()
	path := benchWriteStateFile(b, dir, 100)
	sm := NewStateManager(path)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		m, err := sm.LoadMap()
		if err != nil {
			b.Fatalf("StateManager.LoadMap: %v", err)
		}
		_ = m
	}
}

// BenchmarkStateManager_Get measures StateManager.Get for a task in the middle
// of a 100-entry state file (linear scan path).
func BenchmarkStateManager_Get(b *testing.B) {
	dir := b.TempDir()
	path := benchWriteStateFile(b, dir, 100)
	sm := NewStateManager(path)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		state, err := sm.Get("T-050")
		if err != nil {
			b.Fatalf("StateManager.Get: %v", err)
		}
		_ = state
	}
}

// --- TaskSelector construction benchmarks ------------------------------------

// BenchmarkNewTaskSelector_10 measures constructing a TaskSelector from 10
// specs, including specMap allocation and population.
func BenchmarkNewTaskSelector_10(b *testing.B) {
	benchmarkNewTaskSelector(b, 10)
}

// BenchmarkNewTaskSelector_100 measures constructing a TaskSelector from 100
// specs.
func BenchmarkNewTaskSelector_100(b *testing.B) {
	benchmarkNewTaskSelector(b, 100)
}

// benchmarkNewTaskSelector is the shared implementation for NewTaskSelector
// construction benchmarks.
func benchmarkNewTaskSelector(b *testing.B, n int) {
	b.Helper()
	specs := make([]*ParsedTaskSpec, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		specs[i] = &ParsedTaskSpec{
			ID:           id,
			Title:        "Task " + id,
			Dependencies: []string{},
			BlockedBy:    []string{},
			Blocks:       []string{},
		}
	}
	phases := []Phase{
		{ID: 1, Name: "All Tasks", StartTask: "T-001", EndTask: fmt.Sprintf("T-%03d", n)},
	}
	sm := NewStateManager(filepath.Join(b.TempDir(), "nonexistent.conf"))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		sel := NewTaskSelector(specs, sm, phases)
		_ = sel
	}
}

// --- SelectNext benchmarks at different scales and dependency shapes ----------

// BenchmarkSelectNext_Flat_10 measures SelectNext on a phase of 10 tasks with
// no dependencies, where the first not_started task is always selected.
func BenchmarkSelectNext_Flat_10(b *testing.B) {
	benchmarkSelectNextFlat(b, 10)
}

// BenchmarkSelectNext_Flat_100 measures SelectNext on a phase of 100 flat
// tasks. The first half are completed; the hot path scans to task 51.
func BenchmarkSelectNext_Flat_100(b *testing.B) {
	benchmarkSelectNextFlat(b, 100)
}

// benchmarkSelectNextFlat is the shared flat-graph SelectNext benchmark.
func benchmarkSelectNextFlat(b *testing.B, n int) {
	b.Helper()
	specs := make([]*ParsedTaskSpec, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		specs[i] = &ParsedTaskSpec{
			ID:           id,
			Title:        "Task " + id,
			Dependencies: []string{},
			BlockedBy:    []string{},
			Blocks:       []string{},
		}
	}

	phases := []Phase{
		{ID: 1, Name: "Bench Phase", StartTask: "T-001", EndTask: fmt.Sprintf("T-%03d", n)},
	}

	// Mark the first half as completed so SelectNext has to scan.
	dir := b.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	var sb strings.Builder
	for i := 1; i <= n/2; i++ {
		fmt.Fprintf(&sb, "T-%03d|completed|claude||\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		b.Fatalf("writing state: %v", err)
	}
	sm := NewStateManager(path)
	sel := NewTaskSelector(specs, sm, phases)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		spec, err := sel.SelectNext(1)
		if err != nil {
			b.Fatalf("SelectNext: %v", err)
		}
		_ = spec
	}
}

// BenchmarkSelectNext_Chain_10 measures SelectNext on a linear dependency
// chain of 10 tasks where each task depends on the previous one. The first
// half are completed; the hot path must verify the chain before selecting.
func BenchmarkSelectNext_Chain_10(b *testing.B) {
	benchmarkSelectNextChain(b, 10)
}

// BenchmarkSelectNext_Chain_100 measures SelectNext on a linear dependency
// chain of 100 tasks.
func BenchmarkSelectNext_Chain_100(b *testing.B) {
	benchmarkSelectNextChain(b, 100)
}

// benchmarkSelectNextChain is the shared chain-dependency SelectNext benchmark.
func benchmarkSelectNextChain(b *testing.B, n int) {
	b.Helper()
	specs := make([]*ParsedTaskSpec, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		var deps []string
		if i > 0 {
			deps = []string{fmt.Sprintf("T-%03d", i)}
		}
		specs[i] = &ParsedTaskSpec{
			ID:           id,
			Title:        "Task " + id,
			Dependencies: deps,
			BlockedBy:    []string{},
			Blocks:       []string{},
		}
	}

	phases := []Phase{
		{ID: 1, Name: "Chain Phase", StartTask: "T-001", EndTask: fmt.Sprintf("T-%03d", n)},
	}

	// Mark the first half as completed.
	dir := b.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	var sb strings.Builder
	for i := 1; i <= n/2; i++ {
		fmt.Fprintf(&sb, "T-%03d|completed|claude||\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		b.Fatalf("writing state: %v", err)
	}
	sm := NewStateManager(path)
	sel := NewTaskSelector(specs, sm, phases)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		spec, err := sel.SelectNext(1)
		if err != nil {
			b.Fatalf("SelectNext: %v", err)
		}
		_ = spec
	}
}

// --- SelectNextInRange benchmarks --------------------------------------------

// BenchmarkSelectNextInRange_100 measures SelectNextInRange across 100 tasks
// with a flat dependency graph, simulating the --phase all mode.
func BenchmarkSelectNextInRange_100(b *testing.B) {
	const n = 100
	specs := make([]*ParsedTaskSpec, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		specs[i] = &ParsedTaskSpec{
			ID:           id,
			Title:        "Task " + id,
			Dependencies: []string{},
			BlockedBy:    []string{},
			Blocks:       []string{},
		}
	}

	// Three phases covering the full range.
	phases := []Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-034"},
		{ID: 2, Name: "Phase 2", StartTask: "T-035", EndTask: "T-067"},
		{ID: 3, Name: "Phase 3", StartTask: "T-068", EndTask: "T-100"},
	}

	// Mark the first 60 as completed.
	dir := b.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	var sb strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&sb, "T-%03d|completed|claude||\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		b.Fatalf("writing state: %v", err)
	}
	sm := NewStateManager(path)
	sel := NewTaskSelector(specs, sm, phases)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		spec, err := sel.SelectNextInRange("T-001", "T-100")
		if err != nil {
			b.Fatalf("SelectNextInRange: %v", err)
		}
		_ = spec
	}
}

// --- Phase progress benchmarks -----------------------------------------------

// BenchmarkIsPhaseComplete_100 measures IsPhaseComplete on a phase of 100
// tasks, all completed, so the method must scan the full state map.
func BenchmarkIsPhaseComplete_100(b *testing.B) {
	const n = 100
	specs := make([]*ParsedTaskSpec, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		specs[i] = &ParsedTaskSpec{
			ID:           id,
			Title:        "Task " + id,
			Dependencies: []string{},
			BlockedBy:    []string{},
			Blocks:       []string{},
		}
	}

	phases := []Phase{
		{ID: 1, Name: "Complete Phase", StartTask: "T-001", EndTask: fmt.Sprintf("T-%03d", n)},
	}

	dir := b.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "T-%03d|completed|claude||\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		b.Fatalf("writing state: %v", err)
	}
	sm := NewStateManager(path)
	sel := NewTaskSelector(specs, sm, phases)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		done, err := sel.IsPhaseComplete(1)
		if err != nil {
			b.Fatalf("IsPhaseComplete: %v", err)
		}
		_ = done
	}
}

// BenchmarkBlockedTasks_100 measures BlockedTasks on a phase of 100 tasks
// where all tasks have unsatisfied dependencies, forcing a full scan.
func BenchmarkBlockedTasks_100(b *testing.B) {
	const n = 100
	specs := make([]*ParsedTaskSpec, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		// Every task depends on T-999, which is never in the state file.
		specs[i] = &ParsedTaskSpec{
			ID:           id,
			Title:        "Task " + id,
			Dependencies: []string{"T-999"},
			BlockedBy:    []string{},
			Blocks:       []string{},
		}
	}

	phases := []Phase{
		{ID: 1, Name: "All Blocked", StartTask: "T-001", EndTask: fmt.Sprintf("T-%03d", n)},
	}

	sm := NewStateManager(filepath.Join(b.TempDir(), "nonexistent.conf"))
	sel := NewTaskSelector(specs, sm, phases)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		blocked, err := sel.BlockedTasks(1)
		if err != nil {
			b.Fatalf("BlockedTasks: %v", err)
		}
		_ = blocked
	}
}
