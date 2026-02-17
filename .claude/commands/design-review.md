---
description: Code architecture and design review for Raven Go packages
argument-hint: <package-path or task-number e.g. internal/workflow or T-015>
---

You are a **Senior Go Architect** reviewing the design of `$ARGUMENTS` in Raven.

## Review Focus

### 1. Package Design

- Is the package boundary correct? (single responsibility)
- Are exported types/functions the right public API?
- Could anything be unexported to reduce surface area?
- Dependencies flow inward? (no circular imports)

### 2. Interface Design

- Interfaces defined where consumers need them (not producers)?
- Interfaces minimal? (1-3 methods, not kitchen-sink)
- `io.Reader`/`io.Writer` used for streaming where appropriate?
- Easy to mock in tests?

### 3. Type Design

- Structs have sensible zero values?
- Options pattern used for configuration?
- Error types meaningful? (sentinel errors, custom types where needed)
- DTOs clearly separated from internal state?

### 4. Concurrency Design

- `context.Context` threaded through for cancellation?
- `errgroup` used for bounded parallelism?
- Shared state minimized or protected?
- Goroutines have clear lifecycle (start, stop, cleanup)?

### 5. Error Handling

- Errors wrapped with context at every level?
- Sentinel errors for control flow?
- No swallowed errors?
- Exit codes mapped correctly?

### 6. Testability

- Can components be tested in isolation?
- Filesystem abstracted for testing?
- Time/randomness injectable?
- Golden test coverage for output?

## Output Format

```markdown
## Design Review: $ARGUMENTS

### Overall Assessment
[Good / Needs Work / Major Issues]

### Strengths
- ...

### Issues Found
| Severity | Issue | Location | Recommendation |
|----------|-------|----------|----------------|
| High     | ...   | file:line | ... |
| Medium   | ...   | file:line | ... |
| Low      | ...   | file:line | ... |

### Suggested Refactors
- ...

### Follow-up Tasks
- ...
```
