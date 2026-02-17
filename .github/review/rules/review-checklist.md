# Review Checklist

## Correctness (Must Pass)

- [ ] `go build ./cmd/raven/` compiles without errors
- [ ] `go vet ./...` passes with zero warnings
- [ ] `go test ./...` passes all tests
- [ ] `go test -race ./...` detects no races
- [ ] No new global mutable state introduced
- [ ] All errors wrapped with context (`fmt.Errorf("context: %w", err)`)
- [ ] No swallowed errors (unchecked error returns)
- [ ] Exported types and functions have doc comments

## Cobra CLI Contracts

- [ ] Commands remain script-friendly (stable stdout/stderr separation)
- [ ] Exit codes follow Raven contract (0=success, 1=error, 2=partial, 3=cancelled)
- [ ] Flag interactions are validated and deterministic
- [ ] `--dry-run` mode outputs planned actions without execution
- [ ] `--json` / machine-readable outputs stay parseable and stable
- [ ] Diagnostics use `charmbracelet/log`, not `fmt.Printf`

## Concurrency

- [ ] `errgroup.Group` used with `SetLimit()` for bounded parallelism
- [ ] No goroutine leaks (context cancellation triggers cleanup)
- [ ] Channels closed exactly once, no send after close
- [ ] Shared state protected by mutex or serialized through channels
- [ ] `context.Context` propagated to all long-running operations
- [ ] TUI `Update()` is the only place state mutates (Elm architecture)

## Workflow Engine

- [ ] State machine transitions are valid and complete (no unreachable states)
- [ ] Checkpoint serialization is deterministic (stable JSON key ordering)
- [ ] Resume from checkpoint produces identical behavior to fresh run from that state
- [ ] Event ordering is consistent for TUI consumption
- [ ] Step handlers respect context cancellation

## Agent Adapters

- [ ] Agent subprocess cleaned up on context cancellation (`cmd.Process.Kill`)
- [ ] Rate-limit regexes have test coverage for known patterns
- [ ] Output streaming doesn't block on full buffers
- [ ] `RunResult` populated correctly on all exit paths (including errors)

## Deterministic Behavior

- [ ] Output ordering is stable (sort maps/slices before rendering)
- [ ] No map-iteration nondeterminism leaks into output or state files
- [ ] Task state file updates are atomic (write-rename pattern or flock)
- [ ] Checkpoint JSON uses sorted keys

## Security

- [ ] No command injection in `os/exec` calls (args as separate strings)
- [ ] File paths validated (no path traversal from task IDs or config)
- [ ] No sensitive data in logs, checkpoint files, or agent output logs
- [ ] Subprocess timeout enforced for agent calls
- [ ] Dependency/toolchain changes are justified and safe

## Testing

- [ ] Table-driven tests for new logic with meaningful subtest names
- [ ] Edge cases tested (empty input, nil, boundary values, error paths)
- [ ] `t.Helper()` in test helpers
- [ ] `t.TempDir()` for filesystem tests
- [ ] Golden tests for output formatting where appropriate
- [ ] Mock agents used for integration tests (no real API calls in CI)

## Nice to Have

- [ ] Benchmark tests for hot paths (JSON extraction, state serialization)
- [ ] Fuzz tests for parsing functions (TOML, task specs, rate-limit patterns)
- [ ] Example tests for public API documentation
