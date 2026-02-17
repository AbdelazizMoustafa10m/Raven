#!/usr/bin/env bash
#
# test_phase_pipeline.sh -- Unit smoke tests for phase-pipeline failure semantics.
#
# Validates that run_implementation() properly propagates blocked outcomes:
# - TASK_BLOCKED text => IMPLEMENT_STATUS=blocked and non-zero return
# - exit code 2 => IMPLEMENT_STATUS=blocked and non-zero return
# - non-blocked non-zero => IMPLEMENT_STATUS=failed
# - success => IMPLEMENT_STATUS=completed
# Also validates:
# - run_review_once fails hard on non-zero review command exit
#
# Usage: ./scripts/test_phase_pipeline.sh
#

set -euo pipefail

PASS=0
FAIL=0
TEST_NAME=""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PHASE_PIPELINE_SCRIPT="$SCRIPT_DIR/phase-pipeline.sh"
TEST_TMP_DIR=""

begin_test() {
    TEST_NAME="$1"
    printf "  TEST: %-60s " "$TEST_NAME"
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

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="${3:-}"
    if echo "$haystack" | grep -q "$needle"; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected to contain: '$needle'"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    return 1
}

cleanup() {
    if [[ -n "$TEST_TMP_DIR" && -d "$TEST_TMP_DIR" ]]; then
        rm -rf "$TEST_TMP_DIR"
    fi
}
trap cleanup EXIT

if [[ ! -f "$PHASE_PIPELINE_SCRIPT" ]]; then
    echo "ERROR: Cannot find $PHASE_PIPELINE_SCRIPT"
    exit 1
fi

# Helper: extract a bash function by name from a file using brace counting.
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
            if (depth == 0 && found) { found=0; exit }
        }
    ' "$file"
}

# Pull in the real implementation logic under test.
eval "$(extract_function "$PHASE_PIPELINE_SCRIPT" extract_implementation_block_reason)"
eval "$(extract_function "$PHASE_PIPELINE_SCRIPT" run_implementation)"
eval "$(extract_function "$PHASE_PIPELINE_SCRIPT" run_review_once)"

# Test harness globals used by run_implementation().
_SYM_RUNNING=""
_SYM_WARN=""
_SYM_CROSS=""
_SYM_CHECK=""
_DIM=""
_BOLD=""
_RESET=""
_YELLOW=""
_RED=""
_GREEN=""

SKIP_IMPLEMENT="false"
PROJECT_ROOT=""
IMPL_AGENT="codex"
PHASE_ID="1"
RUN_DIR=""
REPORT_DIR=""
IMPLEMENT_STATUS="not-run"
IMPLEMENT_REASON=""
BASE_BRANCH="main"
REVIEW_CONCURRENCY="2"
REVIEW_MODE="all"
REVIEW_AGENT="codex"
DRY_RUN="false"
REVIEW_VERDICT="NOT_RUN"

# Stubs for run_implementation dependencies.
CAPTURE_RC=0
CAPTURE_OUTPUT=""
LAST_LOG_STEP=""
PERSIST_CALLS=0
DIE_CALLS=0
LAST_DIE_MSG=""
EXTRACT_FROM_CONSOLIDATED_CALLS=0
EXTRACT_FROM_LOG_CALLS=0
EXTRACT_FROM_CONSOLIDATED_RESULT="APPROVED"
EXTRACT_FROM_LOG_RESULT="UNKNOWN"

assert_expected_branch() {
    : # no-op for unit tests
}

die() {
    LAST_DIE_MSG="$1"
    DIE_CALLS=$((DIE_CALLS + 1))
    return 1
}

log() {
    : # no-op for unit tests
}

log_step() {
    LAST_LOG_STEP="$2"
}

persist_metadata() {
    PERSIST_CALLS=$((PERSIST_CALLS + 1))
}

capture_cmd() {
    local output_file="$1"
    shift
    printf "%s\n" "$CAPTURE_OUTPUT" > "$output_file"
    return "$CAPTURE_RC"
}

_verdict_styled() {
    local verdict="$1"
    printf "%s" "$verdict"
}

extract_verdict_from_consolidated() {
    EXTRACT_FROM_CONSOLIDATED_CALLS=$((EXTRACT_FROM_CONSOLIDATED_CALLS + 1))
    echo "$EXTRACT_FROM_CONSOLIDATED_RESULT"
}

extract_verdict() {
    EXTRACT_FROM_LOG_CALLS=$((EXTRACT_FROM_LOG_CALLS + 1))
    echo "$EXTRACT_FROM_LOG_RESULT"
}

setup_test_env() {
    TEST_TMP_DIR=$(mktemp -d /tmp/phase-pipeline-unit-XXXXXX)
    PROJECT_ROOT="$TEST_TMP_DIR/project"
    RUN_DIR="$TEST_TMP_DIR/run"
    REPORT_DIR="$TEST_TMP_DIR/report"
    mkdir -p "$PROJECT_ROOT/scripts" "$PROJECT_ROOT/scripts/review" "$RUN_DIR" "$REPORT_DIR"

    # Presence only; script is not executed because capture_cmd is stubbed.
    cat > "$PROJECT_ROOT/scripts/ralph_codex.sh" <<'EOF_IMPL'
#!/usr/bin/env bash
echo "stub"
EOF_IMPL
    chmod +x "$PROJECT_ROOT/scripts/ralph_codex.sh"

    cat > "$PROJECT_ROOT/scripts/review/review.sh" <<'EOF_REVIEW'
#!/usr/bin/env bash
echo "stub review"
EOF_REVIEW
    chmod +x "$PROJECT_ROOT/scripts/review/review.sh"

    IMPLEMENT_STATUS="not-run"
    IMPLEMENT_REASON=""
    REVIEW_VERDICT="NOT_RUN"
    LAST_LOG_STEP=""
    PERSIST_CALLS=0
    DIE_CALLS=0
    LAST_DIE_MSG=""
    EXTRACT_FROM_CONSOLIDATED_CALLS=0
    EXTRACT_FROM_LOG_CALLS=0
    EXTRACT_FROM_CONSOLIDATED_RESULT="APPROVED"
    EXTRACT_FROM_LOG_RESULT="UNKNOWN"
}

echo ""
echo "Phase Pipeline Blocked-Path Unit Tests"
echo "======================================"
echo ""

setup_test_env

echo "--- extract_implementation_block_reason ---"
begin_test "Extracts TASK_BLOCKED reason text"
log_file_1="$RUN_DIR/block-1.log"
cat > "$log_file_1" <<'EOF_LOG'
[2026-02-16 22:16:30] TASK_BLOCKED: T-016 requires T-001 T-002
EOF_LOG
reason="$(extract_implementation_block_reason "$log_file_1" || true)"
if assert_eq "T-016 requires T-001 T-002" "$reason"; then
    pass_test
fi

begin_test "Returns non-zero when no block pattern exists"
log_file_2="$RUN_DIR/block-2.log"
cat > "$log_file_2" <<'EOF_LOG'
normal implementation output
EOF_LOG
if extract_implementation_block_reason "$log_file_2" >/dev/null 2>&1; then
    fail_test "Expected non-zero when no blocked marker exists"
else
    pass_test
fi

echo ""
echo "--- run_implementation ---"

begin_test "TASK_BLOCKED output marks implementation as blocked"
CAPTURE_RC=0
CAPTURE_OUTPUT='[2026-02-16 22:16:30] TASK_BLOCKED: T-016 requires T-001 T-002'
if run_implementation; then
    fail_test "Expected run_implementation to return non-zero for blocked output"
else
    if assert_eq "blocked" "$IMPLEMENT_STATUS" && \
       assert_eq "T-016 requires T-001 T-002" "$IMPLEMENT_REASON" && \
       assert_contains "$LAST_LOG_STEP" "Implementation blocked"; then
        pass_test
    fi
fi

begin_test "Exit code 2 marks implementation as blocked"
CAPTURE_RC=2
CAPTURE_OUTPUT='implementation interrupted'
if run_implementation; then
    fail_test "Expected run_implementation to return non-zero for rc=2"
else
    if assert_eq "blocked" "$IMPLEMENT_STATUS" && \
       assert_contains "$IMPLEMENT_REASON" "exited with code 2"; then
        pass_test
    fi
fi

begin_test "Non-zero non-blocked exit marks implementation as failed"
CAPTURE_RC=1
CAPTURE_OUTPUT='generic implementation failure'
if run_implementation; then
    fail_test "Expected run_implementation to return non-zero for rc=1"
else
    if assert_eq "failed" "$IMPLEMENT_STATUS" && \
       assert_contains "$IMPLEMENT_REASON" "exited with code 1"; then
        pass_test
    fi
fi

begin_test "Success path remains completed"
CAPTURE_RC=0
CAPTURE_OUTPUT='task completed'
if run_implementation; then
    if assert_eq "completed" "$IMPLEMENT_STATUS" && \
       assert_eq "" "$IMPLEMENT_REASON"; then
        pass_test
    fi
else
    fail_test "Expected success path to return 0"
fi

echo ""
echo "--- run_review_once ---"

begin_test "Non-zero review command fails hard"
CAPTURE_RC=9
CAPTURE_OUTPUT='REQUEST_CHANGES'
DIE_CALLS=0
LAST_DIE_MSG=""
EXTRACT_FROM_CONSOLIDATED_CALLS=0
EXTRACT_FROM_LOG_CALLS=0
EXTRACT_FROM_CONSOLIDATED_RESULT="COMMENT"
EXTRACT_FROM_LOG_RESULT="REQUEST_CHANGES"
REVIEW_VERDICT="NOT_RUN"
if run_review_once 0; then
    fail_test "Expected run_review_once to return non-zero when review command exits non-zero"
else
    if assert_eq "1" "$DIE_CALLS" "die should be called on review command failure" && \
       assert_contains "$LAST_DIE_MSG" "exit code 9" "error should include review command exit code" && \
       assert_eq "NOT_RUN" "$REVIEW_VERDICT" "should stop before deriving a verdict"; then
        pass_test
    fi
fi

begin_test "Fallback verdict extraction uses review log when consolidated is UNKNOWN"
CAPTURE_RC=0
CAPTURE_OUTPUT='review complete'
DIE_CALLS=0
LAST_DIE_MSG=""
EXTRACT_FROM_CONSOLIDATED_CALLS=0
EXTRACT_FROM_LOG_CALLS=0
EXTRACT_FROM_CONSOLIDATED_RESULT="UNKNOWN"
EXTRACT_FROM_LOG_RESULT="REQUEST_CHANGES"
if run_review_once 1; then
    if assert_eq "REQUEST_CHANGES" "$REVIEW_VERDICT" "should fall back to log verdict" && \
       assert_eq "0" "$DIE_CALLS" "die should not be called"; then
        pass_test
    fi
else
    fail_test "Expected fallback verdict path to return 0"
fi

echo ""
echo "=========================="
echo "Results: $PASS passed, $FAIL failed"
echo "=========================="

if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi

echo ""
echo "All blocked-path unit tests passed!"
exit 0
