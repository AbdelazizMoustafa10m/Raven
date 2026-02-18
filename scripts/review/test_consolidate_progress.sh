#!/usr/bin/env bash
#
# test_consolidate_progress.sh -- Tests for consolidate-progress.sh
#
# Tests argument parsing, idempotency guard, dry-run mode, the embedded Python
# helper (extract and replace commands), and the missing-file error path.
#
# Usage: ./scripts/review/test_consolidate_progress.sh
#

set -euo pipefail

if ((BASH_VERSINFO[0] < 4)); then
    printf 'ERROR: Bash 4+ required (found %s).\n' "$BASH_VERSION" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Globals
# ---------------------------------------------------------------------------

PASS=0
FAIL=0
TEST_NAME=""
LAST_RC=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONSOLIDATE_SCRIPT="$SCRIPT_DIR/consolidate-progress.sh"
# The real project root is used only for sourcing phases.conf (via phases-lib.sh).
# Individual tests create their own fake PROJECT_ROOT trees with a
# docs/tasks/PROGRESS.md that the script reads.
REAL_PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Master temp dir: each test builds its own subdirectory inside.
MASTER_TMP=""

# ---------------------------------------------------------------------------
# Test harness helpers
# ---------------------------------------------------------------------------

begin_test() {
    TEST_NAME="$1"
    printf "  TEST: %-62s " "$TEST_NAME"
}

pass_test() {
    printf "OK\n"
    PASS=$((PASS + 1))
}

fail_test() {
    local msg="${1:-}"
    printf "FAIL\n"
    [[ -n "$msg" ]] && printf "    %s\n" "$msg"
    FAIL=$((FAIL + 1))
}

# Run command, capture combined stdout+stderr to a file; store exit code in
# LAST_RC without triggering set -e.
run_capture() {
    local output_file="$1"
    shift
    set +e
    "$@" >"$output_file" 2>&1
    LAST_RC=$?
    set -e
}

# Run command capturing stdout and stderr separately; store exit code in LAST_RC.
run_capture_split() {
    local stdout_file="$1"
    local stderr_file="$2"
    shift 2
    set +e
    "$@" >"$stdout_file" 2>"$stderr_file"
    LAST_RC=$?
    set -e
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local label="${3:-}"
    if printf '%s' "$haystack" | grep -qF -- "$needle"; then
        return 0
    fi
    printf "FAIL\n"
    printf "    Expected to contain: '%s'\n" "$needle"
    [[ -n "$label" ]] && printf "    Note: %s\n" "$label"
    FAIL=$((FAIL + 1))
    return 1
}

assert_not_contains() {
    local haystack="$1"
    local needle="$2"
    local label="${3:-}"
    if ! printf '%s' "$haystack" | grep -qF -- "$needle"; then
        return 0
    fi
    printf "FAIL\n"
    printf "    Expected NOT to contain: '%s'\n" "$needle"
    [[ -n "$label" ]] && printf "    Note: %s\n" "$label"
    FAIL=$((FAIL + 1))
    return 1
}

assert_exit_code() {
    local expected="$1"
    local actual="$LAST_RC"
    local label="${2:-}"
    if [[ "$expected" -eq "$actual" ]]; then
        return 0
    fi
    printf "FAIL\n"
    printf "    Expected exit code %s, got %s\n" "$expected" "$actual"
    [[ -n "$label" ]] && printf "    Note: %s\n" "$label"
    FAIL=$((FAIL + 1))
    return 1
}

assert_file_unchanged() {
    local before_file="$1"
    local after_file="$2"
    local label="${3:-}"
    if cmp -s "$before_file" "$after_file"; then
        return 0
    fi
    printf "FAIL\n"
    printf "    File was modified but should not have been\n"
    [[ -n "$label" ]] && printf "    Note: %s\n" "$label"
    FAIL=$((FAIL + 1))
    return 1
}

cleanup() {
    if [[ -n "$MASTER_TMP" && -d "$MASTER_TMP" ]]; then
        rm -rf "$MASTER_TMP"
    fi
}
trap cleanup EXIT

# Create a fresh per-test temp directory.
make_test_dir() {
    local td
    td="$(mktemp -d "${MASTER_TMP}/test-XXXXXX")"
    printf '%s' "$td"
}

# ---------------------------------------------------------------------------
# Guard: ensure the script under test exists
# ---------------------------------------------------------------------------

if [[ ! -f "$CONSOLIDATE_SCRIPT" ]]; then
    printf 'ERROR: script not found: %s\n' "$CONSOLIDATE_SCRIPT" >&2
    exit 1
fi

MASTER_TMP="$(mktemp -d /tmp/consolidate-progress-test-XXXXXX)"

# ---------------------------------------------------------------------------
# Helper: make_fake_project_root
#
# The script under test derives PROJECT_ROOT from BASH_SOURCE[0]:
#   SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
#   PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
#
# To isolate tests from the real PROGRESS.md we mirror the directory layout
# into a temp tree:
#
#   $fakeroot/
#     scripts/
#       review/
#         consolidate-progress.sh   (copy of real script)
#       phases-lib.sh               (copy of real lib)
#     docs/
#       tasks/
#         phases.conf               (copy of real conf)
#         PROGRESS.md               (test-controlled fixture)
#
# Then we run $fakeroot/scripts/review/consolidate-progress.sh so that
# BASH_SOURCE[0] produces the correct SCRIPT_DIR, making PROJECT_ROOT=$fakeroot
# and PROGRESS_FILE=$fakeroot/docs/tasks/PROGRESS.md.
#
# Returns the fake root path via stdout.
# ---------------------------------------------------------------------------

make_fake_project_root() {
    local td
    td="$(mktemp -d "${MASTER_TMP}/fakeroot-XXXXXX")"

    mkdir -p "$td/scripts/review" "$td/docs/tasks"

    # Copy the scripts (not symlinks: we want isolated, immutable copies).
    cp "$CONSOLIDATE_SCRIPT" "$td/scripts/review/consolidate-progress.sh"
    cp "$REAL_PROJECT_ROOT/scripts/phases-lib.sh" "$td/scripts/phases-lib.sh"

    # Re-use the real phases.conf so phase validation succeeds.
    cp "$REAL_PROJECT_ROOT/docs/tasks/phases.conf" "$td/docs/tasks/phases.conf"

    # Create an empty placeholder PROGRESS.md; each test overwrites it.
    touch "$td/docs/tasks/PROGRESS.md"

    printf '%s' "$td"
}

# Convenience: return the path to the test script copy inside a fake root.
fake_script() {
    local fakeroot="$1"
    printf '%s/scripts/review/consolidate-progress.sh' "$fakeroot"
}

# ---------------------------------------------------------------------------
# Helper: write a minimal PROGRESS.md with T-001 through T-003 sections
# This fixture covers phase 1 (T-001:T-015) and bleeds into phase 2 start.
# ---------------------------------------------------------------------------

write_fixture_progress() {
    local path="$1"
    cat > "$path" << 'FIXTURE'
# Raven Task Progress Log

## Summary

| Status | Count |
|--------|-------|
| Completed | 3 |

---

## Completed Tasks

### T-001: Go Project Initialization

- **Status:** Completed
- **Date:** 2026-01-01

**What was built:**

- Module initialized

**Files created/modified:**

- `go.mod`

**Verification:**

- `go build ./cmd/raven/` pass

---

### T-002: Makefile with Build Targets

- **Status:** Completed
- **Date:** 2026-01-02

**What was built:**

- GNU Makefile with 12 targets

**Files created/modified:**

- `Makefile`

**Verification:**

- `go build ./cmd/raven/` pass

---

### T-003: Build Info Package

- **Status:** Completed
- **Date:** 2026-01-03

**What was built:**

- Info struct and GetInfo() function

**Files created/modified:**

- `internal/buildinfo/buildinfo.go`

**Verification:**

- `go build ./cmd/raven/` pass

---

### T-016: Task Spec Markdown Parser

- **Status:** Completed
- **Date:** 2026-01-16

**What was built:**

- Parser for phase 2

---

## Not Started Tasks

Some content after the tasks.
FIXTURE
}

# ---------------------------------------------------------------------------
# Helper: write a stub claude binary to a fake bin/ dir.
# The stub writes a minimal phase block to stdout (simulating a successful
# consolidation response).
# ---------------------------------------------------------------------------

write_mock_claude() {
    local bin_dir="$1"
    mkdir -p "$bin_dir"
    cat > "$bin_dir/claude" << 'MOCK_CLAUDE'
#!/usr/bin/env bash
# Stub claude -- reads stdin, writes a minimal phase block to stdout.
cat > /dev/null  # consume stdin (the prompt)
cat << 'PHASE_BLOCK'
### Phase 1: Foundation (T-001 to T-015)

- **Status:** Completed
- **Date:** 2026-01-03
- **Tasks Completed:** 3 tasks

#### Features Implemented

| Feature | Tasks | Description |
| ------- | ----- | ----------- |
| Project scaffolding | T-001, T-002, T-003 | Initial module, Makefile, buildinfo |

---
PHASE_BLOCK
MOCK_CLAUDE
    chmod +x "$bin_dir/claude"
}

# ---------------------------------------------------------------------------
# Helper: write a stub git binary that is a no-op (prevents real commits).
# ---------------------------------------------------------------------------

write_mock_git() {
    local bin_dir="$1"
    mkdir -p "$bin_dir"
    cat > "$bin_dir/git" << 'MOCK_GIT'
#!/usr/bin/env bash
# Stub git -- records calls, pretends everything is fine.
# For "diff --cached --quiet" return 1 so the real script thinks there is
# something to commit (exercises the commit branch) but git commit is a no-op.
case "$*" in
    *"diff --cached --quiet"*) exit 1 ;;
    *) exit 0 ;;
esac
MOCK_GIT
    chmod +x "$bin_dir/git"
}

# ---------------------------------------------------------------------------
# Extract the Python helper from the script into a temp file.
# We do this by sourcing write_python_helper from the script, then calling it.
# ---------------------------------------------------------------------------

extract_python_helper() {
    local dest="$1"
    # Source only the write_python_helper function definition (avoid running main).
    # We use a subshell with PHASE_ID forced set and source the script up to
    # the function -- simpler: just call the script with a fake helper extraction.
    #
    # Strategy: source the script's write_python_helper function by evaluating
    # the function body directly from the file.
    local func_body
    func_body="$(awk '
        BEGIN { found=0; depth=0 }
        !found && /^write_python_helper\(\)/ { found=1 }
        found {
            for (i=1; i<=length($0); i++) {
                c = substr($0, i, 1)
                if (c == "{") depth++
                else if (c == "}") depth--
            }
            print
            if (depth == 0 && found) { found=0; exit }
        }
    ' "$CONSOLIDATE_SCRIPT")"
    eval "$func_body"
    write_python_helper "$dest"
}

# ---------------------------------------------------------------------------
# SECTION 1: Argument parsing
# ---------------------------------------------------------------------------

echo ""
echo "Consolidate-Progress Script Tests"
echo "=================================="
echo ""
echo "--- Argument Parsing ---"

# Test: missing --phase
begin_test "Missing --phase exits 1 with 'required' in output"
td="$(make_test_dir)"
run_capture "$td/out.txt" "$CONSOLIDATE_SCRIPT"
output="$(cat "$td/out.txt")"
if assert_exit_code 1 "missing --phase should exit 1" && \
   assert_contains "$output" "required" "output must mention 'required'"; then
    pass_test
fi

# Test: unknown phase ID
# validate_single_phase runs before PROGRESS.md is checked, so no file setup needed.
begin_test "Unknown --phase 99 exits 1 with 'Unknown phase' in output"
td="$(make_test_dir)"
run_capture "$td/out.txt" "$CONSOLIDATE_SCRIPT" --phase 99
output="$(cat "$td/out.txt")"
if assert_exit_code 1 "unknown phase should exit 1" && \
   assert_contains "$output" "Unknown phase" "output must mention 'Unknown phase'"; then
    pass_test
fi

# Test: --help exits 0 and prints usage
begin_test "--help exits 0 and prints usage text"
td="$(make_test_dir)"
run_capture "$td/out.txt" "$CONSOLIDATE_SCRIPT" --help
output="$(cat "$td/out.txt")"
if assert_exit_code 0 "--help should exit 0" && \
   assert_contains "$output" "Usage" "usage block must be present"; then
    pass_test
fi

# Test: --help prints --phase option description
begin_test "--help output contains --phase option"
if assert_contains "$output" "--phase" "--phase option must appear in help"; then
    pass_test
fi

# Test: --help prints --dry-run option description
begin_test "--help output contains --dry-run option"
if assert_contains "$output" "--dry-run" "--dry-run option must appear in help"; then
    pass_test
fi

# ---------------------------------------------------------------------------
# SECTION 2: Idempotency
# ---------------------------------------------------------------------------

echo ""
echo "--- Idempotency ---"

# Test: when the first task header (### T-001:) is absent, script skips.
# The script reads PROGRESS_FILE="$PROJECT_ROOT/docs/tasks/PROGRESS.md", so we
# point PROJECT_ROOT at a fake root that contains a pre-consolidated PROGRESS.md.
begin_test "Already consolidated: skips with exit 0 when T-001 header absent"
fakeroot="$(make_fake_project_root)"
progress_file="$fakeroot/docs/tasks/PROGRESS.md"
cat > "$progress_file" << 'CONSOLIDATED'
# Raven Task Progress Log

### Phase 1: Foundation (T-001 to T-015)

- **Status:** Completed

---
CONSOLIDATED
cp "$progress_file" "$progress_file.before"

run_capture_split "$fakeroot/stdout.txt" "$fakeroot/stderr.txt" \
    "$(fake_script "$fakeroot")" --phase 1

stdout="$(cat "$fakeroot/stdout.txt")"
stderr="$(cat "$fakeroot/stderr.txt")"
combined="${stdout}${stderr}"

if assert_exit_code 0 "should exit 0 when already consolidated" && \
   assert_contains "$combined" "already consolidated" "must print 'already consolidated'" && \
   assert_file_unchanged "$progress_file.before" "$progress_file" "file must not change"; then
    pass_test
fi

# Test: when the first task header IS present, the script does NOT skip.
# Use --dry-run so execution stops before calling claude.
begin_test "Not yet consolidated: proceeds (no skip) when T-001 header present"
fakeroot="$(make_fake_project_root)"
write_fixture_progress "$fakeroot/docs/tasks/PROGRESS.md"

run_capture_split "$fakeroot/stdout.txt" "$fakeroot/stderr.txt" \
    "$(fake_script "$fakeroot")" --phase 1 --dry-run

stdout="$(cat "$fakeroot/stdout.txt")"
stderr="$(cat "$fakeroot/stderr.txt")"
combined="${stdout}${stderr}"

if assert_exit_code 0 "dry-run should exit 0 when header present" && \
   assert_not_contains "$combined" "already consolidated" "must NOT skip when header present"; then
    pass_test
fi

# ---------------------------------------------------------------------------
# SECTION 3: Dry-run mode
# ---------------------------------------------------------------------------

echo ""
echo "--- Dry-Run Mode ---"

# Test: dry-run exits 0
begin_test "Dry-run exits 0"
fakeroot="$(make_fake_project_root)"
write_fixture_progress "$fakeroot/docs/tasks/PROGRESS.md"
cp "$fakeroot/docs/tasks/PROGRESS.md" "$fakeroot/docs/tasks/PROGRESS.md.before"

run_capture_split "$fakeroot/stdout.txt" "$fakeroot/stderr.txt" \
    "$(fake_script "$fakeroot")" --phase 1 --dry-run

if assert_exit_code 0 "dry-run must exit 0"; then
    pass_test
fi

# Test: dry-run prints DRY-RUN in output
begin_test "Dry-run prints DRY-RUN in output"
stdout="$(cat "$fakeroot/stdout.txt")"
stderr="$(cat "$fakeroot/stderr.txt")"
combined="${stdout}${stderr}"
if assert_contains "$combined" "DRY-RUN" "must mention DRY-RUN"; then
    pass_test
fi

# Test: dry-run prints the task count
begin_test "Dry-run prints count of tasks found (3 tasks)"
# The fixture has T-001, T-002, T-003 in phase 1 range.
if assert_contains "$combined" "3" "must mention number of tasks found"; then
    pass_test
fi

# Test: dry-run does NOT modify PROGRESS.md
begin_test "Dry-run makes no changes to PROGRESS.md"
if assert_file_unchanged \
       "$fakeroot/docs/tasks/PROGRESS.md.before" \
       "$fakeroot/docs/tasks/PROGRESS.md" \
       "PROGRESS.md must be unchanged"; then
    pass_test
fi

# Test: dry-run makes no git commits.
# Put a sentinel git stub on PATH that records every call.
begin_test "Dry-run makes no git commits"
fakeroot2="$(make_fake_project_root)"
write_fixture_progress "$fakeroot2/docs/tasks/PROGRESS.md"
mkdir -p "$fakeroot2/bin"
git_log_file="$fakeroot2/git-calls.log"
cat > "$fakeroot2/bin/git" << GITSENT
#!/usr/bin/env bash
printf '%s\n' "\$@" >> "$git_log_file"
exit 0
GITSENT
chmod +x "$fakeroot2/bin/git"

run_capture_split "$fakeroot2/stdout.txt" "$fakeroot2/stderr.txt" \
    env PATH="$fakeroot2/bin:$PATH" \
        "$(fake_script "$fakeroot2")" --phase 1 --dry-run

# dry-run exits before the git block; commit must not appear in the log.
git_log=""
[[ -f "$git_log_file" ]] && git_log="$(cat "$git_log_file")"
if assert_not_contains "$git_log" "commit" "git commit must not be invoked in dry-run"; then
    pass_test
fi

# ---------------------------------------------------------------------------
# SECTION 4: Missing PROGRESS.md
# ---------------------------------------------------------------------------

echo ""
echo "--- Missing PROGRESS.md ---"

# Create a fake root but DELETE the PROGRESS.md so the script hits the "not found" check.
begin_test "Missing PROGRESS.md exits 1 with 'not found' in output"
fakeroot="$(make_fake_project_root)"
rm -f "$fakeroot/docs/tasks/PROGRESS.md"

run_capture "$fakeroot/out.txt" \
    "$(fake_script "$fakeroot")" --phase 1

output="$(cat "$fakeroot/out.txt")"
if assert_exit_code 1 "should exit 1 when PROGRESS.md missing" && \
   assert_contains "$output" "not found" "must mention 'not found'"; then
    pass_test
fi

# ---------------------------------------------------------------------------
# SECTION 5: Python helper -- extract command
# ---------------------------------------------------------------------------

echo ""
echo "--- Python Helper: extract ---"

begin_test "Extract returns only in-range task sections"
td="$(make_test_dir)"
pyhelper="$td/helper.py"
extract_python_helper "$pyhelper"

# Write a PROGRESS.md spanning two phases (tasks 1-3 in phase 1, task 16 in phase 2).
write_fixture_progress "$td/PROGRESS.md"

# Extract only phase 1 tasks (1-15).
extracted="$(python3 "$pyhelper" extract "$td/PROGRESS.md" 1 15)"

if assert_contains "$extracted" "### T-001:" "T-001 must be in extract output" && \
   assert_contains "$extracted" "### T-002:" "T-002 must be in extract output" && \
   assert_contains "$extracted" "### T-003:" "T-003 must be in extract output" && \
   assert_not_contains "$extracted" "### T-016:" "T-016 (phase 2) must NOT be in extract output"; then
    pass_test
fi

begin_test "Extract returns only phase-2 section when range is 16-30"
td2="$(make_test_dir)"
pyhelper2="$td2/helper.py"
extract_python_helper "$pyhelper2"
write_fixture_progress "$td2/PROGRESS.md"

extracted2="$(python3 "$pyhelper2" extract "$td2/PROGRESS.md" 16 30)"

if assert_contains "$extracted2" "### T-016:" "T-016 must be in phase-2 extract" && \
   assert_not_contains "$extracted2" "### T-001:" "T-001 must NOT be in phase-2 extract" && \
   assert_not_contains "$extracted2" "### T-002:" "T-002 must NOT be in phase-2 extract"; then
    pass_test
fi

begin_test "Extract for out-of-range returns empty output"
td3="$(make_test_dir)"
pyhelper3="$td3/helper.py"
extract_python_helper "$pyhelper3"
write_fixture_progress "$td3/PROGRESS.md"

extracted3="$(python3 "$pyhelper3" extract "$td3/PROGRESS.md" 50 60)"

if [[ -z "$(printf '%s' "$extracted3" | tr -d '[:space:]')" ]]; then
    pass_test
else
    fail_test "Expected empty output for out-of-range extract, got: $extracted3"
fi

begin_test "Extracted sections preserve task content"
td4="$(make_test_dir)"
pyhelper4="$td4/helper.py"
extract_python_helper "$pyhelper4"
write_fixture_progress "$td4/PROGRESS.md"

extracted4="$(python3 "$pyhelper4" extract "$td4/PROGRESS.md" 1 15)"

if assert_contains "$extracted4" "Module initialized" "T-001 body content must be present" && \
   assert_contains "$extracted4" "GNU Makefile" "T-002 body content must be present" && \
   assert_contains "$extracted4" "GetInfo()" "T-003 body content must be present"; then
    pass_test
fi

# ---------------------------------------------------------------------------
# SECTION 6: Python helper -- replace command
# ---------------------------------------------------------------------------

echo ""
echo "--- Python Helper: replace ---"

begin_test "Replace removes T-001 and T-002 sections and inserts new block"
td="$(make_test_dir)"
pyhelper="$td/helper.py"
extract_python_helper "$pyhelper"
write_fixture_progress "$td/PROGRESS.md"

# Write the replacement block (simulating what claude would return).
cat > "$td/new_block.md" << 'NEW_BLOCK'
### Phase 1: Foundation (T-001 to T-002)

- **Status:** Completed
- **Date:** 2026-01-02
- **Tasks Completed:** 2 tasks

---
NEW_BLOCK

python3 "$pyhelper" replace "$td/PROGRESS.md" 1 2 "$td/new_block.md"
result="$(cat "$td/PROGRESS.md")"

if assert_not_contains "$result" "### T-001:" "T-001 section must be gone after replace" && \
   assert_not_contains "$result" "### T-002:" "T-002 section must be gone after replace" && \
   assert_contains "$result" "### Phase 1: Foundation" "new consolidated block must be present"; then
    pass_test
fi

begin_test "Replace preserves content outside the replaced range"
td="$(make_test_dir)"
pyhelper="$td/helper.py"
extract_python_helper "$pyhelper"
write_fixture_progress "$td/PROGRESS.md"

cat > "$td/new_block.md" << 'NEW_BLOCK'
### Phase 1: Foundation (T-001 to T-002)

- **Status:** Completed

---
NEW_BLOCK

python3 "$pyhelper" replace "$td/PROGRESS.md" 1 2 "$td/new_block.md"
result="$(cat "$td/PROGRESS.md")"

# T-003 and T-016 are outside the replaced range (1-2); they must still be present.
if assert_contains "$result" "### T-003:" "T-003 (not in replace range) must survive" && \
   assert_contains "$result" "### T-016:" "T-016 (phase 2, not in replace range) must survive"; then
    pass_test
fi

begin_test "Replace preserves file header and footer content"
td="$(make_test_dir)"
pyhelper="$td/helper.py"
extract_python_helper "$pyhelper"
write_fixture_progress "$td/PROGRESS.md"

cat > "$td/new_block.md" << 'NEW_BLOCK'
### Phase 1: Foundation (T-001 to T-015)

- **Status:** Completed

---
NEW_BLOCK

python3 "$pyhelper" replace "$td/PROGRESS.md" 1 15 "$td/new_block.md"
result="$(cat "$td/PROGRESS.md")"

# Content before first task header must survive.
if assert_contains "$result" "# Raven Task Progress Log" "file header must survive replace" && \
   assert_contains "$result" "## Summary" "summary section must survive replace"; then
    pass_test
fi

begin_test "Replace with no matching sections is a no-op (exits 0)"
td="$(make_test_dir)"
pyhelper="$td/helper.py"
extract_python_helper "$pyhelper"
write_fixture_progress "$td/PROGRESS.md"
cp "$td/PROGRESS.md" "$td/PROGRESS.md.before"

cat > "$td/new_block.md" << 'NEW_BLOCK'
### Phase 5: PRD Decomposition

- **Status:** Completed

---
NEW_BLOCK

# Range 56-65 doesn't exist in the fixture.
set +e
python3 "$pyhelper" replace "$td/PROGRESS.md" 56 65 "$td/new_block.md"
rc=$?
set -e

if [[ "$rc" -eq 0 ]] && assert_file_unchanged "$td/PROGRESS.md.before" "$td/PROGRESS.md" "file must not change when no sections match"; then
    pass_test
fi

begin_test "Replace with a full-phase range removes all matching sections"
td="$(make_test_dir)"
pyhelper="$td/helper.py"
extract_python_helper "$pyhelper"
write_fixture_progress "$td/PROGRESS.md"

cat > "$td/new_block.md" << 'NEW_BLOCK'
### Phase 1: Foundation (T-001 to T-015)

- **Status:** Completed
- **Tasks Completed:** 3 tasks

---
NEW_BLOCK

# Replace entire phase 1 range (1-15); T-001, T-002, T-003 all fall in this range.
python3 "$pyhelper" replace "$td/PROGRESS.md" 1 15 "$td/new_block.md"
result="$(cat "$td/PROGRESS.md")"

if assert_not_contains "$result" "### T-001:" "T-001 must be gone" && \
   assert_not_contains "$result" "### T-002:" "T-002 must be gone" && \
   assert_not_contains "$result" "### T-003:" "T-003 must be gone" && \
   assert_contains "$result" "Tasks Completed:** 3 tasks" "new block content must be present" && \
   assert_contains "$result" "### T-016:" "T-016 (phase 2) must be untouched"; then
    pass_test
fi

# ---------------------------------------------------------------------------
# SECTION 7: Full integration -- non-dry-run with mock claude and git
# ---------------------------------------------------------------------------

echo ""
echo "--- Integration: Mock claude and git ---"

begin_test "Non-dry-run with mock claude consolidates and calls git commit"
fakeroot="$(make_fake_project_root)"
write_fixture_progress "$fakeroot/docs/tasks/PROGRESS.md"

# Create mock bin directory with both claude and git stubs.
mock_bin="$fakeroot/bin"
write_mock_claude "$mock_bin"

git_log_file="$fakeroot/git-calls.log"
# Git stub that logs all calls; pretends PROGRESS.md changed so commit runs.
cat > "$mock_bin/git" << GIT_INT
#!/usr/bin/env bash
printf '%s\n' "\$@" >> "$git_log_file"
# diff --cached --quiet: return 1 meaning there ARE staged changes.
case "\$*" in
    *"diff --cached --quiet"*) exit 1 ;;
    *) exit 0 ;;
esac
GIT_INT
chmod +x "$mock_bin/git"

run_capture_split "$fakeroot/stdout.txt" "$fakeroot/stderr.txt" \
    env PATH="$mock_bin:$PATH" \
        "$(fake_script "$fakeroot")" --phase 1

stdout="$(cat "$fakeroot/stdout.txt")"
stderr="$(cat "$fakeroot/stderr.txt")"
combined="${stdout}${stderr}"

git_log=""
[[ -f "$git_log_file" ]] && git_log="$(cat "$git_log_file")"

if assert_exit_code 0 "integration run should exit 0" && \
   assert_contains "$combined" "consolidated" "output must mention consolidation" && \
   assert_contains "$git_log" "commit" "git commit must be called"; then
    pass_test
fi

begin_test "Non-dry-run removes per-task headers from PROGRESS.md"
result="$(cat "$fakeroot/docs/tasks/PROGRESS.md")"
if assert_not_contains "$result" "### T-001:" "T-001 must be removed from PROGRESS.md" && \
   assert_not_contains "$result" "### T-002:" "T-002 must be removed from PROGRESS.md" && \
   assert_not_contains "$result" "### T-003:" "T-003 must be removed from PROGRESS.md"; then
    pass_test
fi

begin_test "Non-dry-run inserts consolidated phase block into PROGRESS.md"
result="$(cat "$fakeroot/docs/tasks/PROGRESS.md")"
if assert_contains "$result" "### Phase 1:" "consolidated phase block must be present"; then
    pass_test
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

echo ""
echo "=================================="
printf "Results: %d passed, %d failed\n" "$PASS" "$FAIL"
echo "=================================="

if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi

echo ""
echo "All consolidate-progress tests passed!"
exit 0
