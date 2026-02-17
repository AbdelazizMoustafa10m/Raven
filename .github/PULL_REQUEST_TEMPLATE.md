## Description

<!-- What does this PR do? Link any related issues or task specs. -->

## Type of Change

- [ ] Bug fix (non-breaking change fixing an issue)
- [ ] New feature (non-breaking change adding functionality)
- [ ] Breaking change (fix or feature causing existing functionality to change)
- [ ] Refactor (code change that neither fixes a bug nor adds a feature)
- [ ] Documentation / config update

## Self-Review Checklist

### Go / Architecture

- [ ] Exported functions/types have doc comments
- [ ] Errors are wrapped with context (`fmt.Errorf("...: %w", err)`)
- [ ] No unintended global mutable state introduced
- [ ] Public APIs stay consistent (or breaking changes are documented)
- [ ] `context.Context` propagated to all long-running operations

### Concurrency

- [ ] `errgroup` used with `SetLimit()` for bounded parallelism
- [ ] No goroutine leaks (context cancellation triggers cleanup)
- [ ] Channels closed exactly once, no send after close
- [ ] TUI state mutations only in `Update()` (Elm architecture)

### Security / Safety

- [ ] No secrets committed in source, fixtures, or docs
- [ ] No command injection in `os/exec` calls (args as separate strings)
- [ ] File paths validated (no path traversal from task IDs or config)
- [ ] Regex/glob additions are safe for untrusted input (no ReDoS)

### Verification

- [ ] `go build ./cmd/raven/` passes
- [ ] `go vet ./...` passes
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` passes
- [ ] `go mod tidy` produces no diff
- [ ] Tested locally (describe below)

### Task Tracking

- [ ] Relevant `docs/tasks/T-XXX-*.md` acceptance criteria are satisfied
- [ ] `docs/tasks/PROGRESS.md` updated if task completion status changed

## Testing Done

<!-- How did you verify this works? -->
