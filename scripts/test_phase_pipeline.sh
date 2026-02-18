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

if ((BASH_VERSINFO[0] < 4)); then
    printf 'ERROR: Bash 4+ is required (found %s).\n' "$BASH_VERSION" >&2
    exit 1
fi

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
    if echo "$haystack" | grep -qF -- "$needle"; then
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
IMPL_MODEL=""
IMPL_MODEL_PRESET=""
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
STUB_REMAINING_RC=0
STUB_REMAINING_OUTPUT="0"

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

count_phase_remaining_tasks() {
    local _phase_id="$1"
    echo "$STUB_REMAINING_OUTPUT"
    return "$STUB_REMAINING_RC"
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
    IMPL_MODEL=""
    IMPL_MODEL_PRESET=""
    REVIEW_VERDICT="NOT_RUN"
    LAST_LOG_STEP=""
    PERSIST_CALLS=0
    DIE_CALLS=0
    LAST_DIE_MSG=""
    EXTRACT_FROM_CONSOLIDATED_CALLS=0
    EXTRACT_FROM_LOG_CALLS=0
    EXTRACT_FROM_CONSOLIDATED_RESULT="APPROVED"
    EXTRACT_FROM_LOG_RESULT="UNKNOWN"
    STUB_REMAINING_RC=0
    STUB_REMAINING_OUTPUT="0"
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
STUB_REMAINING_RC=0
STUB_REMAINING_OUTPUT="0"
if run_implementation; then
    if assert_eq "completed" "$IMPLEMENT_STATUS" && \
       assert_eq "" "$IMPLEMENT_REASON"; then
        pass_test
    fi
else
    fail_test "Expected success path to return 0"
fi

begin_test "Success exit with remaining tasks marks implementation as blocked"
CAPTURE_RC=0
CAPTURE_OUTPUT='task completed'
STUB_REMAINING_RC=0
STUB_REMAINING_OUTPUT="3"
if run_implementation; then
    fail_test "Expected blocked status when remaining tasks are non-zero"
else
    if assert_eq "blocked" "$IMPLEMENT_STATUS" && \
       assert_contains "$IMPLEMENT_REASON" "3 task(s) remain"; then
        pass_test
    fi
fi

begin_test "Success exit with invalid remaining-count marks implementation as failed"
CAPTURE_RC=0
CAPTURE_OUTPUT='task completed'
STUB_REMAINING_RC=0
STUB_REMAINING_OUTPUT="not-a-number"
if run_implementation; then
    fail_test "Expected failure when remaining-task verifier is invalid"
else
    if assert_eq "failed" "$IMPLEMENT_STATUS" && \
       assert_contains "$IMPLEMENT_REASON" "invalid value"; then
        pass_test
    fi
fi

begin_test "Success exit with verifier error marks implementation as failed"
CAPTURE_RC=0
CAPTURE_OUTPUT='task completed'
STUB_REMAINING_RC=7
STUB_REMAINING_OUTPUT=""
if run_implementation; then
    fail_test "Expected failure when remaining-task verifier errors"
else
    if assert_eq "failed" "$IMPLEMENT_STATUS" && \
       assert_contains "$IMPLEMENT_REASON" "could not be verified"; then
        pass_test
    fi
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
# Note: the test's die() stub returns 1 instead of exit 1. When called inside
# an if-context or || context, bash suppresses set -e for the entire call chain,
# so die() returning 1 doesn't stop run_review_once. We verify the important
# behavior: die() was called with the correct error message.
run_review_once 0 || true
if assert_eq "1" "$DIE_CALLS" "die should be called on review command failure" && \
   assert_contains "$LAST_DIE_MSG" "exit code 9" "error should include review command exit code"; then
    pass_test
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
echo "--- resolve_impl_model ---"

# Extract the model-related functions
eval "$(extract_function "$PHASE_PIPELINE_SCRIPT" resolve_impl_model)"
eval "$(extract_function "$PHASE_PIPELINE_SCRIPT" get_model_presets_for_agent)"
eval "$(extract_function "$PHASE_PIPELINE_SCRIPT" get_default_model_preset)"

# Define model preset mappings directly (must match phase-pipeline.sh)
declare -A CLAUDE_MODEL_PRESETS=(
    ["opus"]="claude-opus-4-6"
    ["sonnet"]="claude-sonnet-4-6"
)
declare -A CODEX_MODEL_PRESETS=(
    ["default"]="gpt-5.3-codex"
    ["o3"]="o3"
)

begin_test "resolve_impl_model maps claude opus preset to full model ID"
IMPL_AGENT="claude"
IMPL_MODEL_PRESET="opus"
IMPL_MODEL=""
resolve_impl_model
if assert_eq "claude-opus-4-6" "$IMPL_MODEL"; then
    pass_test
fi

begin_test "resolve_impl_model maps claude sonnet preset to full model ID"
IMPL_AGENT="claude"
IMPL_MODEL_PRESET="sonnet"
IMPL_MODEL=""
resolve_impl_model
if assert_eq "claude-sonnet-4-6" "$IMPL_MODEL"; then
    pass_test
fi

begin_test "resolve_impl_model maps codex default preset to full model ID"
IMPL_AGENT="codex"
IMPL_MODEL_PRESET="default"
IMPL_MODEL=""
resolve_impl_model
if assert_eq "gpt-5.3-codex" "$IMPL_MODEL"; then
    pass_test
fi

begin_test "resolve_impl_model passes through full model ID with hyphens"
IMPL_AGENT="claude"
IMPL_MODEL_PRESET="claude-custom-model-v1"
IMPL_MODEL=""
resolve_impl_model
if assert_eq "claude-custom-model-v1" "$IMPL_MODEL"; then
    pass_test
fi

begin_test "resolve_impl_model passes through full model ID with dots"
IMPL_AGENT="codex"
IMPL_MODEL_PRESET="gpt-4.5-turbo"
IMPL_MODEL=""
resolve_impl_model
if assert_eq "gpt-4.5-turbo" "$IMPL_MODEL"; then
    pass_test
fi

begin_test "resolve_impl_model leaves IMPL_MODEL empty when preset is empty"
IMPL_AGENT="claude"
IMPL_MODEL_PRESET=""
IMPL_MODEL="should-be-cleared"
resolve_impl_model
if assert_eq "" "$IMPL_MODEL"; then
    pass_test
fi

begin_test "get_model_presets_for_agent returns claude presets"
presets=$(get_model_presets_for_agent "claude")
if assert_eq "opus sonnet" "$presets"; then
    pass_test
fi

begin_test "get_model_presets_for_agent returns codex presets"
presets=$(get_model_presets_for_agent "codex")
if assert_eq "default o3" "$presets"; then
    pass_test
fi

begin_test "get_default_model_preset returns opus for claude"
preset=$(get_default_model_preset "claude")
if assert_eq "opus" "$preset"; then
    pass_test
fi

begin_test "get_default_model_preset returns default for codex"
preset=$(get_default_model_preset "codex")
if assert_eq "default" "$preset"; then
    pass_test
fi

echo ""
echo "--- resolve_impl_model cross-agent validation ---"

begin_test "resolve_impl_model rejects claude preset for codex agent"
IMPL_AGENT="codex"
IMPL_MODEL_PRESET="opus"
IMPL_MODEL=""
DIE_CALLS=0
LAST_DIE_MSG=""
resolve_impl_model || true
if assert_eq "1" "$DIE_CALLS" "die should be called for cross-agent preset" && \
   assert_contains "$LAST_DIE_MSG" "claude preset" "should mention it's a claude preset"; then
    pass_test
fi

begin_test "resolve_impl_model rejects codex preset for claude agent"
IMPL_AGENT="claude"
IMPL_MODEL_PRESET="o3"
IMPL_MODEL=""
DIE_CALLS=0
LAST_DIE_MSG=""
resolve_impl_model || true
if assert_eq "1" "$DIE_CALLS" "die should be called for cross-agent preset" && \
   assert_contains "$LAST_DIE_MSG" "codex preset" "should mention it's a codex preset"; then
    pass_test
fi

begin_test "resolve_impl_model rejects unknown short preset name"
IMPL_AGENT="claude"
IMPL_MODEL_PRESET="banana"
IMPL_MODEL=""
DIE_CALLS=0
LAST_DIE_MSG=""
resolve_impl_model || true
if assert_eq "1" "$DIE_CALLS" "die should be called for unknown preset" && \
   assert_contains "$LAST_DIE_MSG" "Unknown model preset" "should say unknown preset"; then
    pass_test
fi

echo ""
echo "--- run_implementation with model ---"

# Reset test environment
setup_test_env

begin_test "Implementation passes --model when IMPL_MODEL is set"
IMPL_MODEL="claude-opus-4-6"
CAPTURE_RC=0
CAPTURE_OUTPUT='task completed'
CAPTURED_IMPL_ARGS=""

# Override capture_cmd to capture the command arguments
capture_cmd() {
    local output_file="$1"
    shift
    CAPTURED_IMPL_ARGS="$*"
    printf "%s\n" "$CAPTURE_OUTPUT" > "$output_file"
    return "$CAPTURE_RC"
}

if run_implementation; then
    if assert_contains "$CAPTURED_IMPL_ARGS" "--model claude-opus-4-6"; then
        pass_test
    fi
else
    fail_test "Expected run_implementation to succeed"
fi

begin_test "Implementation omits --model when IMPL_MODEL is empty"
IMPL_MODEL=""
CAPTURE_RC=0
CAPTURE_OUTPUT='task completed'
CAPTURED_IMPL_ARGS=""

if run_implementation; then
    if echo "$CAPTURED_IMPL_ARGS" | grep -q "\-\-model"; then
        fail_test "--model should not be present when IMPL_MODEL is empty"
    else
        pass_test
    fi
else
    fail_test "Expected run_implementation to succeed"
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
