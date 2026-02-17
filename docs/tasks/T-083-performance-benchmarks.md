# T-083: Performance Benchmarking Suite

## Metadata
| Field | Value |
|-------|-------|
| Priority | Should Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-001, T-004, T-005, T-010, T-013, T-018, T-066 |
| Blocked By | T-066 |
| Blocks | T-087 |

## Goal
Create a comprehensive performance benchmarking suite that measures Raven's startup time, configuration loading, task parsing, concurrent agent overhead, workflow engine throughput, and TUI frame rate. These benchmarks establish baselines for the PRD's performance targets (startup time, TUI <100ms per frame, 5+ concurrent agents) and catch regressions in CI.

## Background
Per PRD Section 4 (Success Metrics), Raven has explicit performance targets: "Dashboard update latency with 5 agents < 100ms per frame" and implied targets for startup speed ("Time from install to first raven implement < 3 minutes" -- which includes configuration time but startup must be negligible). Per PRD Section 7 (Phase 7), the project requires "Performance benchmarking (startup time, concurrent agent overhead, TUI frame rate)."

Go 1.24 introduced `testing.B.Loop` which provides more accurate benchmark measurements by automatically excluding setup and cleanup from timing. The benchmarks should use this new API where appropriate.

## Technical Specifications
### Implementation Approach
Create benchmark files alongside the packages they test, following Go conventions (`*_test.go` with `Benchmark*` functions). Organize benchmarks into categories: startup, config, task, agent, workflow, and TUI. Use `testing.B` with `b.Loop()` (Go 1.24+) for the hot paths and traditional `b.N` for compatibility. Create a `make bench` target and a CI workflow step that runs benchmarks and reports results.

### Key Components
- **`cmd/raven/bench_test.go`**: Binary startup time benchmark (exec + version command)
- **`internal/config/bench_test.go`**: Config loading and validation benchmarks
- **`internal/task/bench_test.go`**: Task parsing, dependency resolution, and selection benchmarks
- **`internal/agent/bench_test.go`**: Agent construction and concurrent coordination overhead
- **`internal/workflow/bench_test.go`**: Workflow engine step execution and state checkpoint benchmarks
- **`internal/tui/bench_test.go`**: TUI model update and view rendering benchmarks
- **`internal/review/bench_test.go`**: JSON extraction and finding consolidation benchmarks
- **`scripts/bench-report.sh`**: Script to run benchmarks and format results

### API/Interface Contracts
```go
// cmd/raven/bench_test.go
package main_test

import (
	"os/exec"
	"testing"
)

func BenchmarkBinaryStartup(b *testing.B) {
	// Measures cold start: exec raven version
	binary := buildTestBinary(b)
	b.ResetTimer()
	for b.Loop() {
		cmd := exec.Command(binary, "version")
		if err := cmd.Run(); err != nil {
			b.Fatal(err)
		}
	}
}

// internal/config/bench_test.go
package config_test

import (
	"testing"
)

func BenchmarkConfigLoad(b *testing.B) {
	// Measures TOML parsing + defaults + validation
	configPath := setupTestConfig(b)
	b.ResetTimer()
	for b.Loop() {
		_, err := Load(configPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConfigValidation(b *testing.B) {
	cfg := setupValidConfig(b)
	b.ResetTimer()
	for b.Loop() {
		_ = cfg.Validate()
	}
}

// internal/task/bench_test.go
package task_test

import (
	"testing"
)

func BenchmarkTaskParsing(b *testing.B) {
	// Parse a single task markdown file
	content := loadTestFixture(b, "testdata/sample-task.md")
	b.ResetTimer()
	for b.Loop() {
		_, err := ParseTaskSpec(content)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNextTaskSelection(b *testing.B) {
	// Select next task from 100-task dependency graph
	b.Run("10_tasks", func(b *testing.B) {
		state := setupTaskState(b, 10)
		b.ResetTimer()
		for b.Loop() {
			_, _ = SelectNextTask(state, PhaseRange{1, 10})
		}
	})
	b.Run("100_tasks", func(b *testing.B) {
		state := setupTaskState(b, 100)
		b.ResetTimer()
		for b.Loop() {
			_, _ = SelectNextTask(state, PhaseRange{1, 100})
		}
	})
}

// internal/agent/bench_test.go
package agent_test

import (
	"testing"
)

func BenchmarkConcurrentAgentOverhead(b *testing.B) {
	// Measures goroutine creation + channel coordination for N agents
	for _, n := range []int{1, 3, 5, 10} {
		b.Run(fmt.Sprintf("%d_agents", n), func(b *testing.B) {
			agents := setupMockAgents(b, n)
			b.ResetTimer()
			for b.Loop() {
				runConcurrentAgents(b, agents)
			}
		})
	}
}

// internal/tui/bench_test.go
package tui_test

import (
	"testing"
)

func BenchmarkTUIViewRender(b *testing.B) {
	// Measures full view rendering at 80x24
	model := setupTestModel(b, 80, 24)
	b.ResetTimer()
	for b.Loop() {
		_ = model.View()
	}
}

func BenchmarkTUIUpdate(b *testing.B) {
	// Measures state update processing
	model := setupTestModel(b, 120, 40)
	msgs := generateTestMessages(b, 100)
	b.ResetTimer()
	for b.Loop() {
		for _, msg := range msgs {
			model, _ = model.Update(msg)
		}
	}
}

// internal/workflow/bench_test.go
package workflow_test

import (
	"testing"
)

func BenchmarkStateCheckpoint(b *testing.B) {
	// Measures JSON serialization of workflow state to disk
	state := setupTestWorkflowState(b)
	dir := b.TempDir()
	b.ResetTimer()
	for b.Loop() {
		if err := state.Checkpoint(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateRestore(b *testing.B) {
	// Measures JSON deserialization of workflow state from disk
	state := setupTestWorkflowState(b)
	dir := b.TempDir()
	state.Checkpoint(dir)
	b.ResetTimer()
	for b.Loop() {
		_, err := RestoreState(dir, state.ID)
		if err != nil {
			b.Fatal(err)
		}
	}
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| testing | stdlib (Go 1.24+) | Benchmark framework with b.Loop() support |
| os/exec | stdlib | Binary startup time measurement |
| stretchr/testify | v1.9+ | Test helper assertions |

## Acceptance Criteria
- [ ] Benchmark files exist in all key packages: config, task, agent, workflow, tui, review, cmd/raven
- [ ] `go test -bench=. ./...` runs all benchmarks without errors
- [ ] Binary startup time benchmark measures cold start (exec + version) at under 200ms
- [ ] Config loading benchmark measures TOML parse + validate cycle
- [ ] Task parsing benchmark measures markdown parsing throughput
- [ ] Next-task selection benchmark covers 10, 100 task graphs
- [ ] Concurrent agent overhead benchmark covers 1, 3, 5, 10 agent counts
- [ ] TUI view rendering benchmark targets <100ms per frame at 120x40 terminal size
- [ ] TUI update benchmark measures message processing throughput
- [ ] Workflow state checkpoint/restore benchmarks measure I/O overhead
- [ ] `make bench` target runs all benchmarks with memory allocation stats (`-benchmem`)
- [ ] Benchmark results are stable (low variance) across runs
- [ ] CI workflow includes benchmark run (without regression detection for v2.0, tracked manually)

## Testing Requirements
### Unit Tests
- Benchmark test helper functions (setupTestConfig, setupTaskState, etc.) are correct
- Mock agents produce expected behavior for concurrent benchmarks

### Integration Tests
- All benchmarks complete without timeout (default 10min limit should be sufficient)
- Binary startup benchmark actually runs the compiled binary
- Benchmark results are consistent across 3 consecutive runs (< 20% variance)

### Edge Cases to Handle
- Benchmarks that involve file I/O should use `b.TempDir()` to avoid polluting the workspace
- Binary startup benchmark needs to build the binary first (use `TestMain` or build in setup)
- TUI benchmarks should test with various terminal sizes (80x24, 120x40, 200x50)
- Large task graphs (100+ tasks) should be generated programmatically, not from fixtures
- Agent overhead benchmarks should use mock agents that return immediately (not real agent calls)

## Implementation Notes
### Recommended Approach
1. Start with the simplest benchmarks: config loading and task parsing
2. Create test helper functions for generating test fixtures programmatically
3. Add binary startup benchmark using `exec.Command` pattern
4. Add TUI benchmarks -- these are the most important for the <100ms target
5. Add concurrent agent overhead benchmarks with mock agents
6. Add workflow checkpoint/restore benchmarks
7. Create `make bench` target: `go test -bench=. -benchmem -benchtime=3s ./...`
8. Run benchmarks 3 times and verify stability
9. Document baseline results in a `BENCHMARKS.md` file for future reference

### Potential Pitfalls
- `b.Loop()` is a Go 1.24+ feature. If CI uses an older Go version, fall back to `for i := 0; i < b.N; i++`. However, since the project requires Go 1.24+, this should not be an issue.
- Binary startup benchmarks are inherently slower and more variable than in-process benchmarks. Use `-benchtime=5s` or `-count=5` for stability.
- TUI view rendering benchmarks depend on lipgloss string building, which may allocate heavily. Use `-benchmem` to track allocations.
- File I/O benchmarks (checkpoint/restore) are affected by OS caching. The first iteration may be slower. `b.ResetTimer()` after setup mitigates this.
- Do not benchmark actual agent subprocess execution (that depends on external tools). Only benchmark the coordination overhead.

### Security Considerations
- Benchmarks should not connect to external services or APIs
- Temporary files created by benchmarks should be cleaned up (`b.TempDir()` handles this)
- No sensitive data should appear in benchmark fixtures

## References
- [Go testing.B.Loop (Go 1.24)](https://go.dev/blog/testing-b-loop)
- [Go Benchmarking Documentation](https://pkg.go.dev/testing#hdr-Benchmarks)
- [Benchmarking in Go: Comprehensive Handbook](https://betterstack.com/community/guides/scaling-go/golang-benchmarking/)
- [Writing Benchmarks in Go (Dave Cheney)](https://dave.cheney.net/2013/06/30/how-to-write-benchmarks-in-go)
- [PRD Section 4: Success Metrics](docs/prd/PRD-Raven.md)