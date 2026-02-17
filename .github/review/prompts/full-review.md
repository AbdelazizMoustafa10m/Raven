# Full Code Review

You are reviewing code changes for **Raven**, a Go-based AI workflow orchestration command center.

## Project Brief
{{PROJECT_BRIEF}}

## Changes to Review
{{DIFF}}

## Review Checklist

### Correctness
- Does the implementation match the task specification?
- Are edge cases handled (nil, empty, boundaries)?
- Any potential panics or unhandled errors?

### Go Idioms
- Errors wrapped with context (`fmt.Errorf`)
- No global mutable state
- Interfaces for testability
- Proper `context.Context` usage
- No unnecessary `init()` functions

### Concurrency
- Shared state protected (mutex/channels)?
- `errgroup` used correctly?
- No goroutine leaks?
- Context cancellation respected?

### Testing
- Table-driven tests with meaningful names?
- Edge cases covered?
- `t.Helper()` in test helpers?
- `t.TempDir()` for filesystem tests?

### Security
- No command injection in os/exec calls?
- File paths validated?
- No secrets in logs?

## Output Format

Respond with a JSON object:
```json
{
  "findings": [
    {
      "severity": "high|medium|low|info",
      "category": "correctness|style|performance|security|testing",
      "file": "path/to/file.go",
      "line": 42,
      "description": "What is wrong",
      "suggestion": "How to fix it"
    }
  ],
  "verdict": "APPROVED|CHANGES_NEEDED|BLOCKING",
  "summary": "Brief overall assessment"
}
```
