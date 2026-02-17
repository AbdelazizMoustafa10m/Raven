#!/usr/bin/env bash
#
# consolidate-progress.sh -- Consolidate per-task PROGRESS.md entries into one phase block.
#
# Called from create-pr.sh before gh pr create so every PR reflects the lean file.
#
# Usage:
#   ./scripts/review/consolidate-progress.sh --phase <id> [--dry-run]
#
# What it does:
#   1. Sources phases-lib.sh to resolve the task range for the phase.
#   2. Checks idempotency: skips if a ### Phase N: block already exists.
#   3. Extracts per-task ### T-XXX: sections from PROGRESS.md via Python.
#   4. Calls claude-sonnet-4-6 to generate a compact phase-level block.
#   5. Replaces the per-task entries in-place via Python.
#   6. Commits the result.
#

set -eo pipefail

if ((BASH_VERSINFO[0] < 4)); then
    printf 'ERROR: Bash 4+ required (found %s).\n' "$BASH_VERSION" >&2
    printf '  macOS fix: brew install bash\n' >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source-path=SCRIPTDIR
source "$SCRIPT_DIR/../phases-lib.sh"

PHASE_ID=""
DRY_RUN="false"
PROGRESS_FILE="$PROJECT_ROOT/docs/tasks/PROGRESS.md"
MODEL="claude-sonnet-4-6"

# ── UI ──────────────────────────────────────────────────────────────

if [[ -t 1 ]]; then
    _BOLD=$'\033[1m'      _DIM=$'\033[2m'       _RESET=$'\033[0m'
    _RED=$'\033[0;31m'    _GREEN=$'\033[0;32m'   _YELLOW=$'\033[1;33m'
    _CYAN=$'\033[0;36m'   _WHITE=$'\033[1;37m'   _BG_RED=$'\033[41m'
else
    _BOLD='' _DIM='' _RESET='' _RED='' _GREEN='' _YELLOW='' _CYAN='' _WHITE='' _BG_RED=''
fi

_SYM_CHECK="${_GREEN}✓${_RESET}"
_SYM_ARROW="${_CYAN}▸${_RESET}"
_SYM_WARN="${_YELLOW}⚠${_RESET}"

log_step() {
    local icon="$1" msg="$2" ts
    ts="$(date '+%H:%M:%S')"
    printf '  %b%s%b  %b %s\n' "$_DIM" "$ts" "$_RESET" "$icon" "$msg"
}

die() {
    printf '\n  %b%b ERROR %b %b%s%b\n\n' "$_BG_RED" "$_WHITE" "$_RESET" "$_RED" "$1" "$_RESET" >&2
    exit 1
}

# ── Args ────────────────────────────────────────────────────────────

usage() {
    cat <<'USAGE'
Consolidate per-task PROGRESS.md entries into one compact phase block.

Usage:
  ./scripts/review/consolidate-progress.sh --phase <id> [--dry-run]

Options:
  --phase <id>   Phase number (matches phases.conf)
  --dry-run      Preview what would happen without writing or committing
  -h, --help     Show this help
USAGE
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --phase)   PHASE_ID="$2"; shift 2 ;;
            --dry-run) DRY_RUN="true"; shift ;;
            -h|--help) usage; exit 0 ;;
            *) die "Unknown option: $1" ;;
        esac
    done
    if [[ -z "$PHASE_ID" ]]; then
        die "--phase is required"
    fi
}

# ── Python helpers ───────────────────────────────────────────────────
#
# Write a standalone Python3 script to a temp file.
# Using a file avoids heredoc quoting conflicts when new_content contains
# shell-special characters.
#

write_python_helper() {
    local pyfile="$1"
    cat > "$pyfile" << 'PYEOF'
#!/usr/bin/env python3
"""
consolidate_helper.py  command  progress_file  start_num  end_num  [new_content_file]

Commands:
  extract  -- print task sections for the given numeric range to stdout
  replace  -- replace those sections with the content in new_content_file
"""
import sys
import re

command         = sys.argv[1]
fname           = sys.argv[2]
s_num           = int(sys.argv[3])
e_num           = int(sys.argv[4])

with open(fname, encoding="utf-8") as fh:
    content = fh.read()

# Find all ### T-NNN: header positions
TASK_RE = re.compile(r"^### T-(\d+):", re.MULTILINE)
matches = list(TASK_RE.finditer(content))


def section_end(idx: int) -> int:
    """Return the content end-offset for the task section at matches[idx]."""
    if idx + 1 < len(matches):
        # End just before the next task header
        return matches[idx + 1].start()
    # Last task: end at the next top-level section or EOF
    next_major = re.search(r"^## ", content[matches[idx].start():], re.MULTILINE)
    if next_major:
        return matches[idx].start() + next_major.start()
    return len(content)


in_range = [
    (i, m, int(m.group(1)))
    for i, m in enumerate(matches)
    if s_num <= int(m.group(1)) <= e_num
]

if command == "extract":
    parts = []
    for i, m, _ in in_range:
        chunk = content[m.start() : section_end(i)].rstrip()
        parts.append(chunk)
    print("\n\n".join(parts))

elif command == "replace":
    new_content_file = sys.argv[5]
    with open(new_content_file, encoding="utf-8") as fh:
        new_block = fh.read().strip()

    if not in_range:
        sys.exit(0)

    first_idx, first_m, _ = in_range[0]
    last_idx,  last_m,  _ = in_range[-1]

    replace_start = first_m.start()
    replace_end   = section_end(last_idx)

    new_content = content[:replace_start] + new_block + "\n\n" + content[replace_end:]

    with open(fname, "w", encoding="utf-8") as fh:
        fh.write(new_content)
PYEOF
}

# ── Core ────────────────────────────────────────────────────────────

main() {
    parse_args "$@"

    if ! validate_single_phase "$PHASE_ID"; then
        die "Unknown phase: '$PHASE_ID'. Valid phases: ${ALL_PHASE_IDS[*]}"
    fi

    local range_var="PHASE_RANGES_${PHASE_ID}"
    local range="${!range_var}"             # e.g. T-001:T-015
    local start_task="${range#T-}"
    start_task="${start_task%%:*}"          # e.g. 001
    local end_task="${range##*T-}"          # e.g. 015
    local start_num=$((10#$start_task))
    local end_num=$((10#$end_task))
    local ptitle
    ptitle="$(phase_title "$PHASE_ID")"

    if [[ ! -f "$PROGRESS_FILE" ]]; then
        die "PROGRESS.md not found: $PROGRESS_FILE"
    fi

    # Idempotency: if the first per-task header for this phase no longer exists in the
    # file, the entries have already been consolidated into a phase block -- skip.
    if ! grep -q "^### T-${start_task}:" "$PROGRESS_FILE" 2>/dev/null; then
        log_step "$_SYM_CHECK" \
            "Phase $PHASE_ID already consolidated (T-${start_task} not found) -- skipping"
        return 0
    fi

    # Set up temp files; cleaned up on exit
    local pyfile prompt_file consolidated_file
    pyfile="$(mktemp "${TMPDIR:-/tmp}/consolidate-XXXXXX.py")"
    prompt_file="$(mktemp "${TMPDIR:-/tmp}/consolidate-prompt-XXXXXX.md")"
    consolidated_file="$(mktemp "${TMPDIR:-/tmp}/consolidate-out-XXXXXX.md")"
    # shellcheck disable=SC2064
    trap "rm -f '$pyfile' '$prompt_file' '$consolidated_file'" EXIT

    write_python_helper "$pyfile"

    # Extract per-task sections
    local task_content
    task_content="$(python3 "$pyfile" extract "$PROGRESS_FILE" "$start_num" "$end_num")"

    if [[ -z "$task_content" ]]; then
        log_step "$_SYM_WARN" "No per-task entries found for T-${start_task} to T-${end_task} -- skipping"
        return 0
    fi

    local task_count
    task_count="$(printf '%s' "$task_content" | grep -c '^### T-[0-9]' || true)"
    log_step "$_SYM_ARROW" \
        "Consolidating $task_count task entries → Phase $PHASE_ID: $ptitle  ${_DIM}(${MODEL})${_RESET}"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_step "${_DIM}⊘${_RESET}" \
            "DRY-RUN: would condense $task_count tasks into one phase block and commit"
        return 0
    fi

    # Build prompt
    cat > "$prompt_file" << PROMPT_EOF
You are compressing verbose per-task PROGRESS.md entries into a single compact phase block for the Raven project.

## Target format

Follow this structure exactly -- adapt sections to what the tasks actually built:

\`\`\`markdown
### Phase N: Display Name (T-XXX to T-YYY)

- **Status:** Completed
- **Date:** YYYY-MM-DD
- **Tasks Completed:** N tasks

#### Features Implemented

| Feature | Tasks | Description |
| ------- | ----- | ----------- |
| Short feature name | T-XXX, T-YYY | One-line description of what was built |

#### Key Technical Decisions

1. **Decision name** -- brief rationale

#### Key Files Reference

| Purpose | Location |
| ------- | -------- |
| Short description | \`path/to/file.go\` |

#### Verification

- \`go build ./cmd/raven/\` pass
- \`go vet ./...\` pass
- \`go test ./...\` pass

---
\`\`\`

## Rules

- Start with exactly: \`### Phase ${PHASE_ID}: ${ptitle} (T-${start_task} to T-${end_task})\`
- Merge all "Files created/modified" lists into a single **Key Files Reference** table grouped by purpose
- Keep technical names exact: package names, type names, function names, file paths
- Include a **Key Technical Decisions** section only if there were explicit decisions in the tasks
- Use dates from the task entries; if none are present use today: $(date +%Y-%m-%d)
- Omit empty sections entirely
- Output ONLY the markdown block -- no preamble, no explanation, no outer code fences
- End the block with a single \`---\` line

## Per-task entries to consolidate

${task_content}
PROMPT_EOF

    # Call Claude
    if ! claude -p --model "$MODEL" < "$prompt_file" > "$consolidated_file"; then
        die "claude exited with non-zero status while consolidating phase $PHASE_ID"
    fi

    if [[ ! -s "$consolidated_file" ]]; then
        die "claude returned empty output for phase $PHASE_ID consolidation"
    fi

    # Strip any outer code fences claude may have added
    local stripped
    stripped="$(mktemp "${TMPDIR:-/tmp}/consolidate-stripped-XXXXXX.md")"
    # shellcheck disable=SC2064
    trap "rm -f '$pyfile' '$prompt_file' '$consolidated_file' '$stripped'" EXIT
    sed '/^```/d' "$consolidated_file" > "$stripped"
    mv "$stripped" "$consolidated_file"

    # Replace per-task sections in PROGRESS.md
    python3 "$pyfile" replace "$PROGRESS_FILE" "$start_num" "$end_num" "$consolidated_file"

    # Commit only if the file actually changed
    git -C "$PROJECT_ROOT" add "$PROGRESS_FILE"
    if git -C "$PROJECT_ROOT" diff --cached --quiet "$PROGRESS_FILE" 2>/dev/null; then
        log_step "$_SYM_WARN" "PROGRESS.md unchanged after consolidation -- nothing to commit"
        return 0
    fi

    git -C "$PROJECT_ROOT" commit \
        -m "docs(progress): consolidate phase $PHASE_ID entries into phase block"

    log_step "$_SYM_CHECK" \
        "PROGRESS.md consolidated and committed  ${_DIM}(${task_count} entries → 1 phase block)${_RESET}"
}

main "$@"
