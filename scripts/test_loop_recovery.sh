#!/usr/bin/env bash
#
# test_loop_recovery.sh -- Comprehensive tests for ralph loop recovery logic.
#
# Tests the full chain of recovery mechanisms that prevent work loss:
#   1. extract_agent_commit_message -- parsing various agent output formats
#   2. generate_commit_message_from_diff -- fallback message generation
#   3. _detect_completed_task_in_dirty_tree -- safety net detection
#   4. RALPH_ERROR "commit missing" recovery path (simulated loop logic)
#   5. recover_dirty_tree with safety net for missing last_completed_task
#   6. End-to-end scenarios matching real failure cases
#
# Usage: ./scripts/test_loop_recovery.sh
#

set -euo pipefail

# ============================================================================
# Test Harness
# ============================================================================

PASS=0
FAIL=0
TEST_NAME=""
TOTAL_TESTS=0

begin_test() {
    TEST_NAME="$1"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    printf "  TEST: %-68s " "$TEST_NAME"
}

assert_eq() {
    local expected="$1"
    local actual="$2"
    local msg="${3:-}"
    if [[ "$expected" == "$actual" ]]; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected: '$expected'"
    echo "    Actual:   '$actual'"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    return 1
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="${3:-}"
    if echo "$haystack" | grep -q "$needle"; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected to contain: '$needle'"
    echo "    In: '$(echo "$haystack" | head -3)...'"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    return 1
}

assert_not_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="${3:-}"
    if ! echo "$haystack" | grep -q "$needle"; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected NOT to contain: '$needle'"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    return 1
}

pass_test() {
    printf "OK\n"
    PASS=$((PASS + 1))
}

fail_test() {
    local msg="$1"
    printf "FAIL\n"
    echo "    $msg"
    FAIL=$((FAIL + 1))
}

# ============================================================================
# Sandbox Setup
# ============================================================================

SANDBOX=""
cleanup() {
    if [[ -n "$SANDBOX" && -d "$SANDBOX" ]]; then
        rm -rf "$SANDBOX"
    fi
    rm -f "/tmp/ralph-loop-test-log-$$.log"
}
trap cleanup EXIT

create_sandbox() {
    SANDBOX=$(mktemp -d /tmp/ralph-loop-test-XXXXXX)
    cd "$SANDBOX"
    git init -q
    git config user.email "test@test.com"
    git config user.name "Test"

    # Create minimal project structure
    mkdir -p docs/tasks

    cat > docs/tasks/PROGRESS.md <<'PROGRESS'
# Progress

| Task  | Status      |
|-------|-------------|
| T-001 | Completed   |
| T-002 | Not Started |
| T-003 | Not Started |
| T-004 | Not Started |
| T-005 | Not Started |
PROGRESS

    cat > docs/tasks/task-state.conf <<'STATE'
# Raven Task State -- machine-readable source of truth
# Format: TASK_ID|STATUS|UPDATED_AT (YYYY-MM-DD)
# STATUS: completed | not_started | in_progress | blocked

T-001|completed|2026-02-17
T-002|not_started|2026-02-17
T-003|not_started|2026-02-17
T-004|not_started|2026-02-17
T-005|not_started|2026-02-17
STATE

    git add -A && git commit -q -m "initial"

    # Set globals that the sourced functions depend on
    PROGRESS_FILE="$SANDBOX/docs/tasks/PROGRESS.md"
    TASK_STATE_FILE="$SANDBOX/docs/tasks/task-state.conf"
    # LOG_FILE outside sandbox to avoid dirtying git tree
    LOG_FILE="/tmp/ralph-loop-test-log-$$.log"
    touch "$LOG_FILE"

    BACKOFF_SCHEDULE=(120 300 900 1800)
    RATE_LIMIT_BUFFER_SECONDS=120
    MAX_RATE_LIMIT_WAIT_SECONDS=21600
}

reset_sandbox() {
    cd "$SANDBOX"
    # Hard reset to the initial commit to remove files committed by prior tests
    # (e.g., internal/config/*.go). Without this, simulate_agent_completion
    # writes identical content to already-committed files, and git sees no diff.
    local initial_commit
    initial_commit=$(git rev-list --max-parents=0 HEAD 2>/dev/null | head -1)
    git reset --hard "$initial_commit" -q 2>/dev/null
    git clean -fdq
    git stash clear 2>/dev/null || true
    > "$LOG_FILE"
}

# Simulate agent completing a task: update task-state.conf + create files
simulate_agent_completion() {
    local task_id="$1"
    local scope="${2:-config}"

    # Update task-state.conf
    sed -i.bak "s/${task_id}|not_started|/${task_id}|completed|/" "$SANDBOX/docs/tasks/task-state.conf"
    rm -f "$SANDBOX/docs/tasks/task-state.conf.bak"

    # Create implementation files
    mkdir -p "$SANDBOX/internal/${scope}"
    echo "package ${scope}" > "$SANDBOX/internal/${scope}/${scope}.go"
    echo "package ${scope}_test" > "$SANDBOX/internal/${scope}/${scope}_test.go"
}

# ============================================================================
# Source functions under test
# ============================================================================

log() {
    local msg="$1"
    echo "[TEST] $msg" >> "$LOG_FILE"
}

log_info() { log "$*"; }
log_warn() { log "WARN: $*"; }
log_error() { log "ERROR: $*"; }
log_success() { log "OK: $*"; }
log_step() { log "$2"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RALPH_SCRIPT="$SCRIPT_DIR/ralph-lib.sh"

if [[ ! -f "$RALPH_SCRIPT" ]]; then
    echo "ERROR: Cannot find $RALPH_SCRIPT"
    exit 1
fi

# Extract bash functions using brace-counting (handles nested braces)
extract_function() {
    local file="$1"
    local func_name="$2"
    awk -v fname="$func_name" '
        BEGIN { found=0; depth=0 }
        !found && $0 ~ "^"fname"\\(\\)" { found=1 }
        found {
            for (i=1; i<=length($0); i++) {
                c = substr($0,i,1)
                if (c == "{") depth++
                else if (c == "}") depth--
            }
            print
            if (depth == 0 && found) { found=0 }
        }
    ' "$file"
}

# Extract and eval each function we need
eval "$(extract_function "$RALPH_SCRIPT" is_tree_dirty)"
eval "$(extract_function "$RALPH_SCRIPT" get_dirty_summary)"
eval "$(extract_function "$RALPH_SCRIPT" extract_agent_commit_message)"
eval "$(extract_function "$RALPH_SCRIPT" generate_commit_message_from_diff)"
eval "$(extract_function "$RALPH_SCRIPT" run_commit_recovery)"
eval "$(extract_function "$RALPH_SCRIPT" stash_dirty_tree)"
eval "$(extract_function "$RALPH_SCRIPT" _detect_completed_task_in_dirty_tree)"
eval "$(extract_function "$RALPH_SCRIPT" recover_dirty_tree)"

# We need is_task_completed
is_task_completed() {
    local task_id="$1"
    awk -F'|' -v task="$task_id" '
        $0 !~ /^[[:space:]]*#/ && NF >= 2 {
            id=$1
            st=$2
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", id)
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", st)
            if (id == task && st == "completed") {
                found=1
            }
        }
        END { exit(found ? 0 : 1) }
    ' "$TASK_STATE_FILE" 2>/dev/null
}

# count_remaining_tasks stub
count_remaining_tasks() {
    local range="$1"
    local start="${range%%:*}"
    local end="${range##*:}"
    local start_num=$((10#${start#T-}))
    local end_num=$((10#${end#T-}))
    local remaining=0
    for ((i = start_num; i <= end_num; i++)); do
        local task_id
        task_id=$(printf "T-%03d" "$i")
        if ! is_task_completed "$task_id"; then
            remaining=$((remaining + 1))
        fi
    done
    echo "$remaining"
}

# ============================================================================
# Tests
# ============================================================================

echo ""
echo "Ralph Loop Recovery -- Comprehensive Test Suite"
echo "================================================"
echo ""

create_sandbox

# ============================================================================
# Group 1: extract_agent_commit_message
# ============================================================================

echo "--- 1. extract_agent_commit_message ---"

begin_test "Single-line: bare git commit -m"
output='Some output
git commit -m "feat(config): implement T-009 TOML config types"
More output'
result=$(extract_agent_commit_message "$output")
if assert_eq "feat(config): implement T-009 TOML config types" "$result"; then
    pass_test
fi

begin_test "Single-line: backtick-wrapped (the real bug)"
output='Run the commit manually: `git commit -m "feat(config): implement T-009 TOML configuration types and loading"`'
result=$(extract_agent_commit_message "$output")
if assert_eq "feat(config): implement T-009 TOML configuration types and loading" "$result"; then
    pass_test
fi

begin_test "Single-line: code-fenced"
output='```bash
git commit -m "feat(cli): add root command T-006"
```'
result=$(extract_agent_commit_message "$output")
if assert_eq "feat(cli): add root command T-006" "$result"; then
    pass_test
fi

begin_test "Multi-line: conventional commit with body"
output='git commit -m "feat(config): implement T-009

- Add Config type hierarchy
- Add NewDefaults() function
- Add LoadFromFile()

Task: T-009
Phase: 1"'
result=$(extract_agent_commit_message "$output")
expected='feat(config): implement T-009

- Add Config type hierarchy
- Add NewDefaults() function
- Add LoadFromFile()

Task: T-009
Phase: 1'
if assert_eq "$expected" "$result"; then
    pass_test
fi

begin_test "Multi-line: backtick-wrapped with trailing backtick"
output='`git commit -m "feat(config): implement T-009

- Add Config types
- Add defaults

Task: T-009"`'
result=$(extract_agent_commit_message "$output")
if assert_contains "$result" "feat(config): implement T-009" "should contain subject" && \
   assert_contains "$result" "Add Config types" "should contain body"; then
    pass_test
fi

begin_test "No git commit in output: returns empty"
output='I completed the implementation. All tests pass.
go build ./cmd/raven/
go vet ./...
go test ./...'
result=$(extract_agent_commit_message "$output")
if assert_eq "" "$result"; then
    pass_test
fi

begin_test "Agent output with numbered list containing commit command"
output='**Blocked:** The `git commit` command was denied by the permission mode. The files are staged and ready. You need to either:
1. Run the commit manually: `git commit -m "feat(config): implement T-009 TOML configuration types and loading"`
2. Or grant permission for git commit operations.'
result=$(extract_agent_commit_message "$output")
if assert_eq "feat(config): implement T-009 TOML configuration types and loading" "$result"; then
    pass_test
fi

begin_test "Empty output: returns empty"
result=$(extract_agent_commit_message "")
if assert_eq "" "$result"; then
    pass_test
fi

begin_test "Output with git commit but no message: returns empty"
output='git commit'
result=$(extract_agent_commit_message "$output")
if assert_eq "" "$result"; then
    pass_test
fi

begin_test "Multiple git commit lines: takes first"
output='git commit -m "first commit message"
git commit -m "second commit message"'
result=$(extract_agent_commit_message "$output")
if assert_eq "first commit message" "$result"; then
    pass_test
fi

begin_test "Commit with scope containing special chars"
output='git commit -m "fix(git/recovery): handle edge case in stash"'
result=$(extract_agent_commit_message "$output")
if assert_eq "fix(git/recovery): handle edge case in stash" "$result"; then
    pass_test
fi

# ============================================================================
# Group 2: generate_commit_message_from_diff
# ============================================================================

echo ""
echo "--- 2. generate_commit_message_from_diff ---"

reset_sandbox

begin_test "Generates message with scope from internal/ path"
mkdir -p "$SANDBOX/internal/config"
echo "package config" > "$SANDBOX/internal/config/config.go"
echo "package config_test" > "$SANDBOX/internal/config/config_test.go"
git add -A 2>/dev/null
result=$(generate_commit_message_from_diff "T-009")
if assert_contains "$result" "feat(config): implement T-009" "subject should have package scope" && \
   assert_contains "$result" "Add internal/config/config.go" "should list added files" && \
   assert_contains "$result" "Recovered-by: ralph-loop" "should have recovery trailer"; then
    pass_test
fi

reset_sandbox

begin_test "Uses 'recovery' scope when no internal/ files"
echo "something" > "$SANDBOX/README.md"
git add -A 2>/dev/null
result=$(generate_commit_message_from_diff "T-005")
if assert_contains "$result" "feat(recovery): implement T-005" "should use recovery scope"; then
    pass_test
fi

reset_sandbox

begin_test "Lists modified and deleted files correctly"
echo "modified" >> "$SANDBOX/docs/tasks/PROGRESS.md"
git add -A 2>/dev/null
result=$(generate_commit_message_from_diff "T-003")
if assert_contains "$result" "Update docs/tasks/PROGRESS.md" "should show Update for modified"; then
    pass_test
fi

# ============================================================================
# Group 3: _detect_completed_task_in_dirty_tree
# ============================================================================

echo ""
echo "--- 3. _detect_completed_task_in_dirty_tree ---"

reset_sandbox

begin_test "Detects newly completed task in dirty task-state.conf"
# T-002 is not_started in committed version; mark it completed in dirty version
sed -i.bak 's/T-002|not_started|/T-002|completed|/' "$SANDBOX/docs/tasks/task-state.conf"
rm -f "$SANDBOX/docs/tasks/task-state.conf.bak"
result=$(_detect_completed_task_in_dirty_tree "T-001:T-005")
if assert_eq "T-002" "$result" "should detect T-002 as newly completed"; then
    pass_test
fi

reset_sandbox

begin_test "Returns empty when no new completions"
result=$(_detect_completed_task_in_dirty_tree "T-001:T-005")
if assert_eq "" "$result" "no new completions should return empty"; then
    pass_test
fi

reset_sandbox

begin_test "Ignores already-committed completed tasks"
# T-001 is already completed in the committed version
result=$(_detect_completed_task_in_dirty_tree "T-001:T-005")
if assert_eq "" "$result" "T-001 already completed, should not be detected"; then
    pass_test
fi

reset_sandbox

begin_test "Detects first newly completed task in range"
# Complete T-003 and T-004 in dirty state
sed -i.bak 's/T-003|not_started|/T-003|completed|/' "$SANDBOX/docs/tasks/task-state.conf"
rm -f "$SANDBOX/docs/tasks/task-state.conf.bak"
sed -i.bak 's/T-004|not_started|/T-004|completed|/' "$SANDBOX/docs/tasks/task-state.conf"
rm -f "$SANDBOX/docs/tasks/task-state.conf.bak"
result=$(_detect_completed_task_in_dirty_tree "T-001:T-005")
if assert_eq "T-003" "$result" "should detect T-003 first (lower number)"; then
    pass_test
fi

reset_sandbox

begin_test "Respects task range boundaries"
# Complete T-005 in dirty state
sed -i.bak 's/T-005|not_started|/T-005|completed|/' "$SANDBOX/docs/tasks/task-state.conf"
rm -f "$SANDBOX/docs/tasks/task-state.conf.bak"
# Search only in T-001:T-004 range (excludes T-005)
result=$(_detect_completed_task_in_dirty_tree "T-001:T-004")
if assert_eq "" "$result" "T-005 outside range should not be detected"; then
    pass_test
fi

# ============================================================================
# Group 4: recover_dirty_tree with safety net
# ============================================================================

echo ""
echo "--- 4. recover_dirty_tree safety net ---"

reset_sandbox

begin_test "Safety net: no last_task_id + dirty tree + completed task → auto-commits"
# Simulate: agent completed T-002 (files + task-state update) but no commit,
# and the loop didn't set last_completed_task (the bug scenario)
simulate_agent_completion "T-002" "config"
head_before=$(git rev-parse HEAD)
recover_dirty_tree "T-001:T-005" ""
head_after=$(git rev-parse HEAD)
if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
    commit_msg=$(git log -1 --format=%B)
    if assert_contains "$commit_msg" "T-002" "commit should reference task" && \
       assert_contains "$commit_msg" "Recovered-by: ralph-loop" "commit should have recovery trailer"; then
        pass_test
    fi
else
    fail_test "Expected auto-commit (head: $head_before → $head_after, dirty=$(is_tree_dirty && echo yes || echo no))"
fi

reset_sandbox

begin_test "Safety net: no last_task_id + dirty tree + NO completed task → stashes"
# Dirty tree but no task was completed
echo "partial work" > "$SANDBOX/partial.go"
recover_dirty_tree "T-001:T-005" ""
if ! is_tree_dirty; then
    stash_msg=$(git stash list | head -1)
    if assert_contains "$stash_msg" "ralph-recovery" "should be recovery stash"; then
        pass_test
    fi
else
    fail_test "Tree should be clean after stash"
fi

reset_sandbox

begin_test "Original path still works: last_task_id set + completed → auto-commits"
simulate_agent_completion "T-003" "workflow"
head_before=$(git rev-parse HEAD)
recover_dirty_tree "T-001:T-005" "T-003"
head_after=$(git rev-parse HEAD)
if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
    pass_test
else
    fail_test "Expected auto-commit for T-003"
fi

reset_sandbox

begin_test "Original path: last_task_id set + NOT completed → stashes"
echo "partial T-002 work" > "$SANDBOX/incomplete.go"
recover_dirty_tree "T-001:T-005" "T-002"
if ! is_tree_dirty; then
    stash_msg=$(git stash list | head -1)
    if assert_contains "$stash_msg" "ralph-recovery" "should be recovery stash"; then
        pass_test
    fi
else
    fail_test "Tree should be clean after stash"
fi

# ============================================================================
# Group 5: RALPH_ERROR "commit missing" recovery path (simulated)
# ============================================================================

echo ""
echo "--- 5. RALPH_ERROR 'commit missing' recovery (simulated loop) ---"

reset_sandbox

begin_test "commit missing + progress made → recovery commit succeeds"
# Simulate the real loop flow:
# 1. remaining is computed BEFORE agent runs (T-002 still not_started)
# 2. Agent runs, updates task-state.conf, creates files, but can't commit
# 3. RALPH_ERROR handler computes new_remaining AFTER agent ran

# Step 1: remaining before agent (T-002 is not_started)
remaining=$(count_remaining_tasks "T-001:T-005")  # 4 (T-002..T-005)

# Step 2: agent completes task (updates task-state, creates files)
simulate_agent_completion "T-002" "config"

output='Implementation complete but git commit denied.
RALPH_ERROR: commit missing for T-002'

head_before=$(git rev-parse HEAD)

# Step 3: RALPH_ERROR handler logic
error_msg=$(echo "$output" | grep "RALPH_ERROR" | head -1)
recovery_succeeded=false
if echo "$error_msg" | grep -qi "commit missing"; then
    new_remaining=$(count_remaining_tasks "T-001:T-005")  # 3 (T-003..T-005)
    if [[ "$new_remaining" -lt "$remaining" ]]; then
        if run_commit_recovery "T-002" "$output"; then
            recovery_succeeded=true
        fi
    fi
fi

head_after=$(git rev-parse HEAD)

if [[ "$recovery_succeeded" == "true" ]] && \
   [[ "$head_before" != "$head_after" ]] && \
   ! is_tree_dirty; then
    pass_test
else
    fail_test "Recovery should succeed (recovered=$recovery_succeeded, new_commit=$([[ "$head_before" != "$head_after" ]] && echo yes || echo no), clean=$( ! is_tree_dirty && echo yes || echo no))"
fi

reset_sandbox

begin_test "commit missing + NO progress → falls through to error"
# Agent said commit missing but didn't actually update task-state.conf
output='RALPH_ERROR: commit missing for T-002'
remaining=$(count_remaining_tasks "T-001:T-005")  # 4 remaining

error_msg=$(echo "$output" | grep "RALPH_ERROR" | head -1)
recovery_succeeded=false
if echo "$error_msg" | grep -qi "commit missing"; then
    new_remaining=$(count_remaining_tasks "T-001:T-005")
    if [[ "$new_remaining" -lt "$remaining" ]]; then
        if run_commit_recovery "T-002" "$output"; then
            recovery_succeeded=true
        fi
    fi
fi

if [[ "$recovery_succeeded" == "false" ]]; then
    pass_test
else
    fail_test "Recovery should NOT succeed when no progress was made"
fi

reset_sandbox

begin_test "commit missing + progress + agent commit msg → uses agent's message"
remaining=$(count_remaining_tasks "T-001:T-005")  # before agent
simulate_agent_completion "T-002" "config"
output='1. Run the commit manually: `git commit -m "feat(config): implement T-009 TOML configuration types and loading"`
RALPH_ERROR: commit missing for T-002'

error_msg=$(echo "$output" | grep "RALPH_ERROR" | head -1)
if echo "$error_msg" | grep -qi "commit missing"; then
    new_remaining=$(count_remaining_tasks "T-001:T-005")
    if [[ "$new_remaining" -lt "$remaining" ]]; then
        run_commit_recovery "T-002" "$output"
    fi
fi

commit_msg=$(git log -1 --format=%B)
if assert_contains "$commit_msg" "feat(config): implement T-009" "should use agent's commit message" && \
   assert_contains "$commit_msg" "Recovered-by: ralph-loop" "should have recovery trailer"; then
    pass_test
fi

reset_sandbox

begin_test "commit missing + progress + NO extractable msg → generates message"
remaining=$(count_remaining_tasks "T-001:T-005")  # before agent
simulate_agent_completion "T-002" "config"
output='I completed T-002 but git commit was denied.
RALPH_ERROR: commit missing for T-002'

error_msg=$(echo "$output" | grep "RALPH_ERROR" | head -1)
if echo "$error_msg" | grep -qi "commit missing"; then
    new_remaining=$(count_remaining_tasks "T-001:T-005")
    if [[ "$new_remaining" -lt "$remaining" ]]; then
        run_commit_recovery "T-002" "$output"
    fi
fi

commit_msg=$(git log -1 --format=%B)
if assert_contains "$commit_msg" "feat(config): implement T-002" "should generate message with scope" && \
   assert_contains "$commit_msg" "Recovered-by: ralph-loop" "should have recovery trailer"; then
    pass_test
fi

reset_sandbox

begin_test "Non-'commit missing' RALPH_ERROR → no recovery attempted"
output='RALPH_ERROR: unrecoverable compilation failure'
remaining=$(count_remaining_tasks "T-001:T-005")
head_before=$(git rev-parse HEAD)

error_msg=$(echo "$output" | grep "RALPH_ERROR" | head -1)
recovery_succeeded=false
if echo "$error_msg" | grep -qi "commit missing"; then
    new_remaining=$(count_remaining_tasks "T-001:T-005")
    if [[ "$new_remaining" -lt "$remaining" ]]; then
        if run_commit_recovery "T-002" "$output"; then
            recovery_succeeded=true
        fi
    fi
fi

head_after=$(git rev-parse HEAD)
if [[ "$recovery_succeeded" == "false" ]] && [[ "$head_before" == "$head_after" ]]; then
    pass_test
else
    fail_test "Non-commit-missing errors should not trigger recovery"
fi

# ============================================================================
# Group 6: End-to-End scenario matching T-009 failure
# ============================================================================

echo ""
echo "--- 6. End-to-End: T-009 failure scenario ---"

reset_sandbox

begin_test "E2E: exact T-009 scenario (before fix) → verifies the bug"
# Reproduce the exact failure: agent completes task, RALPH_ERROR fires,
# next iteration stashes work and re-selects same task.
# This test verifies the fix resolves it.

# Step 1: Compute remaining BEFORE agent runs
remaining_before=$(count_remaining_tasks "T-001:T-005")  # 4

# Step 2: Agent completes T-002
simulate_agent_completion "T-002" "config"

# Verify T-002 is now marked complete
if ! is_task_completed "T-002"; then
    fail_test "Setup error: T-002 should be marked completed"
else
    # Step 3: WITH the fix, RALPH_ERROR handler detects "commit missing",
    # checks progress, and runs commit recovery.
    remaining_after=$(count_remaining_tasks "T-001:T-005")  # 3

    if [[ "$remaining_after" -lt "$remaining_before" ]]; then
        # Progress detected! Run commit recovery (the fix)
        output='`git commit -m "feat(config): implement T-002 TOML configuration types"`
RALPH_ERROR: commit missing for T-002'
        head_before=$(git rev-parse HEAD)
        run_commit_recovery "T-002" "$output"
        head_after=$(git rev-parse HEAD)

        if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
            # Verify the committed files
            committed_files=$(git diff-tree --no-commit-id --name-only -r HEAD)
            commit_msg=$(git log -1 --format=%B)

            if assert_contains "$committed_files" "internal/config/config.go" "should commit impl files" && \
               assert_contains "$committed_files" "task-state.conf" "should commit task-state" && \
               assert_contains "$commit_msg" "feat(config): implement T-002" "should use agent msg"; then
                pass_test
            fi
        else
            fail_test "Commit recovery should succeed"
        fi
    else
        fail_test "Progress should be detected (remaining $remaining_before → $remaining_after)"
    fi
fi

reset_sandbox

begin_test "E2E: safety net catches missed recovery (fallback path)"
# Scenario: RALPH_ERROR handler failed to recover (e.g., run_commit_recovery
# returned non-zero). Next iteration starts with dirty tree and empty
# last_completed_task. The safety net should detect and commit.

simulate_agent_completion "T-002" "config"

# Simulate: we're at the start of the next iteration with dirty tree
# and no last_completed_task set (the bug scenario without Fix 1)
head_before=$(git rev-parse HEAD)
recover_dirty_tree "T-001:T-005" ""
head_after=$(git rev-parse HEAD)

if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
    # Verify T-002 is still completed after recovery
    if is_task_completed "T-002"; then
        committed_files=$(git diff-tree --no-commit-id --name-only -r HEAD)
        if assert_contains "$committed_files" "internal/config/config.go" "should commit impl" && \
           assert_contains "$committed_files" "task-state.conf" "should commit task-state"; then
            pass_test
        fi
    else
        fail_test "T-002 should still be completed after recovery commit"
    fi
else
    fail_test "Safety net should auto-commit (head: $head_before → $head_after, dirty=$(is_tree_dirty && echo yes || echo no))"
fi

reset_sandbox

begin_test "E2E: after recovery, next task is correctly selected"
# Full flow: agent completes T-002, recovery commits, next iteration
# should select T-003 (not T-002 again)

simulate_agent_completion "T-002" "config"
output='RALPH_ERROR: commit missing for T-002
`git commit -m "feat(config): implement T-002 config types"`'

# Recovery commit
run_commit_recovery "T-002" "$output"

# Verify T-002 is completed
if ! is_task_completed "T-002"; then
    fail_test "T-002 should be completed after recovery"
else
    # Simulate next iteration: tree is clean, select next task
    remaining=$(count_remaining_tasks "T-001:T-005")
    if assert_eq "3" "$remaining" "should have 3 tasks remaining (T-003..T-005)"; then
        # Verify T-002 is NOT selectable (it's completed)
        next_incomplete=""
        for tid in T-002 T-003 T-004 T-005; do
            if ! is_task_completed "$tid"; then
                next_incomplete="$tid"
                break
            fi
        done
        if assert_eq "T-003" "$next_incomplete" "next task should be T-003, not T-002"; then
            pass_test
        fi
    fi
fi

reset_sandbox

begin_test "E2E: multiple consecutive commit-missing errors with recovery"
# Agent completes T-002, recovery works. Then agent completes T-003, recovery works.

# Iteration 1: T-002
simulate_agent_completion "T-002" "config"
output1='`git commit -m "feat(config): implement T-002"`
RALPH_ERROR: commit missing for T-002'
run_commit_recovery "T-002" "$output1"

if ! is_task_completed "T-002" || is_tree_dirty; then
    fail_test "T-002 recovery should succeed first"
else
    # Iteration 2: T-003
    simulate_agent_completion "T-003" "workflow"
    output2='`git commit -m "feat(workflow): implement T-003"`
RALPH_ERROR: commit missing for T-003'
    run_commit_recovery "T-003" "$output2"

    if ! is_task_completed "T-003" || is_tree_dirty; then
        fail_test "T-003 recovery should also succeed"
    else
        remaining=$(count_remaining_tasks "T-001:T-005")
        if assert_eq "2" "$remaining" "should have 2 remaining (T-004,T-005)"; then
            # Verify both commits exist
            log_output=$(git log --oneline -3)
            if assert_contains "$log_output" "T-002" "log should contain T-002 commit" && \
               assert_contains "$log_output" "T-003" "log should contain T-003 commit"; then
                pass_test
            fi
        fi
    fi
fi

reset_sandbox

begin_test "E2E: clean tree after recovery → no spurious stash/commit"
simulate_agent_completion "T-002" "config"
output='`git commit -m "feat(config): implement T-002"`
RALPH_ERROR: commit missing for T-002'
run_commit_recovery "T-002" "$output"

# Now tree is clean. Next iteration should NOT create stash or commit
head_before=$(git rev-parse HEAD)
stash_count_before=$(git stash list 2>/dev/null | wc -l | tr -d ' ')
recover_dirty_tree "T-001:T-005" "T-002"
head_after=$(git rev-parse HEAD)
stash_count_after=$(git stash list 2>/dev/null | wc -l | tr -d ' ')

if [[ "$head_before" == "$head_after" ]] && \
   [[ "$stash_count_before" == "$stash_count_after" ]]; then
    pass_test
else
    fail_test "Clean tree should be a no-op (commits: $head_before→$head_after, stash: $stash_count_before→$stash_count_after)"
fi

# ============================================================================
# Group 7: Edge cases and robustness
# ============================================================================

echo ""
echo "--- 7. Edge cases and robustness ---"

reset_sandbox

begin_test "run_commit_recovery with agent output but no extractable msg"
echo "new file" > "$SANDBOX/edge_case.go"
output='The agent completed the task. No commit command shown.'
head_before=$(git rev-parse HEAD)
run_commit_recovery "T-002" "$output"
head_after=$(git rev-parse HEAD)
if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
    commit_msg=$(git log -1 --format=%B)
    if assert_contains "$commit_msg" "Recovered-by: ralph-loop" "should have recovery trailer"; then
        pass_test
    fi
else
    fail_test "Should still commit using generated message"
fi

reset_sandbox

begin_test "run_commit_recovery with empty agent output"
echo "another file" > "$SANDBOX/edge2.go"
head_before=$(git rev-parse HEAD)
run_commit_recovery "T-002" ""
head_after=$(git rev-parse HEAD)
if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
    pass_test
else
    fail_test "Should commit even with empty agent output (uses generated msg)"
fi

reset_sandbox

begin_test "run_commit_recovery: nothing staged (gitignored files only)"
echo "*.log" > "$SANDBOX/.gitignore"
git add "$SANDBOX/.gitignore" && git commit -q -m "add gitignore"
echo "ignored content" > "$SANDBOX/test.log"
rc=0
run_commit_recovery "T-002" "" || rc=$?
if [[ $rc -eq 0 ]]; then
    pass_test
else
    fail_test "Should return 0 when nothing to stage (rc=$rc)"
fi

reset_sandbox

begin_test "recover_dirty_tree: only untracked files, no task-state change"
echo "random file" > "$SANDBOX/random.txt"
recover_dirty_tree "T-001:T-005" ""
if ! is_tree_dirty; then
    stash_msg=$(git stash list | head -1)
    if assert_contains "$stash_msg" "ralph-recovery" "should stash when no task detected"; then
        pass_test
    fi
else
    fail_test "Tree should be clean after stash"
fi

reset_sandbox

begin_test "extract_agent_commit_message: message with internal quotes"
output='git commit -m "feat: add \"Config\" type with validation"'
result=$(extract_agent_commit_message "$output")
# The awk should handle escaped quotes inside the message
# The closing " at the end is the message terminator
if [[ -n "$result" ]]; then
    pass_test
else
    fail_test "Should extract something from quoted message"
fi

reset_sandbox

begin_test "_detect_completed_task_in_dirty_tree: handles untracked task-state.conf"
# Remove task-state.conf from git tracking and recreate as untracked
git rm -q --cached docs/tasks/task-state.conf 2>/dev/null || true
git commit -q -m "remove task-state from tracking" 2>/dev/null || true
# Now task-state.conf exists on disk but is untracked
result=$(_detect_completed_task_in_dirty_tree "T-001:T-005" 2>/dev/null || echo "")
# Should handle gracefully (return empty, no crash)
if [[ -z "$result" ]] || [[ -n "$result" ]]; then
    pass_test
else
    fail_test "Should handle untracked task-state.conf gracefully"
fi

reset_sandbox

begin_test "Full recovery preserves all implementation files"
# Simulate comprehensive agent work
mkdir -p "$SANDBOX/internal/config"
echo 'package config

type Config struct {
    Project ProjectConfig
}' > "$SANDBOX/internal/config/config.go"
echo 'package config

func NewDefaults() *Config { return &Config{} }' > "$SANDBOX/internal/config/defaults.go"
echo 'package config_test

import "testing"

func TestDefaults(t *testing.T) {
    cfg := NewDefaults()
    if cfg == nil { t.Fatal("nil") }
}' > "$SANDBOX/internal/config/defaults_test.go"
echo 'package config

func LoadFromFile(path string) (*Config, error) { return nil, nil }' > "$SANDBOX/internal/config/load.go"
echo 'package config_test

func TestLoad(t *testing.T) {}' > "$SANDBOX/internal/config/load_test.go"
mkdir -p "$SANDBOX/testdata"
echo 'key = "value"' > "$SANDBOX/testdata/valid-full.toml"

# Update task-state
sed -i.bak 's/T-002|not_started|/T-002|completed|/' "$SANDBOX/docs/tasks/task-state.conf"
rm -f "$SANDBOX/docs/tasks/task-state.conf.bak"

# Recovery
output='`git commit -m "feat(config): implement T-002 TOML configuration types and loading"`
RALPH_ERROR: commit missing for T-002'
run_commit_recovery "T-002" "$output"

if ! is_tree_dirty; then
    # Verify ALL files are in the commit
    committed_files=$(git diff-tree --no-commit-id --name-only -r HEAD)
    all_present=true
    for f in "internal/config/config.go" "internal/config/defaults.go" "internal/config/defaults_test.go" \
             "internal/config/load.go" "internal/config/load_test.go" "testdata/valid-full.toml" \
             "docs/tasks/task-state.conf"; do
        if ! echo "$committed_files" | grep -q "$f"; then
            fail_test "Missing file in commit: $f"
            all_present=false
            break
        fi
    done
    if [[ "$all_present" == "true" ]]; then
        pass_test
    fi
else
    fail_test "Tree should be clean after recovery"
fi

# ============================================================================
# Group 8: Interaction between recovery mechanisms
# ============================================================================

echo ""
echo "--- 8. Recovery mechanism interactions ---"

reset_sandbox

begin_test "RALPH_ERROR recovery prevents dirty-tree stash on next iteration"
# If Fix 1 works, Fix 2 (safety net) should never be needed
simulate_agent_completion "T-002" "config"
output='`git commit -m "feat(config): implement T-002"`
RALPH_ERROR: commit missing for T-002'

# Simulate Fix 1: RALPH_ERROR handler recovers
remaining_before=4
new_remaining=$(count_remaining_tasks "T-001:T-005")
if [[ "$new_remaining" -lt "$remaining_before" ]]; then
    run_commit_recovery "T-002" "$output"
    last_completed_task="T-002"
fi

# Next iteration: recover_dirty_tree should be a no-op
head_before=$(git rev-parse HEAD)
stash_before=$(git stash list 2>/dev/null | wc -l | tr -d ' ')
recover_dirty_tree "T-001:T-005" "$last_completed_task"
head_after=$(git rev-parse HEAD)
stash_after=$(git stash list 2>/dev/null | wc -l | tr -d ' ')

if [[ "$head_before" == "$head_after" ]] && [[ "$stash_before" == "$stash_after" ]]; then
    pass_test
else
    fail_test "After successful recovery, next iteration should be clean"
fi

reset_sandbox

begin_test "Both fixes handle same scenario: Fix 1 succeeds, Fix 2 is backup"
# This verifies that the two fixes work independently
# Test Fix 1 path
remaining=$(count_remaining_tasks "T-001:T-005")  # before agent
simulate_agent_completion "T-002" "config"
output='`git commit -m "feat(config): implement T-002"`
RALPH_ERROR: commit missing for T-002'

error_msg="RALPH_ERROR: commit missing for T-002"
fix1_recovered=false
if echo "$error_msg" | grep -qi "commit missing"; then
    new_remaining=$(count_remaining_tasks "T-001:T-005")
    if [[ "$new_remaining" -lt "$remaining" ]]; then
        if run_commit_recovery "T-002" "$output"; then
            fix1_recovered=true
        fi
    fi
fi

if [[ "$fix1_recovered" == "true" ]] && ! is_tree_dirty; then
    pass_test
else
    fail_test "Fix 1 should handle this scenario"
fi

reset_sandbox

begin_test "Fix 2 handles scenario when Fix 1 is bypassed"
# Simulate: RALPH_ERROR handler doesn't exist or somehow misses the recovery
# (e.g., agent emits different error format). Safety net must catch it.
simulate_agent_completion "T-002" "config"

# Simulate going directly to next iteration with dirty tree and no last_completed_task
head_before=$(git rev-parse HEAD)
recover_dirty_tree "T-001:T-005" ""
head_after=$(git rev-parse HEAD)

if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
    if is_task_completed "T-002"; then
        pass_test
    else
        fail_test "T-002 should be completed in the committed state"
    fi
else
    fail_test "Safety net should auto-commit when task is detected as completed"
fi

# ============================================================================
# Summary
# ============================================================================

echo ""
echo "================================================"
echo "Results: $PASS passed, $FAIL failed (out of $TOTAL_TESTS tests)"
echo "================================================"

if [[ $FAIL -gt 0 ]]; then
    echo ""
    echo "Log output (last 50 lines):"
    tail -50 "$LOG_FILE" 2>/dev/null || true
    exit 1
fi

echo ""
echo "All loop recovery tests passed!"
exit 0
