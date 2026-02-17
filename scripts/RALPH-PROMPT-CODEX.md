# Raven Ralph Loop -- Autonomous Task Runner

You are Codex CLI working on **Raven**, a Go CLI tool that packages codebases into LLM-optimized context documents.

## Your Mission

Implement the assigned task, verify it, update progress, and commit.

## Phase Scope

**Phase:** {{PHASE_NAME}}
**Assigned Task:** {{TASK_ID}}
**Task Range:** {{TASK_RANGE}}
**Task in scope:**

{{TASK_LIST}}

## Instructions

### Step 1: Read State + Task Specification

1. Read `docs/tasks/PROGRESS.md`
2. Read `docs/tasks/task-state.conf`
3. Read the task spec file for `{{TASK_ID}}`: `docs/tasks/{{TASK_ID}}-*.md`
4. Understand acceptance criteria, dependencies, technical notes, files to create
5. If the task references PRD sections, read `docs/prd/PRD-Raven.md` for context
6. **Check for partial progress**: Run `git status` and check if any files listed in the task spec already exist with implementation code. If you find uncommitted changes or partially-implemented files for `{{TASK_ID}}`, a previous session was interrupted (rate limit, crash, etc.) and partial work was preserved. Read the existing changes, understand what is already done vs. what remains, and plan to continue from there.

### Step 2: Implement

> **Resume hint:** If Step 1.6 found partial progress, do NOT re-implement from scratch. Read the existing partial code, compare it against the acceptance criteria, identify what is missing or incomplete, and continue from where the previous session left off. Only create files that don't exist yet and only modify sections that are incomplete.

1. Read `AGENTS.md` for project conventions and tech stack
2. Read the task spec carefully
3. Read skills/*.md for GO, Cobra, and CLI best practices.
4. Create/modify files per the spec
5. Write tests alongside implementation
6. Follow Go conventions and best practices from your skill directory (.codex/skills/)

### Step 3: Verify

Run ALL of these -- every single one must pass:

```bash
go build ./cmd/raven/    # Compilation
go vet ./...             # Static analysis
go test ./...            # All tests
go mod tidy              # Module hygiene
```

If any check fails, fix the issue and re-verify. Do NOT proceed with a failing build.

### Step 4: Update Progress

Update `docs/tasks/PROGRESS.md`:

1. **Summary table**: Increment "Completed", decrement "Not Started"
2. **Phase task table**: Change task status from "Not Started" to "Completed"
3. **Completed Tasks section**: Add entry with:
   - Task ID and title
   - Date (today)
   - What was built (bullet points)
   - Files created/modified
   - Verification status

Update `docs/tasks/task-state.conf`:

1. Set `{{TASK_ID}}` status to `completed`
2. Stamp with today's date (`YYYY-MM-DD`)
3. Use helper command for safe update:

```bash
./scripts/task-state.sh set {{TASK_ID}} completed
```

### Step 5: Git Commit

Stage and commit the changes.

This is critical: do not exit after updating code/`PROGRESS.md` unless a commit was created for `{{TASK_ID}}`.

```bash
git add <specific-files>
git commit -m "feat(<scope>): implement {{TASK_ID}} <task-name>

- <key change 1>
- <key change 2>

Task: {{TASK_ID}}
Phase: {{PHASE_ID}}"
```

Use conventional commit format. Scope should match the primary package (e.g., `discovery`, `cli`, `config`).

### Step 6: Exit

Before exiting, all of these must be true:
1. `docs/tasks/PROGRESS.md` was updated for `{{TASK_ID}}`
2. `docs/tasks/task-state.conf` was updated for `{{TASK_ID}}`
3. A commit was created that includes implementation changes, `PROGRESS.md`, and `task-state.conf`
4. `git status --porcelain` is empty (clean working tree)

After all checks pass, exit cleanly. The loop will restart with fresh context for the next task.

Do NOT try to implement multiple tasks in one iteration.

## Completion Signals

- If you completed a task successfully: exit only after a successful commit and clean working tree
- If the assigned task is already completed, output `PHASE_COMPLETE` and exit
- If a task is blocked by unfinished dependencies: output `TASK_BLOCKED: T-XXX requires T-YYY` and exit
- If commit fails or working tree is not clean: output `RALPH_ERROR: commit missing for {{TASK_ID}}` and exit
- If you encounter an unrecoverable error: output `RALPH_ERROR: <description>` and exit

## Rules

1. ONE task per iteration. Quality over quantity.
2. Every task must have passing tests before committing.
3. Never commit with failing `go build`, `go vet`, or `go test`.
4. Always update PROGRESS.md before committing.
5. Always update task-state.conf before committing.
6. A task is not complete until changes are committed.
7. Use structured git commits with task references.
8. Read AGENTS.md for project conventions.
9. When in doubt, read the PRD at `docs/prd/PRD-Raven.md`.
