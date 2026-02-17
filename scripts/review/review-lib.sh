#!/usr/bin/env bash
# Shared functions for Raven review scripts.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
if git -C "$PROJECT_ROOT" rev-parse --show-toplevel >/dev/null 2>&1; then
  PROJECT_ROOT="$(git -C "$PROJECT_ROOT" rev-parse --show-toplevel)"
fi

PROJECT_NAME="$(basename "$PROJECT_ROOT")"
REPORTS_BASE="$PROJECT_ROOT/reports/review"
RUN_TIMESTAMP="$(date '+%Y-%m-%d_%H-%M-%S')"
REVIEW_WORKSPACE="$PROJECT_ROOT/.review-workspace"
RUN_DIR="$REVIEW_WORKSPACE/review-$RUN_TIMESTAMP"
RAW_DIR="$RUN_DIR/raw"
ARCHIVE_DIR="$REPORTS_BASE/$RUN_TIMESTAMP"
LATEST_LINK="$REPORTS_BASE/latest"

REVIEW_DIR="$PROJECT_ROOT/.github/review"
PROMPTS_DIR="$REVIEW_DIR/prompts"
RULES_DIR="$REVIEW_DIR/rules"
JSON_EXTRACT_SCRIPT="$REVIEW_DIR/scripts/json-extract.js"

REVIEW_PASSES=(full-review security)
ALL_AGENTS=(claude codex gemini)

INLINE_DIFF_THRESHOLD="${INLINE_DIFF_THRESHOLD:-3000}"
REVIEW_RETENTION_DAYS="${REVIEW_RETENTION_DAYS:-14}"

CHANGE_DESCRIPTION=""
CURRENT_BRANCH=""
BASE_REF=""
REVIEW_FILE_COUNT=0
HIGH_RISK_COUNT=0
DIFF_LINES=0
REVIEW_ARCHIVED=false
REVIEW_USING_ARCHIVE=false

PROJECT_BRIEF="Raven is a Go-based AI workflow orchestration command center that manages the full lifecycle of AI-assisted software development -- from PRD decomposition to implementation, code review, fix application, and pull request creation. It orchestrates multiple AI agents (Claude, Codex, Gemini) through configurable workflow pipelines, providing both a headless CLI for automation and an interactive TUI dashboard for real-time monitoring and control."

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

log_info()    { echo -e "${BLUE}INFO${NC} $*"; }
log_success() { echo -e "${GREEN}OK${NC}   $*"; }
log_warn()    { echo -e "${YELLOW}WARN${NC} $*"; }
log_error()   { echo -e "${RED}ERR${NC}  $*"; }
log_header()  { echo -e "\n${BOLD}${CYAN}=== $* ===${NC}\n"; }
log_dim()     { echo -e "${DIM}$*${NC}"; }

die() {
  log_error "$*"
  return 1
}

is_valid_agent() {
  local agent="$1"
  local allowed
  for allowed in "${ALL_AGENTS[@]}"; do
    if [[ "$allowed" == "$agent" ]]; then
      return 0
    fi
  done
  return 1
}

agent_bin() {
  local agent="$1"
  case "$agent" in
    claude|codex|gemini) echo "$agent" ;;
    *) return 1 ;;
  esac
}

# Agent model defaults. Override via env vars:
#   CLAUDE_REVIEW_MODEL, CODEX_REVIEW_MODEL, GEMINI_REVIEW_MODEL
agent_model() {
  local agent="$1"
  case "$agent" in
    claude) echo "${CLAUDE_REVIEW_MODEL:-claude-opus-4-6}" ;;
    codex) echo "${CODEX_REVIEW_MODEL:-gpt-5-codex}" ;;
    gemini) echo "${GEMINI_REVIEW_MODEL:-gemini-2.5-pro}" ;;
    *) return 1 ;;
  esac
}

assert_tool() {
  local tool="$1"
  if command -v "$tool" >/dev/null 2>&1; then
    log_success "$tool -> $(command -v "$tool")"
    return 0
  fi
  log_error "$tool not found"
  return 1
}

check_jq_version() {
  local jq_version
  jq_version="$(jq --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+' | head -1)"
  if [[ -z "$jq_version" ]]; then
    log_warn "Could not determine jq version"
    return 0
  fi
  local major minor
  major="${jq_version%%.*}"
  minor="${jq_version##*.}"
  if (( major < 1 || (major == 1 && minor < 6) )); then
    log_warn "jq $jq_version detected; consolidation requires jq >= 1.6 (todateiso8601)"
  fi
}

resolve_base_ref() {
  local candidate="$1"
  if git -C "$PROJECT_ROOT" rev-parse --verify "$candidate" >/dev/null 2>&1; then
    echo "$candidate"
    return 0
  fi

  if git -C "$PROJECT_ROOT" rev-parse --verify "origin/$candidate" >/dev/null 2>&1; then
    echo "origin/$candidate"
    return 0
  fi

  return 1
}

cleanup_workspace_on_exit() {
  if [[ "$REVIEW_ARCHIVED" == "true" ]]; then
    return 0
  fi

  if [[ -d "$RUN_DIR" ]]; then
    rm -rf "$RUN_DIR"
  fi

  if [[ -d "$REVIEW_WORKSPACE" ]]; then
    rmdir "$REVIEW_WORKSPACE" >/dev/null 2>&1 || true
  fi
}

ensure_review_dirs() {
  mkdir -p "$RAW_DIR"
  trap cleanup_workspace_on_exit EXIT
  log_info "Workspace: $RUN_DIR"
}

cleanup_old_archives() {
  if [[ ! "$REVIEW_RETENTION_DAYS" =~ ^[0-9]+$ ]]; then
    return 0
  fi

  if (( REVIEW_RETENTION_DAYS <= 0 )); then
    return 0
  fi

  [[ -d "$REPORTS_BASE" ]] || return 0

  local old_dir
  while IFS= read -r old_dir; do
    [[ -z "$old_dir" ]] && continue
    [[ "$old_dir" == "$ARCHIVE_DIR" ]] && continue
    rm -rf "$old_dir"
  done < <(find "$REPORTS_BASE" -mindepth 1 -maxdepth 1 -type d -mtime +"$REVIEW_RETENTION_DAYS" 2>/dev/null)
}

archive_review_run() {
  if [[ "$REVIEW_USING_ARCHIVE" == "true" ]]; then
    return 0
  fi

  if [[ ! -d "$RUN_DIR" ]]; then
    return 0
  fi

  mkdir -p "$REPORTS_BASE"
  mv "$RUN_DIR" "$ARCHIVE_DIR"

  RUN_DIR="$ARCHIVE_DIR"
  RAW_DIR="$RUN_DIR/raw"
  REVIEW_ARCHIVED=true

  rm -f "$LATEST_LINK"
  ln -s "$ARCHIVE_DIR" "$LATEST_LINK"

  if [[ -d "$REVIEW_WORKSPACE" ]]; then
    rmdir "$REVIEW_WORKSPACE" >/dev/null 2>&1 || true
  fi

  cleanup_old_archives
  log_success "Archived review run -> $ARCHIVE_DIR"
}

use_latest_review_run() {
  if [[ ! -e "$LATEST_LINK" ]]; then
    die "No latest review run found at $LATEST_LINK"
    return 1
  fi

  local resolved
  resolved="$(cd "$LATEST_LINK" 2>/dev/null && pwd)" || {
    die "Failed to resolve latest review run"
    return 1
  }

  if [[ ! -d "$resolved/raw" ]]; then
    die "Latest review run is missing raw outputs: $resolved/raw"
    return 1
  fi

  RUN_DIR="$resolved"
  RAW_DIR="$RUN_DIR/raw"
  REVIEW_USING_ARCHIVE=true

  log_info "Using archived run: $RUN_DIR"
}

read_context_file() {
  local file_path="$1"
  if [[ -f "$file_path" ]]; then
    cat "$file_path"
  else
    echo "(missing file: $file_path)"
  fi
}

build_change_context() {
  local base_ref="$1"

  local commit_log=""
  commit_log="$(git -C "$PROJECT_ROOT" log --format='%h %s%n%n%b' --no-merges "$base_ref"...HEAD 2>/dev/null | head -200)"

  local context=""
  if [[ -n "$CHANGE_DESCRIPTION" ]]; then
    context+="Author description:\n$CHANGE_DESCRIPTION\n\n"
  fi

  if [[ -n "$commit_log" ]]; then
    context+="Branch commit history:\n$commit_log\n"
  fi

  if [[ -n "$context" ]]; then
    printf '%b' "$context" > "$RAW_DIR/change-context.txt"
  else
    : > "$RAW_DIR/change-context.txt"
  fi
}

collect_diff() {
  local requested_base_ref="$1"

  local resolved_base_ref
  resolved_base_ref="$(resolve_base_ref "$requested_base_ref")" || {
    die "Base ref '$requested_base_ref' not found"
    return 1
  }

  BASE_REF="$resolved_base_ref"
  CURRENT_BRANCH="$(git -C "$PROJECT_ROOT" rev-parse --abbrev-ref HEAD)"

  : > "$RAW_DIR/changed-files.txt"
  : > "$RAW_DIR/review-files.txt"
  : > "$RAW_DIR/high-risk-files.txt"

  git -C "$PROJECT_ROOT" diff --name-only "$BASE_REF"...HEAD > "$RAW_DIR/changed-files.txt" 2>/dev/null || \
    git -C "$PROJECT_ROOT" diff --name-only "$BASE_REF" HEAD > "$RAW_DIR/changed-files.txt"

  local changed_count
  changed_count="$(wc -l < "$RAW_DIR/changed-files.txt" | tr -d ' ')"
  if [[ "$changed_count" == "0" ]]; then
    die "No changed files between $BASE_REF and HEAD"
    return 1
  fi

  local review_ext_pattern='(\\.go$|go\\.mod$|go\\.sum$|\\.toml$|\\.yaml$|\\.yml$|\\.json$|\\.sh$|Dockerfile$|Makefile$|\\.md$)'
  local high_risk_pattern='^(cmd/raven/|internal/(cli|config|security|pipeline|server|workflows|diff|discovery|output|tokenizer)/|scripts/|\\.github/workflows/|go\\.mod$|go\\.sum$)'

  local file
  while IFS= read -r file; do
    [[ -z "$file" ]] && continue

    if [[ "$file" =~ $review_ext_pattern ]]; then
      echo "$file" >> "$RAW_DIR/review-files.txt"
    fi

    if [[ "$file" =~ $high_risk_pattern ]]; then
      echo "$file" >> "$RAW_DIR/high-risk-files.txt"
      if [[ ! "$file" =~ $review_ext_pattern ]]; then
        echo "$file" >> "$RAW_DIR/review-files.txt"
      fi
    fi
  done < "$RAW_DIR/changed-files.txt"

  if [[ ! -s "$RAW_DIR/review-files.txt" ]]; then
    cp "$RAW_DIR/changed-files.txt" "$RAW_DIR/review-files.txt"
  fi

  sort -u "$RAW_DIR/review-files.txt" -o "$RAW_DIR/review-files.txt"
  if [[ -s "$RAW_DIR/high-risk-files.txt" ]]; then
    sort -u "$RAW_DIR/high-risk-files.txt" -o "$RAW_DIR/high-risk-files.txt"
  fi

  local review_count
  local high_risk_count
  review_count="$(wc -l < "$RAW_DIR/review-files.txt" | tr -d ' ')"
  high_risk_count="$(wc -l < "$RAW_DIR/high-risk-files.txt" | tr -d ' ')"

  local review_files=()
  while IFS= read -r file; do
    [[ -n "$file" ]] && review_files+=("$file")
  done < "$RAW_DIR/review-files.txt"

  if (( ${#review_files[@]} > 0 )); then
    git -C "$PROJECT_ROOT" diff --unified=5 "$BASE_REF"...HEAD -- "${review_files[@]}" > "$RAW_DIR/review.diff" 2>/dev/null || \
      git -C "$PROJECT_ROOT" diff --unified=5 "$BASE_REF" HEAD -- "${review_files[@]}" > "$RAW_DIR/review.diff"
  else
    : > "$RAW_DIR/review.diff"
  fi

  DIFF_LINES="$(wc -l < "$RAW_DIR/review.diff" | tr -d ' ')"
  REVIEW_FILE_COUNT="$review_count"
  HIGH_RISK_COUNT="$high_risk_count"

  build_change_context "$BASE_REF"

  log_success "Branch: $CURRENT_BRANCH -> $BASE_REF"
  log_dim "Changed files: $changed_count"
  log_dim "Review files: $REVIEW_FILE_COUNT"
  log_dim "High-risk files: $HIGH_RISK_COUNT"
  log_dim "Diff lines: $DIFF_LINES"
}

build_full_review_prompt_inline() {
  local agent="$1"
  local review_prompt_content
  review_prompt_content="$(read_context_file "$PROMPTS_DIR/full-review.md")"
  local patterns_content
  patterns_content="$(read_context_file "$RULES_DIR/project-patterns.md")"
  local checklist_content
  checklist_content="$(read_context_file "$RULES_DIR/review-checklist.md")"
  local changed_files
  changed_files="$(cat "$RAW_DIR/review-files.txt")"
  local diff_content
  diff_content="$(cat "$RAW_DIR/review.diff")"
  local change_context
  change_context="$(cat "$RAW_DIR/change-context.txt")"

  cat <<PROMPT
You are running a Raven pull-request review.

=== PROJECT BRIEF ===
$PROJECT_BRIEF

=== REVIEW INSTRUCTIONS ===
$review_prompt_content

=== PROJECT PATTERNS ===
$patterns_content

=== REVIEW CHECKLIST ===
$checklist_content

=== REVIEW TARGET ===
Project: $PROJECT_NAME
Branch: $CURRENT_BRANCH -> $BASE_REF
Agent label: $agent
Review pass: full-review
Changed review files ($REVIEW_FILE_COUNT):
$changed_files

=== CHANGE CONTEXT ===
$change_context

=== UNIFIED DIFF ===
$diff_content

=== OUTPUT CONTRACT (STRICT JSON ONLY) ===
Return ONLY one JSON object, no markdown, no commentary, no code fences.
Required schema:
{
  "schema_version": "1.0",
  "pass": "full-review",
  "agent": "${agent}",
  "verdict": "APPROVE|COMMENT|REQUEST_CHANGES",
  "summary": "string",
  "highlights": ["string"],
  "findings": [
    {
      "severity": "critical|high|medium|low|suggestion",
      "category": "security|config|cobra-cli|pipeline|testing|determinism|performance|docs|other",
      "path": "string",
      "line": 1,
      "title": "string",
      "details": "string",
      "suggested_fix": "string"
    }
  ]
}
PROMPT
}

build_full_review_prompt_fileref() {
  local agent="$1"
  local review_prompt_content
  review_prompt_content="$(read_context_file "$PROMPTS_DIR/full-review.md")"
  local patterns_content
  patterns_content="$(read_context_file "$RULES_DIR/project-patterns.md")"
  local checklist_content
  checklist_content="$(read_context_file "$RULES_DIR/review-checklist.md")"
  local changed_files
  changed_files="$(cat "$RAW_DIR/review-files.txt")"
  local change_context
  change_context="$(cat "$RAW_DIR/change-context.txt")"

  cat <<PROMPT
You are running a Raven pull-request review.

=== PROJECT BRIEF ===
$PROJECT_BRIEF

=== REVIEW INSTRUCTIONS ===
$review_prompt_content

=== PROJECT PATTERNS ===
$patterns_content

=== REVIEW CHECKLIST ===
$checklist_content

=== REVIEW TARGET ===
Project: $PROJECT_NAME
Branch: $CURRENT_BRANCH -> $BASE_REF
Agent label: $agent
Review pass: full-review
Changed review files ($REVIEW_FILE_COUNT):
$changed_files

=== CHANGE CONTEXT ===
$change_context

=== DIFF ACCESS ===
Read these files from the filesystem:
1. Unified diff: $RAW_DIR/review.diff
2. Review files list: $RAW_DIR/review-files.txt
3. High-risk files: $RAW_DIR/high-risk-files.txt
4. Changed files: $RAW_DIR/changed-files.txt
Repository root: $PROJECT_ROOT

=== OUTPUT CONTRACT (STRICT JSON ONLY) ===
Return ONLY one JSON object, no markdown, no commentary, no code fences.
Required schema:
{
  "schema_version": "1.0",
  "pass": "full-review",
  "agent": "${agent}",
  "verdict": "APPROVE|COMMENT|REQUEST_CHANGES",
  "summary": "string",
  "highlights": ["string"],
  "findings": [
    {
      "severity": "critical|high|medium|low|suggestion",
      "category": "security|config|cobra-cli|pipeline|testing|determinism|performance|docs|other",
      "path": "string",
      "line": 1,
      "title": "string",
      "details": "string",
      "suggested_fix": "string"
    }
  ]
}
PROMPT
}

build_security_review_prompt_inline() {
  local agent="$1"
  local security_prompt_content
  security_prompt_content="$(read_context_file "$PROMPTS_DIR/security-review.md")"
  local patterns_content
  patterns_content="$(read_context_file "$RULES_DIR/project-patterns.md")"
  local checklist_content
  checklist_content="$(read_context_file "$RULES_DIR/review-checklist.md")"
  local high_risk_files=""
  if [[ -s "$RAW_DIR/high-risk-files.txt" ]]; then
    high_risk_files="$(cat "$RAW_DIR/high-risk-files.txt")"
  else
    high_risk_files="$(cat "$RAW_DIR/review-files.txt")"
  fi
  local diff_content
  diff_content="$(cat "$RAW_DIR/review.diff")"
  local change_context
  change_context="$(cat "$RAW_DIR/change-context.txt")"

  cat <<PROMPT
You are running a Raven security review.

=== PROJECT BRIEF ===
$PROJECT_BRIEF

=== SECURITY INSTRUCTIONS ===
$security_prompt_content

=== PROJECT PATTERNS ===
$patterns_content

=== REVIEW CHECKLIST ===
$checklist_content

=== REVIEW TARGET ===
Project: $PROJECT_NAME
Branch: $CURRENT_BRANCH -> $BASE_REF
Agent label: $agent
Review pass: security
High-risk focus files ($HIGH_RISK_COUNT):
$high_risk_files

=== CHANGE CONTEXT ===
$change_context

=== UNIFIED DIFF ===
$diff_content

=== OUTPUT CONTRACT (STRICT JSON ONLY) ===
Return ONLY one JSON object, no markdown, no commentary, no code fences.
Required schema:
{
  "schema_version": "1.0",
  "pass": "security",
  "agent": "${agent}",
  "verdict": "SECURE|NEEDS_FIXES",
  "summary": "string",
  "highlights": ["string"],
  "findings": [
    {
      "severity": "critical|high|medium|low",
      "category": "security|supply-chain|config|secrets|input-validation|access-control|other",
      "path": "string",
      "line": 1,
      "title": "string",
      "details": "string",
      "suggested_fix": "string"
    }
  ]
}
PROMPT
}

build_security_review_prompt_fileref() {
  local agent="$1"
  local security_prompt_content
  security_prompt_content="$(read_context_file "$PROMPTS_DIR/security-review.md")"
  local patterns_content
  patterns_content="$(read_context_file "$RULES_DIR/project-patterns.md")"
  local checklist_content
  checklist_content="$(read_context_file "$RULES_DIR/review-checklist.md")"
  local high_risk_files=""
  if [[ -s "$RAW_DIR/high-risk-files.txt" ]]; then
    high_risk_files="$(cat "$RAW_DIR/high-risk-files.txt")"
  else
    high_risk_files="$(cat "$RAW_DIR/review-files.txt")"
  fi
  local change_context
  change_context="$(cat "$RAW_DIR/change-context.txt")"

  cat <<PROMPT
You are running a Raven security review.

=== PROJECT BRIEF ===
$PROJECT_BRIEF

=== SECURITY INSTRUCTIONS ===
$security_prompt_content

=== PROJECT PATTERNS ===
$patterns_content

=== REVIEW CHECKLIST ===
$checklist_content

=== REVIEW TARGET ===
Project: $PROJECT_NAME
Branch: $CURRENT_BRANCH -> $BASE_REF
Agent label: $agent
Review pass: security
High-risk focus files ($HIGH_RISK_COUNT):
$high_risk_files

=== CHANGE CONTEXT ===
$change_context

=== DIFF ACCESS ===
Read these files from the filesystem:
1. Unified diff: $RAW_DIR/review.diff
2. High-risk files list: $RAW_DIR/high-risk-files.txt
3. Review files list: $RAW_DIR/review-files.txt
4. Changed files list: $RAW_DIR/changed-files.txt
Repository root: $PROJECT_ROOT

=== OUTPUT CONTRACT (STRICT JSON ONLY) ===
Return ONLY one JSON object, no markdown, no commentary, no code fences.
Required schema:
{
  "schema_version": "1.0",
  "pass": "security",
  "agent": "${agent}",
  "verdict": "SECURE|NEEDS_FIXES",
  "summary": "string",
  "highlights": ["string"],
  "findings": [
    {
      "severity": "critical|high|medium|low",
      "category": "security|supply-chain|config|secrets|input-validation|access-control|other",
      "path": "string",
      "line": 1,
      "title": "string",
      "details": "string",
      "suggested_fix": "string"
    }
  ]
}
PROMPT
}

build_review_prompt() {
  local agent="$1"
  local pass="$2"
  local mode="inline"
  if (( DIFF_LINES > INLINE_DIFF_THRESHOLD )); then
    mode="fileref"
  fi

  case "$pass:$mode" in
    full-review:inline) build_full_review_prompt_inline "$agent" ;;
    full-review:fileref) build_full_review_prompt_fileref "$agent" ;;
    security:inline) build_security_review_prompt_inline "$agent" ;;
    security:fileref) build_security_review_prompt_fileref "$agent" ;;
    *) return 1 ;;
  esac
}

write_parse_error_payload() {
  local pass="$1"
  local agent="$2"
  local output_file="$3"
  local reason="$4"

  local verdict="COMMENT"
  if [[ "$pass" == "security" ]]; then
    verdict="NEEDS_FIXES"
  fi

  cat > "$output_file" <<JSON
{
  "schema_version": "1.0",
  "pass": "$pass",
  "agent": "$agent",
  "verdict": "$verdict",
  "summary": "$reason",
  "highlights": [],
  "findings": [],
  "parse_error": true
}
JSON
}

extract_json_candidate() {
  local raw_file="$1"
  local extracted_json="$2"

  if command -v node >/dev/null 2>&1 && [[ -f "$JSON_EXTRACT_SCRIPT" ]]; then
    RAW_FILE="$raw_file" EXTRACTED_JSON="$extracted_json" JSON_EXTRACT_SCRIPT="$JSON_EXTRACT_SCRIPT" node <<'NODE'
const fs = require('fs')

const rawPath = process.env.RAW_FILE
const outPath = process.env.EXTRACTED_JSON
const helperPath = process.env.JSON_EXTRACT_SCRIPT

const raw = fs.readFileSync(rawPath, 'utf8')
const { extractJsonCandidate } = require(helperPath)
const candidate = extractJsonCandidate(raw)
if (!candidate) {
  process.exit(2)
}

const parsed = JSON.parse(candidate)
fs.writeFileSync(outPath, JSON.stringify(parsed, null, 2))
NODE
    return $?
  fi

  if jq . "$raw_file" > "$extracted_json" 2>/dev/null; then
    return 0
  fi

  return 1
}

normalize_review_json() {
  local pass="$1"
  local agent="$2"
  local input_json="$3"
  local output_json="$4"

  if ! jq --arg pass "$pass" --arg agent "$agent" '
    def to_array(v):
      if (v | type) == "array" then v else [] end;

    def to_text(v; d):
      if v == null then d else (v | tostring) end;

    def to_line(v):
      if v == null then 0
      elif (v | type) == "number" then (if v < 0 then 0 else (v | floor) end)
      elif (v | type) == "string" then (v | tonumber? // 0)
      else 0
      end;

    def norm_full_verdict(v):
      (to_text(v; "COMMENT") | ascii_upcase) as $x
      | if ($x == "APPROVE" or $x == "COMMENT" or $x == "REQUEST_CHANGES") then $x else "COMMENT" end;

    def norm_sec_verdict(v):
      (to_text(v; "NEEDS_FIXES") | ascii_upcase) as $x
      | if ($x == "SECURE" or $x == "NEEDS_FIXES") then $x else "NEEDS_FIXES" end;

    def norm_full_sev(v):
      (to_text(v; "medium") | ascii_downcase) as $x
      | if ($x == "critical" or $x == "high" or $x == "medium" or $x == "low" or $x == "suggestion") then $x else "medium" end;

    def norm_sec_sev(v):
      (to_text(v; "medium") | ascii_downcase) as $x
      | if ($x == "critical" or $x == "high" or $x == "medium" or $x == "low") then $x else "medium" end;

    def normalize_findings:
      to_array(.findings)
      | map(
          . as $f
          | {
              severity: (if $pass == "security" then norm_sec_sev($f.severity) else norm_full_sev($f.severity) end),
              category: to_text(($f.category // (if $pass == "security" then "security" else "other" end)); (if $pass == "security" then "security" else "other" end)),
              path: to_text(($f.path // $f.file // $f.filename // $f.file_path); ""),
              line: to_line($f.line // $f.lines // $f.line_number // $f.start_line),
              title: to_text($f.title; "Untitled finding"),
              details: to_text(($f.details // $f.description // $f.body); "No details provided."),
              suggested_fix: to_text(($f.suggested_fix // $f.suggestion // $f.fix // $f.recommendation); "")
            }
        );

    {
      schema_version: "1.0",
      pass: $pass,
      agent: $agent,
      verdict: (if $pass == "security" then norm_sec_verdict(.verdict) else norm_full_verdict(.verdict) end),
      summary: to_text(.summary; "No summary provided."),
      highlights: (if $pass == "full-review" then (to_array(.highlights) | map(to_text(.; "")) | map(select(length > 0))) else [] end),
      findings: normalize_findings,
      parse_error: (.parse_error // false)
    }
  ' "$input_json" > "$output_json"; then
    write_parse_error_payload "$pass" "$agent" "$output_json" "Failed to normalize agent output"
    return 1
  fi

  return 0
}

run_agent_command() {
  local agent="$1"
  local prompt="$2"
  local raw_output_file="$3"
  local log_file="$4"
  local model
  model="$(agent_model "$agent")"

  case "$agent" in
    claude)
      claude -p "$prompt" --model "$model" > "$raw_output_file" 2> "$log_file"
      ;;
    codex)
      codex exec "$prompt" -m "$model" -s read-only > "$raw_output_file" 2> "$log_file"
      ;;
    gemini)
      gemini -p "$prompt" -m "$model" --approval-mode plan > "$raw_output_file" 2> "$log_file"
      ;;
    *)
      return 1
      ;;
  esac
}

run_single_review() {
  local agent="$1"
  local pass="$2"
  local dry_run="${3:-false}"

  local raw_output_file="$RAW_DIR/${pass}_${agent}.raw.txt"
  local log_file="$RAW_DIR/${pass}_${agent}.log"
  local extracted_json="$RAW_DIR/${pass}_${agent}.extracted.json"
  local normalized_json="$RAW_DIR/${pass}_${agent}.json"

  if [[ "$dry_run" == "true" ]]; then
    log_dim "[DRY-RUN] $agent -> $pass"
    log_dim "[DRY-RUN] command: $(agent_bin "$agent") (model: $(agent_model "$agent"))"
    return 0
  fi

  if ! command -v "$(agent_bin "$agent")" >/dev/null 2>&1; then
    write_parse_error_payload "$pass" "$agent" "$normalized_json" "Agent binary missing: $(agent_bin "$agent")"
    log_warn "$agent -> $pass skipped (binary missing)"
    return 0
  fi

  log_info "$agent -> $pass started"

  local prompt
  prompt="$(build_review_prompt "$agent" "$pass")"

  local start_time
  local end_time
  start_time="$(date +%s)"

  local exit_code=0
  if run_agent_command "$agent" "$prompt" "$raw_output_file" "$log_file"; then
    exit_code=0
  else
    exit_code=$?
  fi

  end_time="$(date +%s)"
  local duration=$((end_time - start_time))

  if [[ $exit_code -ne 0 || ! -s "$raw_output_file" ]]; then
    write_parse_error_payload "$pass" "$agent" "$normalized_json" "Agent execution failed (exit=$exit_code). Check ${pass}_${agent}.log"
    log_warn "$agent -> $pass failed (exit=$exit_code, ${duration}s)"
    return 0
  fi

  if extract_json_candidate "$raw_output_file" "$extracted_json"; then
    normalize_review_json "$pass" "$agent" "$extracted_json" "$normalized_json" >/dev/null || true
  else
    write_parse_error_payload "$pass" "$agent" "$normalized_json" "Could not extract JSON payload from agent output"
  fi

  local finding_count
  finding_count="$(jq '.findings | length' "$normalized_json" 2>/dev/null || echo "0")"
  log_success "$agent -> $pass completed (${duration}s, findings=$finding_count)"

  return 0
}

run_consolidation() {
  local dry_run="${1:-false}"
  local consolidated_file="$RUN_DIR/consolidated.json"

  if [[ "$dry_run" == "true" ]]; then
    log_dim "[DRY-RUN] consolidation -> $consolidated_file"
    return 0
  fi

  local review_json_files=()
  local candidate
  for candidate in "$RAW_DIR"/full-review_*.json "$RAW_DIR"/security_*.json; do
    [[ -f "$candidate" ]] && review_json_files+=("$candidate")
  done

  if (( ${#review_json_files[@]} == 0 )); then
    die "No normalized review JSON files found for consolidation"
    return 1
  fi

  jq -s '
    def rank($severity):
      if $severity == "critical" then 5
      elif $severity == "high" then 4
      elif $severity == "medium" then 3
      elif $severity == "low" then 2
      elif $severity == "suggestion" then 1
      else 0
      end;

    def finding_key($f):
      (($f.path // "") + "|" + (($f.line // 0 | tostring)) + "|" + (($f.title // "" | ascii_downcase)) + "|" + (($f.pass // "" | ascii_downcase)));

    def normalize_flat($r):
      [($r.findings // [])[] | {
        pass: ($r.pass // "full-review"),
        agent: ($r.agent // "unknown"),
        severity: (.severity // "medium"),
        category: (.category // (if ($r.pass // "") == "security" then "security" else "other" end)),
        path: (.path // ""),
        line: (.line // 0),
        title: (.title // "Untitled finding"),
        details: (.details // "No details provided."),
        suggested_fix: (.suggested_fix // "")
      }];

    . as $runs
    | ([$runs[] | normalize_flat(.)[]]) as $flat
    | (reduce $flat[] as $item ({};
        (finding_key($item)) as $key
        | if (.[$key] | type) == "object" then
            .[$key] = (
              .[$key]
              | .source_agents = ((.source_agents + [$item.agent]) | unique | sort)
              | .agent_count = (.source_agents | length)
              | if rank($item.severity) > rank(.severity) then .severity = $item.severity else . end
              | if ($item.details | tostring | length) > (.details | tostring | length) then .details = $item.details else . end
              | if ($item.suggested_fix | tostring | length) > (.suggested_fix | tostring | length) then .suggested_fix = $item.suggested_fix else . end
            )
          else
            . + {
              ($key): {
                pass: $item.pass,
                severity: $item.severity,
                category: $item.category,
                path: $item.path,
                line: $item.line,
                title: $item.title,
                details: $item.details,
                suggested_fix: $item.suggested_fix,
                source_agents: [$item.agent],
                agent_count: 1
              }
            }
          end
      )) as $dedup
    | ($dedup | to_entries | map(.value)) as $unique
    | ($unique | sort_by(-rank(.severity), .path, .line, .title)) as $sorted
    | ($flat | length) as $total_raw
    | ($sorted | map(select(.severity == "critical")) | length) as $critical_count
    | ($sorted | map(select(.severity == "high")) | length) as $high_count
    | ($runs | map(select((.parse_error // false) == true)) | length) as $parse_error_runs
    | ($parse_error_runs > 0) as $has_parse_error
    | {
        schema_version: "1.0",
        generated_at: (now | strftime("%Y-%m-%dT%H:%M:%SZ")),
        summary: (if ($sorted | length) == 0 then "No actionable findings across all executed agents and passes." else "Consolidated findings from Raven multi-agent review." end),
        verdict: (
          if $critical_count > 0 or $high_count >= 3 then "REQUEST_CHANGES"
          elif $has_parse_error then "COMMENT"
          elif ($sorted | length) == 0 then "APPROVE"
          elif ($sorted | all(.severity == "suggestion")) then "APPROVE"
          else "COMMENT"
          end
        ),
        stats: {
          total_raw_findings: $total_raw,
          unique_findings: ($sorted | length),
          duplicates_removed: ($total_raw - ($sorted | length)),
          parse_error_runs: $parse_error_runs
        },
        findings: $sorted
      }
  ' "${review_json_files[@]}" > "$consolidated_file"

  local unique_count
  unique_count="$(jq '.stats.unique_findings // 0' "$consolidated_file" 2>/dev/null || echo "0")"
  log_success "Consolidation complete (unique findings=$unique_count)"
}

generate_review_report() {
  local report_file="$RUN_DIR/review-report.md"
  local consolidated_file="$RUN_DIR/consolidated.json"

  if [[ ! -f "$consolidated_file" ]]; then
    die "Cannot generate report: missing $consolidated_file"
    return 1
  fi

  local verdict
  verdict="$(jq -r '.verdict // "COMMENT"' "$consolidated_file" 2>/dev/null || echo "COMMENT")"
  local summary
  summary="$(jq -r '.summary // ""' "$consolidated_file" 2>/dev/null)"
  local total_raw
  total_raw="$(jq '.stats.total_raw_findings // 0' "$consolidated_file" 2>/dev/null || echo "0")"
  local unique
  unique="$(jq '.stats.unique_findings // 0' "$consolidated_file" 2>/dev/null || echo "0")"
  local deduped
  deduped="$(jq '.stats.duplicates_removed // 0' "$consolidated_file" 2>/dev/null || echo "0")"

  {
    echo "# Raven Review Report"
    echo
    echo "- Generated: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "- Branch: ${CURRENT_BRANCH:-unknown} -> ${BASE_REF:-unknown}"
    echo "- Verdict: $verdict"
    echo
    echo "## Scope"
    echo
    echo "- Review files: ${REVIEW_FILE_COUNT:-0}"
    echo "- High-risk files: ${HIGH_RISK_COUNT:-0}"
    echo "- Diff lines: ${DIFF_LINES:-0}"
    echo
    echo "## Consolidation"
    echo
    echo "- Raw findings: $total_raw"
    echo "- Unique findings: $unique"
    echo "- Duplicates removed: $deduped"
    echo
    echo "## Summary"
    echo
    echo "$summary"
    echo
    echo "## Findings"
    echo

    jq -r '
      if (.findings | length) == 0 then
        "No findings."
      else
        .findings[] |
        "- [\(.severity)] \(.title) :: \(.path)" +
        (if (.line // 0) > 0 then ":\(.line)" else "" end) +
        " (agents: " + ((.source_agents // ["unknown"]) | join(",")) + ")"
      end
    ' "$consolidated_file"
  } > "$report_file"

  log_success "Report written -> $report_file"
}

print_run_paths() {
  log_info "Run directory: $RUN_DIR"
  log_info "Raw outputs: $RAW_DIR"
  log_info "Consolidated: $RUN_DIR/consolidated.json"
  log_info "Report: $RUN_DIR/review-report.md"
  if [[ -e "$LATEST_LINK" ]]; then
    log_info "Latest link: $LATEST_LINK"
  fi
}
