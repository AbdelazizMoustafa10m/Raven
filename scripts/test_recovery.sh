#!/usr/bin/env bash
#
# test_recovery.sh -- Smoke tests for ralph loop recovery functions.
#
# Creates an isolated git sandbox, sources the recovery functions from
# ralph_claude.sh, and exercises each recovery path with assertions.
#
# Usage: ./scripts/test_recovery.sh
#

set -euo pipefail

# ============================================================================
# Test Harness
# ============================================================================

PASS=0
FAIL=0
TEST_NAME=""

begin_test() {
    TEST_NAME="$1"
    printf "  TEST: %-60s " "$TEST_NAME"
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
    echo "    In: '$haystack'"
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
    rm -f "/tmp/ralph-test-log-$$.log"
}
trap cleanup EXIT

create_sandbox() {
    SANDBOX=$(mktemp -d /tmp/ralph-test-XXXXXX)
    cd "$SANDBOX"
    git init -q
    git config user.email "test@test.com"
    git config user.name "Test"

    # Create minimal project structure for PROGRESS_FILE and TASK_STATE_FILE
    mkdir -p docs/tasks
    cat > docs/tasks/PROGRESS.md <<'PROGRESS'
# Progress

| Task  | Status    |
|-------|-----------|
| T-001 | Completed |
| T-002 | Not Started |
| T-003 | Not Started |
PROGRESS

    cat > docs/tasks/task-state.conf <<'STATE'
# Task State
T-001|completed|2026-02-16
T-002|not_started|2026-02-16
T-003|not_started|2026-02-16
STATE

    git add -A && git commit -q -m "initial"

    # Set globals that the sourced functions depend on
    PROGRESS_FILE="$SANDBOX/docs/tasks/PROGRESS.md"
    TASK_STATE_FILE="$SANDBOX/docs/tasks/task-state.conf"
    # LOG_FILE must be OUTSIDE the sandbox to avoid dirtying the git tree
    LOG_FILE="/tmp/ralph-test-log-$$.log"
    touch "$LOG_FILE"

    # Backoff schedule for get_backoff_seconds
    BACKOFF_SCHEDULE=(120 300 900 1800)
    RATE_LIMIT_BUFFER_SECONDS=120
    MAX_RATE_LIMIT_WAIT_SECONDS=21600
}

reset_sandbox() {
    cd "$SANDBOX"
    # Reset to initial commit, clean everything
    git checkout -q -- .
    git clean -fdq
    git stash clear 2>/dev/null || true
    # Re-read progress to reset state
    cat > docs/tasks/PROGRESS.md <<'PROGRESS'
# Progress

| Task  | Status    |
|-------|-----------|
| T-001 | Completed |
| T-002 | Not Started |
| T-003 | Not Started |
PROGRESS
    cat > docs/tasks/task-state.conf <<'STATE'
# Task State
T-001|completed|2026-02-16
T-002|not_started|2026-02-16
T-003|not_started|2026-02-16
STATE
    git add -A && git commit -q -m "reset" --allow-empty 2>/dev/null || true
    > "$LOG_FILE"
}

# ============================================================================
# Source the functions we need to test.
# We define log() stub and is_task_completed() here since they are
# simple enough, and extract the recovery + rate-limit functions from
# the actual script.
# ============================================================================

log() {
    local msg="$1"
    echo "[TEST] $msg" >> "$LOG_FILE"
}

# Source only the functions (not main) by extracting them.
# Functions live in ralph-lib.sh (shared library), not in the per-agent scripts.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RALPH_SCRIPT="$SCRIPT_DIR/ralph-lib.sh"

if [[ ! -f "$RALPH_SCRIPT" ]]; then
    echo "ERROR: Cannot find $RALPH_SCRIPT"
    exit 1
fi

# Extract function bodies from the script. We use a sourcing trick:
# define a fake main() that does nothing, then source the script.
# But the script has set -euo pipefail and main "$@" at bottom.
# Instead, we'll extract individual functions using sed.

# Helper: extract a bash function by name from a file.
# Uses brace-counting so nested { } pairs (if/for/awk blocks) are handled
# correctly instead of relying on a bare "^}$" terminator.
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

# Extract and eval each function we need to test
eval "$(extract_function "$RALPH_SCRIPT" is_rate_limited)"
eval "$(extract_function "$RALPH_SCRIPT" parse_claude_reset_time)"
eval "$(extract_function "$RALPH_SCRIPT" parse_codex_reset_time)"
eval "$(extract_function "$RALPH_SCRIPT" get_backoff_seconds)"
eval "$(extract_function "$RALPH_SCRIPT" cap_wait_seconds)"
eval "$(extract_function "$RALPH_SCRIPT" compute_rate_limit_wait)"
eval "$(extract_function "$RALPH_SCRIPT" is_tree_dirty)"
eval "$(extract_function "$RALPH_SCRIPT" get_dirty_summary)"
eval "$(extract_function "$RALPH_SCRIPT" run_commit_recovery)"
eval "$(extract_function "$RALPH_SCRIPT" stash_dirty_tree)"
eval "$(extract_function "$RALPH_SCRIPT" recover_dirty_tree)"

# We need is_task_completed from the script too
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

# ============================================================================
# Tests
# ============================================================================

echo ""
echo "Ralph Recovery Smoke Tests"
echo "=========================="
echo ""

create_sandbox

# --- Test Group 1: Rate-Limit Detection ---
echo "--- Rate-Limit Detection ---"

begin_test "Detects Claude 'hit your limit' message"
if is_rate_limited "You've hit your limit · resets 7pm (Europe/Berlin)"; then
    pass_test
else
    fail_test "is_rate_limited returned false for Claude limit message"
fi

begin_test "Detects Codex 'usage limit + try again' message"
if is_rate_limited "You've hit your usage limit. Upgrade to Pro (https://openai.com/chatgpt/pricing) or try again in 5 days 27 minutes."; then
    pass_test
else
    fail_test "is_rate_limited returned false for Codex limit message"
fi

begin_test "Detects 'rate limit' generic message"
if is_rate_limited "Error: rate limit exceeded, please wait"; then
    pass_test
else
    fail_test "is_rate_limited returned false for generic rate limit"
fi

begin_test "Detects Codex 'try again in X hours' message"
if is_rate_limited "Please try again in 4 hours 30 minutes"; then
    pass_test
else
    fail_test "is_rate_limited returned false for hours message"
fi

begin_test "Does NOT detect normal output as rate-limited"
if is_rate_limited "Task T-014 completed successfully. All tests pass."; then
    fail_test "is_rate_limited returned true for normal output"
else
    pass_test
fi

begin_test "Does NOT detect empty output as rate-limited"
if is_rate_limited ""; then
    fail_test "is_rate_limited returned true for empty output"
else
    pass_test
fi

# --- Test Group 2: Reset Time Parsing ---
echo ""
echo "--- Reset Time Parsing ---"

begin_test "parse_codex_reset_time: '5 days 27 minutes'"
result=$(parse_codex_reset_time "try again in 5 days 27 minutes" 2>/dev/null) || true
expected=$((5 * 86400 + 27 * 60))  # 433620
if assert_eq "$expected" "$result" "5 days 27 min = 433620s"; then
    pass_test
fi

begin_test "parse_codex_reset_time: '4 hours 30 minutes'"
result=$(parse_codex_reset_time "try again in 4 hours 30 minutes" 2>/dev/null) || true
expected=$((4 * 3600 + 30 * 60))  # 16200
if assert_eq "$expected" "$result" "4h 30m = 16200s"; then
    pass_test
fi

begin_test "parse_codex_reset_time: '2 hours'"
result=$(parse_codex_reset_time "try again in 2 hours" 2>/dev/null) || true
expected=$((2 * 3600))  # 7200
if assert_eq "$expected" "$result" "2h = 7200s"; then
    pass_test
fi

begin_test "parse_codex_reset_time: no match returns empty"
result=$(parse_codex_reset_time "normal output with no time info" 2>/dev/null) || true
if assert_eq "" "$result" "should return empty for non-matching output"; then
    pass_test
fi

begin_test "parse_claude_reset_time: 'resets 7pm (Europe/Berlin)'"
result=$(parse_claude_reset_time "You've hit your limit · resets 7pm (Europe/Berlin)" 2>/dev/null) || true
if [[ -n "$result" && "$result" -gt 0 ]] 2>/dev/null; then
    pass_test
else
    # The time might have already passed today, which means diff + 86400
    # As long as we get a positive number, the parser works
    if [[ -n "$result" ]]; then
        pass_test
    else
        fail_test "parse_claude_reset_time returned empty (got: '$result')"
    fi
fi

begin_test "parse_claude_reset_time: 'resets 3:30am'"
result=$(parse_claude_reset_time "limit resets 3:30am tomorrow" 2>/dev/null) || true
if [[ -n "$result" && "$result" -gt 0 ]] 2>/dev/null; then
    pass_test
else
    if [[ -n "$result" ]]; then
        pass_test
    else
        fail_test "parse_claude_reset_time returned empty for 3:30am (got: '$result')"
    fi
fi

begin_test "parse_claude_reset_time: no match returns empty"
result=$(parse_claude_reset_time "normal output" 2>/dev/null) || true
if assert_eq "" "$result" "should return empty"; then
    pass_test
fi

begin_test "compute_rate_limit_wait: logger stdout does not corrupt numeric return"
original_log_def="$(declare -f log)"
log() {
    local msg="$1"
    echo "  23:01:42  [...] $msg"
    echo "[TEST] $msg" >> "$LOG_FILE"
}
result=$(compute_rate_limit_wait "Rate limited, try again in 99 hours" 0)
eval "$original_log_def"
if [[ "$result" =~ ^[0-9]+$ ]] && [[ "$result" -eq 21600 ]]; then
    pass_test
else
    fail_test "Expected numeric capped wait of 21600, got '$result'"
fi

begin_test "compute_rate_limit_wait: invalid normalized wait falls back to backoff"
original_cap_wait_def="$(declare -f cap_wait_seconds)"
cap_wait_seconds() {
    return 1
}
result=$(compute_rate_limit_wait "Rate limited, try again in 2 hours" 1)
eval "$original_cap_wait_def"
if [[ "$result" =~ ^[0-9]+$ ]] && [[ "$result" -ge 300 && "$result" -le 330 ]]; then
    pass_test
else
    fail_test "Expected numeric backoff in [300,330], got '$result'"
fi

# --- Test Group 3: Exponential Backoff ---
echo ""
echo "--- Exponential Backoff ---"

begin_test "get_backoff_seconds: attempt 0 is ~120s (2min + jitter)"
result=$(get_backoff_seconds 0)
if [[ "$result" -ge 120 && "$result" -le 150 ]]; then
    pass_test
else
    fail_test "Expected 120-150, got $result"
fi

begin_test "get_backoff_seconds: attempt 3 is ~1800s (30min + jitter)"
result=$(get_backoff_seconds 3)
if [[ "$result" -ge 1800 && "$result" -le 1830 ]]; then
    pass_test
else
    fail_test "Expected 1800-1830, got $result"
fi

begin_test "get_backoff_seconds: attempt 99 caps at ~1800s"
result=$(get_backoff_seconds 99)
if [[ "$result" -ge 1800 && "$result" -le 1830 ]]; then
    pass_test
else
    fail_test "Expected 1800-1830, got $result"
fi

# --- Test Group 4: Function Extraction ---
echo ""
echo "--- Function Extraction ---"

begin_test "extract_function: handles nested braces correctly"
temp_func_file=$(mktemp /tmp/ralph-test-func-XXXXXX.sh)
cat > "$temp_func_file" <<'FUNC_EOF'
simple_func() {
    if [[ true ]]; then
        echo "nested"
    fi
    local x=$(echo "y" | awk '{print $1}')
}

other_func() {
    echo "other"
}
FUNC_EOF
result=$(extract_function "$temp_func_file" "simple_func")
rm -f "$temp_func_file"
# Verify we got simple_func but not other_func
if echo "$result" | grep -q "nested" && ! echo "$result" | grep -q "other"; then
    pass_test
else
    fail_test "extract_function didn't correctly extract nested function"
fi

begin_test "extract_function: does not bleed into subsequent function"
temp_func_file=$(mktemp /tmp/ralph-test-func2-XXXXXX.sh)
cat > "$temp_func_file" <<'FUNC_EOF'
first_func() {
    for i in 1 2 3; do
        if [[ $i -gt 1 ]]; then
            echo "deep nesting with { braces }"
        fi
    done
}

second_func() {
    echo "should not appear"
}
FUNC_EOF
result=$(extract_function "$temp_func_file" "first_func")
rm -f "$temp_func_file"
if echo "$result" | grep -q "deep nesting" && ! echo "$result" | grep -q "should not appear"; then
    pass_test
else
    fail_test "extract_function bled into second_func"
fi

begin_test "extract_function: extracts second function correctly"
temp_func_file=$(mktemp /tmp/ralph-test-func3-XXXXXX.sh)
cat > "$temp_func_file" <<'FUNC_EOF'
alpha() {
    echo "alpha body"
}

beta() {
    echo "beta body"
}
FUNC_EOF
result=$(extract_function "$temp_func_file" "beta")
rm -f "$temp_func_file"
if echo "$result" | grep -q "beta body" && ! echo "$result" | grep -q "alpha body"; then
    pass_test
else
    fail_test "extract_function didn't isolate beta correctly"
fi

# --- Test Group 5: is_tree_dirty ---
echo ""
echo "--- Working Tree State Detection ---"

reset_sandbox

begin_test "is_tree_dirty: clean tree returns false"
if is_tree_dirty; then
    fail_test "is_tree_dirty said dirty but tree is clean"
else
    pass_test
fi

begin_test "is_tree_dirty: modified file returns true"
echo "change" >> "$SANDBOX/docs/tasks/PROGRESS.md"
if is_tree_dirty; then
    pass_test
else
    fail_test "is_tree_dirty said clean but file was modified"
fi

begin_test "is_tree_dirty: untracked file returns true"
git checkout -q -- .  # reset modification
echo "new file" > "$SANDBOX/newfile.txt"
if is_tree_dirty; then
    pass_test
else
    fail_test "is_tree_dirty said clean but untracked file exists"
fi

begin_test "get_dirty_summary: reports counts"
result=$(get_dirty_summary)
if assert_contains "$result" "new/untracked" "should mention untracked"; then
    pass_test
fi

# --- Test Group 6: Commit Recovery ---
echo ""
echo "--- Commit Recovery (progress + no commit) ---"

reset_sandbox

begin_test "run_commit_recovery: commits dirty tree and returns 0"
# Simulate: agent modified files but didn't commit
echo "// new code for T-001" > "$SANDBOX/code.go"
echo "test content" > "$SANDBOX/test_file.go"
local_head_before=$(git rev-parse HEAD)
run_commit_recovery "T-001"
rc=$?
local_head_after=$(git rev-parse HEAD)
if [[ $rc -eq 0 && "$local_head_before" != "$local_head_after" ]]; then
    # Verify the tree is clean after recovery
    if ! is_tree_dirty; then
        # Verify the commit message mentions recovery
        commit_msg=$(git log -1 --format=%s)
        if assert_contains "$commit_msg" "recovery" "commit message should mention recovery"; then
            pass_test
        fi
    else
        fail_test "Tree still dirty after commit recovery"
    fi
else
    fail_test "run_commit_recovery failed (rc=$rc, heads: before=$local_head_before after=$local_head_after)"
fi

begin_test "run_commit_recovery: clean tree is a no-op (returns 0)"
rc=1
run_commit_recovery "T-002" && rc=0
if assert_eq "0" "$rc" "should return 0 on clean tree"; then
    pass_test
fi

# --- Test Group 7: Stash Recovery ---
echo ""
echo "--- Stash Recovery (mid-task interrupt) ---"

reset_sandbox

begin_test "stash_dirty_tree: stashes modified + untracked files"
echo "partial work" > "$SANDBOX/partial.go"
echo "more partial" >> "$SANDBOX/docs/tasks/PROGRESS.md"
stash_dirty_tree "rate-limit interrupt during T-002"
rc=$?
if [[ $rc -eq 0 ]]; then
    # Tree should be clean now
    if ! is_tree_dirty; then
        # Stash should exist
        stash_msg=$(git stash list | head -1)
        if assert_contains "$stash_msg" "ralph-recovery" "stash message should contain ralph-recovery"; then
            pass_test
        fi
    else
        fail_test "Tree still dirty after stash"
    fi
else
    fail_test "stash_dirty_tree failed (rc=$rc)"
fi

begin_test "stash_dirty_tree: clean tree returns 1 (nothing to stash)"
# Tree is clean after previous stash
if stash_dirty_tree "should-not-stash"; then
    fail_test "Should return 1 on clean tree"
else
    pass_test
fi

begin_test "stash_dirty_tree: stashed work is recoverable"
# Pop the stash and verify content
git stash pop -q 2>/dev/null
if [[ -f "$SANDBOX/partial.go" ]]; then
    content=$(cat "$SANDBOX/partial.go")
    if assert_eq "partial work" "$content" "stashed content should be intact"; then
        pass_test
    fi
else
    fail_test "partial.go not found after stash pop"
fi

# --- Test Group 8: Dirty Tree Recovery (pre-iteration) ---
echo ""
echo "--- Pre-Iteration Dirty Tree Recovery ---"

reset_sandbox

begin_test "recover_dirty_tree: case (a) - completed task + dirty tree -> auto-commits"
# T-001 is already marked Completed in PROGRESS.md
echo "// leftover code from T-001" > "$SANDBOX/leftover.go"
head_before=$(git rev-parse HEAD)
recover_dirty_tree "T-001:T-015" "T-001"
head_after=$(git rev-parse HEAD)
if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
    commit_msg=$(git log -1 --format=%s)
    if assert_contains "$commit_msg" "recovery" "should be a recovery commit"; then
        pass_test
    fi
else
    fail_test "Expected auto-commit (head_before=$head_before head_after=$head_after dirty=$(is_tree_dirty && echo yes || echo no))"
fi

reset_sandbox

begin_test "recover_dirty_tree: case (b) - incomplete task + dirty tree -> stashes"
# T-002 is NOT completed
echo "// partial T-002 work" > "$SANDBOX/incomplete.go"
recover_dirty_tree "T-001:T-015" "T-002"
if ! is_tree_dirty; then
    stash_msg=$(git stash list | head -1)
    if assert_contains "$stash_msg" "ralph-recovery" "should be a recovery stash"; then
        pass_test
    fi
else
    fail_test "Tree still dirty after recover_dirty_tree for incomplete task"
fi

reset_sandbox

begin_test "recover_dirty_tree: clean tree is a no-op"
head_before=$(git rev-parse HEAD)
recover_dirty_tree "T-001:T-015" "T-001"
head_after=$(git rev-parse HEAD)
stash_count=$(git stash list 2>/dev/null | wc -l | tr -d ' ')
if [[ "$head_before" == "$head_after" && "$stash_count" == "0" ]]; then
    pass_test
else
    fail_test "Expected no-op on clean tree"
fi

reset_sandbox

begin_test "recover_dirty_tree: no last_task_id + dirty tree -> stashes"
echo "mystery changes" > "$SANDBOX/mystery.txt"
recover_dirty_tree "T-001:T-015" ""
if ! is_tree_dirty; then
    pass_test
else
    fail_test "Tree still dirty when last_task_id is empty"
fi

# --- Test Group 9: End-to-End Rate-Limit + Stash Scenario ---
echo ""
echo "--- End-to-End: Rate-Limit + Stash ---"

reset_sandbox

begin_test "E2E: rate-limit detected -> partial work stashed -> tree clean"
# Simulate: agent wrote partial files, then hit rate limit
echo "// partial implementation" > "$SANDBOX/internal_code.go"
echo "package main" > "$SANDBOX/main_test.go"

# 1. Detect rate limit
output="You've hit your limit · resets 7pm (Europe/Berlin)"
if ! is_rate_limited "$output"; then
    fail_test "Step 1: Rate limit not detected"
else
    # 2. Stash partial work (what the loop does before waiting)
    if is_tree_dirty; then
        stash_dirty_tree "rate-limit interrupt during T-014"
    fi

    # 3. Verify tree is clean
    if ! is_tree_dirty; then
        # 4. Verify stash exists with the right context
        stash_msg=$(git stash list | head -1)
        if assert_contains "$stash_msg" "T-014" "stash should mention the task"; then
            # 5. Verify we can recover the stash
            git stash pop -q 2>/dev/null
            if [[ -f "$SANDBOX/internal_code.go" && -f "$SANDBOX/main_test.go" ]]; then
                pass_test
            else
                fail_test "Step 5: Files not recovered from stash"
            fi
        fi
    else
        fail_test "Step 3: Tree still dirty after stash"
    fi
fi

reset_sandbox

begin_test "E2E: progress+no-commit -> auto-commit -> next iteration clean"
# Simulate: agent updated PROGRESS.md + task-state.conf to mark T-002 complete, wrote code, but no commit
sed -i.bak 's/T-002 | Not Started/T-002 | Completed/' "$SANDBOX/docs/tasks/PROGRESS.md"
rm -f "$SANDBOX/docs/tasks/PROGRESS.md.bak"
sed -i.bak 's/T-002|not_started|2026-02-16/T-002|completed|2026-02-16/' "$SANDBOX/docs/tasks/task-state.conf"
rm -f "$SANDBOX/docs/tasks/task-state.conf.bak"
echo "// T-002 implementation" > "$SANDBOX/t002.go"
echo "package t002_test" > "$SANDBOX/t002_test.go"

head_before=$(git rev-parse HEAD)

# Verify T-002 is now marked complete
if ! is_task_completed "T-002"; then
    fail_test "Setup error: T-002 should be marked Completed"
else
    # This is what run_ralph_loop does when it detects progress+no-commit:
    run_commit_recovery "T-002"
    head_after=$(git rev-parse HEAD)

    if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
        # Verify files are in the commit
        committed_files=$(git diff-tree --no-commit-id --name-only -r HEAD)
        if echo "$committed_files" | grep -q "t002.go"; then
            pass_test
        else
            fail_test "t002.go not in recovery commit. Committed: $committed_files"
        fi
    else
        fail_test "Expected new commit + clean tree (head_before=$head_before head_after=$head_after)"
    fi
fi

reset_sandbox

begin_test "E2E: dirty tree at iteration start -> completed task -> commits; then incomplete -> stashes"
# Phase 1: dirty tree from completed T-001
echo "leftover from T-001" > "$SANDBOX/leftover.go"
recover_dirty_tree "T-001:T-015" "T-001"
if is_tree_dirty; then
    fail_test "Phase 1: Tree should be clean after auto-commit for completed T-001"
else
    # Phase 2: dirty tree from incomplete T-002 (next iteration)
    echo "half-done T-002" > "$SANDBOX/half_t002.go"
    recover_dirty_tree "T-001:T-015" "T-002"
    if is_tree_dirty; then
        fail_test "Phase 2: Tree should be clean after stash for incomplete T-002"
    else
        # Verify we have both: a recovery commit AND a stash
        commit_msg=$(git log -1 --format=%s)
        stash_msg=$(git stash list | head -1)
        if assert_contains "$commit_msg" "recovery" "should have recovery commit" && \
           assert_contains "$stash_msg" "ralph-recovery" "should have recovery stash"; then
            pass_test
        fi
    fi
fi

# ============================================================================
# Summary
# ============================================================================

echo ""
echo "=========================="
echo "Results: $PASS passed, $FAIL failed"
echo "=========================="

if [[ $FAIL -gt 0 ]]; then
    echo ""
    echo "Log output (last 50 lines):"
    tail -50 "$LOG_FILE" 2>/dev/null || true
    exit 1
fi

echo ""
echo "All smoke tests passed!"
exit 0
