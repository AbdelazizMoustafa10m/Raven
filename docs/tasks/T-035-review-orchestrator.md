# T-035: Multi-Agent Parallel Review Orchestrator

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 14-20hrs |
| Dependencies | T-021, T-031, T-032, T-033, T-034, T-058 |
| Blocked By | T-021, T-031, T-032, T-033, T-034, T-058 |
| Blocks | T-041 |

## Goal
Implement the multi-agent parallel review orchestrator that fans out review requests to N agents concurrently using `errgroup.Group` with `SetLimit(concurrency)`, collects structured JSON findings from each agent, extracts JSON from freeform output using the shared `internal/jsonutil/` utility (T-058), and consolidates results into a unified review. This is the central coordinator of the review pipeline.

## Background
Per PRD Section 5.5, the review pipeline fans out review requests to N agents concurrently with a configurable concurrency limit. Each review goroutine captures agent output, extracts JSON findings using the robust extractor (T-058), validates the result, and sends it to a collector. The orchestrator uses `errgroup.Group` with `SetLimit(concurrency)` for bounded parallel execution -- the same pattern used by the PRD decomposition workers (T-059). The PRD specifies two review modes: `all` (every agent reviews everything) and `split` (each agent reviews different files).

## Technical Specifications
### Implementation Approach
Create `internal/review/orchestrator.go` containing a `ReviewOrchestrator` that takes a `DiffResult`, a list of agent names, concurrency settings, and review mode. It constructs per-agent prompts via the PromptBuilder (T-033), launches review workers using `errgroup`, collects results through a mutex-protected slice, extracts JSON from agent output using `jsonutil.ExtractInto` (T-058), validates results, and passes all agent results to the Consolidator (T-034). Emits structured events for TUI consumption.

### Key Components
- **ReviewOrchestrator**: Top-level coordinator for the review pipeline
- **reviewWorker**: Individual goroutine that invokes one agent and collects its findings
- **ReviewEvent**: Structured events emitted during review (started, agent_started, agent_completed, consolidated, etc.)
- **OrchestratorResult**: Final output containing consolidated review, stats, and per-agent details

### API/Interface Contracts
```go
// internal/review/orchestrator.go

type ReviewOrchestrator struct {
    agentRegistry  agent.Registry
    diffGen        *DiffGenerator
    promptBuilder  *PromptBuilder
    consolidator   *Consolidator
    concurrency    int
    logger         *log.Logger
    events         chan<- ReviewEvent
}

type ReviewEvent struct {
    Type      string // review_started, agent_started, agent_completed, agent_error, rate_limited, consolidated
    Agent     string
    Message   string
    Timestamp time.Time
}

type OrchestratorResult struct {
    Consolidated *ConsolidatedReview
    Stats        *ConsolidationStats
    DiffResult   *DiffResult
    Duration     time.Duration
    AgentErrors  []AgentError
}

type AgentError struct {
    Agent   string
    Err     error
    Message string
}

func NewReviewOrchestrator(
    registry agent.Registry,
    diffGen *DiffGenerator,
    promptBuilder *PromptBuilder,
    consolidator *Consolidator,
    concurrency int,
    logger *log.Logger,
    events chan<- ReviewEvent,
) *ReviewOrchestrator

// Run executes the full review pipeline:
// 1. Generate diff
// 2. Build per-agent prompts
// 3. Fan out review requests (errgroup with SetLimit)
// 4. Extract JSON from each agent's output
// 5. Consolidate results
func (ro *ReviewOrchestrator) Run(ctx context.Context, opts ReviewOpts) (*OrchestratorResult, error)

// DryRun returns the planned review actions without executing agents.
func (ro *ReviewOrchestrator) DryRun(ctx context.Context, opts ReviewOpts) (string, error)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| golang.org/x/sync/errgroup | v0.19+ | Bounded parallel review execution |
| internal/agent (T-021) | - | Agent interface and registry |
| internal/review (T-031) | - | Finding, ReviewResult, AgentReviewResult types |
| internal/review/diff (T-032) | - | DiffGenerator for diff production |
| internal/review/prompt (T-033) | - | PromptBuilder for per-agent prompts |
| internal/review/consolidate (T-034) | - | Consolidator for deduplication |
| internal/jsonutil (T-058) | - | JSON extraction from freeform agent output |
| time | stdlib | Duration tracking |
| sync | stdlib | Mutex for result collection |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Fans out review requests to N agents concurrently with `errgroup.SetLimit(concurrency)`
- [ ] Each agent receives a properly constructed review prompt (via PromptBuilder)
- [ ] Agent output is captured and JSON is extracted using `jsonutil.ExtractInto` (T-058)
- [ ] Extracted JSON is validated using `ReviewResult.Validate()`
- [ ] All valid agent results are passed to Consolidator for deduplication
- [ ] Agent errors (non-zero exit, timeout, rate limit) are captured but do not abort the entire review
- [ ] In `all` mode: every agent reviews all files
- [ ] In `split` mode: files are distributed across agents using SplitFiles (T-032)
- [ ] Emits ReviewEvent messages at each stage for TUI consumption
- [ ] Context cancellation gracefully stops all review workers
- [ ] DryRun shows planned review actions (agents, files per agent, prompt summary) without execution
- [ ] Unit tests achieve 85% coverage using mock agents

## Testing Requirements
### Unit Tests
- 2 agents, concurrency 2, both succeed: OrchestratorResult has consolidated findings from both
- 3 agents, concurrency 1: agents run sequentially (verify via timing or event order)
- 1 agent fails (non-zero exit), 1 succeeds: OrchestratorResult has findings from successful agent plus error record
- All agents fail: OrchestratorResult has zero findings and agent errors
- Split mode with 4 files and 2 agents: each agent gets 2 files
- All mode with 4 files and 2 agents: each agent gets 4 files
- Agent output with JSON embedded in markdown: jsonutil extracts correctly
- Agent output with no valid JSON: treated as error, reported in AgentErrors
- Context cancelled mid-review: partial results returned
- DryRun produces expected output without invoking agents
- ReviewEvents emitted in correct order: review_started -> agent_started (per agent) -> agent_completed (per agent) -> consolidated
- Rate-limited agent: event emitted, error captured

### Integration Tests
- Full review pipeline with mock agents producing canned JSON output
- Review with real DiffResult from a test git repo

### Edge Cases to Handle
- Zero agents specified: return error before starting
- Single agent (no parallelism needed, but should still work through errgroup path)
- Agent produces valid JSON but with unexpected extra fields (should be tolerated)
- Agent produces multiple JSON objects in output (use first valid ReviewResult)
- Very slow agent combined with fast agent: fast agent's results available immediately
- All agents rate-limited simultaneously
- Concurrency set higher than number of agents (should work, just no queuing)

## Implementation Notes
### Recommended Approach
1. Validate inputs: at least one agent, concurrency > 0, diff generates successfully
2. Generate diff via DiffGenerator
3. If split mode, divide files with SplitFiles; if all mode, each agent gets all files
4. Create errgroup with context: `g, ctx := errgroup.WithContext(ctx)`
5. Set concurrency limit: `g.SetLimit(concurrency)`
6. For each agent, launch `g.Go(func() error { ... })`:
   a. Emit `agent_started` event
   b. Build prompt via PromptBuilder.BuildForAgent
   c. Invoke agent via `agent.Run(ctx, opts)`
   d. Extract JSON from RunResult.Stdout using `jsonutil.ExtractInto`
   e. Validate extracted ReviewResult
   f. Append AgentReviewResult to mutex-protected results slice
   g. Emit `agent_completed` or `agent_error` event
   h. Return nil (do not propagate per-agent errors to errgroup -- collect them instead)
7. Wait for all workers: `g.Wait()`
8. Pass all results to Consolidator.Consolidate
9. Build and return OrchestratorResult

### Potential Pitfalls
- Do NOT return errors from individual worker goroutines to errgroup -- errgroup.Wait() returns the first error and cancels the context. Instead, always return nil from workers and collect errors in a separate slice
- The mutex-protected results slice must be properly sized or use a buffered channel instead
- JSON extraction may fail if the agent uses a different output format -- always capture raw output for debugging
- Agent names must match registry keys exactly -- validate before launching workers
- Ensure events channel is buffered or consumed by a separate goroutine to prevent blocking workers

### Security Considerations
- Review prompts may contain sensitive code from the diff -- ensure prompts are not logged at info level
- Agent subprocess execution inherits environment variables -- no additional secrets should be passed

## References
- [PRD Section 5.5 - Multi-Agent Review Pipeline](docs/prd/PRD-Raven.md)
- [errgroup documentation](https://pkg.go.dev/golang.org/x/sync/errgroup)
- [errgroup best practices with SetLimit](https://oneuptime.com/blog/post/2026-01-07-go-errgroup/view)
