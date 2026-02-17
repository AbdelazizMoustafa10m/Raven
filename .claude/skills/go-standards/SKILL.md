---
name: go-standards
description: Go coding standards with MUST/SHOULD/CAN severity. Enforces idiomatic Go, error handling, concurrency safety, testing, and security. Reference for all Go code in Raven.
---

# Go Standards

## Severity System

- **MUST** -- Enforced by CI/review. Violations block merge.
- **SHOULD** -- Strong recommendations. Deviations require rationale in PR.
- **CAN** -- Allowed without extra approval. Use when beneficial.

---

## 1 -- Before Coding

- **BP-1 (MUST)** Ask clarifying questions for ambiguous requirements.
- **BP-2 (MUST)** Draft and confirm an approach (API shape, data flow, failure modes) before writing code.
- **BP-3 (SHOULD)** When >2 approaches exist, list pros/cons and rationale.
- **BP-4 (SHOULD)** Define testing strategy (unit/integration) and observability signals up front.

## 2 -- Modules and Dependencies

- **MD-1 (SHOULD)** Prefer stdlib; introduce deps only with clear payoff; track transitive size and licenses.
- **MD-2 (CAN)** Use `govulncheck` for dependency auditing and updates.

## 3 -- Code Style

- **CS-1 (MUST)** Enforce `gofmt` and `go vet` on all code.
- **CS-2 (MUST)** Avoid stutter in names: `package kv; type Store` not `KVStore` in `kv`.
- **CS-3 (SHOULD)** Small interfaces near consumers; prefer composition over inheritance.
- **CS-4 (SHOULD)** Avoid reflection on hot paths; prefer generics when it clarifies and speeds.
- **CS-5 (MUST)** Use input structs for functions receiving more than 2 arguments. Context is always a separate first parameter, never in the input struct.
- **CS-6 (SHOULD)** Declare function input structs immediately before the function that consumes them.
- **CS-7 (MUST)** Use `any` instead of `interface{}`.
- **CS-8 (SHOULD)** Avoid `else`; use early return, continue, or break.
- **CS-9 (MUST)** Import grouping order: stdlib, third-party, org, local. Separate with blank lines.
- **CS-10 (SHOULD)** Reduce nesting with early returns. Handle the error case first, then continue with the happy path.
- **CS-11 (SHOULD)** Reduce scope of variables; declare closest to first use.
- **CS-12 (SHOULD)** Use raw string literals to avoid escaping.
- **CS-13 (MUST)** Use field names in struct initialization (no positional).
- **CS-14 (SHOULD)** Group similar declarations together.
- **CS-15 (SHOULD)** Prefer `strconv` over `fmt` for string/number conversions.
- **CS-16 (SHOULD)** Nil is a valid slice; don't return empty slices just to avoid nil.
- **CS-17 (SHOULD)** Use `"time"` package types properly; never use raw `int` for durations.
- **CS-18 (SHOULD)** Use field tags in marshaled structs (JSON, TOML, etc.).

```go
// CS-5: Input struct for 3+ arguments (context stays outside)
type WalkInput struct {
    Root       string
    MaxDepth   int
    IgnoreFile string
}

func Walk(ctx context.Context, in WalkInput) ([]FileDescriptor, error) {
    // ...
}

// CS-9: Import ordering
import (
    "context"
    "fmt"
    "os"

    "github.com/spf13/cobra"
    "golang.org/x/sync/errgroup"

    "github.com/raven/raven/internal/config"
    "github.com/raven/raven/internal/discovery"
)

// CS-8: Early return instead of else
func process(fd FileDescriptor) error {
    if fd.IsBinary {
        return ErrSkipped
    }
    // happy path continues at top level
    return nil
}
```

## 4 -- Errors

- **ERR-1 (MUST)** Wrap with `%w` and context: `fmt.Errorf("open %s: %w", path, err)`.
- **ERR-2 (MUST)** Use `errors.Is`/`errors.As` for control flow; never match error strings.
- **ERR-3 (SHOULD)** Define sentinel errors in the package; document their behavior.
- **ERR-4 (CAN)** Use `context.WithCancelCause` and `context.Cause` for propagating error causes.
- **ERR-5 (MUST)** Handle errors once: log OR return, never both.
- **ERR-6 (MUST)** Do not panic; return errors instead. Panics are reserved for truly unrecoverable programmer bugs.
- **ERR-7 (SHOULD)** Prefer `errors.New` for simple static errors; custom types for errors needing fields.
- **ERR-8 (SHOULD)** Use `%w` when callers need `Is`/`As`; use `%v` to intentionally break the error chain.
- **ERR-9 (MUST)** Handle type assertion failures with comma-ok pattern.

```go
// ERR-1 + ERR-3: Sentinel errors and wrapping
var (
    ErrSkipped   = errors.New("file skipped")
    ErrBudgetExc = errors.New("token budget exceeded")
)

func processFile(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("read %s: %w", path, err)
    }
    // ...
    return nil
}

// ERR-9: Type assertion with comma-ok
val, ok := msg.(tea.KeyMsg)
if !ok {
    return m, nil
}
```

## 5 -- Concurrency

- **CC-1 (MUST)** The sender closes channels; receivers never close.
- **CC-2 (MUST)** Tie goroutine lifetime to a `context.Context`; prevent leaks.
- **CC-3 (MUST)** Protect shared state with `sync.Mutex`/`atomic`; no "probably safe" races.
- **CC-4 (SHOULD)** Use `errgroup` for fan-out work; cancel on first error.
- **CC-5 (CAN)** Prefer buffered channels only with documented rationale (throughput/back-pressure).
- **CC-6 (MUST)** Never fire-and-forget goroutines. Every goroutine must be tracked and joined.

```go
// CC-2 + CC-4: errgroup with context cancellation
func walkParallel(ctx context.Context, paths []string) ([]FileDescriptor, error) {
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(runtime.NumCPU())

    var mu sync.Mutex
    var results []FileDescriptor

    for _, p := range paths {
        g.Go(func() error {
            fd, err := processPath(ctx, p)
            if err != nil {
                return fmt.Errorf("process %s: %w", p, err)
            }
            mu.Lock()
            results = append(results, fd)
            mu.Unlock()
            return nil
        })
    }

    if err := g.Wait(); err != nil {
        return nil, err
    }
    return results, nil
}
```

## 6 -- Contexts

- **CTX-1 (MUST)** `ctx context.Context` is always the first parameter; never store ctx in structs.
- **CTX-2 (MUST)** Propagate non-nil `ctx`; honor `Done`, deadlines, and timeouts.
- **CTX-3 (CAN)** Expose `WithX(ctx)` helpers that derive deadlines from config.

```go
// CTX-1: Context is always first, before the input struct
func (s *Scanner) Scan(ctx context.Context, in ScanInput) ([]Finding, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // proceed with scan...
}
```

## 7 -- Testing

- **T-1 (MUST)** Table-driven tests; deterministic and hermetic by default.
- **T-2 (MUST)** Run `-race` in CI; add `t.Cleanup` for teardown.
- **T-3 (SHOULD)** Mark safe tests with `t.Parallel()`.
- **T-4 (SHOULD)** Split success and error test paths into separate test functions.
- **T-5 (SHOULD)** Use testify: `require` for fatal assertions, `assert` for soft checks.
- **T-6 (MUST)** Document only public interfaces, types, functions, methods.

```go
// T-1 + T-5: Table-driven test with testify
func TestTokenizer_Count(t *testing.T) {
    tests := []struct {
        name    string
        content string
        want    int
    }{
        {name: "empty", content: "", want: 0},
        {name: "single word", content: "hello", want: 1},
        {name: "go function", content: "func main() {}", want: 5},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            got, err := tokenizer.Count(tt.content)
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

## 8 -- Logging and Observability

- **OBS-1 (MUST)** Structured logging with `slog`; use levels and consistent field names.
- **OBS-2 (SHOULD)** Correlate logs, metrics, and traces via request IDs from context.

```go
slog.Info("pipeline stage complete",
    "stage", "discovery",
    "files", len(results),
    "duration", elapsed,
)
```

## 9 -- Performance

- **PERF-1 (MUST)** Measure before optimizing: `pprof`, `go test -bench`, `benchstat`.
- **PERF-2 (SHOULD)** Avoid allocations on hot paths; reuse buffers; prefer `bytes`/`strings` APIs.
- **PERF-3 (CAN)** Add microbenchmarks for critical functions and track regressions in CI.
- **PERF-4 (SHOULD)** Specify container capacity; pre-allocate slices and maps when size is known.

```go
// PERF-4: Pre-allocate with known capacity
files := make([]FileDescriptor, 0, len(paths))
tierMap := make(map[string]int, len(files))
```

## 10 -- Configuration

- **CFG-1 (MUST)** Config via env/flags; validate on startup; fail fast on invalid config.
- **CFG-2 (MUST)** Treat config as immutable after init; pass explicitly, not via globals.
- **CFG-3 (SHOULD)** Provide sane defaults and clear documentation.

## 11 -- APIs and Boundaries

- **API-1 (MUST)** Document exported items: `// Foo does ...`; keep exported surface minimal.
- **API-2 (MUST)** Accept interfaces where variation is needed; return concrete types unless abstraction is required.
- **API-3 (SHOULD)** Keep functions small, orthogonal, and composable.
- **API-4 (SHOULD)** Avoid embedding types in public structs (leaks implementation).
- **API-5 (CAN)** Use functional options pattern for extensibility.
- **API-6 (MUST)** Verify interface compliance at compile time.
- **API-7 (SHOULD)** Avoid mutable globals. If unavoidable, protect with mutex.
- **API-8 (SHOULD)** Avoid `init()`. Prefer explicit initialization.
- **API-9 (MUST)** Exit only in `main()`. All other code returns errors.

```go
// API-6: Compile-time interface compliance
var _ discovery.Walker = (*FileWalker)(nil)

// API-5: Functional options
type Option func(*Server)

func WithLogger(l *slog.Logger) Option {
    return func(s *Server) { s.logger = l }
}

func WithTimeout(d time.Duration) Option {
    return func(s *Server) { s.timeout = d }
}

func NewServer(addr string, opts ...Option) *Server {
    s := &Server{addr: addr, timeout: 30 * time.Second}
    for _, opt := range opts {
        opt(s)
    }
    return s
}
```

## 12 -- Security

- **SEC-1 (MUST)** Validate inputs; set explicit I/O timeouts; prefer TLS everywhere.
- **SEC-2 (MUST)** Never log secrets; manage secrets outside code (env/secret manager).
- **SEC-3 (SHOULD)** Limit filesystem/network access by default; principle of least privilege.
- **SEC-4 (CAN)** Add fuzz tests for untrusted inputs (redaction patterns, config parsing).

## 13 -- CI/CD

- **CI-1 (MUST)** Lint, vet, test (`-race`), and build on every PR; cache modules/builds.
- **CI-2 (MUST)** Reproducible builds with `-trimpath`; embed version via `-ldflags "-X main.version=$TAG"`.

## 14 -- Tooling Gates

- **G-1 (MUST)** `go vet ./...` passes.
- **G-2 (MUST)** `golangci-lint run` passes with project config.
- **G-3 (MUST)** `go test -race ./...` passes.

## 15 -- Defensive Coding

- **DC-1 (MUST)** Copy slices and maps at package boundaries; do not hold references to caller data.
- **DC-2 (SHOULD)** Zero-value mutexes are valid; never use pointer to mutex.
- **DC-3 (SHOULD)** Defer to clean up resources (files, locks, connections).
- **DC-4 (SHOULD)** Channel size is one or none; document rationale for larger buffers.
- **DC-5 (SHOULD)** Start enums at one; reserve zero value for "unset".

```go
// DC-1: Copy slices at boundaries
func (c *Config) SetIgnorePatterns(patterns []string) {
    c.patterns = make([]string, len(patterns))
    copy(c.patterns, patterns)
}

// DC-5: Enums start at one
type Tier int

const (
    TierUnset Tier = iota // 0 = unset/invalid
    TierCritical          // 1
    TierImportant         // 2
    TierNormal            // 3
)
```