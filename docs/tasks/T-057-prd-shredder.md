# T-057: PRD Shredder -- Single Agent Call Producing Epic-Level JSON

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-056 |
| Blocked By | T-056 |
| Blocks | T-059 |

## Goal
Implement the "shred" phase (Phase 1) of the PRD decomposition workflow: a single agent call that reads a PRD file and produces a structured epic-level JSON breakdown. This is the entry point of the scatter-gather pipeline that divides a monolithic PRD into parallelizable epic-sized work units.

## Background
Per PRD Section 5.8, Phase 1 is a single agent invocation that reads the entire PRD and produces an `EpicBreakdown` JSON structure. The shredder constructs a prompt containing the PRD text, the target JSON schema, and instructions for epic identification. It invokes the configured agent, extracts the JSON from the output (using the same JSON extraction logic from the review pipeline), validates against the epic schema (T-056), and retries up to 3 times on malformed output with augmented error context.

The agent writes structured JSON to a file (per PRD: "agents write JSON to a file rather than stdout") to maximize reliability across agent types.

## Technical Specifications
### Implementation Approach
Create `internal/prd/shredder.go` containing a `Shredder` struct that takes an `agent.Agent`, the PRD file path, and a working directory. It constructs the shred prompt using `text/template`, invokes the agent with instructions to write JSON to a designated output file, reads and validates the output, and retries on failure.

### Key Components
- **Shredder**: Orchestrates the single-agent PRD-to-epics call
- **ShredPrompt**: Template for the shred prompt with PRD content, JSON schema, and instructions
- **ShredResult**: Contains the validated EpicBreakdown and metadata (duration, retries)

### API/Interface Contracts
```go
// internal/prd/shredder.go

type Shredder struct {
    agent      agent.Agent
    workDir    string
    maxRetries int
    logger     *log.Logger
}

type ShredOpts struct {
    PRDPath       string
    OutputFile    string   // e.g., $workDir/epic-breakdown.json
    Model         string   // override agent default
    Effort        string   // override agent default
}

type ShredResult struct {
    Breakdown *EpicBreakdown
    Duration  time.Duration
    Retries   int
    OutputFile string
}

func NewShredder(agent agent.Agent, workDir string, opts ...ShredderOption) *Shredder
func (s *Shredder) Shred(ctx context.Context, opts ShredOpts) (*ShredResult, error)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/agent | - | Agent interface for LLM invocation |
| internal/prd (T-056) | - | EpicBreakdown schema and validation |
| text/template | stdlib | Prompt template rendering |
| encoding/json | stdlib | JSON parsing of agent output |
| os | stdlib | File I/O for PRD reading and JSON output |

## Acceptance Criteria
- [ ] Shredder reads a PRD markdown file and constructs a prompt with the full PRD content
- [ ] Agent is invoked with instructions to write epic JSON to a specific output file path
- [ ] Output JSON is parsed and validated against EpicBreakdown schema
- [ ] On validation failure, retries up to 3 times with validation errors appended to prompt
- [ ] On max retries exceeded, returns error with all validation attempts
- [ ] ShredResult contains the parsed EpicBreakdown, timing, and retry count
- [ ] Works with any agent implementing the agent.Agent interface
- [ ] Unit tests achieve 90% coverage using mock agent
- [ ] Emits structured events for progress tracking (shred_started, shred_completed, shred_retry)

## Testing Requirements
### Unit Tests
- Shredder with mock agent producing valid JSON returns ShredResult with 0 retries
- Shredder with mock agent producing invalid JSON retries and succeeds on second attempt
- Shredder with mock agent producing invalid JSON for all attempts returns error with details
- PRD file not found returns clear error
- Agent returning rate-limit error propagates correctly
- Context cancellation stops retry loop
- Prompt template contains full PRD content

### Integration Tests
- End-to-end shred with a sample PRD file (using mock agent with canned output)

### Edge Cases to Handle
- Very large PRD files (>100KB) -- ensure prompt is not truncated
- PRD with non-UTF8 characters
- Agent output containing JSON embedded in markdown fences
- Agent writing to wrong file path (fallback to stdout extraction)
- Empty PRD file

## Implementation Notes
### Recommended Approach
1. Read PRD file content with os.ReadFile
2. Render prompt template with PRD content, JSON schema example, and output file path
3. Invoke agent with prompt, setting WorkDir to the working directory
4. Check if output file exists; if not, attempt JSON extraction from agent stdout (fallback)
5. Parse JSON and validate with EpicBreakdown.Validate()
6. On validation failure, append FormatValidationErrors() to prompt and retry
7. Return ShredResult on success

### Potential Pitfalls
- Agents may wrap JSON in markdown code fences (```json ... ```) -- use the JSON extraction utility from the review pipeline (internal/review/extract.go)
- Some agents may not reliably write to file -- always have stdout fallback extraction
- PRD content in the prompt needs careful escaping if using template actions

### Security Considerations
- Validate PRD file path is within project directory (no path traversal)
- Cap PRD file size at 1MB to prevent memory issues in prompt construction

## References
- [PRD Section 5.8 - Phase 1 Shred](docs/prd/PRD-Raven.md)
- [Go text/template documentation](https://pkg.go.dev/text/template)
