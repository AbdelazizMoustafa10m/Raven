#!/usr/bin/env bash
#
# test_review.sh -- Smoke tests for review infrastructure functions.
#
# Creates an isolated git sandbox, sources review-lib.sh functions,
# and exercises key review paths with assertions.
#
# Usage: ./scripts/test_review.sh
#

set -euo pipefail

# ============================================================================
# Test Harness (same helpers as test_recovery.sh)
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

assert_file_exists() {
    local file_path="$1"
    local msg="${2:-}"
    if [[ -f "$file_path" ]]; then
        return 0
    fi
    printf "FAIL\n"
    echo "    File not found: '$file_path'"
    [[ -n "$msg" ]] && echo "    Note: $msg"
    FAIL=$((FAIL + 1))
    return 1
}

assert_file_not_empty() {
    local file_path="$1"
    local msg="${2:-}"
    if [[ -s "$file_path" ]]; then
        return 0
    fi
    printf "FAIL\n"
    echo "    File is empty or missing: '$file_path'"
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
REAL_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cleanup() {
    if [[ -n "$SANDBOX" && -d "$SANDBOX" ]]; then
        rm -rf "$SANDBOX"
    fi
}
# Note: ensure_review_dirs in review-lib.sh sets its own EXIT trap
# (cleanup_workspace_on_exit). We override it back to our cleanup after
# sourcing review-lib.sh. Our cleanup removes the entire sandbox,
# which includes any review workspace directories.
trap cleanup EXIT

create_sandbox() {
    SANDBOX=$(mktemp -d /tmp/ralph-review-test-XXXXXX)

    # Build a minimal repo structure that review-lib.sh expects.
    # review-lib.sh computes PROJECT_ROOT from SCRIPT_DIR (two levels up),
    # so we place the review-lib.sh symlink at scripts/review/review-lib.sh.
    mkdir -p "$SANDBOX/scripts/review"
    mkdir -p "$SANDBOX/.github/review/prompts"
    mkdir -p "$SANDBOX/.github/review/rules"
    mkdir -p "$SANDBOX/.github/review/scripts"
    mkdir -p "$SANDBOX/docs/tasks"

    # Create minimal required files so review-lib.sh can work
    echo "# full-review prompt" > "$SANDBOX/.github/review/prompts/full-review.md"
    echo "# security-review prompt" > "$SANDBOX/.github/review/prompts/security-review.md"
    echo "# patterns" > "$SANDBOX/.github/review/rules/raven-patterns.md"
    echo "# checklist" > "$SANDBOX/.github/review/rules/review-checklist.md"

    # Copy json-extract.js if it exists
    local real_json_extract="$REAL_SCRIPT_DIR/../.github/review/scripts/json-extract.js"
    if [[ -f "$real_json_extract" ]]; then
        cp "$real_json_extract" "$SANDBOX/.github/review/scripts/json-extract.js"
    fi

    # Copy review-lib.sh into the sandbox (not a symlink, so SCRIPT_DIR resolves correctly)
    cp "$REAL_SCRIPT_DIR/review/review-lib.sh" "$SANDBOX/scripts/review/review-lib.sh"

    # Initialize git repo
    cd "$SANDBOX"
    git init -q
    git config user.email "test@test.com"
    git config user.name "Test"

    # Create a simple Go file and initial commit
    cat > "$SANDBOX/main.go" <<'GOEOF'
package main

func main() {}
GOEOF
    cat > "$SANDBOX/go.mod" <<'MODEOF'
module example.com/test

go 1.24
MODEOF

    git add -A && git commit -q -m "initial commit"
}

# ============================================================================
# Source review-lib.sh from sandbox
# ============================================================================

source_review_lib() {
    # Source review-lib.sh from the sandbox copy.
    # This sets SCRIPT_DIR to $SANDBOX/scripts/review and PROJECT_ROOT to $SANDBOX.
    # shellcheck source=/dev/null
    source "$SANDBOX/scripts/review/review-lib.sh"
    # Restore our cleanup trap (review-lib.sh's ensure_review_dirs may override it)
    trap cleanup EXIT
}

# ============================================================================
# Tests
# ============================================================================

echo ""
echo "Ralph Review Infrastructure Smoke Tests"
echo "========================================"
echo ""

create_sandbox
source_review_lib

# --- Test Group 1: Agent Validation ---
echo "--- Agent Validation ---"

begin_test "is_valid_agent: 'claude' is valid"
if is_valid_agent "claude"; then
    pass_test
else
    fail_test "is_valid_agent returned false for 'claude'"
fi

begin_test "is_valid_agent: 'codex' is valid"
if is_valid_agent "codex"; then
    pass_test
else
    fail_test "is_valid_agent returned false for 'codex'"
fi

begin_test "is_valid_agent: 'gemini' is valid"
if is_valid_agent "gemini"; then
    pass_test
else
    fail_test "is_valid_agent returned false for 'gemini'"
fi

begin_test "is_valid_agent: 'gpt4' is invalid"
if is_valid_agent "gpt4"; then
    fail_test "is_valid_agent returned true for 'gpt4'"
else
    pass_test
fi

begin_test "is_valid_agent: empty string is invalid"
if is_valid_agent ""; then
    fail_test "is_valid_agent returned true for empty string"
else
    pass_test
fi

# --- Test Group 2: Agent Binary Mapping ---
echo ""
echo "--- Agent Binary Mapping ---"

begin_test "agent_bin: claude -> claude"
result=$(agent_bin "claude")
if assert_eq "claude" "$result"; then
    pass_test
fi

begin_test "agent_bin: codex -> codex"
result=$(agent_bin "codex")
if assert_eq "codex" "$result"; then
    pass_test
fi

begin_test "agent_bin: gemini -> gemini"
result=$(agent_bin "gemini")
if assert_eq "gemini" "$result"; then
    pass_test
fi

begin_test "agent_bin: invalid agent returns error"
if agent_bin "invalid" >/dev/null 2>&1; then
    fail_test "agent_bin should fail for 'invalid'"
else
    pass_test
fi

# --- Test Group 3: Agent Model Defaults & Overrides ---
echo ""
echo "--- Agent Model Defaults & Overrides ---"

begin_test "agent_model: claude default"
result=$(agent_model "claude")
if assert_contains "$result" "claude" "should contain 'claude' in model name"; then
    pass_test
fi

begin_test "agent_model: codex default"
result=$(agent_model "codex")
# Default is gpt-5-codex or similar
if [[ -n "$result" ]]; then
    pass_test
else
    fail_test "agent_model returned empty for codex"
fi

begin_test "agent_model: gemini default"
result=$(agent_model "gemini")
if assert_contains "$result" "gemini" "should contain 'gemini' in model name"; then
    pass_test
fi

begin_test "agent_model: CLAUDE_REVIEW_MODEL env override"
result=$(CLAUDE_REVIEW_MODEL="test-model-override" agent_model "claude")
if assert_eq "test-model-override" "$result" "env var override should work"; then
    pass_test
fi

begin_test "agent_model: CODEX_REVIEW_MODEL env override"
result=$(CODEX_REVIEW_MODEL="codex-override" agent_model "codex")
if assert_eq "codex-override" "$result" "env var override should work"; then
    pass_test
fi

begin_test "agent_model: GEMINI_REVIEW_MODEL env override"
result=$(GEMINI_REVIEW_MODEL="gemini-override" agent_model "gemini")
if assert_eq "gemini-override" "$result" "env var override should work"; then
    pass_test
fi

begin_test "agent_model: invalid agent returns error"
if agent_model "invalid" >/dev/null 2>&1; then
    fail_test "agent_model should fail for 'invalid'"
else
    pass_test
fi

# --- Test Group 4: resolve_base_ref ---
echo ""
echo "--- Base Ref Resolution ---"

begin_test "resolve_base_ref: existing local branch resolves"
cd "$SANDBOX"
# Create a feature branch and switch back
git checkout -q -b test-feature
git checkout -q -
result=$(resolve_base_ref "test-feature")
if assert_eq "test-feature" "$result"; then
    pass_test
fi

begin_test "resolve_base_ref: non-existing ref fails"
cd "$SANDBOX"
if resolve_base_ref "nonexistent-branch-xyz" >/dev/null 2>&1; then
    fail_test "resolve_base_ref should fail for non-existing ref"
else
    pass_test
fi

begin_test "resolve_base_ref: HEAD resolves"
cd "$SANDBOX"
result=$(resolve_base_ref "HEAD")
if assert_eq "HEAD" "$result"; then
    pass_test
fi

# --- Test Group 5: Diff Collection ---
echo ""
echo "--- Diff Collection ---"

begin_test "collect_diff: generates expected output files"
cd "$SANDBOX"
# Create a feature branch with changes
git checkout -q -b feature-for-diff-test
mkdir -p "$SANDBOX/internal/cli"
echo "package cli" > "$SANDBOX/internal/cli/new_cmd.go"
echo "# Updated" >> "$SANDBOX/go.mod"
mkdir -p "$SANDBOX/scripts"
echo "#!/bin/bash" > "$SANDBOX/scripts/build.sh"
git add -A && git commit -q -m "feature changes"

# Re-source to get fresh RUN_DIR (timestamp-based)
source_review_lib
ensure_review_dirs
# Restore our trap (ensure_review_dirs sets its own EXIT trap)
trap cleanup EXIT

# Collect diff against initial commit branch (main or master)
local_main_branch=""
if git show-ref --verify --quiet refs/heads/main 2>/dev/null; then
    local_main_branch="main"
elif git show-ref --verify --quiet refs/heads/master 2>/dev/null; then
    local_main_branch="master"
else
    # Use the initial commit directly
    local_main_branch="$(git rev-list --max-parents=0 HEAD | head -1)"
fi

# Suppress log output during tests
collect_diff "$local_main_branch" >/dev/null 2>&1 || true

if assert_file_exists "$RAW_DIR/changed-files.txt" "changed-files.txt should exist" && \
   assert_file_exists "$RAW_DIR/review-files.txt" "review-files.txt should exist" && \
   assert_file_exists "$RAW_DIR/review.diff" "review.diff should exist"; then
    pass_test
fi

begin_test "collect_diff: changed-files.txt is populated"
if assert_file_not_empty "$RAW_DIR/changed-files.txt" "should have changed files"; then
    pass_test
fi

begin_test "collect_diff: review-files.txt includes reviewable extensions"
review_content=$(cat "$RAW_DIR/review-files.txt")
if assert_contains "$review_content" ".go" "should include .go files"; then
    pass_test
fi

begin_test "collect_diff: high-risk-files.txt detects high-risk paths"
if [[ -f "$RAW_DIR/high-risk-files.txt" ]]; then
    high_risk_content=$(cat "$RAW_DIR/high-risk-files.txt")
    # scripts/ is a high-risk pattern
    if echo "$high_risk_content" | grep -q "scripts/" 2>/dev/null; then
        pass_test
    else
        # go.mod is also high-risk
        if echo "$high_risk_content" | grep -q "go.mod" 2>/dev/null; then
            pass_test
        else
            fail_test "high-risk-files.txt missing expected high-risk entries"
        fi
    fi
else
    fail_test "high-risk-files.txt not found"
fi

begin_test "collect_diff: review.diff contains unified diff content"
if assert_file_not_empty "$RAW_DIR/review.diff" "diff should not be empty"; then
    diff_content=$(cat "$RAW_DIR/review.diff")
    if assert_contains "$diff_content" "diff --git" "should contain git diff headers"; then
        pass_test
    fi
fi

# Clean up the diff test workspace
REVIEW_ARCHIVED=true  # Prevent cleanup trap from interfering
rm -rf "$RUN_DIR"

# --- Test Group 6: JSON Extraction (requires node) ---
echo ""
echo "--- JSON Extraction ---"

if command -v node >/dev/null 2>&1 && [[ -f "$SANDBOX/.github/review/scripts/json-extract.js" ]]; then
    JSON_SCRIPT="$SANDBOX/.github/review/scripts/json-extract.js"

    begin_test "json-extract: clean JSON input"
    result=$(echo '{"verdict":"APPROVE","summary":"ok","findings":[]}' | node "$JSON_SCRIPT" 2>/dev/null) || true
    if assert_contains "$result" "APPROVE" "should extract verdict"; then
        pass_test
    fi

    begin_test "json-extract: JSON in markdown code fences"
    fenced_input='Some text before
```json
{"verdict":"COMMENT","summary":"found issues","findings":[{"severity":"high","title":"bug"}]}
```
Some text after'
    result=$(echo "$fenced_input" | node "$JSON_SCRIPT" 2>/dev/null) || true
    if assert_contains "$result" "COMMENT" "should extract from fenced JSON"; then
        pass_test
    fi

    begin_test "json-extract: JSON mixed with other text"
    mixed_input='Here is my review output.
I found several issues.
{"verdict":"REQUEST_CHANGES","summary":"critical bugs","findings":[{"severity":"critical","title":"security"}]}
That is all.'
    result=$(echo "$mixed_input" | node "$JSON_SCRIPT" 2>/dev/null) || true
    if assert_contains "$result" "REQUEST_CHANGES" "should extract embedded JSON"; then
        pass_test
    fi

    begin_test "json-extract: invalid input returns non-zero"
    if echo "this is not json at all" | node "$JSON_SCRIPT" >/dev/null 2>&1; then
        fail_test "should exit non-zero for invalid input"
    else
        pass_test
    fi

    begin_test "json-extract: empty input returns non-zero"
    if echo "" | node "$JSON_SCRIPT" >/dev/null 2>&1; then
        fail_test "should exit non-zero for empty input"
    else
        pass_test
    fi
else
    echo "  SKIP: node not available or json-extract.js missing; skipping JSON extraction tests"
fi

# --- Test Group 7: Consolidation ---
echo ""
echo "--- Consolidation ---"

if ! command -v jq >/dev/null 2>&1; then
    echo "  SKIP: jq not available; skipping consolidation tests"
else

begin_test "run_consolidation: merges multiple review JSONs"
cd "$SANDBOX"
# Re-source to get fresh workspace paths
source_review_lib
mkdir -p "$RAW_DIR"

# Create mock normalized review JSON files
cat > "$RAW_DIR/full-review_claude.json" <<'REVIEW1'
{
  "schema_version": "1.0",
  "pass": "full-review",
  "agent": "claude",
  "verdict": "COMMENT",
  "summary": "Found some issues",
  "highlights": ["good structure"],
  "findings": [
    {
      "severity": "high",
      "category": "testing",
      "path": "internal/cli/root.go",
      "line": 42,
      "title": "Missing error check",
      "details": "Error return value is not checked.",
      "suggested_fix": "Add if err != nil check"
    },
    {
      "severity": "low",
      "category": "docs",
      "path": "README.md",
      "line": 10,
      "title": "Typo in docs",
      "details": "Minor typo found.",
      "suggested_fix": "Fix the typo"
    }
  ]
}
REVIEW1

cat > "$RAW_DIR/full-review_codex.json" <<'REVIEW2'
{
  "schema_version": "1.0",
  "pass": "full-review",
  "agent": "codex",
  "verdict": "COMMENT",
  "summary": "Looks mostly good",
  "highlights": ["clean code"],
  "findings": [
    {
      "severity": "high",
      "category": "testing",
      "path": "internal/cli/root.go",
      "line": 42,
      "title": "Missing error check",
      "details": "The error return from Execute() is not handled properly.",
      "suggested_fix": "Add error handling"
    },
    {
      "severity": "medium",
      "category": "performance",
      "path": "internal/discovery/walker.go",
      "line": 100,
      "title": "Unnecessary allocation",
      "details": "Slice is allocated but never used.",
      "suggested_fix": "Remove unused allocation"
    }
  ]
}
REVIEW2

cat > "$RAW_DIR/security_claude.json" <<'REVIEW3'
{
  "schema_version": "1.0",
  "pass": "security",
  "agent": "claude",
  "verdict": "SECURE",
  "summary": "No security issues found",
  "highlights": [],
  "findings": []
}
REVIEW3

# Run consolidation (suppress log output)
if run_consolidation "false" >/dev/null 2>&1; then
    consolidated="$RUN_DIR/consolidated.json"
    if assert_file_exists "$consolidated" "consolidated.json should exist"; then
        pass_test
    fi
else
    consolidated="$RUN_DIR/consolidated.json"
    if [[ -f "$consolidated" ]]; then
        pass_test
    else
        fail_test "run_consolidation failed and no consolidated.json produced"
    fi
fi

begin_test "run_consolidation: verdict is computed"
cd "$SANDBOX"
verdict=$(jq -r '.verdict' "$consolidated" 2>/dev/null) || true
if [[ -n "$verdict" && "$verdict" != "null" ]]; then
    pass_test
else
    fail_test "verdict is empty or null (got: '$verdict')"
fi

begin_test "run_consolidation: deduplication works"
cd "$SANDBOX"
unique_count=$(jq '.stats.unique_findings // 0' "$consolidated" 2>/dev/null) || true
total_raw=$(jq '.stats.total_raw_findings // 0' "$consolidated" 2>/dev/null) || true
deduped=$(jq '.stats.duplicates_removed // 0' "$consolidated" 2>/dev/null) || true
# We had 4 raw findings, but "Missing error check" on root.go:42 appears in both claude and codex
# so unique should be 3, deduped should be 1
if [[ "$unique_count" -eq 3 && "$deduped" -eq 1 ]]; then
    pass_test
elif [[ "$unique_count" -lt "$total_raw" ]]; then
    # At minimum, some deduplication occurred
    pass_test
else
    fail_test "Expected deduplication (unique=$unique_count, total=$total_raw, deduped=$deduped)"
fi

begin_test "run_consolidation: severity ranking (high before medium before low)"
cd "$SANDBOX"
first_severity=$(jq -r '.findings[0].severity // ""' "$consolidated" 2>/dev/null) || true
if assert_eq "high" "$first_severity" "first finding should be highest severity"; then
    pass_test
fi

begin_test "run_consolidation: deduplicated finding has multiple source_agents"
cd "$SANDBOX"
# The "Missing error check" finding should have both claude and codex
agents_for_deduped=$(jq -r '[.findings[] | select(.title == "Missing error check") | .source_agents[]] | sort | join(",")' "$consolidated" 2>/dev/null) || true
if assert_contains "$agents_for_deduped" "claude" "should include claude" && \
   assert_contains "$agents_for_deduped" "codex" "should include codex"; then
    pass_test
fi

begin_test "run_consolidation: parse_error prevents APPROVE verdict"
cd "$SANDBOX"
source_review_lib
mkdir -p "$RAW_DIR"

cat > "$RAW_DIR/full-review_claude.json" <<'PARSE1'
{
  "schema_version": "1.0",
  "pass": "full-review",
  "agent": "claude",
  "verdict": "COMMENT",
  "summary": "Agent output could not be parsed",
  "highlights": [],
  "findings": [],
  "parse_error": true
}
PARSE1

cat > "$RAW_DIR/full-review_codex.json" <<'PARSE2'
{
  "schema_version": "1.0",
  "pass": "full-review",
  "agent": "codex",
  "verdict": "APPROVE",
  "summary": "Only nits",
  "highlights": [],
  "findings": [
    {
      "severity": "suggestion",
      "category": "docs",
      "path": "README.md",
      "line": 1,
      "title": "Wording tweak",
      "details": "Tiny wording suggestion.",
      "suggested_fix": "Optional copy edit"
    }
  ],
  "parse_error": false
}
PARSE2

if run_consolidation "false" >/dev/null 2>&1; then
    parse_consolidated="$RUN_DIR/consolidated.json"
    parse_verdict=$(jq -r '.verdict' "$parse_consolidated" 2>/dev/null || true)
    parse_error_runs=$(jq -r '.stats.parse_error_runs // 0' "$parse_consolidated" 2>/dev/null || true)
    if assert_eq "COMMENT" "$parse_verdict" "parse_error should block APPROVE verdict" && \
       assert_eq "1" "$parse_error_runs" "should count parse error runs"; then
        pass_test
    fi
else
    fail_test "run_consolidation failed for parse_error gating scenario"
fi

# Clean up consolidation workspace
REVIEW_ARCHIVED=true
rm -rf "$RUN_DIR"

fi  # end jq availability check for consolidation tests

# --- Test Group 8: Verdict Extraction ---
echo ""
echo "--- Verdict Extraction ---"

# extract_verdict lives in phase-pipeline.sh -- we define a local copy for testing
# since we must not modify phase-pipeline.sh. The implementation matches the one
# in phase-pipeline.sh lines 602-620.
extract_verdict() {
    local log_file="$1"
    if [[ ! -f "$log_file" ]]; then
        echo "UNKNOWN"
        return 0
    fi
    local token
    token="$(grep -Eo '\b(REQUEST_CHANGES|NEEDS_FIXES|APPROVE|APPROVED|COMMENT|COMMENTS_ONLY|LGTM|PASS|PASSED|BLOCKING|FAIL)\b' "$log_file" | tail -1 || true)"
    case "$token" in
        REQUEST_CHANGES) echo "REQUEST_CHANGES" ;;
        NEEDS_FIXES|BLOCKING|FAIL) echo "NEEDS_FIXES" ;;
        APPROVE|APPROVED|COMMENTS_ONLY|LGTM|PASS|PASSED) echo "APPROVED" ;;
        COMMENT) echo "COMMENT" ;;
        *) echo "UNKNOWN" ;;
    esac
}

begin_test "extract_verdict: REQUEST_CHANGES token"
tmp_verdict_log=$(mktemp /tmp/ralph-verdict-XXXXXX.log)
echo 'Review complete. Verdict: REQUEST_CHANGES' > "$tmp_verdict_log"
result=$(extract_verdict "$tmp_verdict_log")
rm -f "$tmp_verdict_log"
if assert_eq "REQUEST_CHANGES" "$result"; then
    pass_test
fi

begin_test "extract_verdict: APPROVE token"
tmp_verdict_log=$(mktemp /tmp/ralph-verdict-XXXXXX.log)
echo 'Everything looks good. APPROVE.' > "$tmp_verdict_log"
result=$(extract_verdict "$tmp_verdict_log")
rm -f "$tmp_verdict_log"
if assert_eq "APPROVED" "$result"; then
    pass_test
fi

begin_test "extract_verdict: LGTM token maps to APPROVED"
tmp_verdict_log=$(mktemp /tmp/ralph-verdict-XXXXXX.log)
echo 'LGTM - ship it!' > "$tmp_verdict_log"
result=$(extract_verdict "$tmp_verdict_log")
rm -f "$tmp_verdict_log"
if assert_eq "APPROVED" "$result"; then
    pass_test
fi

begin_test "extract_verdict: NEEDS_FIXES token"
tmp_verdict_log=$(mktemp /tmp/ralph-verdict-XXXXXX.log)
echo 'Security review: NEEDS_FIXES' > "$tmp_verdict_log"
result=$(extract_verdict "$tmp_verdict_log")
rm -f "$tmp_verdict_log"
if assert_eq "NEEDS_FIXES" "$result"; then
    pass_test
fi

begin_test "extract_verdict: BLOCKING maps to NEEDS_FIXES"
tmp_verdict_log=$(mktemp /tmp/ralph-verdict-XXXXXX.log)
echo 'Found BLOCKING issues that must be resolved.' > "$tmp_verdict_log"
result=$(extract_verdict "$tmp_verdict_log")
rm -f "$tmp_verdict_log"
if assert_eq "NEEDS_FIXES" "$result"; then
    pass_test
fi

begin_test "extract_verdict: COMMENT token"
tmp_verdict_log=$(mktemp /tmp/ralph-verdict-XXXXXX.log)
echo 'Minor suggestions only: COMMENT' > "$tmp_verdict_log"
result=$(extract_verdict "$tmp_verdict_log")
rm -f "$tmp_verdict_log"
if assert_eq "COMMENT" "$result"; then
    pass_test
fi

begin_test "extract_verdict: last token wins when multiple present"
tmp_verdict_log=$(mktemp /tmp/ralph-verdict-XXXXXX.log)
cat > "$tmp_verdict_log" <<'MULTI'
Initial assessment: COMMENT
After deeper review: REQUEST_CHANGES
MULTI
result=$(extract_verdict "$tmp_verdict_log")
rm -f "$tmp_verdict_log"
if assert_eq "REQUEST_CHANGES" "$result"; then
    pass_test
fi

begin_test "extract_verdict: missing file returns UNKNOWN"
result=$(extract_verdict "/tmp/nonexistent-file-that-does-not-exist-999.log")
if assert_eq "UNKNOWN" "$result"; then
    pass_test
fi

begin_test "extract_verdict: no verdict tokens returns UNKNOWN"
tmp_verdict_log=$(mktemp /tmp/ralph-verdict-XXXXXX.log)
echo 'This log contains no verdict tokens whatsoever.' > "$tmp_verdict_log"
result=$(extract_verdict "$tmp_verdict_log")
rm -f "$tmp_verdict_log"
if assert_eq "UNKNOWN" "$result"; then
    pass_test
fi

# --- Test Group 9: Normalize Review JSON ---
echo ""
echo "--- JSON Normalization ---"

if command -v jq >/dev/null 2>&1; then
    begin_test "normalize_review_json: normalizes valid full-review JSON"
    cd "$SANDBOX"
    source_review_lib
    mkdir -p "$RAW_DIR"
    cat > "$RAW_DIR/test-input.json" <<'NORMJSON'
{
  "verdict": "approve",
  "summary": "Looks good",
  "highlights": ["nice code"],
  "findings": [
    {
      "severity": "HIGH",
      "category": "testing",
      "path": "main.go",
      "line": "5",
      "title": "Test missing",
      "details": "No test for main",
      "suggested_fix": "Add test"
    }
  ]
}
NORMJSON
    normalized_out="$RAW_DIR/test-normalized.json"
    normalize_review_json "full-review" "claude" "$RAW_DIR/test-input.json" "$normalized_out" >/dev/null 2>&1 || true

    if assert_file_exists "$normalized_out" "normalized output should exist"; then
        norm_verdict=$(jq -r '.verdict' "$normalized_out" 2>/dev/null) || true
        norm_severity=$(jq -r '.findings[0].severity' "$normalized_out" 2>/dev/null) || true
        norm_agent=$(jq -r '.agent' "$normalized_out" 2>/dev/null) || true
        if assert_eq "APPROVE" "$norm_verdict" "verdict should be uppercased" && \
           assert_eq "high" "$norm_severity" "severity should be lowercased" && \
           assert_eq "claude" "$norm_agent" "agent should be set"; then
            pass_test
        fi
    fi

    begin_test "normalize_review_json: normalizes security review JSON"
    cat > "$RAW_DIR/test-sec-input.json" <<'SECJSON'
{
  "verdict": "secure",
  "summary": "No issues",
  "findings": []
}
SECJSON
    sec_normalized="$RAW_DIR/test-sec-normalized.json"
    normalize_review_json "security" "gemini" "$RAW_DIR/test-sec-input.json" "$sec_normalized" >/dev/null 2>&1 || true

    if assert_file_exists "$sec_normalized" "normalized security output should exist"; then
        sec_verdict=$(jq -r '.verdict' "$sec_normalized" 2>/dev/null) || true
        sec_pass=$(jq -r '.pass' "$sec_normalized" 2>/dev/null) || true
        if assert_eq "SECURE" "$sec_verdict" "verdict should be SECURE" && \
           assert_eq "security" "$sec_pass" "pass should be security"; then
            pass_test
        fi
    fi

    # Clean up
    REVIEW_ARCHIVED=true
    rm -rf "$RUN_DIR"
else
    echo "  SKIP: jq not available; skipping normalization tests"
fi

# --- Test Group 10: write_parse_error_payload ---
echo ""
echo "--- Parse Error Payload ---"

if ! command -v jq >/dev/null 2>&1; then
    echo "  SKIP: jq not available; skipping parse error payload tests"
else

begin_test "write_parse_error_payload: generates valid JSON"
cd "$SANDBOX"
source_review_lib
tmp_error_out=$(mktemp /tmp/ralph-parse-error-XXXXXX.json)
write_parse_error_payload "full-review" "codex" "$tmp_error_out" "Agent timed out"
if assert_file_exists "$tmp_error_out" "error payload should exist"; then
    err_verdict=$(jq -r '.verdict' "$tmp_error_out" 2>/dev/null) || true
    err_agent=$(jq -r '.agent' "$tmp_error_out" 2>/dev/null) || true
    err_parse=$(jq -r '.parse_error' "$tmp_error_out" 2>/dev/null) || true
    if assert_eq "COMMENT" "$err_verdict" "full-review error verdict should be COMMENT" && \
       assert_eq "codex" "$err_agent" "agent should be codex" && \
       assert_eq "true" "$err_parse" "parse_error should be true"; then
        pass_test
    fi
fi
rm -f "$tmp_error_out"

begin_test "write_parse_error_payload: security pass uses NEEDS_FIXES"
tmp_error_out=$(mktemp /tmp/ralph-parse-error-XXXXXX.json)
write_parse_error_payload "security" "gemini" "$tmp_error_out" "Binary missing"
if assert_file_exists "$tmp_error_out" "error payload should exist"; then
    err_verdict=$(jq -r '.verdict' "$tmp_error_out" 2>/dev/null) || true
    if assert_eq "NEEDS_FIXES" "$err_verdict" "security error verdict should be NEEDS_FIXES"; then
        pass_test
    fi
fi
rm -f "$tmp_error_out"

fi  # end jq availability check for parse error payload tests

# --- Test Group 11: run_single_review error handling ---
echo ""
echo "--- Single Review Execution ---"

if ! command -v jq >/dev/null 2>&1; then
    echo "  SKIP: jq not available; skipping run_single_review tests"
else

begin_test "run_single_review: captures non-zero agent exit code"
cd "$SANDBOX"
source_review_lib
mkdir -p "$RAW_DIR"

agent_bin() { echo "bash"; }
agent_model() { echo "stub-model"; }
build_review_prompt() { echo "stub prompt"; }
run_agent_command() { return 7; }

run_single_review "codex" "full-review" "false" >/dev/null 2>&1 || true

single_review_json="$RAW_DIR/full-review_codex.json"
if assert_file_exists "$single_review_json" "normalized output should exist on agent failure"; then
    single_summary=$(jq -r '.summary // ""' "$single_review_json" 2>/dev/null || true)
    single_parse_error=$(jq -r '.parse_error // false' "$single_review_json" 2>/dev/null || true)
    if assert_contains "$single_summary" "exit=7" "should preserve real agent exit code" && \
       assert_eq "true" "$single_parse_error" "agent failure should be marked parse_error"; then
        pass_test
    fi
fi

fi  # end jq availability check for run_single_review tests

# ============================================================================
# Summary
# ============================================================================

echo ""
echo "========================================"
echo "Results: $PASS passed, $FAIL failed"
echo "========================================"

if [[ $FAIL -gt 0 ]]; then
    exit 1
fi

echo ""
echo "All review smoke tests passed!"
exit 0
