# T-059: Parallel Epic Decomposition Workers with errgroup

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 14-20hrs |
| Dependencies | T-056, T-057, T-058 |
| Blocked By | T-056, T-058 |
| Blocks | T-061 |

## Goal
Implement the "scatter" phase (Phase 2) of the PRD decomposition workflow: spawn N concurrent agent workers (one per epic) with bounded concurrency via errgroup, each producing a per-epic task JSON file. Workers share rate-limit state and support retry on malformed output with augmented prompts.

## Background
Per PRD Section 5.8, Phase 2 fans out epic decomposition to N concurrent workers. Each worker receives the full PRD (for context), its assigned epic definition, a summary of all other epics (for cross-referencing), and a JSON schema. Workers write structured JSON to files (`$WORK_DIR/epic-NNN.json`). The PRD specifies using `errgroup.Group` with `SetLimit(concurrency)` for bounded parallel execution, the same pattern used in the review pipeline (Section 5.5). Rate-limit handling reuses the implementation loop's rate-limit machinery (Section 5.4).

## Technical Specifications
### Implementation Approach
Create `internal/prd/worker.go` containing a `ScatterOrchestrator` that takes the `EpicBreakdown` from Phase 1, constructs per-epic prompts, and dispatches workers using `errgroup.Group` with `SetLimit(concurrency)`. Each worker invokes the agent, reads the output JSON file, validates with `EpicTaskResult.Validate()`, and retries up to 3 times on failure. Rate-limit signals from any worker pause all workers for the same provider via the shared rate-limit coordinator from `internal/agent/ratelimit.go`.

### Key Components
- **ScatterOrchestrator**: Manages the parallel dispatch and collection of worker results
- **EpicWorker**: Processes a single epic -- prompt construction, agent invocation, JSON extraction, validation
- **WorkerProgress**: Channel-based progress reporting for TUI consumption
- **ScatterResult**: Aggregated results from all workers

### API/Interface Contracts
```go
// internal/prd/worker.go

type ScatterOrchestrator struct {
    agent       agent.Agent
    workDir     string
    concurrency int
    maxRetries  int
    rateLimiter *agent.RateLimitCoordinator // shared with implementation loop
    logger      *log.Logger
    events      chan<- ScatterEvent
}

type ScatterOpts struct {
    PRDContent  string          // Full PRD text for context
    Breakdown   *EpicBreakdown  // From Phase 1 shredder
    Model       string
    Effort      string
}

type ScatterResult struct {
    Results   []*EpicTaskResult // One per epic, ordered by epic ID
    Failures  []ScatterFailure  // Epics that failed after all retries
    Duration  time.Duration
}

type ScatterFailure struct {
    EpicID string
    Errors []ValidationError
    Err    error
}

type ScatterEvent struct {
    Type    string // worker_started, worker_completed, worker_retry, worker_failed, rate_limited
    EpicID  string
    Message string
}

func NewScatterOrchestrator(agent agent.Agent, workDir string, opts ...ScatterOption) *ScatterOrchestrator
func (s *ScatterOrchestrator) Scatter(ctx context.Context, opts ScatterOpts) (*ScatterResult, error)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| golang.org/x/sync/errgroup | v0.19+ | Bounded parallel worker execution |
| internal/agent | - | Agent interface and rate-limit coordinator |
| internal/prd (T-056) | - | EpicTaskResult schema and validation |
| internal/jsonutil (T-058) | - | JSON extraction from agent output |
| text/template | stdlib | Per-epic prompt template rendering |
| encoding/json | stdlib | JSON parsing |

## Acceptance Criteria
- [ ] Spawns one worker per epic with concurrency bounded by SetLimit
- [ ] Each worker constructs a prompt with: PRD content, epic definition, other epic summaries, JSON schema
- [ ] Each worker writes output to `$workDir/epic-{epic_id}.json`
- [ ] Output JSON is extracted and validated against EpicTaskResult schema
- [ ] On validation failure, worker retries up to 3 times with errors in augmented prompt
- [ ] Rate-limit signals from any worker trigger coordinated pause across all workers for same provider
- [ ] ScatterResult contains all successful EpicTaskResults and any failures
- [ ] Progress events are emitted for each worker state change
- [ ] Context cancellation gracefully stops all workers
- [ ] Unit tests achieve 85% coverage using mock agents

## Testing Requirements
### Unit Tests
- 3 epics with concurrency 2: verify only 2 workers run simultaneously
- All workers succeed on first attempt: ScatterResult has 3 results, 0 failures
- One worker fails validation, succeeds on retry: verify retry count and final success
- One worker fails all retries: ScatterResult has 2 results, 1 failure
- Rate-limit on one worker pauses all workers, then resume
- Context cancelled mid-scatter: partial results returned with cancellation error
- Concurrency of 1: workers run sequentially
- Empty EpicBreakdown (0 epics): returns empty ScatterResult without error

### Integration Tests
- Scatter with mock agents producing varied output (valid, invalid, rate-limited)
- Event emission order matches expected worker lifecycle

### Edge Cases to Handle
- Agent writes JSON to stdout instead of file (fallback extraction)
- Agent writes partial JSON to file (retry)
- Worker goroutine panics (errgroup should handle, not crash process)
- All workers hit rate limit simultaneously
- Very large number of epics (20+) with low concurrency

## Implementation Notes
### Recommended Approach
1. Create errgroup with context: `g, ctx := errgroup.WithContext(ctx)`
2. Set concurrency limit: `g.SetLimit(concurrency)`
3. For each epic, launch `g.Go(func() error { ... })` containing the worker logic
4. Worker logic: construct prompt -> invoke agent -> read output file -> extract JSON -> validate -> retry loop
5. Collect results through a mutex-protected slice or buffered channel
6. Wait with `g.Wait()` and aggregate results
7. Rate-limit coordination: before invoking agent, check rate-limit state; after rate-limit signal, update shared state

### Potential Pitfalls
- errgroup.Wait() returns the first error -- but we want to collect ALL results (successes and failures). Use a separate error collection mechanism; do not return error from individual worker goroutines for validation failures -- only for fatal errors (context cancelled, unrecoverable agent failure)
- File path collisions: ensure each worker writes to a unique file (`epic-E-001.json`, not `epic-1.json`)
- Prompt size: with full PRD content + epic definition + other epic summaries, prompts can be very large. Consider summarizing other epics rather than including full definitions
- Rate-limit coordinator must be goroutine-safe (shared across workers)

### Security Considerations
- Workers should not write files outside the designated workDir
- Validate epic IDs before using them in file paths (no path traversal characters)

## References
- [PRD Section 5.8 - Phase 2 Scatter](docs/prd/PRD-Raven.md)
- [errgroup documentation](https://pkg.go.dev/golang.org/x/sync/errgroup)
- [errgroup best practices](https://oneuptime.com/blog/post/2026-01-07-go-errgroup/view)
