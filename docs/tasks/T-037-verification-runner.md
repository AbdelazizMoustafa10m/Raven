# T-037: Verification Command Runner

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-009 |
| Blocked By | T-009 |
| Blocks | T-038, T-039 |

## Goal
Implement a verification command runner that executes a sequence of configurable verification commands (e.g., `go build ./...`, `go test ./...`, `go vet ./...`) after fix application, captures pass/fail results per command, enforces timeouts, and produces a structured verification report. This component is shared between the fix engine (T-038) and PR creation (T-041).

## Background
Per PRD Section 5.6, the fix engine runs configurable verification commands after applying fixes and reports verification results (pass/fail per command). The verification commands come from `raven.toml` under `project.verification_commands`. Per PRD Section 5.7, verification results are also included in the PR body. This component is extracted as a standalone runner because it is used in multiple contexts: after fix cycles, before PR creation, and potentially as a standalone `raven verify` command.

## Technical Specifications
### Implementation Approach
Create `internal/review/verify.go` containing a `VerificationRunner` that takes a list of commands from config, executes each sequentially via `os/exec`, captures stdout/stderr and exit codes, enforces per-command timeouts, and produces a `VerificationReport` with per-command results. Each command runs in the project's working directory with the project's environment.

### Key Components
- **VerificationRunner**: Executes verification commands and collects results
- **CommandResult**: Result of a single verification command (pass/fail, output, duration)
- **VerificationReport**: Aggregated results from all verification commands
- **VerificationStatus**: Overall pass/fail status for the suite

### API/Interface Contracts
```go
// internal/review/verify.go

type VerificationStatus string

const (
    VerificationPassed VerificationStatus = "passed"
    VerificationFailed VerificationStatus = "failed"
)

type CommandResult struct {
    Command   string
    ExitCode  int
    Stdout    string
    Stderr    string
    Duration  time.Duration
    Passed    bool
    TimedOut  bool
}

type VerificationReport struct {
    Status    VerificationStatus
    Results   []CommandResult
    Duration  time.Duration
    Passed    int
    Failed    int
    Total     int
}

type VerificationRunner struct {
    commands    []string
    workDir     string
    timeout     time.Duration // per-command timeout
    logger      *log.Logger
}

func NewVerificationRunner(
    commands []string,
    workDir string,
    timeout time.Duration,
    logger *log.Logger,
) *VerificationRunner

// Run executes all verification commands sequentially and returns a report.
// Stops at first failure if stopOnFailure is true.
func (vr *VerificationRunner) Run(ctx context.Context, stopOnFailure bool) (*VerificationReport, error)

// RunSingle executes a single command and returns its result.
func (vr *VerificationRunner) RunSingle(ctx context.Context, command string) (*CommandResult, error)

// FormatReport produces a human-readable summary of the verification results.
func (vr *VerificationReport) FormatReport() string

// FormatMarkdown produces a markdown-formatted verification section for PR bodies.
func (vr *VerificationReport) FormatMarkdown() string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| os/exec | stdlib | Subprocess execution for verification commands |
| context | stdlib | Timeout and cancellation for commands |
| strings | stdlib | Command parsing (split command string into args) |
| time | stdlib | Duration tracking and timeout enforcement |
| internal/config (T-009) | - | verification_commands from project config |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Executes each verification command via `os/exec` in the project working directory
- [ ] Captures stdout, stderr, and exit code for each command
- [ ] Marks each command as passed (exit code 0) or failed (non-zero exit code)
- [ ] Enforces per-command timeout using `context.WithTimeout`
- [ ] Timed-out commands are marked with `TimedOut: true` and `Passed: false`
- [ ] VerificationReport.Status is `passed` only if all commands pass
- [ ] Supports `stopOnFailure` mode that aborts after first failing command
- [ ] FormatReport produces readable terminal output with pass/fail indicators
- [ ] FormatMarkdown produces GitHub-compatible markdown with code blocks for output
- [ ] Context cancellation stops the currently running command and returns partial results
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- All commands pass: report status is "passed", all results show Passed=true
- First command fails: report status is "failed", first result shows Passed=false
- Second of three commands fails with stopOnFailure=false: all three run, report status "failed"
- Second of three commands fails with stopOnFailure=true: only first two run
- Command times out: result shows TimedOut=true, Passed=false
- Context cancelled during execution: partial results returned
- Empty command list: report with zero results, status "passed"
- Command with arguments parsed correctly (e.g., "go test ./...")
- FormatReport produces expected terminal output
- FormatMarkdown produces valid markdown with code blocks

### Integration Tests
- Run real verification commands ("echo hello", "false") and verify results
- Run "go vet ./..." in a test Go module directory

### Edge Cases to Handle
- Command not found on PATH: error captured in stderr, exit code non-zero
- Command that produces very large output (>1MB): truncate output with note
- Command that hangs indefinitely: timeout kills it
- Commands with shell-specific syntax (pipes, redirects): must be run via `sh -c`
- Commands with environment variables ($GOPATH): inherit from parent process
- Empty command string: skip with warning

## Implementation Notes
### Recommended Approach
1. For each command string, split into executable and arguments using `strings.Fields` for simple cases
2. For commands with shell features (pipes, redirects, globbing like `./...`), use `exec.CommandContext(ctx, "sh", "-c", command)` to run via shell
3. Create a `context.WithTimeout` wrapping the parent context for per-command timeout
4. Capture stdout and stderr using `cmd.CombinedOutput()` or separate pipes
5. Record start time, run command, record end time, compute duration
6. Build CommandResult from the execution
7. After all commands, compute aggregate VerificationReport

### Potential Pitfalls
- `strings.Fields("go test ./...")` correctly splits into `["go", "test", "./..."]` but commands with quoted arguments (e.g., `grep "hello world"`) require shell parsing -- always use `sh -c` to be safe
- On Windows, use `cmd /c` instead of `sh -c` -- detect OS at runtime
- `os/exec` does not use shell by default, so glob patterns like `./...` are handled by the go tool, not the shell. However, other commands may require shell expansion
- Timeout cleanup: when context times out, the process is killed but child processes may linger -- use `cmd.Process.Kill()` and `cmd.Wait()` for cleanup
- Output truncation: for very large output, keep the first and last N lines with a truncation message

### Security Considerations
- Verification commands come from `raven.toml` which is project-controlled -- no user input injection risk
- Do not allow arbitrary commands from CLI flags without warning
- Commands run with the same privileges as the Raven process -- no elevation

## References
- [PRD Section 5.6 - Runs configurable verification commands](docs/prd/PRD-Raven.md)
- [PRD Section 5.7 - Verification results in PR body](docs/prd/PRD-Raven.md)
- [Go os/exec documentation](https://pkg.go.dev/os/exec)
- [Go context.WithTimeout documentation](https://pkg.go.dev/context#WithTimeout)
