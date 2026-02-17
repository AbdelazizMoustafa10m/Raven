# Review Checklist

## Must Pass
- [ ] `go build ./cmd/raven/` compiles without errors
- [ ] `go vet ./...` passes with zero warnings
- [ ] `go test ./...` passes all tests
- [ ] `go test -race ./...` detects no races
- [ ] No new global mutable state introduced
- [ ] All errors wrapped with context (`fmt.Errorf`)
- [ ] No swallowed errors (unchecked error returns)
- [ ] Exported types and functions have doc comments

## Should Pass
- [ ] Table-driven tests for new logic
- [ ] Edge cases tested (empty, nil, boundary)
- [ ] `context.Context` used for cancellation in long ops
- [ ] `io.Reader`/`io.Writer` for streaming where appropriate
- [ ] Package boundary is correct (single responsibility)

## Nice to Have
- [ ] Benchmark tests for hot paths
- [ ] Golden tests for output formatting
- [ ] Fuzz tests for parsing functions
