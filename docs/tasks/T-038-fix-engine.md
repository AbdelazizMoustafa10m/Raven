# T-038: Review Fix Engine

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 14-20hrs |
| Dependencies | T-021, T-031, T-034, T-036, T-037 |
| Blocked By | T-021, T-031, T-034, T-037 |
| Blocks | T-042 |

## Goal
Implement the review fix engine that takes consolidated review findings, generates a structured fix prompt containing findings, affected file contents, project conventions, and verification commands, invokes an AI agent to apply fixes, runs verification commands to confirm correctness, and supports iterative fix-review cycles up to a configurable maximum. This closes the automated review-fix loop.

## Background
Per PRD Section 5.6, the fix engine is an automated system that takes consolidated review findings as input, generates a fix prompt, invokes an agent to apply fixes, and verifies the result. It supports `--max-fix-cycles <n>` for iterative fix-review loops where after each fix attempt, verification commands are run and if they fail, another fix cycle is attempted. The fix prompt includes the git diff of what changed so the agent knows what to fix. Dry-run mode shows the fix prompt without executing.

## Technical Specifications
### Implementation Approach
Create `internal/review/fix.go` containing a `FixEngine` that receives a `ConsolidatedReview`, generates a fix prompt using `text/template`, invokes the configured agent to apply fixes, runs verification commands via the VerificationRunner (T-037), and iterates up to `maxCycles` if verification fails. Each cycle generates a new prompt that includes previous verification failures. The engine emits events for TUI consumption and produces a `FixReport` with the outcome of all cycles.

### Key Components
- **FixEngine**: Orchestrates the fix-verify cycle
- **FixPromptBuilder**: Constructs fix prompts from findings, diff, and conventions
- **FixCycleResult**: Result of a single fix attempt (agent result + verification result)
- **FixReport**: Aggregate report of all fix cycles

### API/Interface Contracts
```go
// internal/review/fix.go

type FixEngine struct {
    agent           agent.Agent
    verifier        *VerificationRunner
    promptBuilder   *FixPromptBuilder
    maxCycles       int
    logger          *log.Logger
    events          chan<- FixEvent
}

type FixOpts struct {
    Findings          []*Finding
    ReviewReport      string              // Markdown review report for context
    BaseBranch        string
    DryRun            bool
    MaxCycles         int                 // Override default max cycles
}

type FixEvent struct {
    Type      string // fix_started, cycle_started, agent_invoked, verification_started, verification_result, cycle_completed, fix_completed
    Cycle     int
    Message   string
    Timestamp time.Time
}

type FixCycleResult struct {
    Cycle              int
    AgentResult        *agent.RunResult
    Verification       *VerificationReport
    DiffAfterFix       string              // git diff showing what the agent changed
    Duration           time.Duration
}

type FixReport struct {
    Cycles          []FixCycleResult
    FinalStatus     VerificationStatus
    TotalCycles     int
    FixesApplied    bool
    Duration        time.Duration
}

type FixPromptBuilder struct {
    conventions     []string // from raven.toml or CLAUDE.md
    verifyCommands  []string // from raven.toml project.verification_commands
    logger          *log.Logger
}

func NewFixEngine(
    agent agent.Agent,
    verifier *VerificationRunner,
    maxCycles int,
    logger *log.Logger,
    events chan<- FixEvent,
) *FixEngine

// Fix runs the fix-verify cycle:
// 1. Generate fix prompt from findings
// 2. Invoke agent
// 3. Run verification commands
// 4. If verification fails and cycles remain, repeat with updated prompt
// 5. Return FixReport
func (fe *FixEngine) Fix(ctx context.Context, opts FixOpts) (*FixReport, error)

// DryRun generates and returns the fix prompt without executing the agent.
func (fe *FixEngine) DryRun(ctx context.Context, opts FixOpts) (string, error)

func NewFixPromptBuilder(
    conventions []string,
    verifyCommands []string,
    logger *log.Logger,
) *FixPromptBuilder

// Build constructs a fix prompt from findings, diff, and previous failures.
func (fpb *FixPromptBuilder) Build(
    findings []*Finding,
    diff string,
    previousFailures []FixCycleResult,
) (string, error)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/agent (T-021) | - | Agent interface for invoking fix agent |
| internal/review (T-031) | - | Finding, ConsolidatedReview types |
| internal/review/verify (T-037) | - | VerificationRunner for post-fix verification |
| internal/git (T-015) | - | Git diff generation after fix for next cycle |
| text/template | stdlib | Fix prompt template rendering |
| time | stdlib | Duration tracking |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Generates a fix prompt containing: review findings, affected file list, file contents (or diff), project conventions, and verification commands
- [ ] Invokes the configured agent with the fix prompt
- [ ] Runs verification commands after agent completes using VerificationRunner
- [ ] If verification passes, returns success after one cycle
- [ ] If verification fails and cycles remain, generates updated prompt with previous failure context and retries
- [ ] Stops after `maxCycles` attempts even if verification still fails
- [ ] FixReport contains results from all cycles
- [ ] FinalStatus reflects the last cycle's verification status
- [ ] DryRun returns the fix prompt without invoking the agent or running verification
- [ ] Emits FixEvent messages at each stage for TUI consumption
- [ ] Context cancellation stops the current cycle and returns partial results
- [ ] Unit tests achieve 85% coverage using mock agent

## Testing Requirements
### Unit Tests
- Single cycle, verification passes: FixReport with 1 cycle, FinalStatus=passed
- Single cycle, verification fails, maxCycles=1: FixReport with 1 cycle, FinalStatus=failed
- Two cycles, first fails, second passes: FixReport with 2 cycles, FinalStatus=passed
- Three cycles, all fail, maxCycles=3: FixReport with 3 cycles, FinalStatus=failed
- DryRun returns prompt string without invoking agent
- Fix prompt includes all finding descriptions and affected files
- Fix prompt includes verification commands
- Updated prompt for cycle 2 includes cycle 1 failure output
- Context cancelled during agent invocation: partial result returned
- Zero findings: no-op, returns empty FixReport with status=passed
- Agent error (non-zero exit): cycle marked as failed, moves to next cycle

### Integration Tests
- Fix cycle with mock agent that "fixes" a failing test (modifies testdata)
- Full cycle with real verification commands on a test project

### Edge Cases to Handle
- Agent modifies files that were not in the findings (acceptable but should be noted)
- Agent creates new files during fix (should be included in diff)
- Verification command itself crashes (not just fails): handle gracefully
- Fix prompt exceeds agent's context window: truncate findings by priority
- Agent produces no changes (empty diff after fix): cycle should be noted as "no changes applied"
- maxCycles=0: skip fix entirely, return empty report

## Implementation Notes
### Recommended Approach
1. Generate fix prompt using FixPromptBuilder:
   - List each finding with file, line, severity, description, and suggestion
   - Include the relevant diff section or affected file contents
   - Include project conventions (from CLAUDE.md or raven.toml)
   - Include verification commands with instruction to run them after fixing
2. For each cycle:
   a. Emit `cycle_started` event
   b. Invoke agent with the fix prompt
   c. Capture the git diff of what the agent changed
   d. Run VerificationRunner
   e. If verification passes, break the loop
   f. If verification fails, build updated prompt with failure context
   g. Emit `cycle_completed` event
3. Build FixReport from all cycle results
4. Emit `fix_completed` event

### Potential Pitfalls
- The fix prompt must not include the full file contents for all findings -- this could exceed context limits. Include the relevant diff hunks or file excerpts instead
- Previous failure context in the retry prompt should be concise -- include only the failing command and its error output, not the full stdout
- The agent may introduce new issues while fixing old ones -- verification catches this but the prompt should warn against it
- Between cycles, check that the agent actually made changes (empty diff = wasted cycle)

### Security Considerations
- The fix agent runs with the same permissions as the review agent -- no privilege escalation
- Fix prompt should not include secrets from the codebase even if they appear in findings
- Verification commands are from config, not from agent output -- do not execute agent-suggested commands

## References
- [PRD Section 5.6 - Review Fix Engine](docs/prd/PRD-Raven.md)
- [Go os/exec documentation](https://pkg.go.dev/os/exec)
- [Go text/template documentation](https://pkg.go.dev/text/template)
