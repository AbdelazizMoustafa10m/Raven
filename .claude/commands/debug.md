---
description: Debug and fix an issue in Raven with systematic root cause analysis
argument-hint: <description of the issue>
---

You are a **Senior Go Engineer** debugging: `$ARGUMENTS`

## Debugging Process

### 1. Reproduce & Understand

- What is the exact error message or unexpected behavior?
- What are the steps to reproduce?
- Check test output and log debug output

### 2. Locate the Problem

- Trace to source file and line
- Review recent changes to affected files
- Use compiler errors and `go vet` output as hints

### 3. Analyze Root Cause

**Compilation Errors:**
- Missing imports or dependencies
- Type mismatches
- Undefined symbols

**Runtime Errors:**
- Nil pointer dereference
- Index out of range
- Deadlock or goroutine leak

**Logic Errors:**
- Incorrect state transitions in workflow engine
- Wrong task selection order
- Rate-limit parsing mismatch
- Config resolution precedence wrong

**Test Failures:**
- Stale golden files (run with `-update`)
- Race conditions (`go test -race`)
- Environment-dependent paths

### 4. Implement Fix

- Make minimal change needed
- Match existing code style and patterns
- Add defensive checks if appropriate
- Write a test that would have caught this bug

### 5. Verify

```bash
go build ./cmd/raven/
go vet ./...
go test ./...
go test -race ./...
```

- Confirm issue is resolved
- Test related functionality still works

### 6. Prevent Recurrence

- Would a test catch this? Write one.
- Should types be tighter? Update them.
- Document if non-obvious gotcha.
