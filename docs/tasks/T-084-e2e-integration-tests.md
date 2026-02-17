# T-084: End-to-End Integration Test Suite with Mock Agents

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 20-30hrs |
| Dependencies | T-006, T-010, T-013, T-018, T-024, T-031, T-043, T-056 |
| Blocked By | T-043 |
| Blocks | T-087 |

## Goal
Build a comprehensive end-to-end integration test suite that exercises Raven's complete workflows -- from CLI invocation through agent execution to output verification -- using mock agent scripts that simulate real AI agent behavior (including rate limits, errors, structured output, and completion signals). These tests validate that all components work together correctly without requiring actual AI agent API access.

## Background
Per PRD Section 7 (Phase 7), Raven requires an "End-to-end integration test suite." Per PRD Section 9 (Risks), "Extensive integration tests with mock agents that simulate rate limits, failures, and slow responses" is the primary mitigation for concurrent agent coordination edge cases. The PRD also calls out "Loss of bash edge-case handling during rewrite" as a high-impact risk, mitigated by "Side-by-side comparison testing."

The mock agents are shell scripts (or small Go programs) placed in `testdata/mock-agents/` that behave like the real `claude` and `codex` CLI tools: they accept the same flags, produce structured output, and can simulate rate limits, errors, and various exit codes. The test suite uses Go's `testing` package with `os/exec` to run the actual `raven` binary against these mock agents in temporary project directories.

## Technical Specifications
### Implementation Approach
Create mock agent scripts in `testdata/mock-agents/` that simulate `claude` and `codex` CLI behavior. Create an `e2e` test package at `tests/e2e/` with test functions that set up temporary project directories, configure Raven to use mock agents, run Raven commands, and verify outputs. Each test creates an isolated environment with its own `raven.toml`, task files, and agent configuration.

### Key Components
- **`testdata/mock-agents/claude`**: Mock Claude CLI script that produces configurable output
- **`testdata/mock-agents/codex`**: Mock Codex CLI script that produces configurable output
- **`testdata/mock-agents/rate-limited-agent`**: Agent that returns rate-limit messages on first N calls
- **`testdata/mock-agents/failing-agent`**: Agent that fails with configurable exit codes
- **`testdata/mock-projects/`**: Template project directories for different test scenarios
- **`tests/e2e/helpers_test.go`**: Test helper functions for project setup, assertion, and cleanup
- **`tests/e2e/implement_test.go`**: Implementation loop E2E tests
- **`tests/e2e/review_test.go`**: Review pipeline E2E tests
- **`tests/e2e/pipeline_test.go`**: Full pipeline E2E tests
- **`tests/e2e/prd_test.go`**: PRD decomposition E2E tests
- **`tests/e2e/resume_test.go`**: Checkpoint/resume E2E tests
- **`tests/e2e/config_test.go`**: Configuration and init E2E tests

### API/Interface Contracts
```bash
#!/usr/bin/env bash
# testdata/mock-agents/claude
# Mock Claude CLI that simulates agent behavior
# Controlled via environment variables:
#   MOCK_EXIT_CODE   - exit code to return (default: 0)
#   MOCK_OUTPUT_FILE - file containing stdout content to emit
#   MOCK_STDERR_FILE - file containing stderr content to emit
#   MOCK_RATE_LIMIT  - if "true", emit rate-limit message and exit 1
#   MOCK_DELAY       - sleep duration before responding (default: 0)
#   MOCK_SIGNAL_FILE - file to write "called" to (for call counting)
set -euo pipefail

# Record invocation for test verification
if [[ -n "${MOCK_SIGNAL_FILE:-}" ]]; then
    echo "called:$*" >> "$MOCK_SIGNAL_FILE"
fi

# Simulate delay
if [[ -n "${MOCK_DELAY:-}" ]]; then
    sleep "$MOCK_DELAY"
fi

# Simulate rate limit
if [[ "${MOCK_RATE_LIMIT:-}" == "true" ]]; then
    echo "Your rate limit will reset in 30 seconds." >&2
    exit 1
fi

# Emit configured output
if [[ -n "${MOCK_OUTPUT_FILE:-}" ]] && [[ -f "$MOCK_OUTPUT_FILE" ]]; then
    cat "$MOCK_OUTPUT_FILE"
fi

if [[ -n "${MOCK_STDERR_FILE:-}" ]] && [[ -f "$MOCK_STDERR_FILE" ]]; then
    cat "$MOCK_STDERR_FILE" >&2
fi

# Emit completion signal
echo "PHASE_COMPLETE"

exit "${MOCK_EXIT_CODE:-0}"
```

```go
// tests/e2e/helpers_test.go
package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// testProject creates an isolated project directory with raven.toml and mock agents.
type testProject struct {
	Dir       string
	BinaryPath string
	t         *testing.T
}

func newTestProject(t *testing.T) *testProject {
	t.Helper()
	dir := t.TempDir()

	// Build raven binary
	binary := filepath.Join(dir, "raven")
	build := exec.Command("go", "build", "-o", binary, "./cmd/raven")
	build.Dir = projectRoot()
	out, err := build.CombinedOutput()
	require.NoError(t, err, "building raven: %s", string(out))

	// Copy mock agents
	mockDir := filepath.Join(dir, "mock-agents")
	copyMockAgents(t, mockDir)

	return &testProject{Dir: dir, BinaryPath: binary, t: t}
}

func (tp *testProject) writeConfig(content string) {
	tp.t.Helper()
	err := os.WriteFile(filepath.Join(tp.Dir, "raven.toml"), []byte(content), 0o644)
	require.NoError(tp.t, err)
}

func (tp *testProject) writeTaskSpec(id, content string) {
	tp.t.Helper()
	tasksDir := filepath.Join(tp.Dir, "docs", "tasks")
	os.MkdirAll(tasksDir, 0o755)
	err := os.WriteFile(filepath.Join(tasksDir, id+".md"), []byte(content), 0o644)
	require.NoError(tp.t, err)
}

func (tp *testProject) run(args ...string) *exec.Cmd {
	cmd := exec.Command(tp.BinaryPath, args...)
	cmd.Dir = tp.Dir
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(tp.Dir, "mock-agents")+":"+os.Getenv("PATH"),
	)
	return cmd
}

func (tp *testProject) runExpectSuccess(args ...string) string {
	tp.t.Helper()
	cmd := tp.run(args...)
	out, err := cmd.CombinedOutput()
	require.NoError(tp.t, err, "raven %v failed: %s", args, string(out))
	return string(out)
}

func (tp *testProject) runExpectFailure(args ...string) (string, int) {
	tp.t.Helper()
	cmd := tp.run(args...)
	out, err := cmd.CombinedOutput()
	require.Error(tp.t, err)
	exitErr, ok := err.(*exec.ExitError)
	require.True(tp.t, ok)
	return string(out), exitErr.ExitCode()
}
```

```go
// tests/e2e/implement_test.go
package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImplementSingleTask(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	tp.writeTaskSpec("T-001-setup", sampleTaskSpec("T-001", "Setup project", nil))
	initGitRepo(t, tp.Dir)

	out := tp.runExpectSuccess("implement", "--task", "T-001", "--agent", "claude", "--max-iterations", "1")

	assert.Contains(t, out, "T-001")
}

func TestImplementWithRateLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	tp := newTestProject(t)
	tp.writeConfig(rateLimitConfig("claude"))
	tp.writeTaskSpec("T-001-setup", sampleTaskSpec("T-001", "Setup project", nil))
	initGitRepo(t, tp.Dir)

	// Configure mock to rate-limit on first call, succeed on second
	setMockBehavior(t, tp.Dir, "claude", mockBehavior{
		RateLimitUntilCall: 2,
		RateLimitDuration:  "1s",
	})

	out := tp.runExpectSuccess("implement", "--task", "T-001", "--agent", "claude",
		"--max-iterations", "3", "--max-limit-waits", "2")

	assert.Contains(t, out, "rate limit")
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| testing | stdlib | Test framework |
| os/exec | stdlib | Binary and mock agent execution |
| stretchr/testify | v1.9+ | Test assertions |
| io/fs | stdlib | File system operations for test setup |

## Acceptance Criteria
- [ ] Mock `claude` agent script exists and accepts the same flags as real `claude` CLI
- [ ] Mock `codex` agent script exists and simulates Codex behavior
- [ ] Rate-limit simulation works correctly (configurable number of calls before succeeding)
- [ ] `raven version` E2E test passes
- [ ] `raven init go-cli` E2E test creates expected project structure
- [ ] `raven config debug` E2E test shows resolved configuration
- [ ] `raven implement --task T-001` E2E test completes with mock agent
- [ ] `raven implement --phase 1` E2E test iterates through multiple tasks
- [ ] `raven review --agents claude` E2E test produces review output
- [ ] `raven pipeline --phase 1 --dry-run` E2E test shows planned steps
- [ ] Rate-limit recovery E2E test verifies automatic retry after rate limit
- [ ] Checkpoint/resume E2E test verifies workflow can be interrupted and resumed
- [ ] Error handling E2E test verifies correct exit codes (1 for error, 3 for user-cancelled)
- [ ] All E2E tests are skipped with `-short` flag
- [ ] E2E tests run in under 5 minutes total
- [ ] `make test-e2e` target runs the E2E suite

## Testing Requirements
### Unit Tests
- Mock agent scripts produce correct output for each configuration mode
- Test helper functions create valid project structures

### Integration Tests
(These ARE the integration tests -- the E2E suite)
- **Config E2E**: `raven init`, `raven config debug`, `raven config validate`
- **Implement E2E**: Single task, phase iteration, dry-run, max-iterations limit
- **Review E2E**: Single-agent review, multi-agent review, review with findings
- **Fix E2E**: Apply fixes from review findings
- **Pipeline E2E**: Full pipeline dry-run, single phase execution
- **PRD E2E**: PRD decomposition with mock agent producing epic JSON
- **Resume E2E**: Interrupt workflow, verify checkpoint, resume from checkpoint
- **Error E2E**: Agent failure, config validation failure, missing prerequisites
- **Rate Limit E2E**: Rate-limit detection, backoff, retry, max-waits exceeded

### Edge Cases to Handle
- Mock agent called with unexpected flags (should not fail, log a warning)
- Empty task directory (no tasks to implement)
- Missing `raven.toml` (should use defaults or error clearly)
- Git repository not initialized (git operations should error gracefully)
- Concurrent test execution (each test uses its own temp directory)
- Long-running mock agents (test timeout should catch stuck processes)
- Mock agent that writes to files (simulating code changes)

## Implementation Notes
### Recommended Approach
1. Create mock agent shell scripts first -- these are the foundation of all E2E tests
2. Create the `testProject` helper struct and builder functions
3. Start with the simplest E2E test: `raven version`
4. Add `raven init go-cli` E2E test
5. Add `raven implement --task T-001` with mock agent
6. Add rate-limit recovery test
7. Add multi-task phase implementation test
8. Add review pipeline test
9. Add pipeline orchestration test (dry-run first, then actual execution)
10. Add checkpoint/resume test
11. Add error handling and edge case tests
12. Create `make test-e2e` target: `go test -v -count=1 ./tests/e2e/ -timeout 10m`

### Potential Pitfalls
- E2E tests are inherently slower than unit tests. Use `testing.Short()` guard and `-short` flag to skip in rapid iteration.
- Mock agent scripts must be executable (`chmod +x`). The test helper should set permissions.
- The `PATH` override in test environment must put mock agents before real agents to prevent accidental real API calls.
- Git operations in tests require a git repository. Use `git init` in the temp directory during setup.
- File system timing: on some OS/filesystem combinations, rapid file writes and reads may have visibility issues. Use `os.Sync()` where needed.
- Windows compatibility: mock agents are bash scripts and will not work on Windows. E2E tests should be skipped on Windows or use Go-based mock agents.
- Test parallelism: E2E tests can run in parallel if each uses its own temp directory. Use `t.Parallel()` where safe.

### Security Considerations
- Mock agents must NEVER call real AI APIs -- verify PATH override prevents this
- Test temp directories are cleaned up automatically by `t.TempDir()`
- No credentials or tokens should be required to run E2E tests
- Mock agents should not execute arbitrary commands from test input

## References
- [Go Testing Package](https://pkg.go.dev/testing)
- [testify Assertions](https://pkg.go.dev/github.com/stretchr/testify/assert)
- [Go Integration Testing Patterns](https://go.dev/doc/tutorial/add-a-test)
- [PRD Section 9: Risks and Mitigations](docs/prd/PRD-Raven.md)