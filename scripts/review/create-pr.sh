#!/usr/bin/env bash

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# ── UI: Detection & Colors ──────────────────────────────────────────

HAS_GUM=false
if command -v gum >/dev/null 2>&1; then
    HAS_GUM=true
fi

if [[ -t 1 ]]; then
    _BOLD=$'\033[1m'      _DIM=$'\033[2m'       _RESET=$'\033[0m'
    _RED=$'\033[0;31m'    _GREEN=$'\033[0;32m'   _YELLOW=$'\033[1;33m'
    _MAGENTA=$'\033[0;35m' _CYAN=$'\033[0;36m'
    _WHITE=$'\033[1;37m'
    _BG_RED=$'\033[41m'
else
    _BOLD='' _DIM='' _RESET=''
    _RED='' _GREEN='' _YELLOW=''
    _MAGENTA='' _CYAN=''
    _WHITE=''
    _BG_RED=''
fi

_SYM_ARROW="${_CYAN}▸${_RESET}"

PHASE_ID=""
BASE_BRANCH="main"
HEAD_BRANCH=""
REVIEW_VERDICT="UNKNOWN"
VERIFICATION_SUMMARY="not provided"
PR_TITLE=""
ARTIFACTS_DIR=""
DRY_RUN="false"

# ── PR Checkbox State ─────────────────────────────────────────────────
VERIFY_BUILD="false"
VERIFY_VET="false"
VERIFY_TEST="false"
VERIFY_TEST_RACE="false"
VERIFY_MOD_TIDY="false"

CHANGE_BUGFIX="false"
CHANGE_FEATURE="false"
CHANGE_BREAKING="false"
CHANGE_REFACTOR="false"
CHANGE_DOCS="false"

PROGRESS_UPDATED="false"

TEMP_BODY_FILE=""

usage() {
    cat <<'USAGE'
Create a GitHub PR for a phase branch.

Usage:
  ./scripts/review/create-pr.sh --phase <id> [options]

Required:
  --phase <id>                 Phase id included in PR metadata

Options:
  --base <branch>              Base branch (default: main)
  --head <branch>              Head branch (default: current branch)
  --review-verdict <value>     Review verdict (default: UNKNOWN)
  --verification-summary <txt> Verification summary line or paragraph
  --title <text>               Explicit PR title
  --artifacts-dir <path>       Optional dir to persist automation metadata
  --dry-run                    Print planned command and rendered body path
  -h, --help                   Show help
USAGE
}

log_step() {
    local icon="$1"
    local msg="$2"
    local ts
    ts="$(date '+%H:%M:%S')"
    printf '  %b%s%b  %b %s\n' "$_DIM" "$ts" "$_RESET" "$icon" "$msg"
}

die() {
    local cols
    cols="${COLUMNS:-$(tput cols 2>/dev/null || echo 80)}"
    local max_width=$(( cols - 6 ))
    (( max_width < 40 )) && max_width=40
    if [[ "$HAS_GUM" == "true" ]]; then
        gum style --foreground 196 --bold --border rounded --border-foreground 196 \
            --padding "0 2" --margin "0 2" --width "$max_width" "ERROR: $1"
    else
        echo ""
        printf '%s' "$1" | fold -s -w "$max_width" | while IFS= read -r line; do
            printf '  %b%b ERROR %b %b%s%b\n' "$_BG_RED" "$_WHITE" "$_RESET" "$_RED" "$line" "$_RESET"
        done
    fi
    exit 1
}

cleanup() {
    if [[ -n "$TEMP_BODY_FILE" && -f "$TEMP_BODY_FILE" ]]; then
        rm -f "$TEMP_BODY_FILE"
    fi
}
trap cleanup EXIT

_on_err() {
    local code=$? line=${1:-?}
    printf '\n  %b%b create-pr error%b  line %d  exit %d\n' \
        "$_RED" "$_BOLD" "$_RESET" "$line" "$code" >&2
    printf '  %b  failed command: %s%b\n' "$_DIM" "${BASH_COMMAND:-?}" "$_RESET" >&2
}
trap '_on_err $LINENO' ERR

resolve_base_ref() {
    if git show-ref --verify --quiet "refs/heads/$BASE_BRANCH"; then
        echo "$BASE_BRANCH"
        return 0
    fi
    if git show-ref --verify --quiet "refs/remotes/origin/$BASE_BRANCH"; then
        echo "origin/$BASE_BRANCH"
        return 0
    fi
    return 1
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --phase)
                PHASE_ID="$2"
                shift 2
                ;;
            --base)
                BASE_BRANCH="$2"
                shift 2
                ;;
            --head)
                HEAD_BRANCH="$2"
                shift 2
                ;;
            --review-verdict)
                REVIEW_VERDICT="$2"
                shift 2
                ;;
            --verification-summary)
                VERIFICATION_SUMMARY="$2"
                shift 2
                ;;
            --title)
                PR_TITLE="$2"
                shift 2
                ;;
            --artifacts-dir)
                ARTIFACTS_DIR="$2"
                shift 2
                ;;
            --dry-run)
                DRY_RUN="true"
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                die "Unknown option: $1"
                ;;
        esac
    done

    if [[ -z "$PHASE_ID" ]]; then
        die "--phase is required"
    fi
}

preflight() {
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        die "Must run inside a git repository"
    fi

    if [[ -z "$HEAD_BRANCH" ]]; then
        HEAD_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
    fi

    if [[ "$DRY_RUN" != "true" ]]; then
        if ! git show-ref --verify --quiet "refs/heads/$HEAD_BRANCH"; then
            die "Head branch '$HEAD_BRANCH' not found locally"
        fi

        if ! resolve_base_ref >/dev/null; then
            die "Could not resolve base branch '$BASE_BRANCH' locally or at origin/$BASE_BRANCH"
        fi
    fi

    if [[ "$DRY_RUN" != "true" ]]; then
        if ! command -v gh >/dev/null 2>&1; then
            die "GitHub CLI (gh) is required"
        fi
    fi
}

build_commit_list() {
    local base_ref="$1"
    git log --reverse --pretty='- %h %s (%an)' "${base_ref}..${HEAD_BRANCH}"
}

ensure_has_commits() {
    local base_ref="$1"
    local count
    count="$(git rev-list --count "${base_ref}..${HEAD_BRANCH}")"
    if [[ "$count" -eq 0 ]]; then
        die "No commits found between '$base_ref' and '$HEAD_BRANCH'"
    fi
}

render_pr_body() {
    local base_ref="$1"
    local template="$PROJECT_ROOT/.github/PULL_REQUEST_TEMPLATE.md"

    if [[ ! -f "$template" ]]; then
        die "PR template not found at .github/PULL_REQUEST_TEMPLATE.md"
    fi

    TEMP_BODY_FILE="$(mktemp "${TMPDIR:-/tmp}/raven-pr-body-XXXXXX.md")"

    cp "$template" "$TEMP_BODY_FILE"

    local commits
    commits="$(build_commit_list "$base_ref")"

    cat >> "$TEMP_BODY_FILE" <<EOF_BODY

## Automation Metadata

- Phase ID: ${PHASE_ID}
- Review Verdict: ${REVIEW_VERDICT}
- Base Branch: ${BASE_BRANCH}
- Base Ref Used: ${base_ref}
- Head Branch: ${HEAD_BRANCH}
- Generated At (UTC): $(date -u +%Y-%m-%dT%H:%M:%SZ)

## Verification Summary

${VERIFICATION_SUMMARY}

## Commits in Scope

${commits}
EOF_BODY
}

default_title() {
    local latest
    latest="$(git log -1 --pretty='%s' "$HEAD_BRANCH")"
    echo "phase ${PHASE_ID}: ${latest}"
}

persist_artifact_metadata() {
    if [[ -z "$ARTIFACTS_DIR" ]]; then
        return 0
    fi

    mkdir -p "$ARTIFACTS_DIR"

    cat > "$ARTIFACTS_DIR/pr-create.env" <<EOF_META
phase=$PHASE_ID
base_branch=$BASE_BRANCH
head_branch=$HEAD_BRANCH
review_verdict=$REVIEW_VERDICT
dry_run=$DRY_RUN
body_file=$TEMP_BODY_FILE
generated_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF_META
}

consolidate_progress() {
    local consolidate_script="$SCRIPT_DIR/consolidate-progress.sh"
    if [[ ! -f "$consolidate_script" ]]; then
        log_step "$_SYM_ARROW" "consolidate-progress.sh not found -- skipping PROGRESS.md consolidation"
        return 0
    fi
    log_step "$_SYM_ARROW" "Consolidating PROGRESS.md for phase ${PHASE_ID}"
    "$consolidate_script" --phase "$PHASE_ID"
}

# ── PR Checkbox Helpers ───────────────────────────────────────────────

# Replace "- [ ] <suffix>" with "- [x] <suffix>" in the body file.
# Uses Python for exact-string matching so backticks and parens need no escaping.
tick_checkbox() {
    local file="$1"
    local suffix="$2"
    python3 - "$file" "$suffix" << 'PYEOF'
import sys
path, suffix = sys.argv[1], sys.argv[2]
content = open(path).read()
open(path, 'w').write(content.replace('- [ ] ' + suffix, '- [x] ' + suffix, 1))
PYEOF
}

run_go_verifications() {
    if [[ ! -f "$PROJECT_ROOT/go.mod" ]]; then
        return 0
    fi

    log_step "$_SYM_ARROW" "Running Go verification checks"

    local out=""

    # go build
    if out="$(cd "$PROJECT_ROOT" && go build ./cmd/raven/ 2>&1)"; then
        VERIFY_BUILD="true"
        log_step "$_SYM_CHECK" "go build ./cmd/raven/"
    else
        log_step "$_SYM_CROSS" "go build ./cmd/raven/  ${_DIM}${out}${_RESET}"
    fi

    # go vet
    if out="$(cd "$PROJECT_ROOT" && go vet ./... 2>&1)"; then
        VERIFY_VET="true"
        log_step "$_SYM_CHECK" "go vet ./..."
    else
        log_step "$_SYM_CROSS" "go vet ./..."
    fi

    # go test
    if out="$(cd "$PROJECT_ROOT" && go test ./... 2>&1)"; then
        VERIFY_TEST="true"
        log_step "$_SYM_CHECK" "go test ./..."
    else
        log_step "$_SYM_CROSS" "go test ./..."
    fi

    # go test -race
    if out="$(cd "$PROJECT_ROOT" && go test -race ./... 2>&1)"; then
        VERIFY_TEST_RACE="true"
        log_step "$_SYM_CHECK" "go test -race ./..."
    else
        log_step "$_SYM_CROSS" "go test -race ./..."
    fi

    # go mod tidy: run, compare originals, restore if drift detected
    local tmp_dir
    tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/raven-modtidy-XXXXXX")"
    cp "$PROJECT_ROOT/go.mod" "$tmp_dir/go.mod"
    if [[ -f "$PROJECT_ROOT/go.sum" ]]; then
        cp "$PROJECT_ROOT/go.sum" "$tmp_dir/go.sum"
    else
        touch "$tmp_dir/go.sum"
    fi

    (cd "$PROJECT_ROOT" && go mod tidy 2>/dev/null)

    if cmp -s "$PROJECT_ROOT/go.mod" "$tmp_dir/go.mod" && \
       cmp -s "$PROJECT_ROOT/go.sum" "$tmp_dir/go.sum"; then
        VERIFY_MOD_TIDY="true"
        log_step "$_SYM_CHECK" "go mod tidy produces no diff"
    else
        log_step "$_SYM_CROSS" "go mod tidy produced changes ${_DIM}(restoring)${_RESET}"
        cp "$tmp_dir/go.mod" "$PROJECT_ROOT/go.mod"
        cp "$tmp_dir/go.sum" "$PROJECT_ROOT/go.sum"
    fi
    rm -rf "$tmp_dir"
}

detect_change_types() {
    local base_ref="$1"
    local subjects
    subjects="$(git log --pretty='%s' "${base_ref}..${HEAD_BRANCH}" 2>/dev/null || true)"

    if printf '%s\n' "$subjects" | grep -qE '^fix(\([^)]+\))?!?:'; then
        CHANGE_BUGFIX="true"
    fi
    if printf '%s\n' "$subjects" | grep -qE '^feat(\([^)]+\))?!?:'; then
        CHANGE_FEATURE="true"
    fi
    if printf '%s\n' "$subjects" | grep -qE '^refactor(\([^)]+\))?!?:'; then
        CHANGE_REFACTOR="true"
    fi
    if printf '%s\n' "$subjects" | grep -qE '^docs(\([^)]+\))?!?:'; then
        CHANGE_DOCS="true"
    fi
    # Breaking change: scope with ! before the colon
    if printf '%s\n' "$subjects" | grep -qE '^[a-z]+(\([^)]+\))?!:'; then
        CHANGE_BREAKING="true"
    fi

    # PROGRESS.md touched anywhere in the branch commits
    if git log --name-only "${base_ref}..${HEAD_BRANCH}" 2>/dev/null \
            | grep -q "docs/tasks/PROGRESS\.md"; then
        PROGRESS_UPDATED="true"
    fi
}

fill_pr_checkboxes() {
    # Type of Change
    if [[ "$CHANGE_BUGFIX"   == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" "Bug fix (non-breaking change fixing an issue)"
    fi
    if [[ "$CHANGE_FEATURE"  == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" "New feature (non-breaking change adding functionality)"
    fi
    if [[ "$CHANGE_BREAKING" == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" "Breaking change (fix or feature causing existing functionality to change)"
    fi
    if [[ "$CHANGE_REFACTOR" == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" "Refactor (code change that neither fixes a bug nor adds a feature)"
    fi
    if [[ "$CHANGE_DOCS"     == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" "Documentation / config update"
    fi

    # Verification
    if [[ "$VERIFY_BUILD"     == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" '`go build ./cmd/raven/` passes'
    fi
    if [[ "$VERIFY_VET"       == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" '`go vet ./...` passes'
    fi
    if [[ "$VERIFY_TEST"      == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" '`go test ./...` passes'
    fi
    if [[ "$VERIFY_TEST_RACE" == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" '`go test -race ./...` passes'
    fi
    if [[ "$VERIFY_MOD_TIDY"  == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" '`go mod tidy` produces no diff'
    fi

    # Task Tracking
    if [[ "$PROGRESS_UPDATED" == "true" ]]; then
        tick_checkbox "$TEMP_BODY_FILE" '`docs/tasks/PROGRESS.md` updated if task completion status changed'
    fi
}

create_pr() {
    local title="$PR_TITLE"
    if [[ -z "$title" ]]; then
        title="$(default_title)"
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        local body_basename
        body_basename="$(basename "$TEMP_BODY_FILE")"
        echo ""
        if [[ "$HAS_GUM" == "true" ]]; then
            gum style --bold --foreground 255 --border rounded --border-foreground 179 \
                --padding "0 2" --margin "0 2" "PR Preview (dry-run)"
        else
            printf '  %b╭────────────────────────────────────╮%b\n' "$_YELLOW" "$_RESET"
            printf '  %b│%b  %b%bPR Preview (dry-run)%b              %b│%b\n' \
                "$_YELLOW" "$_RESET" "$_BOLD" "$_WHITE" "$_RESET" "$_YELLOW" "$_RESET"
            printf '  %b╰────────────────────────────────────╯%b\n' "$_YELLOW" "$_RESET"
        fi
        echo ""
        log_step "$_SYM_ARROW" "Title:  ${_BOLD}${title}${_RESET}"
        log_step "$_SYM_ARROW" "Base:   ${BASE_BRANCH}"
        log_step "$_SYM_ARROW" "Head:   ${HEAD_BRANCH}"
        log_step "$_SYM_ARROW" "Body:   ${_DIM}${body_basename} (saved to ${TMPDIR:-/tmp})${_RESET}"
        echo ""
        return 0
    fi

    log_step "$_SYM_ARROW" "Pushing ${_BOLD}${HEAD_BRANCH}${_RESET} to origin"
    git push -u origin "$HEAD_BRANCH"

    gh pr create \
        --base "$BASE_BRANCH" \
        --head "$HEAD_BRANCH" \
        --title "$title" \
        --body-file "$TEMP_BODY_FILE"
}

main() {
    parse_args "$@"
    preflight

    if [[ "$DRY_RUN" == "true" ]]; then
        # In dry-run, branches may not exist (multi-phase dry-run).
        # Use BASE_BRANCH as-is for rendering.
        local base_ref="$BASE_BRANCH"
        TEMP_BODY_FILE="$(mktemp "${TMPDIR:-/tmp}/raven-pr-body-XXXXXX.md")"

        local template="$PROJECT_ROOT/.github/PULL_REQUEST_TEMPLATE.md"
        if [[ -f "$template" ]]; then
            cp "$template" "$TEMP_BODY_FILE"
        else
            : > "$TEMP_BODY_FILE"
        fi

        cat >> "$TEMP_BODY_FILE" <<EOF_BODY

## Automation Metadata

- Phase ID: ${PHASE_ID}
- Review Verdict: ${REVIEW_VERDICT}
- Base Branch: ${BASE_BRANCH}
- Base Ref Used: ${base_ref}
- Head Branch: ${HEAD_BRANCH}
- Generated At (UTC): $(date -u +%Y-%m-%dT%H:%M:%SZ)

## Verification Summary

${VERIFICATION_SUMMARY}

## Commits in Scope

(dry-run: commit list unavailable)
EOF_BODY
        persist_artifact_metadata
        create_pr
        return 0
    fi

    local base_ref
    base_ref="$(resolve_base_ref)"

    ensure_has_commits "$base_ref"
    render_pr_body "$base_ref"
    detect_change_types "$base_ref"
    run_go_verifications
    fill_pr_checkboxes
    persist_artifact_metadata
    consolidate_progress
    create_pr
}

main "$@"
