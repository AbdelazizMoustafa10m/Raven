---
description: Review code changes for Raven with Go idioms and quality focus
argument-hint: <files or branch to review>
---

Review `$ARGUMENTS` for Raven and provide structured feedback:

## 1. Summary

What changed and why it matters.

## 2. Correctness

- Does it meet the acceptance criteria from the task spec?
- Are edge cases handled (empty inputs, nil, errors)?
- Any potential panics or unhandled errors?

## 3. Go Idioms

- Errors wrapped with context? (`fmt.Errorf("context: %w", err)`)
- No global mutable state?
- Interfaces used for testability?
- Proper use of `context.Context`?
- No unnecessary `init()` functions?

## 4. Concurrency Safety

- Shared state protected with mutexes or channels?
- `errgroup` used correctly for parallel work?
- No goroutine leaks (context cancellation respected)?
- Race-safe? (`go test -race`)

## 5. Performance

- Unnecessary allocations in hot paths?
- Large files streamed vs loaded into memory?
- `io.Reader`/`io.Writer` used for streaming?
- Benchmark tests for critical paths?

## 6. Testing

- Table-driven tests with meaningful names?
- Edge cases covered?
- Golden tests for output formatting?
- `t.Helper()` used in test helpers?
- `t.TempDir()` for filesystem tests?

## 7. Code Quality

- Follows existing package patterns?
- Proper error handling (no swallowed errors)?
- Structured logging with charmbracelet/log (not fmt.Printf)?
- Exported types have doc comments?

## 8. Follow-up Items

Concrete TODOs with file paths and line numbers.
