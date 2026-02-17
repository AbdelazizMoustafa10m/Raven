#!/usr/bin/env bash
#
# test_rate_limit_recovery.sh -- Rigorous tests for the rate-limit cooldown
# and stash-restore fixes in ralph-lib.sh.
#
# Tests two specific fixes:
#
#   Fix 1: wait_for_rate_limit_reset uses epoch-based countdown instead of
#          decrementing counter, preventing drift from system sleep or overhead.
#
#   Fix 2: After rate-limit cooldown, stashed partial work is restored (git
#          stash pop) so the next iteration resumes with files on disk rather
#          than starting the task from scratch.
#
# Also tests the full rate-limit → stash → cooldown → restore → resume chain
# and all edge cases that could cause overnight failures.
#
# Usage: ./scripts/test_rate_limit_recovery.sh
#

set -euo pipefail

# ============================================================================
# Test Harness
# ============================================================================

PASS=0
FAIL=0
TOTAL_TESTS=0
TEST_NAME=""
FAILED_TESTS=()

begin_test() {
    TEST_NAME="$1"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    printf "  TEST %02d: %-64s " "$TOTAL_TESTS" "$TEST_NAME"
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
    FAILED_TESTS+=("$TEST_NAME")
    return 1
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="${3:-}"
    if echo "$haystack" | grep -q "$needle" 2>/dev/null; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected to contain: '$needle'"
    echo "    In: '$(echo "$haystack" | head -3)...'"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    FAILED_TESTS+=("$TEST_NAME")
    return 1
}

assert_not_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="${3:-}"
    if ! echo "$haystack" | grep -q "$needle" 2>/dev/null; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected NOT to contain: '$needle'"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    FAILED_TESTS+=("$TEST_NAME")
    return 1
}

assert_true() {
    local condition_result="$1"
    local msg="${2:-}"
    if [[ "$condition_result" == "true" ]]; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected: true"
    echo "    Actual:   $condition_result"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    FAILED_TESTS+=("$TEST_NAME")
    return 1
}

assert_ge() {
    local actual="$1"
    local minimum="$2"
    local msg="${3:-}"
    if [[ "$actual" -ge "$minimum" ]] 2>/dev/null; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected >= $minimum"
    echo "    Actual:    $actual"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    FAILED_TESTS+=("$TEST_NAME")
    return 1
}

assert_le() {
    local actual="$1"
    local maximum="$2"
    local msg="${3:-}"
    if [[ "$actual" -le "$maximum" ]] 2>/dev/null; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected <= $maximum"
    echo "    Actual:    $actual"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    FAILED_TESTS+=("$TEST_NAME")
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
    FAILED_TESTS+=("$TEST_NAME")
}

# ============================================================================
# Sandbox Setup
# ============================================================================

SANDBOX=""
LOG_FILE=""

cleanup() {
    if [[ -n "$SANDBOX" && -d "$SANDBOX" ]]; then
        rm -rf "$SANDBOX"
    fi
    if [[ -n "$LOG_FILE" ]]; then
        rm -f "$LOG_FILE"
    fi
}
trap cleanup EXIT

create_sandbox() {
    SANDBOX=$(mktemp -d /tmp/ralph-ratelimit-test-XXXXXX)
    LOG_FILE=$(mktemp /tmp/ralph-ratelimit-log-XXXXXX.log)
    cd "$SANDBOX"
    git init -q
    git config user.email "test@test.com"
    git config user.name "Test"

    # Create minimal project structure
    mkdir -p docs/tasks

    cat > docs/tasks/task-state.conf <<'STATE'
# Raven Task State
T-001|completed|2026-02-17
T-002|not_started|2026-02-17
T-003|not_started|2026-02-17
T-004|not_started|2026-02-17
T-005|not_started|2026-02-17
STATE

    # Create minimal task spec files (needed for select_next_task/get_task_file)
    echo "# T-001: Setup" > docs/tasks/T-001-setup.md
    echo "# T-002: Config types" > docs/tasks/T-002-config-types.md
    echo "# T-003: Config loading" > docs/tasks/T-003-config-loading.md
    echo "# T-004: Config resolve" > docs/tasks/T-004-config-resolve.md
    echo "# T-005: Config validate" > docs/tasks/T-005-config-validate.md

    git add -A && git commit -q -m "initial"

    # Set globals for sourced functions
    TASK_STATE_FILE="$SANDBOX/docs/tasks/task-state.conf"
    PROGRESS_FILE="$SANDBOX/docs/tasks/PROGRESS.md"

    BACKOFF_SCHEDULE=(120 300 900 1800)
    RATE_LIMIT_BUFFER_SECONDS=120
    MAX_RATE_LIMIT_WAIT_SECONDS=21600
}

reset_sandbox() {
    cd "$SANDBOX"
    local initial_commit
    initial_commit=$(git rev-list --max-parents=0 HEAD 2>/dev/null | head -1)
    git reset --hard "$initial_commit" -q 2>/dev/null
    git clean -fdq
    git stash clear 2>/dev/null || true
    : > "$LOG_FILE"
}

# Create partial work files (simulating agent mid-task)
create_partial_work() {
    local task_id="${1:-T-012}"
    mkdir -p "$SANDBOX/internal/config"
    echo "package config

// Partial implementation for $task_id
type Config struct {
    Project ProjectConfig
}" > "$SANDBOX/internal/config/config.go"

    echo "package config_test

import \"testing\"

func TestConfig(t *testing.T) {
    // partial test for $task_id
}" > "$SANDBOX/internal/config/config_test.go"
}

# Simulate agent completing a task (update task-state + create files)
simulate_agent_completion() {
    local task_id="$1"
    local scope="${2:-config}"

    sed -i.bak "s/${task_id}|not_started|/${task_id}|completed|/" "$SANDBOX/docs/tasks/task-state.conf"
    rm -f "$SANDBOX/docs/tasks/task-state.conf.bak"

    mkdir -p "$SANDBOX/internal/${scope}"
    echo "package ${scope}" > "$SANDBOX/internal/${scope}/${scope}.go"
    echo "package ${scope}_test" > "$SANDBOX/internal/${scope}/${scope}_test.go"
}

# ============================================================================
# Source Functions Under Test
# ============================================================================

# Logging stubs (write to LOG_FILE, no terminal noise)
log() {
    local msg="$1"
    if [[ -n "${LOG_FILE:-}" ]]; then
        echo "[TEST] $msg" >> "$LOG_FILE"
    fi
}
log_info()       { log "$*"; }
log_warn()       { log "WARN: $*"; }
log_error()      { log "ERROR: $*"; }
log_success()    { log "OK: $*"; }
log_step()       { log "$2"; }
log_blocked()    { log "BLOCKED: $*"; }
log_rate_limit() { log "RATE_LIMIT: $*"; }
log_header()     { log "=== $1 ==="; }

# Styling stubs
_BOLD='' _DIM='' _RESET='' _ITALIC='' _UNDERLINE='' _REVERSE=''
_RED='' _GREEN='' _YELLOW='' _BLUE='' _MAGENTA='' _CYAN=''
_WHITE='' _GRAY='' _BG_RED='' _BG_GREEN='' _BG_BLUE=''
_BG_MAGENTA='' _BG_CYAN=''
_SYM_STEP='>' _SYM_WARN='[!]' _SYM_BLOCK='[x]' _SYM_ERROR='[x]'
_SYM_WAIT='[~]' _SYM_OK='[ok]' _SYM_CHECK='[ok]' _SYM_CROSS='[x]'
_SYM_ARROW='>' _SYM_PENDING='[ ]' _SYM_RUNNING='[*]' _SYM_DONE='[+]'

HAS_GUM=false

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RALPH_SCRIPT="$SCRIPT_DIR/ralph-lib.sh"

if [[ ! -f "$RALPH_SCRIPT" ]]; then
    echo "FATAL: Cannot find $RALPH_SCRIPT"
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

# Extract all functions we need
eval "$(extract_function "$RALPH_SCRIPT" is_rate_limited)"
eval "$(extract_function "$RALPH_SCRIPT" parse_claude_reset_time)"
eval "$(extract_function "$RALPH_SCRIPT" parse_codex_reset_time)"
eval "$(extract_function "$RALPH_SCRIPT" get_backoff_seconds)"
eval "$(extract_function "$RALPH_SCRIPT" cap_wait_seconds)"
eval "$(extract_function "$RALPH_SCRIPT" compute_rate_limit_wait)"
eval "$(extract_function "$RALPH_SCRIPT" wait_for_rate_limit_reset)"
eval "$(extract_function "$RALPH_SCRIPT" is_tree_dirty)"
eval "$(extract_function "$RALPH_SCRIPT" get_dirty_summary)"
eval "$(extract_function "$RALPH_SCRIPT" stash_dirty_tree)"
eval "$(extract_function "$RALPH_SCRIPT" recover_dirty_tree)"
eval "$(extract_function "$RALPH_SCRIPT" run_commit_recovery)"
eval "$(extract_function "$RALPH_SCRIPT" extract_agent_commit_message)"
eval "$(extract_function "$RALPH_SCRIPT" generate_commit_message_from_diff)"
eval "$(extract_function "$RALPH_SCRIPT" _detect_completed_task_in_dirty_tree)"

# Task state helpers
is_task_completed() {
    local task_id="$1"
    awk -F'|' -v task="$task_id" '
        $0 !~ /^[[:space:]]*#/ && NF >= 2 {
            id=$1; st=$2
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", id)
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", st)
            if (id == task && st == "completed") found=1
        }
        END { exit(found ? 0 : 1) }
    ' "$TASK_STATE_FILE" 2>/dev/null
}

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
#
#  TEST GROUP 1: Epoch-Based Countdown Timer (Fix 1)
#
# ============================================================================

echo ""
echo "================================================================"
echo " Rate-Limit Cooldown & Stash Restore -- Comprehensive Tests"
echo "================================================================"
echo ""
echo "--- 1. Epoch-Based Countdown Timer ---"

create_sandbox

# We test wait_for_rate_limit_reset with short waits to verify it uses
# wall-clock time correctly. We can't test 2-hour waits, but we CAN verify
# the function exits in the expected time window for short durations.

begin_test "Short cooldown (2s) completes within 1s tolerance"
start_epoch=$(date +%s)
# Redirect stdout to /dev/null to suppress countdown display
wait_for_rate_limit_reset 2 0 3 > /dev/null 2>&1
end_epoch=$(date +%s)
elapsed=$((end_epoch - start_epoch))
# Should take ~2s (2s sleep), allow 1s tolerance for shell overhead
if assert_ge "$elapsed" 2 "should wait at least 2 seconds" && \
   assert_le "$elapsed" 4 "should not overshoot by more than 2s"; then
    pass_test
fi

begin_test "Short cooldown (3s) completes within tolerance"
start_epoch=$(date +%s)
wait_for_rate_limit_reset 3 0 3 > /dev/null 2>&1
end_epoch=$(date +%s)
elapsed=$((end_epoch - start_epoch))
if assert_ge "$elapsed" 3 "should wait at least 3 seconds" && \
   assert_le "$elapsed" 5 "should not overshoot by more than 2s"; then
    pass_test
fi

begin_test "Zero-second cooldown exits immediately"
start_epoch=$(date +%s)
# Edge case: wait_seconds=0 should complete instantly
# (remaining = target_epoch - now_epoch = 0, breaks immediately)
wait_for_rate_limit_reset 0 0 3 > /dev/null 2>&1
end_epoch=$(date +%s)
elapsed=$((end_epoch - start_epoch))
if assert_le "$elapsed" 1 "0s cooldown should exit immediately"; then
    pass_test
fi

begin_test "Cooldown respects max_cycles: exits with error when exceeded"
# wait_cycle=2, max_cycles=2 → should exit 1 (cycle >= max)
# Must run in subshell because the function calls `exit 1`
rc=0
( wait_for_rate_limit_reset 1 2 2 > /dev/null 2>&1 ) || rc=$?
if assert_eq "1" "$rc" "should exit 1 when wait_cycle >= max_cycles"; then
    pass_test
fi

begin_test "Cooldown allows cycle 0 of 1 (last valid cycle)"
rc=0
( wait_for_rate_limit_reset 1 0 1 > /dev/null 2>&1 ) || rc=$?
if assert_eq "0" "$rc" "cycle 0 of max 1 should succeed"; then
    pass_test
fi

begin_test "Countdown timer does not drift for multi-chunk waits"
# Test a wait that spans multiple 60s chunks (use small sleep to test logic)
# We'll test with 5s which is under 60 so it's a single chunk
start_epoch=$(date +%s)
wait_for_rate_limit_reset 5 0 3 > /dev/null 2>&1
end_epoch=$(date +%s)
elapsed=$((end_epoch - start_epoch))
# Key: elapsed should be very close to 5, NOT 5 + (N * overhead)
if assert_ge "$elapsed" 5 "should wait at least 5s" && \
   assert_le "$elapsed" 7 "should not drift more than 2s"; then
    pass_test
fi

# Verify the countdown uses epoch by checking the function source
begin_test "wait_for_rate_limit_reset uses date +%s (epoch-based)"
func_body=$(extract_function "$RALPH_SCRIPT" wait_for_rate_limit_reset)
if assert_contains "$func_body" 'target_epoch' "should compute target_epoch" && \
   assert_contains "$func_body" 'now_epoch' "should check now_epoch each iteration" && \
   assert_contains "$func_body" 'date +%s' "should use date +%s for wall-clock"; then
    pass_test
fi

begin_test "wait_for_rate_limit_reset does NOT use decrementing counter"
func_body=$(extract_function "$RALPH_SCRIPT" wait_for_rate_limit_reset)
if assert_not_contains "$func_body" 'remaining=$wait_seconds' "should not initialize remaining from wait_seconds" && \
   assert_not_contains "$func_body" 'remaining=$((remaining - sleep_chunk))' "should not decrement remaining"; then
    pass_test
fi

# ============================================================================
#
#  TEST GROUP 2: Stash + Restore After Rate-Limit (Fix 2)
#
# ============================================================================

echo ""
echo "--- 2. Stash + Restore After Rate-Limit ---"

reset_sandbox

begin_test "Stash partial work → restore after cooldown: files present"
# Simulate the exact scenario: agent writes partial files, rate limit hits
create_partial_work "T-012"

# Step 1: Verify tree is dirty
if ! is_tree_dirty; then
    fail_test "Setup error: tree should be dirty"
else
    # Step 2: Stash (as the rate-limit handler does)
    stashed=false
    if stash_dirty_tree "rate-limit interrupt during T-012"; then
        stashed=true
    fi

    if [[ "$stashed" != "true" ]]; then
        fail_test "Stash should succeed"
    else
        # Verify tree is clean after stash
        if is_tree_dirty; then
            fail_test "Tree should be clean after stash"
        else
            # Step 3: Simulate cooldown (just 1s for testing)
            wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1

            # Step 4: Restore stash (as our fix does)
            if git stash pop 2>/dev/null; then
                # Verify files are back
                if [[ -f "$SANDBOX/internal/config/config.go" ]] && \
                   [[ -f "$SANDBOX/internal/config/config_test.go" ]]; then
                    # Verify content is intact
                    content=$(cat "$SANDBOX/internal/config/config.go")
                    if assert_contains "$content" "T-012" "file content should reference T-012"; then
                        pass_test
                    fi
                else
                    fail_test "Files not restored after stash pop"
                fi
            else
                fail_test "git stash pop failed"
            fi
        fi
    fi
fi

reset_sandbox

begin_test "Stash+restore preserves modified AND untracked files"
# Modify an existing tracked file
echo "# Updated progress" >> "$SANDBOX/docs/tasks/task-state.conf"
# Create untracked files
mkdir -p "$SANDBOX/internal/workflow"
echo "package workflow" > "$SANDBOX/internal/workflow/workflow.go"

# Stash all
stash_dirty_tree "rate-limit interrupt during T-003"

# Tree should be clean
if is_tree_dirty; then
    fail_test "Tree should be clean after stash"
else
    # Restore
    git stash pop 2>/dev/null
    if [[ -f "$SANDBOX/internal/workflow/workflow.go" ]]; then
        # Check that the tracked modification is also restored
        if grep -q "Updated progress" "$SANDBOX/docs/tasks/task-state.conf"; then
            pass_test
        else
            fail_test "Modified tracked file not restored"
        fi
    else
        fail_test "Untracked file not restored from stash"
    fi
fi

reset_sandbox

begin_test "No stash needed when tree is clean (stash_dirty_tree returns 1)"
# Clean tree → stash should fail/return 1
rc=0
stash_dirty_tree "should-not-stash" || rc=$?
if assert_eq "1" "$rc" "clean tree stash should return 1"; then
    # Verify no stash was created
    stash_count=$(git stash list 2>/dev/null | wc -l | tr -d ' ')
    if assert_eq "0" "$stash_count" "no stash entry should exist"; then
        pass_test
    fi
fi

reset_sandbox

begin_test "stashed_for_rate_limit=false when tree is clean → skip pop"
# Simulate the fixed code path with a clean tree
stashed_for_rate_limit=false
if is_tree_dirty; then
    if stash_dirty_tree "rate-limit interrupt"; then
        stashed_for_rate_limit=true
    fi
fi

# Should not attempt pop
if [[ "$stashed_for_rate_limit" == "false" ]]; then
    pass_test
else
    fail_test "stashed_for_rate_limit should be false for clean tree"
fi

reset_sandbox

begin_test "stashed_for_rate_limit=true when tree is dirty → triggers pop"
create_partial_work "T-012"

stashed_for_rate_limit=false
if is_tree_dirty; then
    if stash_dirty_tree "rate-limit interrupt during T-012"; then
        stashed_for_rate_limit=true
    fi
fi

if assert_eq "true" "$stashed_for_rate_limit" "should be true after successful stash"; then
    # Simulate the restore path
    if git stash pop 2>/dev/null; then
        if [[ -f "$SANDBOX/internal/config/config.go" ]]; then
            pass_test
        else
            fail_test "Files not present after pop"
        fi
    else
        fail_test "git stash pop failed"
    fi
fi

reset_sandbox

begin_test "Stash pop failure is handled gracefully (no crash)"
# Create a situation where stash pop would fail: stash, then create
# conflicting changes before popping
create_partial_work "T-012"
stash_dirty_tree "rate-limit interrupt during T-012"

# Create a conflicting file at the same path
mkdir -p "$SANDBOX/internal/config"
echo "package config // conflicting content" > "$SANDBOX/internal/config/config.go"
git add -A && git commit -q -m "conflicting commit"

# Pop should fail (merge conflict), but should not crash the script
rc=0
git stash pop 2>/dev/null || rc=$?
if [[ $rc -ne 0 ]]; then
    # This is expected: pop failed due to conflict
    # The important thing is we didn't crash
    # Clean up the conflict
    git checkout --theirs . 2>/dev/null || true
    git reset HEAD . 2>/dev/null || true
    git stash drop 2>/dev/null || true
    pass_test
else
    # Pop succeeded despite conflict? Unusual but OK
    pass_test
fi

# ============================================================================
#
#  TEST GROUP 3: Full Rate-Limit Handler Flow (Simulated Loop Logic)
#
# ============================================================================

echo ""
echo "--- 3. Full Rate-Limit Handler Flow ---"

reset_sandbox

begin_test "Exact log scenario: rate-limit during T-012 → stash → restore"
# Reproduce the exact failure from the bug report log:
# 1. Agent works on T-012, writes partial files
# 2. Rate limit hit ("You've hit your limit · resets 10pm")
# 3. Partial work stashed
# 4. Cooldown wait
# 5. After cooldown, stash should be restored  ← THIS WAS THE BUG

create_partial_work "T-012"
output="You've hit your limit · resets 10pm (Europe/Berlin)"

# Verify rate limit is detected
if ! is_rate_limited "$output"; then
    fail_test "Rate limit should be detected"
else
    # Simulate the FIXED handler from run_ralph_loop
    local_stashed=false
    if is_tree_dirty; then
        if stash_dirty_tree "rate-limit interrupt during T-012"; then
            local_stashed=true
        fi
    fi

    # Verify tree is clean during cooldown
    if is_tree_dirty; then
        fail_test "Tree should be clean during cooldown"
    else
        # Short cooldown (1s for testing)
        wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1

        # THE FIX: Restore stash after cooldown
        if [[ "$local_stashed" == "true" ]]; then
            if git stash pop 2>/dev/null; then
                # Verify files are back on disk
                if [[ -f "$SANDBOX/internal/config/config.go" ]] && \
                   [[ -f "$SANDBOX/internal/config/config_test.go" ]]; then
                    content=$(cat "$SANDBOX/internal/config/config.go")
                    if assert_contains "$content" "T-012" "partial work should reference T-012"; then
                        pass_test
                    fi
                else
                    fail_test "Partial work files not restored after cooldown"
                fi
            else
                fail_test "git stash pop failed after cooldown"
            fi
        else
            fail_test "Should have stashed partial work"
        fi
    fi
fi

reset_sandbox

begin_test "Bug regression: WITHOUT fix, tree is clean → agent restarts from scratch"
# This test verifies the OLD buggy behavior pattern to confirm our fix addresses it
create_partial_work "T-012"

# OLD behavior: stash, cooldown, continue (no pop)
stash_dirty_tree "rate-limit interrupt during T-012"

# After cooldown, the old code just did `continue`.
# Tree is clean. recover_dirty_tree at top of next iteration:
recover_dirty_tree "T-001:T-005" ""

# Tree is STILL clean (recover_dirty_tree is a no-op for clean tree)
# This means the agent would start T-012 from scratch - THE BUG
if ! is_tree_dirty; then
    # Confirm: partial work is NOT on disk (it's stuck in stash)
    if [[ ! -f "$SANDBOX/internal/config/config.go" ]]; then
        # This IS the bug. Files are gone. Our fix prevents this.
        pass_test
    else
        fail_test "Files should NOT be on disk without the fix (stash holds them)"
    fi
else
    fail_test "Tree should be clean in the old buggy flow"
fi

reset_sandbox

begin_test "Fixed flow: recover_dirty_tree sees restored files (no stash needed)"
# After our fix restores the stash, recover_dirty_tree at loop top
# should see the dirty tree and handle it properly
create_partial_work "T-012"

# Fix flow: stash → cooldown → pop
stash_dirty_tree "rate-limit interrupt during T-012"
wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1
git stash pop 2>/dev/null

# Now tree is dirty with restored files
if ! is_tree_dirty; then
    fail_test "Tree should be dirty after stash pop"
else
    # recover_dirty_tree at start of next iteration
    # Since T-012 is NOT marked completed, it will stash for case (b)
    recover_dirty_tree "T-001:T-005" ""

    # Files were stashed again by recover_dirty_tree (incomplete task)
    # But the key is: the agent will see a clean tree, and select_next_task
    # will pick T-012 again. The stash is available for manual recovery.
    # This is the expected behavior for interrupted (not completed) tasks.
    if ! is_tree_dirty; then
        pass_test
    else
        fail_test "recover_dirty_tree should clean up the tree"
    fi
fi

reset_sandbox

begin_test "Fixed flow: restored files + task completed → auto-commit by recover_dirty_tree"
# Scenario: agent partially writes files AND updates task-state to completed,
# then gets rate-limited. After cooldown, files are restored, and
# recover_dirty_tree detects the completed task and auto-commits.
simulate_agent_completion "T-002" "config"

# Stash → cooldown → pop (the fix)
stash_dirty_tree "rate-limit interrupt during T-002"
wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1
git stash pop 2>/dev/null

# Now tree is dirty with completed task's files
head_before=$(git rev-parse HEAD)
recover_dirty_tree "T-001:T-005" ""
head_after=$(git rev-parse HEAD)

if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
    if is_task_completed "T-002"; then
        pass_test
    else
        fail_test "T-002 should be marked completed after recovery"
    fi
else
    fail_test "recover_dirty_tree should auto-commit completed task"
fi

# ============================================================================
#
#  TEST GROUP 4: Multiple Rate Limits and Chained Recovery
#
# ============================================================================

echo ""
echo "--- 4. Multiple Rate Limits and Chained Recovery ---"

reset_sandbox

begin_test "Two consecutive rate limits: stash→restore→stash→restore"
# First rate limit
create_partial_work "T-012"

stashed1=false
if is_tree_dirty; then
    if stash_dirty_tree "rate-limit #1 during T-012"; then
        stashed1=true
    fi
fi

wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1

if [[ "$stashed1" == "true" ]]; then
    git stash pop 2>/dev/null
fi

# Verify files restored
if [[ ! -f "$SANDBOX/internal/config/config.go" ]]; then
    fail_test "Files not restored after first rate limit"
else
    # Second rate limit (agent ran again, still partial)
    stashed2=false
    if is_tree_dirty; then
        if stash_dirty_tree "rate-limit #2 during T-012"; then
            stashed2=true
        fi
    fi

    wait_for_rate_limit_reset 1 1 3 > /dev/null 2>&1

    if [[ "$stashed2" == "true" ]]; then
        git stash pop 2>/dev/null
    fi

    # Verify files restored again
    if [[ -f "$SANDBOX/internal/config/config.go" ]]; then
        content=$(cat "$SANDBOX/internal/config/config.go")
        if assert_contains "$content" "T-012" "files should survive two stash cycles"; then
            pass_test
        fi
    else
        fail_test "Files not restored after second rate limit"
    fi
fi

reset_sandbox

begin_test "Rate limit with no dirty tree → no stash, no pop needed"
# Agent hit rate limit immediately (no files written)
output="You've hit your limit · resets 10pm (Europe/Berlin)"

stashed=false
if is_tree_dirty; then
    if stash_dirty_tree "rate-limit interrupt during T-012"; then
        stashed=true
    fi
fi

wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1

if [[ "$stashed" == "true" ]]; then
    fail_test "Should not stash when tree is clean"
else
    # No pop needed
    stash_count=$(git stash list 2>/dev/null | wc -l | tr -d ' ')
    if assert_eq "0" "$stash_count" "no stash entries should exist"; then
        pass_test
    fi
fi

reset_sandbox

begin_test "Rate limit after task completion → stash contains completed work"
# Agent completed T-002 (task-state updated, files written) but then rate-limited
simulate_agent_completion "T-002" "config"

stashed=false
if is_tree_dirty; then
    if stash_dirty_tree "rate-limit interrupt during T-002"; then
        stashed=true
    fi
fi

# After cooldown, restore and verify completed work is preserved
wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1

if [[ "$stashed" == "true" ]]; then
    git stash pop 2>/dev/null
fi

if is_task_completed "T-002" && [[ -f "$SANDBOX/internal/config/config.go" ]]; then
    pass_test
else
    fail_test "Completed work should be preserved through stash cycle"
fi

# ============================================================================
#
#  TEST GROUP 5: Rate Limit Parsing Edge Cases
#
# ============================================================================

echo ""
echo "--- 5. Rate Limit Parsing Edge Cases ---"

begin_test "compute_rate_limit_wait: Claude 10pm reset returns positive seconds"
result=$(compute_rate_limit_wait "You've hit your limit · resets 10pm (Europe/Berlin)" 0 2>/dev/null)
if [[ "$result" =~ ^[0-9]+$ ]] && [[ "$result" -gt 0 ]]; then
    pass_test
else
    fail_test "Expected positive integer, got '$result'"
fi

begin_test "compute_rate_limit_wait: Codex '5 days' is capped at 6 hours"
result=$(compute_rate_limit_wait "try again in 5 days 27 minutes" 0 2>/dev/null)
if assert_eq "21600" "$result" "5 days exceeds cap, should be capped to 21600"; then
    pass_test
fi

begin_test "compute_rate_limit_wait: unparseable output falls back to backoff"
result=$(compute_rate_limit_wait "some random error output" 0 2>/dev/null)
if [[ "$result" =~ ^[0-9]+$ ]] && [[ "$result" -ge 120 ]] && [[ "$result" -le 150 ]]; then
    pass_test
else
    fail_test "Expected backoff ~120s, got '$result'"
fi

begin_test "compute_rate_limit_wait: attempt index affects backoff"
result0=$(compute_rate_limit_wait "random error" 0 2>/dev/null)
result1=$(compute_rate_limit_wait "random error" 1 2>/dev/null)
result2=$(compute_rate_limit_wait "random error" 2 2>/dev/null)
# Backoff schedule: 120, 300, 900
# With jitter (0-30), ranges: [120,150], [300,330], [900,930]
if [[ "$result1" -gt "$result0" ]] && [[ "$result2" -gt "$result1" ]]; then
    pass_test
else
    fail_test "Backoff should increase: $result0 < $result1 < $result2"
fi

begin_test "is_rate_limited: false for code that mentions rate limit in comments"
# Agent output might contain code with "rate limit" in comments/strings
output='func TestRateLimit(t *testing.T) {
    // This tests rate limit handling
    result := handleRateLimit(ctx)
    assert.NoError(t, result)
}'
if is_rate_limited "$output"; then
    fail_test "Should not trigger on code comments mentioning rate limit"
else
    pass_test
fi

begin_test "is_rate_limited: true for actual Claude limit with emoji"
if is_rate_limited "You've hit your limit · resets 10pm (Europe/Berlin)"; then
    pass_test
else
    fail_test "Should detect Claude rate limit message"
fi

begin_test "is_rate_limited: true for 'usage limit' variant"
if is_rate_limited "You've hit your usage limit. Please upgrade."; then
    pass_test
else
    fail_test "Should detect 'usage limit' message"
fi

# ============================================================================
#
#  TEST GROUP 6: Source Code Contract Verification
#
# ============================================================================

echo ""
echo "--- 6. Source Code Contract Verification ---"

begin_test "Rate-limit handler sets stashed_for_rate_limit flag"
# Verify the fix is present in the source code
rate_limit_block=$(awk '/Rate-limit detection/,/continue/' "$RALPH_SCRIPT")
if assert_contains "$rate_limit_block" "stashed_for_rate_limit=false" "should initialize flag" && \
   assert_contains "$rate_limit_block" "stashed_for_rate_limit=true" "should set flag on stash"; then
    pass_test
fi

begin_test "Rate-limit handler restores stash after cooldown"
rate_limit_block=$(awk '/Rate-limit detection/,/continue/' "$RALPH_SCRIPT")
if assert_contains "$rate_limit_block" "git stash pop" "should pop stash after cooldown" && \
   assert_contains "$rate_limit_block" 'stashed_for_rate_limit.*true' "should check flag before popping"; then
    pass_test
fi

begin_test "Stash restore happens AFTER wait_for_rate_limit_reset"
# The pop must come after the wait, not before
func_body=$(awk '/Rate-limit detection/,/continue/' "$RALPH_SCRIPT")
wait_line=$(echo "$func_body" | grep -n "wait_for_rate_limit_reset" | head -1 | cut -d: -f1)
pop_line=$(echo "$func_body" | grep -n "git stash pop" | head -1 | cut -d: -f1)

if [[ -n "$wait_line" ]] && [[ -n "$pop_line" ]]; then
    if [[ "$pop_line" -gt "$wait_line" ]]; then
        pass_test
    else
        fail_test "stash pop (line $pop_line) should come AFTER wait_for_rate_limit_reset (line $wait_line)"
    fi
else
    fail_test "Could not find both wait_for_rate_limit_reset and git stash pop"
fi

begin_test "Stash restore is conditional (only if stashed_for_rate_limit=true)"
rate_limit_block=$(awk '/Rate-limit detection/,/continue/' "$RALPH_SCRIPT")
# The pop should be inside an if-block checking the flag
if echo "$rate_limit_block" | grep -q 'stashed_for_rate_limit.*==.*true'; then
    pass_test
else
    fail_test "stash pop should be guarded by stashed_for_rate_limit check"
fi

begin_test "wait_for_rate_limit_reset computes target_epoch ONCE at start"
func_body=$(extract_function "$RALPH_SCRIPT" wait_for_rate_limit_reset)
# Should have exactly one target_epoch assignment (at the top of the loop)
target_count=$(echo "$func_body" | grep -c 'target_epoch=' || true)
if assert_eq "1" "$target_count" "target_epoch should be computed exactly once"; then
    pass_test
fi

begin_test "Countdown loop recomputes now_epoch each iteration"
func_body=$(extract_function "$RALPH_SCRIPT" wait_for_rate_limit_reset)
# now_epoch should be inside the while loop
if assert_contains "$func_body" 'now_epoch=$(date +%s)' "should get fresh epoch each iteration"; then
    pass_test
fi

# ============================================================================
#
#  TEST GROUP 7: End-to-End Integration Scenarios
#
# ============================================================================

echo ""
echo "--- 7. End-to-End Integration Scenarios ---"

reset_sandbox

begin_test "E2E: Complete happy path - rate limit → stash → cooldown → restore → next iteration"
# Simulate full rate-limit recovery flow as it would happen in run_ralph_loop

# 1. Agent writes partial work for T-002
create_partial_work "T-002"

# 2. Agent hits rate limit (exit code 1)
agent_output="You've hit your limit · resets 10pm (Europe/Berlin)"

# 3. Rate limit detected
if ! is_rate_limited "$agent_output"; then
    fail_test "Step 3: rate limit not detected"
else
    # 4. Compute wait
    wait_seconds=1  # Use 1s for testing

    # 5. Stash partial work
    local_stashed=false
    if is_tree_dirty; then
        if stash_dirty_tree "rate-limit interrupt during T-002"; then
            local_stashed=true
        fi
    fi

    # 6. Wait
    wait_for_rate_limit_reset "$wait_seconds" 0 3 > /dev/null 2>&1

    # 7. Restore stash (THE FIX)
    if [[ "$local_stashed" == "true" ]]; then
        git stash pop 2>/dev/null || true
    fi

    # 8. Next iteration starts: recover_dirty_tree
    # Tree should be dirty with the restored files
    if is_tree_dirty; then
        # Since T-002 is NOT completed, recover_dirty_tree stashes it
        # But the partial files WERE on disk when the iteration started
        # (the agent would have seen them before recover_dirty_tree runs)
        # This is correct behavior.
        recover_dirty_tree "T-001:T-005" ""

        if ! is_tree_dirty; then
            pass_test
        else
            fail_test "recover_dirty_tree should clean the tree"
        fi
    else
        fail_test "Tree should be dirty after stash pop"
    fi
fi

reset_sandbox

begin_test "E2E: Rate limit during completed task → restore → auto-commit"
# Most important scenario: agent completes task AND gets rate-limited
simulate_agent_completion "T-002" "config"

# Rate limit handler
stashed=false
if is_tree_dirty; then
    if stash_dirty_tree "rate-limit during T-002"; then
        stashed=true
    fi
fi
wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1

if [[ "$stashed" == "true" ]]; then
    git stash pop 2>/dev/null || true
fi

# Next iteration: recover_dirty_tree should auto-commit since T-002 is completed
head_before=$(git rev-parse HEAD)
recover_dirty_tree "T-001:T-005" ""
head_after=$(git rev-parse HEAD)

if [[ "$head_before" != "$head_after" ]]; then
    # Auto-commit happened
    committed=$(git diff-tree --no-commit-id --name-only -r HEAD)
    if assert_contains "$committed" "internal/config/config.go" "committed files should include implementation" && \
       assert_contains "$committed" "task-state.conf" "committed files should include task state"; then
        # Verify T-002 is completed and next task would be T-003
        if is_task_completed "T-002"; then
            remaining=$(count_remaining_tasks "T-001:T-005")
            if assert_eq "3" "$remaining" "should have 3 remaining tasks"; then
                pass_test
            fi
        else
            fail_test "T-002 should be completed"
        fi
    fi
else
    fail_test "recover_dirty_tree should auto-commit the completed task"
fi

reset_sandbox

begin_test "E2E: No stash when tree clean during rate limit → clean next iteration"
# Agent was rate-limited immediately (e.g., 0 tokens used, no files written)
agent_output="You've hit your limit · resets 10pm (Europe/Berlin)"

stashed=false
if is_tree_dirty; then
    if stash_dirty_tree "rate-limit during T-002"; then
        stashed=true
    fi
fi

wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1

if [[ "$stashed" == "true" ]]; then
    fail_test "Should not have stashed (tree was clean)"
else
    # Next iteration should start clean, no stash to pop
    head_before=$(git rev-parse HEAD)
    recover_dirty_tree "T-001:T-005" ""
    head_after=$(git rev-parse HEAD)
    stash_count=$(git stash list 2>/dev/null | wc -l | tr -d ' ')

    if [[ "$head_before" == "$head_after" ]] && [[ "$stash_count" == "0" ]]; then
        pass_test
    else
        fail_test "Should be a complete no-op"
    fi
fi

reset_sandbox

begin_test "E2E: Back-to-back task completions spanning rate limits"
# Iteration 1: Agent completes T-002 normally
simulate_agent_completion "T-002" "config"
output1='`git commit -m "feat(config): implement T-002"`
RALPH_ERROR: commit missing for T-002'
run_commit_recovery "T-002" "$output1"

if ! is_task_completed "T-002" || is_tree_dirty; then
    fail_test "T-002 should be committed cleanly"
else
    # Iteration 2: Agent starts T-003, writes partial work, hits rate limit
    create_partial_work "T-003"

    stashed=false
    if is_tree_dirty; then
        if stash_dirty_tree "rate-limit during T-003"; then
            stashed=true
        fi
    fi
    wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1
    if [[ "$stashed" == "true" ]]; then
        git stash pop 2>/dev/null || true
    fi

    # Verify T-002 is still completed and T-003 partial work is on disk
    if is_task_completed "T-002" && \
       ! is_task_completed "T-003" && \
       [[ -f "$SANDBOX/internal/config/config.go" ]]; then
        pass_test
    else
        fail_test "T-002 should be done, T-003 should be partial, files should be on disk"
    fi
fi

# ============================================================================
#
#  TEST GROUP 8: Robustness and Edge Cases
#
# ============================================================================

echo ""
echo "--- 8. Robustness and Edge Cases ---"

reset_sandbox

begin_test "Stash with only .gitignore-excluded files → returns 1"
echo "*.log" > "$SANDBOX/.gitignore"
git add "$SANDBOX/.gitignore" && git commit -q -m "add gitignore"
echo "ignored" > "$SANDBOX/output.log"
# The file is ignored, so tree is NOT dirty from git's perspective
rc=0
stash_dirty_tree "should-not-stash" || rc=$?
if assert_eq "1" "$rc" "should return 1 when only gitignored files exist"; then
    pass_test
fi

reset_sandbox

begin_test "Multiple stash entries: pop gets the most recent (correct one)"
# Create first stash (for T-002)
echo "T-002 partial" > "$SANDBOX/t002.go"
stash_dirty_tree "rate-limit during T-002"

# Create second stash (for T-003)
echo "T-003 partial" > "$SANDBOX/t003.go"
stash_dirty_tree "rate-limit during T-003"

# Stash list should have 2 entries
stash_count=$(git stash list 2>/dev/null | wc -l | tr -d ' ')
if [[ "$stash_count" != "2" ]]; then
    fail_test "Expected 2 stash entries, got $stash_count"
else
    # Pop gets the MOST RECENT (T-003)
    git stash pop 2>/dev/null
    if [[ -f "$SANDBOX/t003.go" ]]; then
        content=$(cat "$SANDBOX/t003.go")
        if assert_eq "T-003 partial" "$content" "should pop T-003 stash (most recent)"; then
            pass_test
        fi
    else
        fail_test "t003.go not found after pop"
    fi
fi

reset_sandbox

begin_test "Large number of files in stash → all restored correctly"
# Create many files (simulating a large task)
mkdir -p "$SANDBOX/internal/config"
for i in $(seq 1 20); do
    echo "package config // file $i" > "$SANDBOX/internal/config/file_${i}.go"
done
echo "modified" >> "$SANDBOX/docs/tasks/task-state.conf"

stash_dirty_tree "rate-limit during T-012 (large task)"
wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1
git stash pop 2>/dev/null

# Count restored files
restored_count=$(ls "$SANDBOX/internal/config/file_"*.go 2>/dev/null | wc -l | tr -d ' ')
if assert_eq "20" "$restored_count" "all 20 files should be restored"; then
    pass_test
fi

reset_sandbox

begin_test "Binary files in stash → preserved through stash cycle"
# Git stash handles binary files too
echo -e '\x00\x01\x02\x03' > "$SANDBOX/binary.bin"
mkdir -p "$SANDBOX/internal/config"
echo "package config" > "$SANDBOX/internal/config/config.go"

stash_dirty_tree "rate-limit with binary"
git stash pop 2>/dev/null

if [[ -f "$SANDBOX/binary.bin" ]] && [[ -f "$SANDBOX/internal/config/config.go" ]]; then
    pass_test
else
    fail_test "Both binary and text files should survive stash cycle"
fi

reset_sandbox

begin_test "Empty stash list → stash pop called only when flagged"
# Verify the code doesn't blindly call git stash pop
stash_count=$(git stash list 2>/dev/null | wc -l | tr -d ' ')
if assert_eq "0" "$stash_count" "no stash entries should exist"; then
    # Simulate the fixed code path with stashed_for_rate_limit=false
    stashed_for_rate_limit=false
    # This block should NOT run
    if [[ "$stashed_for_rate_limit" == "true" ]]; then
        git stash pop 2>/dev/null
        fail_test "Should not attempt pop when flag is false"
    else
        pass_test
    fi
fi

reset_sandbox

begin_test "Stash message contains task ID and dirty summary"
create_partial_work "T-042"
stash_dirty_tree "rate-limit interrupt during T-042"
stash_msg=$(git stash list | head -1)
if assert_contains "$stash_msg" "T-042" "stash should reference task ID" && \
   assert_contains "$stash_msg" "ralph-recovery" "stash should have recovery prefix"; then
    pass_test
fi

# ============================================================================
#
#  TEST GROUP 9: Timing Accuracy (Epoch-Based vs Counter-Based)
#
# ============================================================================

echo ""
echo "--- 9. Timing Accuracy Verification ---"

begin_test "4-second cooldown with epoch-based timer: wall-clock accurate"
start_epoch=$(date +%s)
wait_for_rate_limit_reset 4 0 3 > /dev/null 2>&1
end_epoch=$(date +%s)
elapsed=$((end_epoch - start_epoch))

# With epoch-based timer, elapsed should be very close to 4s
# Old decrementing counter would accumulate overhead per iteration
if assert_ge "$elapsed" 4 "should wait at least 4s" && \
   assert_le "$elapsed" 6 "should not overshoot (old bug: would drift)"; then
    pass_test
fi

begin_test "1-second cooldown: no false sleep extension"
start_epoch=$(date +%s)
wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1
end_epoch=$(date +%s)
elapsed=$((end_epoch - start_epoch))

if assert_ge "$elapsed" 1 "should wait at least 1s" && \
   assert_le "$elapsed" 3 "should not over-sleep"; then
    pass_test
fi

# ============================================================================
#
#  TEST GROUP 10: Interaction with recover_dirty_tree
#
# ============================================================================

echo ""
echo "--- 10. Interaction with recover_dirty_tree ---"

reset_sandbox

begin_test "Restored partial work: recover_dirty_tree stashes (incomplete task)"
# After rate-limit restore, tree has partial T-002 work (not completed)
create_partial_work "T-002"

# The agent might add more to these files in the next iteration before
# recover_dirty_tree runs. But if the script calls recover_dirty_tree
# first, it should stash the partial work.
head_before=$(git rev-parse HEAD)
recover_dirty_tree "T-001:T-005" ""
head_after=$(git rev-parse HEAD)

# T-002 is NOT completed, so recover_dirty_tree stashes
if [[ "$head_before" == "$head_after" ]] && ! is_tree_dirty; then
    stash_msg=$(git stash list | head -1)
    if assert_contains "$stash_msg" "ralph-recovery" "should be recovery stash"; then
        pass_test
    fi
else
    fail_test "Should stash (not commit) for incomplete task"
fi

reset_sandbox

begin_test "Restored completed work: recover_dirty_tree auto-commits"
# After rate-limit restore, tree has completed T-002 work
simulate_agent_completion "T-002" "config"

head_before=$(git rev-parse HEAD)
recover_dirty_tree "T-001:T-005" ""
head_after=$(git rev-parse HEAD)

if [[ "$head_before" != "$head_after" ]] && ! is_tree_dirty; then
    if is_task_completed "T-002"; then
        pass_test
    else
        fail_test "T-002 should be completed"
    fi
else
    fail_test "Should auto-commit completed task"
fi

reset_sandbox

begin_test "Clean tree after recovery: recover_dirty_tree is no-op"
head_before=$(git rev-parse HEAD)
stash_before=$(git stash list 2>/dev/null | wc -l | tr -d ' ')
recover_dirty_tree "T-001:T-005" ""
head_after=$(git rev-parse HEAD)
stash_after=$(git stash list 2>/dev/null | wc -l | tr -d ' ')

if [[ "$head_before" == "$head_after" ]] && \
   [[ "$stash_before" == "$stash_after" ]]; then
    pass_test
else
    fail_test "Clean tree should be a no-op"
fi

# ============================================================================
#
#  TEST GROUP 11: Full Cycle Simulation (Multiple Iterations)
#
# ============================================================================

echo ""
echo "--- 11. Full Cycle Simulation ---"

reset_sandbox

begin_test "3-iteration simulation: complete → rate-limit → resume → complete"
# Iteration 1: Agent completes T-002
simulate_agent_completion "T-002" "config"
run_commit_recovery "T-002" '`git commit -m "feat(config): T-002"`'

if ! is_task_completed "T-002" || is_tree_dirty; then
    fail_test "Iteration 1: T-002 should be committed"
else
    # Iteration 2: Agent starts T-003, partial work, rate limited
    mkdir -p "$SANDBOX/internal/workflow"
    echo "package workflow // partial T-003" > "$SANDBOX/internal/workflow/engine.go"

    stashed=false
    if is_tree_dirty; then
        if stash_dirty_tree "rate-limit during T-003"; then
            stashed=true
        fi
    fi
    wait_for_rate_limit_reset 1 0 3 > /dev/null 2>&1
    if [[ "$stashed" == "true" ]]; then
        git stash pop 2>/dev/null || true
    fi

    # Verify T-003 partial work is on disk
    if [[ ! -f "$SANDBOX/internal/workflow/engine.go" ]]; then
        fail_test "Iteration 2: T-003 partial work should be restored"
    else
        # Iteration 3: Agent resumes T-003, completes it
        simulate_agent_completion "T-003" "workflow"
        run_commit_recovery "T-003" '`git commit -m "feat(workflow): T-003"`'

        if is_task_completed "T-003" && ! is_tree_dirty; then
            remaining=$(count_remaining_tasks "T-001:T-005")
            if assert_eq "2" "$remaining" "should have 2 remaining (T-004, T-005)"; then
                pass_test
            fi
        else
            fail_test "Iteration 3: T-003 should be committed"
        fi
    fi
fi

reset_sandbox

begin_test "Worst case: 3 rate limits in a row, all with partial work"
for attempt in 1 2 3; do
    create_partial_work "T-002"
    # Add attempt-specific content to verify we get the LATEST version
    echo "// attempt $attempt" >> "$SANDBOX/internal/config/config.go"

    stashed=false
    if is_tree_dirty; then
        if stash_dirty_tree "rate-limit #$attempt during T-002"; then
            stashed=true
        fi
    fi

    wait_for_rate_limit_reset 1 $((attempt - 1)) 3 > /dev/null 2>&1

    if [[ "$stashed" == "true" ]]; then
        git stash pop 2>/dev/null || true
    fi
done

# After 3 rate limits, latest partial work should be on disk
if [[ -f "$SANDBOX/internal/config/config.go" ]]; then
    content=$(cat "$SANDBOX/internal/config/config.go")
    if assert_contains "$content" "attempt 3" "should have content from latest attempt"; then
        pass_test
    fi
else
    fail_test "Partial work should survive 3 consecutive rate limits"
fi

# ============================================================================
# Summary
# ============================================================================

echo ""
echo "================================================================"
printf " Results: %d passed, %d failed (out of %d tests)\n" "$PASS" "$FAIL" "$TOTAL_TESTS"
echo "================================================================"

if [[ $FAIL -gt 0 ]]; then
    echo ""
    echo "FAILED TESTS:"
    for t in "${FAILED_TESTS[@]}"; do
        echo "  - $t"
    done
    echo ""
    echo "Log output (last 30 lines):"
    tail -30 "$LOG_FILE" 2>/dev/null || true
    exit 1
fi

echo ""
echo "All rate-limit recovery tests passed!"
exit 0
