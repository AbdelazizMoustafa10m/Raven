#!/usr/bin/env bash
#
# test_signal_handling.sh -- Comprehensive tests for strict control-signal
# parsing in ralph-lib.sh and phase completion guarding in phase-pipeline.sh.
#
# Usage: ./scripts/test_signal_handling.sh
#

set -euo pipefail

if ((BASH_VERSINFO[0] < 4)); then
    printf 'ERROR: Bash 4+ is required (found %s).\n' "$BASH_VERSION" >&2
    exit 1
fi

PASS=0
FAIL=0
TOTAL_TESTS=0
TEST_NAME=""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RALPH_LIB_SCRIPT="$SCRIPT_DIR/ralph-lib.sh"
PHASE_PIPELINE_SCRIPT="$SCRIPT_DIR/phase-pipeline.sh"
TMP_DIR=""
ACTIVE_LOG_FILE=""

begin_test() {
    TEST_NAME="$1"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    printf "  TEST %02d: %-66s " "$TOTAL_TESTS" "$TEST_NAME"
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

assert_nonzero() {
    local rc="$1"
    local msg="${2:-}"
    if [[ "$rc" -ne 0 ]]; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected non-zero return code"
    echo "    Actual:   $rc"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    return 1
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="${3:-}"
    if echo "$haystack" | grep -Fq -- "$needle"; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected to contain: '$needle'"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    return 1
}

assert_not_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="${3:-}"
    if ! echo "$haystack" | grep -Fq -- "$needle"; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected NOT to contain: '$needle'"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    return 1
}

assert_nonempty() {
    local value="$1"
    local msg="${2:-}"
    if [[ -n "$value" ]]; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected non-empty value"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    return 1
}

cleanup() {
    if [[ -n "$TMP_DIR" && -d "$TMP_DIR" ]]; then
        rm -rf "$TMP_DIR"
    fi
}
trap cleanup EXIT

# Extract a bash function by name from a file using brace counting.
extract_function() {
    local file="$1"
    local func_name="$2"
    awk -v fname="$func_name" '
        BEGIN { found=0; depth=0 }
        !found && $0 ~ "^"fname"\\(\\)" { found=1 }
        found {
            for (i=1; i<=length($0); i++) {
                c = substr($0, i, 1)
                if (c == "{") depth++
                else if (c == "}") depth--
            }
            print
            if (depth == 0 && found) { found=0; exit }
        }
    ' "$file"
}

load_function_or_die() {
    local file="$1"
    local func_name="$2"
    local src
    src="$(extract_function "$file" "$func_name")"
    if [[ -z "$src" ]]; then
        echo "ERROR: Could not extract function '$func_name' from $file"
        exit 1
    fi
    eval "$src"
}

if [[ ! -f "$RALPH_LIB_SCRIPT" || ! -f "$PHASE_PIPELINE_SCRIPT" ]]; then
    echo "ERROR: Required scripts not found under $SCRIPT_DIR"
    exit 1
fi

TMP_DIR="$(mktemp -d /tmp/signal-handling-test-XXXXXX)"
ACTIVE_LOG_FILE="$TMP_DIR/test.log"
: > "$ACTIVE_LOG_FILE"
STUB_REMAINING_COUNTER_FILE="$TMP_DIR/remaining-counter.txt"
: > "$STUB_REMAINING_COUNTER_FILE"

# Pull in real functions under test.
load_function_or_die "$RALPH_LIB_SCRIPT" _extract_control_signal_line
load_function_or_die "$RALPH_LIB_SCRIPT" run_ralph_loop
load_function_or_die "$PHASE_PIPELINE_SCRIPT" extract_implementation_block_reason
load_function_or_die "$PHASE_PIPELINE_SCRIPT" count_phase_remaining_tasks
load_function_or_die "$PHASE_PIPELINE_SCRIPT" run_implementation

# ---------------------------------------------------------------------------
# Shared stubs and globals
# ---------------------------------------------------------------------------

# Styling globals used by extracted functions.
_BOLD=""
_DIM=""
_RESET=""
_YELLOW=""
_RED=""
_GREEN=""

_SYM_RUNNING="[*]"
_SYM_WARN="[!]"
_SYM_CROSS="[x]"
_SYM_CHECK="[ok]"
_SYM_STEP=">"
_SYM_BLOCK="[x]"
_SYM_OK="[ok]"

ALL_PHASES="1"
AGENT_LABEL="TestAgent"
EFFORT_LABEL="Effort"
LOG_PREFIX="ralph-test"
DEFAULT_MAX_LIMIT_WAITS=1
MAX_LIMIT_WAITS=1
SLEEP_BETWEEN_ITERATIONS=0
COOLDOWN_AFTER_ERROR=30

RUN_LOOP_AGENT_OUTPUT_FILE="$TMP_DIR/agent-output.txt"
RUN_LOOP_LOG_FILE="$TMP_DIR/ralph-loop.log"
RUN_LOOP_RC=0
RUN_LOOP_LOG_CONTENT=""
TEST_AGENT_RC=0
TEST_REMAINING=1
TEST_SELECTION="T-001"
STUB_REMAINING_AFTER_FIRST=""

# Phase-pipeline globals used by run_implementation.
SKIP_IMPLEMENT="false"
PROJECT_ROOT="$TMP_DIR/project"
RUN_DIR="$TMP_DIR/run"
REPORT_DIR="$TMP_DIR/report"
IMPL_AGENT="codex"
IMPL_MODEL=""
IMPL_MODEL_PRESET=""
PHASE_ID="1"
IMPLEMENT_STATUS="not-run"
IMPLEMENT_REASON=""
BASE_BRANCH="main"
DRY_RUN="false"

CAPTURE_CALLS=0
IMPL_CAPTURE_RC=0
IMPL_CAPTURE_OUTPUT=""
VERIFY_CAPTURE_RC=0
VERIFY_CAPTURE_OUTPUT=""

STUB_REMAINING_RC=0
STUB_REMAINING_OUTPUT="1"
STUB_VERIFIER_RC=0
STUB_VERIFIER_OUTPUT="0"

LAST_LOG_STEP=""
PERSIST_CALLS=0
DIE_CALLS=0
LAST_DIE_MSG=""
RUN_IMPL_RC=0

append_test_log() {
    printf '%s\n' "$1" >> "$ACTIVE_LOG_FILE"
}

sleep() {
    :
}

log() {
    append_test_log "$1"
}

log_step() {
    LAST_LOG_STEP="$2"
    append_test_log "$2"
}

log_info() {
    log_step "$_SYM_STEP" "$*"
}

log_warn() {
    log_step "$_SYM_WARN" "$*"
}

log_error() {
    log_step "$_SYM_CROSS" "$*"
}

log_success() {
    log_step "$_SYM_CHECK" "$*"
}

log_blocked() {
    log_step "$_SYM_BLOCK" "$*"
}

log_rate_limit() {
    log_step "$_SYM_WARN" "$*"
}

log_header() {
    :
}

_draw_task_progress() {
    :
}

_ralph_render_banner() {
    :
}

pre_agent_setup() {
    :
}

ralph_register_temp_file() {
    :
}

get_phase_name() {
    echo "Signal Handling Phase"
}

get_phase_range() {
    echo "T-001:T-003"
}

_phase_icon() {
    echo "P1"
}

recover_dirty_tree() {
    :
}

select_next_task() {
    echo "$TEST_SELECTION"
}

get_task_list_for_single() {
    echo "- [ ] T-001: Sample Task"
}

generate_prompt() {
    echo "prompt"
}

get_dry_run_command() {
    :
}

run_agent() {
    local _prompt_file="$1"
    local _model="$2"
    local _effort="$3"
    cat "$RUN_LOOP_AGENT_OUTPUT_FILE"
    return "$TEST_AGENT_RC"
}

is_rate_limited() {
    local _output="$1"
    return 1
}

compute_rate_limit_wait() {
    local _output="$1"
    local _attempt="$2"
    echo "1"
}

get_backoff_seconds() {
    local _attempt="$1"
    echo "1"
}

is_tree_dirty() {
    return 1
}

stash_dirty_tree() {
    local _reason="$1"
    return 0
}

wait_for_rate_limit_reset() {
    local _seconds="$1"
    local _wait_cycle="$2"
    local _max_cycles="$3"
    :
}

run_commit_recovery() {
    local _task="$1"
    local _output="${2:-}"
    return 0
}

count_remaining_tasks() {
    local _range="$1"
    local call_count="0"
    if [[ -f "$STUB_REMAINING_COUNTER_FILE" ]]; then
        call_count="$(cat "$STUB_REMAINING_COUNTER_FILE" 2>/dev/null || echo "0")"
    fi

    if [[ -n "$STUB_REMAINING_AFTER_FIRST" && "$call_count" -ge 1 ]]; then
        echo "$STUB_REMAINING_AFTER_FIRST"
    else
        echo "$STUB_REMAINING_OUTPUT"
    fi
    printf '%s\n' "$((call_count + 1))" > "$STUB_REMAINING_COUNTER_FILE"
    return "$STUB_REMAINING_RC"
}

assert_expected_branch() {
    :
}

die() {
    LAST_DIE_MSG="$1"
    DIE_CALLS=$((DIE_CALLS + 1))
    return 1
}

count_phase_remaining_tasks() {
    local _phase_id="$1"
    echo "$STUB_REMAINING_OUTPUT"
    return "$STUB_REMAINING_RC"
}

capture_cmd() {
    local output_file="$1"
    shift

    local cmd="${1:-}"
    CAPTURE_CALLS=$((CAPTURE_CALLS + 1))

    if [[ "$cmd" == *"/ralph_"*.sh ]]; then
        printf '%s\n' "$IMPL_CAPTURE_OUTPUT" > "$output_file"
        return "$IMPL_CAPTURE_RC"
    fi

    printf '%s\n' "$VERIFY_CAPTURE_OUTPUT" > "$output_file"
    return "$VERIFY_CAPTURE_RC"
}

persist_metadata() {
    PERSIST_CALLS=$((PERSIST_CALLS + 1))
}

setup_pipeline_env() {
    mkdir -p "$PROJECT_ROOT/scripts" "$PROJECT_ROOT/scripts/review" "$RUN_DIR" "$REPORT_DIR"

    cat > "$PROJECT_ROOT/scripts/ralph_codex.sh" <<'EOF_IMPL'
#!/usr/bin/env bash
echo "impl"
EOF_IMPL
    chmod +x "$PROJECT_ROOT/scripts/ralph_codex.sh"

    cat > "$PROJECT_ROOT/scripts/task-state.sh" <<'EOF_TASK_STATE'
#!/usr/bin/env bash
echo "0"
EOF_TASK_STATE
    chmod +x "$PROJECT_ROOT/scripts/task-state.sh"
}

reset_loop_case() {
    : > "$ACTIVE_LOG_FILE"
    : > "$RUN_LOOP_LOG_FILE"
    TEST_AGENT_RC=0
    TEST_SELECTION="T-001"
    STUB_REMAINING_AFTER_FIRST=""
    printf '%s\n' "0" > "$STUB_REMAINING_COUNTER_FILE"
}

run_loop_case() {
    local output="$1"
    local rc="${2:-0}"
    local remaining="${3:-1}"
    local remaining_after_first="${4:-}"

    reset_loop_case
    TEST_AGENT_RC="$rc"
    STUB_REMAINING_RC=0
    STUB_REMAINING_OUTPUT="$remaining"
    STUB_REMAINING_AFTER_FIRST="$remaining_after_first"
    printf '%s\n' "$output" > "$RUN_LOOP_AGENT_OUTPUT_FILE"

    set +e
    (
        LOG_FILE="$RUN_LOOP_LOG_FILE"
        run_ralph_loop "1" "1" "false" "test-model" "high"
    ) >/dev/null 2>&1
    RUN_LOOP_RC=$?
    set -e

    RUN_LOOP_LOG_CONTENT="$(cat "$ACTIVE_LOG_FILE")"
}

reset_impl_case() {
    : > "$ACTIVE_LOG_FILE"
    IMPLEMENT_STATUS="not-run"
    IMPLEMENT_REASON=""
    IMPL_MODEL=""
    CAPTURE_CALLS=0
    IMPL_CAPTURE_RC=0
    IMPL_CAPTURE_OUTPUT=""
    VERIFY_CAPTURE_RC=0
    VERIFY_CAPTURE_OUTPUT=""
    STUB_REMAINING_RC=0
    STUB_REMAINING_OUTPUT="0"
    STUB_VERIFIER_RC=0
    STUB_VERIFIER_OUTPUT="0"
    LAST_LOG_STEP=""
    PERSIST_CALLS=0
    DIE_CALLS=0
    LAST_DIE_MSG=""
}

run_impl_case() {
    set +e
    run_implementation >/dev/null 2>&1
    RUN_IMPL_RC=$?
    set -e
}

setup_pipeline_env

echo ""
echo "Signal Handling + Completion Guard Tests"
echo "========================================="
echo ""

echo "--- ralph-lib strict control-signal parsing ---"

begin_test "Prose with PHASE_COMPLETE/TASK_BLOCKED/RALPH_ERROR does not trigger"
run_loop_case "DetectSignals() docs: PHASE_COMPLETE/TASK_BLOCKED/RALPH_ERROR are reserved tokens."
if assert_eq "0" "$RUN_LOOP_RC" && \
   assert_not_contains "$RUN_LOOP_LOG_CONTENT" "PHASE_COMPLETE signal received!" && \
   assert_not_contains "$RUN_LOOP_LOG_CONTENT" "No more tasks available in this phase." && \
   assert_not_contains "$RUN_LOOP_LOG_CONTENT" "RALPH_ERROR" && \
   assert_contains "$RUN_LOOP_LOG_CONTENT" "No task completed in this iteration."; then
    pass_test
fi

begin_test "Markdown bullet with backticked PHASE_COMPLETE does not trigger"
run_loop_case $'- Update docs with `PHASE_COMPLETE` example for humans.'
if assert_eq "0" "$RUN_LOOP_RC" && \
   assert_not_contains "$RUN_LOOP_LOG_CONTENT" "PHASE_COMPLETE signal received!" && \
   assert_contains "$RUN_LOOP_LOG_CONTENT" "No task completed in this iteration."; then
    pass_test
fi

begin_test "Long sentence mentions token without control intent does not trigger"
run_loop_case "If this passes we might mention PHASE_COMPLETE in release notes, not as a signal."
if assert_eq "0" "$RUN_LOOP_RC" && \
   assert_not_contains "$RUN_LOOP_LOG_CONTENT" "PHASE_COMPLETE signal received!" && \
   assert_contains "$RUN_LOOP_LOG_CONTENT" "No task completed in this iteration."; then
    pass_test
fi

begin_test "Exact PHASE_COMPLETE line triggers completion branch"
run_loop_case "PHASE_COMPLETE" 0 1 0
if assert_eq "0" "$RUN_LOOP_RC" && \
   assert_contains "$RUN_LOOP_LOG_CONTENT" "PHASE_COMPLETE signal received!"; then
    pass_test
fi

begin_test "Timestamp-prefixed PHASE_COMPLETE line triggers completion branch"
run_loop_case "[2026-02-18 10:05:00] PHASE_COMPLETE" 0 1 0
if assert_eq "0" "$RUN_LOOP_RC" && \
   assert_contains "$RUN_LOOP_LOG_CONTENT" "PHASE_COMPLETE signal received!"; then
    pass_test
fi

begin_test "TASK_BLOCKED: reason triggers blocked branch"
run_loop_case "TASK_BLOCKED: waiting on T-010"
if assert_eq "0" "$RUN_LOOP_RC" && \
   assert_contains "$RUN_LOOP_LOG_CONTENT" "TASK_BLOCKED: waiting on T-010" && \
   assert_contains "$RUN_LOOP_LOG_CONTENT" "No more tasks available in this phase."; then
    pass_test
fi

begin_test "RALPH_ERROR: reason triggers error branch"
run_loop_case "RALPH_ERROR: verifier failed"
if assert_eq "0" "$RUN_LOOP_RC" && \
   assert_contains "$RUN_LOOP_LOG_CONTENT" "RALPH_ERROR: verifier failed" && \
   assert_contains "$RUN_LOOP_LOG_CONTENT" "Cooling down for 30s before retry..."; then
    pass_test
fi

echo ""
echo "--- phase-pipeline run_implementation completion guard ---"

begin_test "rc=0 with remaining tasks >0 => blocked + non-zero"
reset_impl_case
IMPL_CAPTURE_RC=0
IMPL_CAPTURE_OUTPUT="implementation finished"
VERIFY_CAPTURE_RC=0
VERIFY_CAPTURE_OUTPUT="2"
STUB_REMAINING_OUTPUT="2"
run_impl_case
if assert_nonzero "$RUN_IMPL_RC" && \
   assert_eq "blocked" "$IMPLEMENT_STATUS" && \
   assert_nonempty "$IMPLEMENT_REASON"; then
    pass_test
fi

begin_test "rc=0 with remaining=0 => completed + zero"
reset_impl_case
IMPL_CAPTURE_RC=0
IMPL_CAPTURE_OUTPUT="implementation finished"
VERIFY_CAPTURE_RC=0
VERIFY_CAPTURE_OUTPUT="0"
STUB_REMAINING_OUTPUT="0"
run_impl_case
if assert_eq "0" "$RUN_IMPL_RC" && \
   assert_eq "completed" "$IMPLEMENT_STATUS" && \
   assert_eq "" "$IMPLEMENT_REASON"; then
    pass_test
fi

begin_test "Verifier failure => failed + non-zero"
reset_impl_case
IMPL_CAPTURE_RC=0
IMPL_CAPTURE_OUTPUT="implementation finished"
VERIFY_CAPTURE_RC=7
VERIFY_CAPTURE_OUTPUT="verifier failed"
STUB_REMAINING_OUTPUT="0"
STUB_REMAINING_RC=7
run_impl_case
if assert_nonzero "$RUN_IMPL_RC" && \
   assert_eq "failed" "$IMPLEMENT_STATUS"; then
    pass_test
fi

begin_test "Verifier non-numeric result => failed + non-zero"
reset_impl_case
IMPL_CAPTURE_RC=0
IMPL_CAPTURE_OUTPUT="implementation finished"
VERIFY_CAPTURE_RC=0
VERIFY_CAPTURE_OUTPUT="not-a-number"
STUB_REMAINING_OUTPUT="not-a-number"
run_impl_case
if assert_nonzero "$RUN_IMPL_RC" && \
   assert_eq "failed" "$IMPLEMENT_STATUS"; then
    pass_test
fi

echo ""
echo "=========================="
echo "Results: $PASS passed, $FAIL failed"
echo "=========================="

if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi

echo ""
echo "All signal-handling tests passed!"
exit 0
