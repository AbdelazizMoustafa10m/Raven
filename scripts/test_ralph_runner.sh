#!/usr/bin/env bash
#
# test_ralph_runner.sh -- Focused tests for ralph runner argument parsing and agent invocation safety.
#
# Coverage:
# - Missing option values in shared parser emit explicit error + usage output.
# - Claude run_agent avoids eval semantics and passes prompt via stdin safely.
#
# Usage: ./scripts/test_ralph_runner.sh
#

set -euo pipefail

PASS=0
FAIL=0
TEST_NAME=""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLAUDE_SCRIPT="$SCRIPT_DIR/ralph_claude.sh"
CODEX_SCRIPT="$SCRIPT_DIR/ralph_codex.sh"
RALPH_LIB_SCRIPT="$SCRIPT_DIR/ralph-lib.sh"
TMP_DIR=""
LAST_RC=0

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

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="${3:-}"
    if echo "$haystack" | grep -Fq "$needle"; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected to contain: '$needle'"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    return 1
}

assert_file_contains_line() {
    local file="$1"
    local expected_line="$2"
    if grep -Fxq -- "$expected_line" "$file"; then
        return 0
    fi
    printf "FAIL\n"
    echo "    Expected line not found in $file:"
    echo "    $expected_line"
    FAIL=$((FAIL + 1))
    return 1
}

cleanup() {
    if [[ -n "$TMP_DIR" && -d "$TMP_DIR" ]]; then
        rm -rf "$TMP_DIR"
    fi
}
trap cleanup EXIT

run_capture() {
    local output_file="$1"
    shift

    set +e
    "$@" >"$output_file" 2>&1
    LAST_RC=$?
    set -e
}

assert_missing_value_error() {
    local script="$1"
    local option="$2"
    local output_file="$3"
    shift 3

    run_capture "$output_file" "$script" "$option" "$@"
    local output
    output=$(cat "$output_file")

    if [[ "$LAST_RC" -ne 1 ]]; then
        fail_test "Expected exit code 1, got $LAST_RC"
        return
    fi

    if assert_contains "$output" "ERROR: Option $option requires a value" && \
       assert_contains "$output" "Usage:"; then
        pass_test
    fi
}

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

if [[ ! -f "$CLAUDE_SCRIPT" || ! -f "$CODEX_SCRIPT" || ! -f "$RALPH_LIB_SCRIPT" ]]; then
    echo "ERROR: Missing required script(s) under $SCRIPT_DIR"
    exit 1
fi

TMP_DIR=$(mktemp -d /tmp/ralph-runner-test-XXXXXX)

echo ""
echo "Ralph Runner Parser + Invocation Safety Tests"
echo "============================================="
echo ""

echo "--- Missing Option Value Handling ---"

missing_shared_opts=(--phase --task --max-iterations --max-limit-waits --model)
for opt in "${missing_shared_opts[@]}"; do
    begin_test "Claude parser rejects missing value for $opt"
    assert_missing_value_error "$CLAUDE_SCRIPT" "$opt" "$TMP_DIR/claude-${opt#--}.log"
done

begin_test "Claude parser rejects missing value for --effort"
assert_missing_value_error "$CLAUDE_SCRIPT" "--effort" "$TMP_DIR/claude-effort.log"

for opt in "${missing_shared_opts[@]}"; do
    begin_test "Codex parser rejects missing value for $opt"
    assert_missing_value_error "$CODEX_SCRIPT" "$opt" "$TMP_DIR/codex-${opt#--}.log"
done

begin_test "Codex parser rejects missing value for --reasoning"
assert_missing_value_error "$CODEX_SCRIPT" "--reasoning" "$TMP_DIR/codex-reasoning.log"

begin_test "Parser treats next option as missing value (Claude --phase --dry-run)"
assert_missing_value_error "$CLAUDE_SCRIPT" "--phase" "$TMP_DIR/claude-phase-next-opt.log" "--dry-run"

echo ""
echo "--- Claude run_agent Safety ---"

begin_test "run_agent function body contains no eval"
run_agent_src=$(extract_function "$CLAUDE_SCRIPT" run_agent)
if echo "$run_agent_src" | grep -Eq '[[:space:]]eval[[:space:]]'; then
    fail_test "run_agent still references eval"
else
    pass_test
fi

begin_test "run_agent passes args safely and reads prompt from stdin"

# shellcheck disable=SC1090
eval "$run_agent_src"

MOCK_BIN="$TMP_DIR/bin"
mkdir -p "$MOCK_BIN"

MOCK_ARGS_FILE="$TMP_DIR/mock-claude-args.txt"
MOCK_STDIN_FILE="$TMP_DIR/mock-claude-stdin.txt"
MARKER_FILE="$TMP_DIR/eval-marker"

cat > "$MOCK_BIN/claude" <<'EOF_MOCK'
#!/usr/bin/env bash
set -euo pipefail

: "${RALPH_TEST_ARGS_FILE:?missing args file}"
: "${RALPH_TEST_STDIN_FILE:?missing stdin file}"

: > "$RALPH_TEST_ARGS_FILE"
for arg in "$@"; do
    printf '%s\n' "$arg" >> "$RALPH_TEST_ARGS_FILE"
done

cat > "$RALPH_TEST_STDIN_FILE"
echo "mock-claude-ok"
EOF_MOCK
chmod +x "$MOCK_BIN/claude"

export PATH="$MOCK_BIN:$PATH"
export RALPH_TEST_ARGS_FILE="$MOCK_ARGS_FILE"
export RALPH_TEST_STDIN_FILE="$MOCK_STDIN_FILE"
CLAUDE_ALLOWED_TOOLS="Edit,Read,Bash(git status*)"

PROMPT_FILE="$TMP_DIR/prompt.md"
cat > "$PROMPT_FILE" <<'EOF_PROMPT'
line 1: test prompt
line 2: symbols ; && $(echo noop) "quotes"
EOF_PROMPT

MODEL_WITH_INJECTION="fake-model; touch $MARKER_FILE"

set +e
agent_out=$(run_agent "$PROMPT_FILE" "$MODEL_WITH_INJECTION" "high")
agent_rc=$?
set -e

if [[ "$agent_rc" -ne 0 ]]; then
    fail_test "run_agent returned non-zero ($agent_rc)"
elif [[ -f "$MARKER_FILE" ]]; then
    fail_test "Model string was executed (marker file exists)"
elif ! assert_contains "$agent_out" "mock-claude-ok"; then
    :
elif ! assert_file_contains_line "$MOCK_ARGS_FILE" "-p"; then
    :
elif ! assert_file_contains_line "$MOCK_ARGS_FILE" "--permission-mode"; then
    :
elif ! assert_file_contains_line "$MOCK_ARGS_FILE" "dontAsk"; then
    :
elif ! assert_file_contains_line "$MOCK_ARGS_FILE" "--model"; then
    :
elif ! assert_file_contains_line "$MOCK_ARGS_FILE" "$MODEL_WITH_INJECTION"; then
    :
elif ! assert_file_contains_line "$MOCK_ARGS_FILE" "--allowedTools"; then
    :
elif ! assert_file_contains_line "$MOCK_ARGS_FILE" "$CLAUDE_ALLOWED_TOOLS"; then
    :
elif ! cmp -s "$PROMPT_FILE" "$MOCK_STDIN_FILE"; then
    fail_test "Prompt content read by claude did not match input file"
else
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
echo "All runner tests passed!"
exit 0
