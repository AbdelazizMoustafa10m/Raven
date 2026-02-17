# T-050: Pipeline Orchestrator Core -- Multi-Phase Lifecycle

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 14-20hrs |
| Dependencies | T-045, T-046, T-049, T-015 |
| Blocked By | T-045, T-049 |
| Blocks | T-051, T-052, T-053, T-054, T-055 |

## Goal
Implement the core pipeline orchestrator that chains the implement -> review -> fix -> PR lifecycle across multiple phases. The orchestrator manages phase iteration, delegates to the workflow engine for per-phase execution, tracks pipeline-level metadata, and supports skip flags and agent selection per stage. This is the highest-level orchestrator in Raven, composing all lower-level subsystems into an end-to-end automated pipeline.

## Background
Per PRD Section 5.9, the pipeline orchestrator:
- "Chains implement -> review -> fix -> PR across multiple phases"
- Supports `--phase <id>`, `--phase all`, `--from-phase <id>`
- Skip flags: `--skip-implement`, `--skip-review`, `--skip-fix`, `--skip-pr`
- Agent selection per stage: `--impl-agent`, `--review-agent`, `--fix-agent`
- Review concurrency: `--review-concurrency`
- Maximum review/fix cycles: `--max-review-cycles`
- "Pipeline metadata tracking: implementation status, review verdict, fix status, PR URL per phase"
- "Resumability: pipeline state is checkpointed per-phase, resume picks up at the failed phase"
- "Pipeline orchestration is itself a workflow (using the generic workflow engine)"

The orchestrator lives in `internal/pipeline/orchestrator.go` and uses the workflow engine (T-045) to run the `implement-review-pr` workflow for each phase.

## Technical Specifications
### Implementation Approach
Create `internal/pipeline/orchestrator.go` with a `PipelineOrchestrator` struct that takes configuration, the workflow engine, agents, git client, and pipeline options. The orchestrator resolves the phase range (single, all, or from-phase), iterates through phases, and for each phase: creates/switches to the phase branch, configures the workflow with phase-specific parameters and skip flags, runs the implement-review-pr workflow via the engine, records phase metadata, and advances to the next phase.

### Key Components
- **PipelineOrchestrator**: Top-level struct managing multi-phase execution
- **PipelineOpts**: Configuration struct with all pipeline flags (phase range, skip flags, agents, concurrency, max cycles)
- **PhaseResult**: Per-phase metadata tracking (status, review verdict, fix status, PR URL, duration)
- **PipelineState**: Extends WorkflowState.Metadata with pipeline-specific tracking (current phase, phase results, overall progress)
- **Phase iteration**: Resolves phase range from config, iterates with skip/resume support

### API/Interface Contracts
```go
// internal/pipeline/orchestrator.go

type PipelineOrchestrator struct {
    engine     *workflow.Engine
    store      *workflow.StateStore
    gitClient  *git.Client
    config     *config.Config
    logger     *log.Logger
    events     chan<- workflow.WorkflowEvent
}

type PipelineOpts struct {
    // Phase selection
    PhaseID       string // single phase ID, "all", or ""
    FromPhase     string // start from this phase ID
    
    // Skip flags
    SkipImplement bool
    SkipReview    bool
    SkipFix       bool
    SkipPR        bool
    
    // Agent selection per stage
    ImplAgent     string // agent name for implementation
    ReviewAgent   string // agent name for review
    FixAgent      string // agent name for fix
    
    // Concurrency and limits
    ReviewConcurrency int // number of concurrent review agents
    MaxReviewCycles   int // max review-fix iterations per phase
    
    // Execution modes
    DryRun       bool
    Interactive  bool // launch wizard first
}

type PhaseResult struct {
    PhaseID      string        `json:"phase_id"`
    PhaseName    string        `json:"phase_name"`
    Status       string        `json:"status"` // pending, implementing, reviewing, fixing, pr_created, completed, failed, skipped
    ImplStatus   string        `json:"impl_status"`
    ReviewVerdict string       `json:"review_verdict"`
    FixStatus    string        `json:"fix_status"`
    PRURL        string        `json:"pr_url"`
    BranchName   string        `json:"branch_name"`
    Duration     time.Duration `json:"duration"`
    Error        string        `json:"error,omitempty"`
}

type PipelineResult struct {
    Phases       []PhaseResult `json:"phases"`
    TotalDuration time.Duration `json:"total_duration"`
    Status       string        `json:"status"` // completed, partial, failed
}

func NewPipelineOrchestrator(
    engine *workflow.Engine,
    store *workflow.StateStore,
    gitClient *git.Client,
    cfg *config.Config,
    opts ...PipelineOption,
) *PipelineOrchestrator

// Run executes the pipeline across the resolved phase range.
func (p *PipelineOrchestrator) Run(ctx context.Context, opts PipelineOpts) (*PipelineResult, error)

// DryRun shows the planned pipeline execution without running anything.
func (p *PipelineOrchestrator) DryRun(opts PipelineOpts) string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/workflow (T-045) | - | Workflow engine for per-phase execution |
| internal/workflow (T-046) | - | StateStore for pipeline checkpointing |
| internal/workflow (T-049) | - | Built-in implement-review-pr definition |
| internal/task (T-018/T-019) | - | Phase config and task state |
| internal/git (T-015) | - | Git operations (branch management) |
| internal/config (T-009) | - | Configuration |
| charmbracelet/log | latest | Structured logging |

## Acceptance Criteria
- [ ] Orchestrator resolves phase range: single phase, all phases, from-phase
- [ ] For each phase: creates phase branch, runs implement-review-pr workflow, records result
- [ ] Skip flags work: --skip-implement skips implementation step in workflow
- [ ] Agent selection: --impl-agent, --review-agent, --fix-agent configure per-stage agents
- [ ] --review-concurrency passes through to review step handler
- [ ] --max-review-cycles limits review-fix iterations
- [ ] PhaseResult tracks: status, review verdict, fix status, PR URL, branch name, duration
- [ ] PipelineResult aggregates all phase results with overall status
- [ ] Pipeline state is checkpointed after each phase completion
- [ ] Resume picks up at the first incomplete phase
- [ ] DryRun mode shows planned phases, branches, and steps without executing
- [ ] Context cancellation stops pipeline between phases
- [ ] Exit code 0 (all phases succeeded), 2 (partial success), 1 (error)
- [ ] Unit tests achieve 80% coverage

## Testing Requirements
### Unit Tests
- Single phase pipeline: runs implement-review-pr for one phase, returns PhaseResult
- All phases pipeline (3 phases): runs all, returns 3 PhaseResults
- From-phase 2 of 3: runs phases 2 and 3
- Skip implement: review step runs directly (implementation skipped)
- Skip review and fix: goes from implement to PR
- Skip PR: stops after fix (no PR created)
- All stages skipped: phase marked as skipped
- Agent selection per stage: correct agents passed to workflow metadata
- Max review cycles: cycles limited, transitions to PR or failure after max
- Phase failure: records error in PhaseResult, overall status is "partial"
- Resume: loads pipeline state, starts from first incomplete phase
- DryRun: returns description without executing
- Context cancellation between phases: partial result returned

### Integration Tests
- Full pipeline run with mock agents and git operations
- Pipeline with checkpoint and resume across phases

### Edge Cases to Handle
- Phase range specified by name not found (clear error message)
- Phase with no tasks (skip silently with warning)
- Git branch creation failure (record error, do not proceed)
- Agent not configured for requested stage (fallback to default agent)
- All phases succeed on first try (happy path)
- First phase fails, second and third never start

## Implementation Notes
### Recommended Approach
1. Create `PipelineOrchestrator` struct with all dependencies
2. `Run()` implementation:
   a. Resolve phase range from PipelineOpts + config phases
   b. Load or create pipeline state (check StateStore for existing run)
   c. For each phase in range:
      - If already completed (from resumed state), skip
      - Create phase branch (T-051)
      - Configure workflow: set metadata with agents, skip flags, concurrency, max cycles
      - Get implement-review-pr definition from T-049
      - Modify definition based on skip flags (remove skipped steps from transitions)
      - Run workflow via engine
      - Record PhaseResult from workflow state
      - Checkpoint pipeline state
   d. Return PipelineResult
3. Pipeline state is stored as a special WorkflowState with metadata containing:
   - `"pipeline_phases"`: JSON array of PhaseResult
   - `"current_phase_index"`: int
   - `"pipeline_opts"`: serialized PipelineOpts
4. DryRun iterates phases and prints planned actions without executing

### Potential Pitfalls
- The pipeline is a meta-workflow that runs sub-workflows. Ensure the sub-workflow's state is separate from the pipeline's state (different run IDs)
- Skip flags should modify the workflow definition's transitions, not be checked inside step handlers. This keeps handlers pure.
- Phase-to-phase dependencies: the review step needs the implementation step's output. If implementation is skipped, review should work on whatever is already on the branch.
- Branch chaining (T-051): each phase's branch is based on the previous phase's branch. The pipeline orchestrator must coordinate this.

### Security Considerations
- Pipeline state may contain PR URLs and branch names -- stored in checkpoint files under .raven/
- Ensure agent names from CLI flags are validated against known agent names before use

## References
- [PRD Section 5.9 - Phase Pipeline Orchestrator](docs/prd/PRD-Raven.md)
- [PRD Section 5.1 - Pipeline is a built-in workflow](docs/prd/PRD-Raven.md)
- [PRD Section 7 - Phase 4 deliverable](docs/prd/PRD-Raven.md)