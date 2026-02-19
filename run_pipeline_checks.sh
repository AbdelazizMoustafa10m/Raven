#!/usr/bin/env bash
# run_pipeline_checks.sh - Local CI pipeline check runner for Raven
# Mirrors all jobs in .github/workflows/ci.yml so you can catch failures before pushing.
#
# Usage:
#   ./run_pipeline_checks.sh              # fail-fast mode (stops on first failure)
#   ./run_pipeline_checks.sh --all        # run all checks even if some fail
#   ./run_pipeline_checks.sh --with-build # include cross-platform build matrix
#
# Requires: go, golangci-lint
# Optional: gum (charmbracelet/gum) for polished UI — degrades gracefully without it.

set -euo pipefail

# ── Options ───────────────────────────────────────────────────────────────────
FAIL_FAST=true
WITH_BUILD=false
for arg in "$@"; do
  case "$arg" in
    --all)        FAIL_FAST=false ;;
    --with-build) WITH_BUILD=true ;;
    -h|--help)
      echo "Usage: $0 [--all] [--with-build]"
      echo "  --all         Run all checks even if some fail"
      echo "  --with-build  Include cross-platform build matrix"
      exit 0
      ;;
  esac
done

# ── Capability detection ─────────────────────────────────────────────────────
HAS_GUM=false
command -v gum &>/dev/null && HAS_GUM=true

# golangci-lint is often in ~/go/bin which may not be on PATH
HAS_LINT=false
GOLANGCI_LINT="golangci-lint"
if command -v golangci-lint &>/dev/null; then
  HAS_LINT=true
elif [[ -x "${GOPATH:-$HOME/go}/bin/golangci-lint" ]]; then
  HAS_LINT=true
  GOLANGCI_LINT="${GOPATH:-$HOME/go}/bin/golangci-lint"
fi

# ── Color palette ────────────────────────────────────────────────────────────
# Warm gold accent (matching phase-pipeline.sh), soft greens/reds for verdicts.
C_GOLD=179        # accent / headings / borders (warm yellow-gold)
C_WHITE=255       # bold text
C_GREEN=114       # pass
C_RED=204         # fail
C_YELLOW=222      # warnings / skip
C_MAGENTA=212     # spinner / running indicator
C_DIM=245         # muted text
C_TEXT=250         # normal text

# ── ANSI fallback colors (when gum is not available) ─────────────────────────
if [[ -t 1 ]]; then
  _BOLD=$'\033[1m'      _DIM=$'\033[2m'       _RESET=$'\033[0m'
  _RED=$'\033[0;31m'    _GREEN=$'\033[0;32m'   _YELLOW=$'\033[1;33m'
  _CYAN=$'\033[0;36m'   _WHITE=$'\033[1;37m'   _GRAY=$'\033[0;90m'
  _MAGENTA=$'\033[0;35m'
else
  _BOLD='' _DIM='' _RESET='' _RED='' _GREEN='' _YELLOW=''
  _CYAN='' _WHITE='' _GRAY='' _MAGENTA=''
fi

# ── Unicode icons (matching phase-pipeline.sh style) ─────────────────────────
_SYM_CHECK="${_GREEN}✓${_RESET}"
_SYM_CROSS="${_RED}✗${_RESET}"
_SYM_WARN="${_YELLOW}⚠${_RESET}"
_SYM_PENDING="${_DIM}○${_RESET}"
_SYM_RUNNING="${_MAGENTA}●${_RESET}"
_SYM_DONE="${_GREEN}●${_RESET}"

# ── Temp directory for captured output ────────────────────────────────────────
TMPDIR_CHECKS=$(mktemp -d)
trap 'rm -rf "$TMPDIR_CHECKS"' EXIT

# ── State ─────────────────────────────────────────────────────────────────────
RESULTS=()        # "PASS|label|duration" or "FAIL|label|duration" or "SKIP|label|0"
STATUSES=()       # parallel array: "pass" | "fail" | "skip" (for progress tracker)
CHECK_LABELS=()   # parallel array: short labels for progress tracker
TOTAL=0
PASSED=0
FAILED=0
SKIPPED=0
PIPELINE_START=0

# ── Progress Tracker ─────────────────────────────────────────────────────────

draw_progress() {
  # draw_progress <current_idx> <total> — renders ●──●──◉──○──○──○
  local current_idx=$1
  local total=$2

  printf '  '
  for ((i=0; i<total; i++)); do
    if ((i < current_idx)); then
      # Completed: use green or red based on result
      local st="${STATUSES[$i]:-pass}"
      case "$st" in
        pass) printf '%b' "${_GREEN}●${_RESET}" ;;
        fail) printf '%b' "${_RED}●${_RESET}" ;;
        skip) printf '%b' "${_YELLOW}○${_RESET}" ;;
      esac
    elif ((i == current_idx)); then
      printf '%b' "${_MAGENTA}◉${_RESET}"
    else
      printf '%b' "${_DIM}○${_RESET}"
    fi
    if ((i < total - 1)); then
      if ((i < current_idx)); then
        printf '%b' "${_GREEN}──${_RESET}"
      elif ((i == current_idx)); then
        printf '%b' "${_DIM}──${_RESET}"
      else
        printf '%b' "${_DIM}──${_RESET}"
      fi
    fi
  done
  echo ""
}

# ── Helpers ───────────────────────────────────────────────────────────────────

header() {
  echo ""
  if $HAS_GUM; then
    local branch
    branch=$(git branch --show-current 2>/dev/null || echo "unknown")

    gum style \
      --bold --foreground "$C_WHITE" \
      --border rounded \
      --border-foreground "$C_GOLD" \
      --padding "1 4" --margin "0 2" \
      "Raven Pipeline Checks" "" \
      "  branch: $branch  |  $(date '+%H:%M:%S')"
  else
    echo "  ${_BOLD}${_YELLOW}╭─────────────────────────────────────╮${_RESET}"
    echo "  ${_YELLOW}│${_RESET}  ${_BOLD}${_WHITE}Raven Pipeline Checks${_RESET}              ${_YELLOW}│${_RESET}"
    echo "  ${_YELLOW}│${_RESET}  ${_DIM}branch: $(git branch --show-current 2>/dev/null || echo unknown)${_RESET}"
    echo "  ${_BOLD}${_YELLOW}╰─────────────────────────────────────╯${_RESET}"
  fi
  echo ""
}

log_step() {
  # log_step <icon> <message>
  local icon="$1" msg="$2"
  local ts
  ts="$(date '+%H:%M:%S')"
  printf '  %b%s%b  %b %s\n' "$_DIM" "$ts" "$_RESET" "$icon" "$msg"
}

show_error_output() {
  local outfile="$1"
  if [[ -s "$outfile" ]]; then
    echo ""
    if $HAS_GUM; then
      gum style --foreground "$C_DIM" --faint "     ── error output ──────────────────────"
      tail -30 "$outfile" | while IFS= read -r line; do
        gum style --foreground "$C_RED" --faint "     $line"
      done
      gum style --foreground "$C_DIM" --faint "     ──────────────────────────────────────"
    else
      printf '     %b── error output ──────────────────────%b\n' "$_DIM" "$_RESET"
      tail -30 "$outfile" | sed 's/^/     /'
      printf '     %b──────────────────────────────────────%b\n' "$_DIM" "$_RESET"
    fi
  fi
}

# ── Check runner ──────────────────────────────────────────────────────────────

run_check() {
  # run_check <label> <spinner_title> <command...>
  local label="$1" spinner_title="$2"; shift 2
  local outfile="$TMPDIR_CHECKS/check_${TOTAL}.log"
  local rc=0
  local start_time end_time duration

  (( TOTAL++ )) || true

  # Draw progress tracker
  draw_progress "$((TOTAL - 1))" "$CHECK_COUNT"

  start_time=$(date +%s)

  # Show running state
  log_step "$_SYM_RUNNING" "${_BOLD}${label}${_RESET}"

  # Run the command in background with output captured to file.
  # gum spin is used purely as a visual spinner that polls the PID —
  # this avoids I/O deadlocks from wrapping the command inside gum spin's subprocess.
  "$@" > "$outfile" 2>&1 &
  local cmd_pid=$!

  if $HAS_GUM; then
    gum spin \
      --spinner dot \
      --spinner.foreground "$C_MAGENTA" \
      --title "  $spinner_title" \
      --title.foreground "$C_DIM" \
      -- bash -c 'while kill -0 "$1" 2>/dev/null; do sleep 0.1; done' _ "$cmd_pid" || true
  fi

  wait "$cmd_pid" || rc=$?

  end_time=$(date +%s)
  duration=$(( end_time - start_time ))

  if [[ $rc -eq 0 ]]; then
    log_step "$_SYM_CHECK" "${label}  ${_DIM}${duration}s${_RESET}"
    RESULTS+=("PASS|$label|$duration")
    STATUSES+=("pass")
    (( PASSED++ )) || true
  else
    log_step "$_SYM_CROSS" "${_RED}${label}${_RESET}  ${_DIM}${duration}s${_RESET}"
    RESULTS+=("FAIL|$label|$duration")
    STATUSES+=("fail")
    (( FAILED++ )) || true
    show_error_output "$outfile"
  fi

  echo ""
  return $rc
}

run_skip() {
  # run_skip <label> <reason> <hint>
  local label="$1" reason="$2" hint="$3"
  (( TOTAL++ )) || true

  draw_progress "$((TOTAL - 1))" "$CHECK_COUNT"
  log_step "$_SYM_WARN" "${_YELLOW}${label}${_RESET}  ${_DIM}${reason}${_RESET}"
  if [[ -n "$hint" ]]; then
    printf '              %b%s%b\n' "$_DIM" "$hint" "$_RESET"
  fi

  RESULTS+=("SKIP|$label|0")
  STATUSES+=("skip")
  (( SKIPPED++ )) || true
  echo ""
}

# ── Define checks (mirroring CI jobs) ─────────────────────────────────────────

checks=()
CHECK_LABELS_INIT=()

# 1. Module hygiene (CI: mod-tidy job)
checks+=("mod_hygiene")
CHECK_LABELS_INIT+=("Tidy")
mod_hygiene() {
  run_check \
    "Module Hygiene  (go mod tidy)" \
    "Checking module hygiene..." \
    bash -c 'go mod tidy && git diff --exit-code go.mod go.sum'
}

# 2. Formatting (gofmt)
checks+=("formatting")
CHECK_LABELS_INIT+=("Fmt")
formatting() {
  run_check \
    "Formatting      (gofmt)" \
    "Checking formatting..." \
    bash -c '
      BAD=$(gofmt -l . 2>&1)
      if [ -n "$BAD" ]; then
        echo "Files need formatting:"
        echo "$BAD"
        echo ""
        echo "Run: gofmt -s -w ."
        exit 1
      fi
    '
}

# 3. Vet (CI: test job step)
checks+=("vetting")
CHECK_LABELS_INIT+=("Vet")
vetting() {
  run_check \
    "Static Analysis (go vet)" \
    "Running go vet..." \
    go vet ./...
}

# 4. Lint (CI: lint job)
checks+=("linting")
CHECK_LABELS_INIT+=("Lint")
linting() {
  if ! $HAS_LINT; then
    run_skip \
      "Lint (golangci-lint)" \
      "not installed" \
      "Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
    return 0
  fi

  run_check \
    "Lint            (golangci-lint)" \
    "Running linter..." \
    "$GOLANGCI_LINT" run --timeout=5m ./...
}

# 5. Tests (CI: test job)
checks+=("testing")
CHECK_LABELS_INIT+=("Test")
testing() {
  run_check \
    "Tests           (go test -race)" \
    "Running tests..." \
    go test -race -count=1 ./...
}

# 6. Build (CI: build job)
checks+=("building")
CHECK_LABELS_INIT+=("Build")
building() {
  run_check \
    "Build           (go build)" \
    "Building binary..." \
    bash -c 'CGO_ENABLED=0 go build -ldflags="-s -w" -o /dev/null ./cmd/raven'
}

# Optionally add cross-platform build matrix
if $WITH_BUILD; then
  checks+=("cross_build")
  CHECK_LABELS_INIT+=("Cross")
  cross_build() {
    local platforms=("darwin/amd64" "darwin/arm64" "linux/amd64" "linux/arm64" "windows/amd64")
    for plat in "${platforms[@]}"; do
      local os="${plat%%/*}"
      local arch="${plat##*/}"
      run_check \
        "Build           ($os/$arch)" \
        "Cross-compiling $os/$arch..." \
        bash -c "CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -ldflags='-s -w' -o /dev/null ./cmd/raven"
    done
  }
fi

CHECK_COUNT=${#checks[@]}
CHECK_LABELS=("${CHECK_LABELS_INIT[@]}")

# ── Summary ───────────────────────────────────────────────────────────────────

summary() {
  local total_elapsed=$(( $(date +%s) - PIPELINE_START ))
  echo ""

  # Final progress bar (all done)
  draw_progress "$TOTAL" "$CHECK_COUNT"
  echo ""

  if $HAS_GUM; then
    # Build result lines
    local lines=""
    for entry in "${RESULTS[@]}"; do
      local verdict label duration
      verdict="${entry%%|*}"
      label="${entry#*|}"
      duration="${label##*|}"
      label="${label%|*}"

      case "$verdict" in
        PASS) lines+="     ✓  ${label}  ${duration}s"$'\n' ;;
        FAIL) lines+="     ✗  ${label}  ${duration}s"$'\n' ;;
        SKIP) lines+="     ⚠  ${label}"$'\n' ;;
      esac
    done

    local title_color="$C_GREEN"
    local title_text="All $PASSED checks passed — ready to push"
    local border_style="rounded"
    if [[ $FAILED -gt 0 ]]; then
      title_color="$C_RED"
      title_text="$FAILED of $(( PASSED + FAILED )) checks failed — fix before pushing"
      border_style="double"
    fi

    gum style \
      --bold --foreground "$title_color" \
      --border "$border_style" \
      --border-foreground "$title_color" \
      --padding "1 4" --margin "0 2" \
      "$title_text" "" \
      "$lines" \
      "  Total: ${total_elapsed}s"
  else
    if [[ $FAILED -eq 0 ]]; then
      printf '  %b╔══════════════════════════════════════════════╗%b\n' "$_GREEN" "$_RESET"
      printf '  %b║%b  %b%bAll %d checks passed — ready to push%b       %b║%b\n' \
        "$_GREEN" "$_RESET" "$_BOLD" "$_GREEN" "$PASSED" "$_RESET" "$_GREEN" "$_RESET"
      printf '  %b╚══════════════════════════════════════════════╝%b\n' "$_GREEN" "$_RESET"
    else
      printf '  %b╔══════════════════════════════════════════════╗%b\n' "$_RED" "$_RESET"
      printf '  %b║%b  %b%b%d of %d checks failed — fix before pushing%b  %b║%b\n' \
        "$_RED" "$_RESET" "$_BOLD" "$_RED" "$FAILED" "$(( PASSED + FAILED ))" "$_RESET" "$_RED" "$_RESET"
      printf '  %b╚══════════════════════════════════════════════╝%b\n' "$_RED" "$_RESET"
    fi
    echo ""
    for entry in "${RESULTS[@]}"; do
      local verdict="${entry%%|*}"
      local rest="${entry#*|}"
      local label="${rest%|*}"
      local duration="${rest##*|}"
      case "$verdict" in
        PASS) printf '     %b✓%b  %s  %b%ss%b\n' "$_GREEN" "$_RESET" "$label" "$_DIM" "$duration" "$_RESET" ;;
        FAIL) printf '     %b✗%b  %s  %b%ss%b\n' "$_RED" "$_RESET" "$label" "$_DIM" "$duration" "$_RESET" ;;
        SKIP) printf '     %b⚠%b  %s\n' "$_YELLOW" "$_RESET" "$label" ;;
      esac
    done
    echo ""
    printf '  %bTotal: %ss%b\n' "$_DIM" "$total_elapsed" "$_RESET"
  fi
  echo ""
}

# ── Run ───────────────────────────────────────────────────────────────────────

PIPELINE_START=$(date +%s)
header

for check_fn in "${checks[@]}"; do
  if ! "$check_fn"; then
    if $FAIL_FAST; then
      echo ""
      log_step "$_SYM_CROSS" "${_RED}Stopping early (fail-fast). Use ${_BOLD}--all${_RESET}${_RED} to run remaining checks.${_RESET}"
      summary
      exit 1
    fi
  fi
done

summary
[[ $FAILED -eq 0 ]] || exit 1
