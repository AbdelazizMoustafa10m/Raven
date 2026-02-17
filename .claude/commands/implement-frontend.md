---
description: Orchestrate TUI implementation using specialized subagents (TUI engineer + testing engineer)
argument-hint: <task-number e.g. T-079>
---

# TUI Implementation Orchestrator

You are the **Orchestrator** coordinating the implementation of TUI task `$ARGUMENTS` for Raven using specialized subagents.

## Workflow Architecture

```
+-------------------------------------------------------------+
|  ORCHESTRATOR (You)                                          |
+-------------------------------------------------------------+
|  1. PRE-FLIGHT CHECKS                                       |
|     - Read PROGRESS.md -> Check if completed/blocked         |
|     - Read task spec -> Understand TUI requirements          |
|     - Verify dependencies -> Ensure blockers resolved        |
+-------------------------------------------------------------+
|  2. SPAWN: frontend-engineer                                 |
|     - Implements Bubble Tea model, views, keybindings        |
|     - Returns: files created, components, keybindings        |
+-------------------------------------------------------------+
|  3. SPAWN: testing-engineer                                  |
|     - Writes model update tests, view tests                  |
|     - Returns: test coverage, verification status            |
+-------------------------------------------------------------+
|  4. FINAL VERIFICATION                                       |
|     - go build ./cmd/raven/                                  |
|     - go vet ./...                                           |
|     - go test ./...                                          |
+-------------------------------------------------------------+
|  5. UPDATE PROGRESS.md                                       |
|     - Mark task complete                                     |
+-------------------------------------------------------------+
```

---

## Step 1: Pre-Flight Checks

### 1.1 Read Progress Log

Read `docs/tasks/PROGRESS.md`:
- Is `$ARGUMENTS` already completed? -> **STOP**
- Check dependency status

### 1.2 Read Task Specification

Read `docs/tasks/$ARGUMENTS-*.md` (glob pattern).

Extract:
- TUI component requirements
- Keybinding specifications
- Layout and styling requirements
- Integration points with workflow engine and agent adapters

### 1.3 Verify Dependencies

All blocking tasks must be complete. If blocked -> **STOP and report**.

---

## Step 2: Spawn TUI Engineer Subagent

**Use the Task tool to spawn the frontend-engineer agent:**

```
Task(
  subagent_type: "frontend-engineer",
  description: "Implement $ARGUMENTS TUI",
  prompt: """
  Implement TUI task $ARGUMENTS for Raven.

  TASK SPECIFICATION:
  [Paste full task spec]

  REQUIREMENTS:
  1. Bubble Tea Elm architecture (Model/Update/View)
  2. Lipgloss styling with consistent theme
  3. Responsive layout (handle terminal resize)
  4. Standard keybindings (Tab, j/k, p, s, q, ?)
  5. Integration with workflow events via tea.Msg

  Report: files created, keybindings, layout decisions.
  """
)
```

---

## Step 3: Spawn Testing Engineer Subagent

```
Task(
  subagent_type: "testing-engineer",
  description: "Test $ARGUMENTS TUI",
  prompt: """
  Write tests for TUI task $ARGUMENTS.

  IMPLEMENTATION SUMMARY:
  [Paste summary from frontend-engineer]

  REQUIREMENTS:
  1. Test Model.Update() for all key messages
  2. Test Model.View() output for different states
  3. Test responsive layout behavior
  4. Verify keybindings produce correct commands

  Report: test files, coverage, acceptance criteria.
  """
)
```

---

## Step 4: Final Verification

```bash
go build ./cmd/raven/    # Must compile
go vet ./...             # Zero warnings
go test ./...            # All tests pass
```

---

## Step 5: Update PROGRESS.md

```markdown
### $ARGUMENTS: <Task Title>

- **Status:** Completed
- **Date:** YYYY-MM-DD

**What was built:**
- [TUI component/feature]

**Files:**
- `internal/tui/file.go`

**Verification:**
- Build: pass | Vet: pass | Tests: pass
```

---

## Subagent Reference

| Subagent | Responsibility |
|----------|----------------|
| `frontend-engineer` | Bubble Tea models, lipgloss views, keybindings |
| `testing-engineer` | Model update tests, view rendering tests |
