#!/usr/bin/env bash

set -eo pipefail

if ((BASH_VERSINFO[0] < 4)); then
    printf 'ERROR: Bash 4+ is required (found %s).\n' "$BASH_VERSION" >&2
    printf '  macOS fix: brew install bash\n' >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
# shellcheck source-path=SCRIPTDIR
source "$SCRIPT_DIR/phases-lib.sh"

# ── UI: Detection & Colors ──────────────────────────────────────────

HAS_GUM=false
if command -v gum >/dev/null 2>&1; then
    HAS_GUM=true
fi

if [[ -t 1 ]]; then
    _BOLD=$'\033[1m'      _DIM=$'\033[2m'       _RESET=$'\033[0m'
    _ITALIC=$'\033[3m'    _UNDERLINE=$'\033[4m'  _REVERSE=$'\033[7m'
    _RED=$'\033[0;31m'    _GREEN=$'\033[0;32m'   _YELLOW=$'\033[1;33m'
    _BLUE=$'\033[0;34m'   _MAGENTA=$'\033[0;35m' _CYAN=$'\033[0;36m'
    _WHITE=$'\033[1;37m'  _GRAY=$'\033[0;90m'
    _BG_RED=$'\033[41m'   _BG_GREEN=$'\033[42m'  _BG_BLUE=$'\033[44m'
    _BG_MAGENTA=$'\033[45m' _BG_CYAN=$'\033[46m'
else
    _BOLD='' _DIM='' _RESET='' _ITALIC='' _UNDERLINE='' _REVERSE=''
    _RED='' _GREEN='' _YELLOW='' _BLUE='' _MAGENTA='' _CYAN=''
    _WHITE='' _GRAY='' _BG_RED='' _BG_GREEN='' _BG_BLUE=''
    _BG_MAGENTA='' _BG_CYAN=''
fi

# Unicode status indicators
_SYM_CHECK="${_GREEN}✓${_RESET}"
_SYM_CROSS="${_RED}✗${_RESET}"
_SYM_ARROW="${_CYAN}▸${_RESET}"
_SYM_PENDING="${_DIM}○${_RESET}"
_SYM_RUNNING="${_MAGENTA}●${_RESET}"
_SYM_DONE="${_GREEN}●${_RESET}"
_SYM_WARN="${_YELLOW}⚠${_RESET}"
_SPINNER_CHARS="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

# ── Phase Configuration ─────────────────────────────────────────────

DEFAULT_IMPL_AGENT="codex"
DEFAULT_IMPL_MODEL=""  # Empty means use agent's default; preset: "opus" or "sonnet" for claude
DEFAULT_REVIEW_MODE="all"
DEFAULT_REVIEW_AGENT="codex"
DEFAULT_REVIEW_CONCURRENCY=4
DEFAULT_MAX_REVIEW_CYCLES=2
DEFAULT_FIX_AGENT="codex"
DEFAULT_BASE_BRANCH="main"

# Model preset mappings (agent -> preset -> full model ID)
declare -A CLAUDE_MODEL_PRESETS=(
    ["opus"]="claude-opus-4-6"
    ["sonnet"]="claude-sonnet-4-6"
)
declare -A CODEX_MODEL_PRESETS=(
    ["default"]="gpt-5.3-codex"
    ["o3"]="o3"
)

PHASE_ID=""
# shellcheck disable=SC2034  # used for debugging/metadata
PHASE_MODE="single"   # single | all | from
FROM_PHASE=""
PHASES_TO_RUN=()
CHAIN_BASE=""          # Tracks previous phase branch for multi-phase chaining
IMPL_AGENT="$DEFAULT_IMPL_AGENT"
IMPL_MODEL="$DEFAULT_IMPL_MODEL"  # Full model ID or empty for agent default
IMPL_MODEL_PRESET=""               # User-selected preset: opus, sonnet, o3, etc.
REVIEW_MODE="$DEFAULT_REVIEW_MODE"
REVIEW_AGENT="$DEFAULT_REVIEW_AGENT"
REVIEW_CONCURRENCY="$DEFAULT_REVIEW_CONCURRENCY"
MAX_REVIEW_CYCLES="$DEFAULT_MAX_REVIEW_CYCLES"
FIX_AGENT="$DEFAULT_FIX_AGENT"
BASE_BRANCH="$DEFAULT_BASE_BRANCH"
SYNC_BASE="false"
SKIP_IMPLEMENT="false"
SKIP_REVIEW="false"
SKIP_FIX="false"
SKIP_PR="false"
DRY_RUN="false"
INTERACTIVE="false"

EXPECTED_BRANCH=""
RUN_ID=""
RUN_DIR=""
REPORT_DIR=""
PIPELINE_LOG=""
METADATA_FILE=""

IMPLEMENT_STATUS="not-run"
IMPLEMENT_REASON=""
REVIEW_VERDICT="NOT_RUN"
REVIEW_CYCLES=0
FIX_CYCLES=0
PR_STATUS="not-run"

# ── Usage ────────────────────────────────────────────────────────────

usage() {
    if [[ "$HAS_GUM" == "true" ]]; then
        gum style --bold --foreground 255 --border double --border-foreground 179 \
            --padding "0 2" --margin "1 0" \
            "Raven Phase Pipeline Orchestrator"
    else
        echo ""
        printf '%b%b  Raven Phase Pipeline Orchestrator%b\n' "$_BOLD" "$_YELLOW" "$_RESET"
    fi
    cat <<USAGE

Usage:
  ./scripts/phase-pipeline.sh --phase <id> [options]
  ./scripts/phase-pipeline.sh --phase all [options]
  ./scripts/phase-pipeline.sh --from-phase <id> [options]
  ./scripts/phase-pipeline.sh --interactive

Required (one of):
  --phase <id>                 Single phase ($FIRST_PHASE_ID-$LAST_PHASE_ID)
  --phase all                  Run all phases sequentially ($FIRST_PHASE_ID → $LAST_PHASE_ID)
  --from-phase <id>            Start from this phase, run sequentially through phase $LAST_PHASE_ID
                               If omitted in an interactive terminal, a wizard is shown.

Options:
  --interactive                Force interactive wizard prompts
  --impl-agent <claude|codex>
  --impl-model <preset|model-id>
                               Model for implementation agent:
                               - claude: opus, sonnet (or full model ID)
                               - codex: default, o3 (or full model ID)
  --review <none|agent|all>
  --review-agent <claude|codex|gemini>
  --review-concurrency <n>
  --max-review-cycles <n>
  --fix-agent <claude|codex|gemini>
  --base <branch>              Base branch (default: main)
  --sync-base                  Fetch + fast-forward base from origin before bootstrap
  --skip-implement             Skip implementation phase
  --skip-review                Skip review phase
  --skip-fix                   Skip review fix cycles
  --skip-pr                    Skip PR creation
  --dry-run                    Print planned commands without executing
  -h, --help                   Show help

Examples:
  ./scripts/phase-pipeline.sh --phase 1                  # Run phase 1 only
  ./scripts/phase-pipeline.sh --phase all                # Run all phases (1 → 10)
  ./scripts/phase-pipeline.sh --from-phase 4             # Run phases 4 → 5 → ... → 10
  ./scripts/phase-pipeline.sh --phase all --skip-pr      # All phases, no PRs
  ./scripts/phase-pipeline.sh --from-phase 2 --dry-run   # Preview from phase 2
USAGE
}

# ── Logging & Errors ─────────────────────────────────────────────────

_log_raw() {
    local msg="$1"
    local ts
    ts="$(date '+%Y-%m-%d %H:%M:%S')"
    if [[ -n "$PIPELINE_LOG" ]]; then
        printf '[%s] %s\n' "$ts" "$msg" >> "$PIPELINE_LOG"
    fi
}

log() {
    local msg="$1"
    local ts
    ts="$(date '+%H:%M:%S')"
    _log_raw "$msg"
    printf '  %b%s%b  %s\n' "$_DIM" "$ts" "$_RESET" "$msg"
}

log_step() {
    local icon="$1"
    local msg="$2"
    local ts
    ts="$(date '+%H:%M:%S')"
    _log_raw "$msg"
    printf '  %b%s%b  %b %s\n' "$_DIM" "$ts" "$_RESET" "$icon" "$msg"
}

log_header() {
    local msg="$1"
    _log_raw "=== $msg ==="
    echo ""
    if [[ "$HAS_GUM" == "true" ]]; then
        gum style --bold --foreground 255 --border rounded --border-foreground 179 \
            --padding "0 2" --margin "0 2" "$msg"
    else
        local len=${#msg}
        local cols
        cols="${COLUMNS:-$(tput cols 2>/dev/null || echo 80)}"
        local max_border=$(( cols - 6 ))  # leave room for "  ╭" prefix + "╮"
        (( max_border < 20 )) && max_border=20
        local border_len=$(( len + 4 ))
        (( border_len > max_border )) && border_len=$max_border
        local border
        border="$(printf '─%.0s' $(seq 1 "$border_len"))"
        printf '  %b╭%s╮%b\n' "$_YELLOW" "$border" "$_RESET"
        printf '  %b│%b  %b%s%b  %b│%b\n' "$_YELLOW" "$_RESET" "$_BOLD$_WHITE" "$msg" "$_RESET" "$_YELLOW" "$_RESET"
        printf '  %b╰%s╯%b\n' "$_YELLOW" "$border" "$_RESET"
    fi
    echo ""
}

die() {
    _log_raw "ERROR: $1"
    local cols
    cols="${COLUMNS:-$(tput cols 2>/dev/null || echo 80)}"
    local max_width=$(( cols - 6 ))  # leave margin for border + padding
    (( max_width < 40 )) && max_width=40
    if [[ "$HAS_GUM" == "true" ]]; then
        gum style --foreground 196 --bold --border rounded --border-foreground 196 \
            --padding "0 2" --margin "0 2" --width "$max_width" "ERROR: $1"
    else
        echo ""
        # Wrap long messages using fold
        printf '%s' "$1" | fold -s -w "$max_width" | while IFS= read -r line; do
            printf '  %b%b ERROR %b %b%s%b\n' "$_BG_RED" "$_WHITE" "$_RESET" "$_RED" "$line" "$_RESET"
        done
    fi
    exit 1
}

# ── Pipeline Stage Display ───────────────────────────────────────────

_stage_status() {
    local label="$1"
    local status="$2"  # pending | running | done | skipped | failed | warning

    local icon padded
    case "$status" in
        pending)  icon="$_SYM_PENDING";  padded="${_DIM}${label}${_RESET}" ;;
        running)  icon="$_SYM_RUNNING";  padded="${_BOLD}${label}${_RESET}" ;;
        done)     icon="$_SYM_CHECK";    padded="${label}" ;;
        skipped)  icon="${_DIM}⊘${_RESET}"; padded="${_DIM}${label}${_RESET}" ;;
        failed)   icon="$_SYM_CROSS";    padded="${_RED}${label}${_RESET}" ;;
        warning)  icon="$_SYM_WARN";     padded="${_YELLOW}${label}${_RESET}" ;;
        *)        icon="$_SYM_PENDING";  padded="${label}" ;;
    esac

    printf '     %b  %b\n' "$icon" "$padded"
}

_draw_phase_progress() {
    local current_idx=$1
    local total=$2

    printf '  '
    for ((i=0; i<total; i++)); do
        if ((i < current_idx)); then
            printf '%b' "${_GREEN}●${_RESET}"
        elif ((i == current_idx)); then
            printf '%b' "${_MAGENTA}◉${_RESET}"
        else
            printf '%b' "${_DIM}○${_RESET}"
        fi
        if ((i < total - 1)); then
            if ((i < current_idx)); then
                printf '%b' "${_GREEN}──${_RESET}"
            else
                printf '%b' "${_DIM}──${_RESET}"
            fi
        fi
    done
    echo ""
}

_draw_stage_tracker() {
    local impl_st="$1"
    local review_st="$2"
    local fix_st="$3"
    local pr_st="$4"

    echo ""
    printf '  %b%bPipeline Stages%b\n' "$_BOLD" "$_CYAN" "$_RESET"
    printf '  %b─────────────────────%b\n' "$_DIM" "$_RESET"
    _stage_status "Branch bootstrap" "done"
    _stage_status "Implementation"   "$impl_st"
    _stage_status "Code review"      "$review_st"
    _stage_status "Fix cycles"       "$fix_st"
    _stage_status "PR creation"      "$pr_st"
    echo ""
}

# ── Core Helpers ─────────────────────────────────────────────────────

_strip_root() { local s="$*" p="$PROJECT_ROOT/"; printf '%s' "${s//$p}"; }

run_cmd() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log_step "${_DIM}⊘${_RESET}" "${_DIM}DRY-RUN: $(_strip_root "$@")${_RESET}"
        return 0
    fi
    "$@"
}

capture_cmd() {
    local output_file="$1"
    shift

    if [[ "$DRY_RUN" == "true" ]]; then
        local display_out
        display_out="$(_strip_root "$output_file")"
        log_step "${_DIM}⊘${_RESET}" "${_DIM}DRY-RUN: $(_strip_root "$@") > ${display_out}${_RESET}"
        : > "$output_file"
        return 0
    fi

    set +e
    "$@" > >(tee "$output_file") 2>&1
    local rc=$?
    set -e
    return $rc
}

# Resolve IMPL_MODEL from IMPL_MODEL_PRESET and IMPL_AGENT.
# If IMPL_MODEL_PRESET is a known preset for the agent, maps to full model ID.
# If IMPL_MODEL_PRESET looks like a full model ID (contains hyphen or dot), uses as-is.
# If empty, leaves IMPL_MODEL empty (agent uses its default).
resolve_impl_model() {
    if [[ -z "$IMPL_MODEL_PRESET" ]]; then
        IMPL_MODEL=""
        return 0
    fi

    local preset="${IMPL_MODEL_PRESET,,}"  # lowercase for matching

    if [[ "$IMPL_AGENT" == "claude" ]]; then
        if [[ -n "${CLAUDE_MODEL_PRESETS[$preset]:-}" ]]; then
            IMPL_MODEL="${CLAUDE_MODEL_PRESETS[$preset]}"
            return 0
        fi
    elif [[ "$IMPL_AGENT" == "codex" ]]; then
        if [[ -n "${CODEX_MODEL_PRESETS[$preset]:-}" ]]; then
            IMPL_MODEL="${CODEX_MODEL_PRESETS[$preset]}"
            return 0
        fi
    fi

    # Cross-agent mismatch: error early instead of failing deep in the agent CLI
    if [[ "$IMPL_AGENT" == "claude" && -n "${CODEX_MODEL_PRESETS[$preset]:-}" ]]; then
        die "--impl-model '$IMPL_MODEL_PRESET' is a codex preset, not valid for claude. Claude presets: $(get_model_presets_for_agent claude)"
        return 1  # safety net if die() doesn't exit
    elif [[ "$IMPL_AGENT" == "codex" && -n "${CLAUDE_MODEL_PRESETS[$preset]:-}" ]]; then
        die "--impl-model '$IMPL_MODEL_PRESET' is a claude preset, not valid for codex. Codex presets: $(get_model_presets_for_agent codex)"
        return 1  # safety net if die() doesn't exit
    fi

    # Not a recognized preset - treat as full model ID if it looks like one
    if [[ "$IMPL_MODEL_PRESET" == *-* || "$IMPL_MODEL_PRESET" == *.* ]]; then
        IMPL_MODEL="$IMPL_MODEL_PRESET"
        return 0
    fi

    # Unknown short name that isn't a preset for any agent
    die "Unknown model preset '$IMPL_MODEL_PRESET' for $IMPL_AGENT agent. Known presets: $(get_model_presets_for_agent "$IMPL_AGENT"). Use a full model ID (e.g., claude-opus-4-6) for custom models."
    return 1  # safety net if die() doesn't exit
}

# Get available model presets for an agent.
# Arguments: agent (claude|codex)
# Outputs: space-separated list of preset names
get_model_presets_for_agent() {
    local agent="$1"
    case "$agent" in
        claude) echo "opus sonnet" ;;
        codex)  echo "default o3" ;;
        *)      echo "" ;;
    esac
}

# Get the default model preset for an agent.
# Arguments: agent (claude|codex)
# Outputs: default preset name
get_default_model_preset() {
    local agent="$1"
    case "$agent" in
        claude) echo "opus" ;;
        codex)  echo "default" ;;
        *)      echo "" ;;
    esac
}

resolve_phases_to_run() {
    if [[ -n "$FROM_PHASE" && -n "$PHASE_ID" ]]; then
        die "Cannot use both --phase and --from-phase"
    fi

    if [[ -n "$FROM_PHASE" ]]; then
        if ! validate_single_phase "$FROM_PHASE"; then
            die "Invalid --from-phase '$FROM_PHASE'. Expected one of: ${ALL_PHASE_IDS[*]}"
        fi
        # shellcheck disable=SC2034
        PHASE_MODE="from"
        local collecting=false
        for p in "${ALL_PHASE_IDS[@]}"; do
            if [[ "$p" == "$FROM_PHASE" ]]; then collecting=true; fi
            if [[ "$collecting" == "true" ]]; then
                PHASES_TO_RUN+=("$p")
            fi
        done
    elif [[ "$PHASE_ID" == "all" ]]; then
        # shellcheck disable=SC2034
        PHASE_MODE="all"
        PHASES_TO_RUN=("${ALL_PHASE_IDS[@]}")
    else
        if ! validate_single_phase "$PHASE_ID"; then
            die "Invalid --phase '$PHASE_ID'. Expected one of: ${ALL_PHASE_IDS[*]}, all"
        fi
        # shellcheck disable=SC2034
        PHASE_MODE="single"
        PHASES_TO_RUN=("$PHASE_ID")
    fi

    if [[ ${#PHASES_TO_RUN[@]} -eq 0 ]]; then
        die "No phases resolved to run"
    fi
}

is_interactive_terminal() {
    [[ -t 0 && -t 1 ]]
}

read_required_input() {
    local prompt="$1"
    local input=""
    read -r -p "$prompt" input || die "Interactive input aborted"
    printf '%s' "$input"
}

# ── Prompt Functions (gum primary + ANSI fallback) ───────────────────

prompt_choice() {
    local out_var="$1"
    local prompt="$2"
    local default_value="$3"
    shift 3
    local options=("$@")

    if [[ "$HAS_GUM" == "true" ]]; then
        local result
        result="$(gum choose \
            --cursor "▸ " \
            --cursor.foreground 212 \
            --header "$prompt" \
            --header.foreground 255 \
            --header.bold \
            --selected "$default_value" \
            --item.foreground 250 \
            "${options[@]}")" || { printf -v "$out_var" '%s' "$default_value"; return 0; }
        printf -v "$out_var" '%s' "$result"
        return 0
    fi

    # ANSI fallback with colored menu
    while true; do
        echo ""
        printf '  %b%b%s%b\n' "$_BOLD" "$_WHITE" "$prompt" "$_RESET"
        local i
        for i in "${!options[@]}"; do
            local value="${options[$i]}"
            if [[ "$value" == "$default_value" ]]; then
                printf '    %b%b%d)%b %b%s%b %b(default)%b\n' \
                    "$_BOLD" "$_CYAN" "$((i + 1))" "$_RESET" \
                    "$_WHITE" "$value" "$_RESET" \
                    "$_DIM" "$_RESET"
            else
                printf '    %b%d)%b %s\n' "$_CYAN" "$((i + 1))" "$_RESET" "$value"
            fi
        done

        local input
        input="$(read_required_input "  ${_DIM}Select${_RESET} [${_CYAN}${default_value}${_RESET}]: ")"
        if [[ -z "$input" ]]; then
            printf -v "$out_var" '%s' "$default_value"
            return 0
        fi

        if [[ "$input" =~ ^[0-9]+$ ]] && (( input >= 1 && input <= ${#options[@]} )); then
            printf -v "$out_var" '%s' "${options[$((input - 1))]}"
            return 0
        fi

        local value
        for value in "${options[@]}"; do
            if [[ "$input" == "$value" ]]; then
                printf -v "$out_var" '%s' "$value"
                return 0
            fi
        done

        printf '  %b%bInvalid choice:%b %s\n' "$_RED" "$_BOLD" "$_RESET" "$input"
    done
}

prompt_yes_no() {
    local out_var="$1"
    local prompt="$2"
    local default_bool="$3"

    if [[ "$HAS_GUM" == "true" ]]; then
        local gum_args=()
        if [[ "$default_bool" == "true" ]]; then
            gum_args+=(--default=yes)
        else
            gum_args+=(--default=no)
        fi
        if gum confirm "$prompt" \
            --affirmative "Yes" --negative "No" \
            --prompt.foreground 255 \
            --prompt.bold \
            --selected.background 179 \
            --selected.foreground 236 \
            --unselected.foreground 240 \
            "${gum_args[@]}"; then
            printf -v "$out_var" '%s' "true"
        else
            printf -v "$out_var" '%s' "false"
        fi
        return 0
    fi

    # ANSI fallback
    local default_hint="${_DIM}y/N${_RESET}"
    if [[ "$default_bool" == "true" ]]; then
        default_hint="${_BOLD}Y${_RESET}${_DIM}/n${_RESET}"
    fi

    while true; do
        local input
        input="$(read_required_input "  $prompt [$default_hint]: ")"

        if [[ -z "$input" ]]; then
            printf -v "$out_var" '%s' "$default_bool"
            return 0
        fi

        local normalized
        normalized="$(printf '%s' "$input" | tr '[:upper:]' '[:lower:]')"

        case "$normalized" in
            y|yes|true|1)
                printf -v "$out_var" '%s' "true"
                return 0
                ;;
            n|no|false|0)
                printf -v "$out_var" '%s' "false"
                return 0
                ;;
            *)
                printf '  %bPlease answer yes or no.%b\n' "$_YELLOW" "$_RESET"
                ;;
        esac
    done
}

prompt_number() {
    local out_var="$1"
    local prompt="$2"
    local default_number="$3"
    local pattern="$4"

    if [[ "$HAS_GUM" == "true" ]]; then
        local result
        result="$(gum input \
            --placeholder "$default_number" \
            --header "$prompt" \
            --header.foreground 255 \
            --header.bold \
            --cursor.foreground 212 \
            --prompt "> " \
            --prompt.foreground 179 \
            --value "$default_number")" || { printf -v "$out_var" '%s' "$default_number"; return 0; }
        if [[ -z "$result" ]]; then
            result="$default_number"
        fi
        if [[ "$result" =~ $pattern ]]; then
            printf -v "$out_var" '%s' "$result"
            return 0
        fi
        printf '  %bInvalid number: %s (using default: %s)%b\n' "$_YELLOW" "$result" "$default_number" "$_RESET"
        printf -v "$out_var" '%s' "$default_number"
        return 0
    fi

    # ANSI fallback
    while true; do
        local input
        printf '\n  %b%b%s%b\n' "$_BOLD" "$_WHITE" "$prompt" "$_RESET"
        input="$(read_required_input "  ${_DIM}Enter number${_RESET} [${_CYAN}${default_number}${_RESET}]: ")"
        if [[ -z "$input" ]]; then
            input="$default_number"
        fi

        if [[ "$input" =~ $pattern ]]; then
            printf -v "$out_var" '%s' "$input"
            return 0
        fi

        printf '  %bInvalid number:%b %s\n' "$_RED" "$_RESET" "$input"
    done
}

prompt_text() {
    local out_var="$1"
    local prompt="$2"
    local default_value="$3"

    if [[ "$HAS_GUM" == "true" ]]; then
        local result
        result="$(gum input \
            --placeholder "$default_value" \
            --header "$prompt" \
            --header.foreground 255 \
            --header.bold \
            --cursor.foreground 212 \
            --prompt "> " \
            --prompt.foreground 179 \
            --value "$default_value")" || { printf -v "$out_var" '%s' "$default_value"; return 0; }
        if [[ -z "$result" ]]; then
            result="$default_value"
        fi
        printf -v "$out_var" '%s' "$result"
        return 0
    fi

    # ANSI fallback
    printf '\n  %b%b%s%b\n' "$_BOLD" "$_WHITE" "$prompt" "$_RESET"
    local input
    input="$(read_required_input "  ${_DIM}Enter value${_RESET} [${_CYAN}${default_value}${_RESET}]: ")"
    if [[ -z "$input" ]]; then
        input="$default_value"
    fi
    printf -v "$out_var" '%s' "$input"
}

prompt_phase_id() {
    local out_var="$1"
    local default_phase="${2:-1}"

    if [[ "$HAS_GUM" == "true" ]]; then
        # Build display items for gum choose
        local items=()
        for phase_id in "${ALL_PHASE_IDS[@]}"; do
            items+=("$(printf '%-3s  %s  %s' "$phase_id" "$(_phase_icon "$phase_id")" "$(phase_title "$phase_id")")")
        done
        items+=("all  🔄  Run all phases sequentially ($FIRST_PHASE_ID → $LAST_PHASE_ID)")
        items+=("from 📍  Start from a specific phase through $LAST_PHASE_ID")

        local result
        result="$(printf '%s\n' "${items[@]}" | gum choose \
            --cursor "▸ " \
            --cursor.foreground 212 \
            --header "Choose phase:" \
            --header.foreground 255 \
            --header.bold \
            --item.foreground 250 \
            --height 14)" || { printf -v "$out_var" '%s' "$default_phase"; return 0; }

        # Extract phase ID (first whitespace-delimited token)
        local selected
        selected="$(printf '%s' "$result" | awk '{print $1}')"
        printf -v "$out_var" '%s' "$selected"
        return 0
    fi

    # ANSI fallback with styled menu
    while true; do
        echo ""
        printf '  %b%bChoose phase:%b\n\n' "$_BOLD" "$_WHITE" "$_RESET"
        local i
        for i in "${!ALL_PHASE_IDS[@]}"; do
            local phase_id="${ALL_PHASE_IDS[$i]}"
            local icon
            icon="$(_phase_icon "$phase_id")"
            local marker=""
            if [[ "$phase_id" == "$default_phase" ]]; then
                marker=" ${_DIM}(default)${_RESET}"
            fi
            printf '    %b%2d)%b  %b%-3s%b  %s  %s%b\n' \
                "$_CYAN" "$((i + 1))" "$_RESET" \
                "$_BOLD" "$phase_id" "$_RESET" \
                "$icon" "$(phase_title "$phase_id")" "$marker"
        done
        printf '    %b────────────────────────────────────────%b\n' "$_DIM" "$_RESET"
        printf '    %b%2d)%b  %ball %b  🔄  Run all phases sequentially (%s → %s)\n' \
            "$_CYAN" "$((${#ALL_PHASE_IDS[@]} + 1))" "$_RESET" "$_BOLD" "$_RESET" "$FIRST_PHASE_ID" "$LAST_PHASE_ID"
        printf '    %b%2d)%b  %bfrom%b  📍  Start from a specific phase through %s\n' \
            "$_CYAN" "$((${#ALL_PHASE_IDS[@]} + 2))" "$_RESET" "$_BOLD" "$_RESET" "$LAST_PHASE_ID"

        echo ""
        local input
        input="$(read_required_input "  ${_DIM}Select phase${_RESET} [${_CYAN}${default_phase}${_RESET}]: ")"
        if [[ -z "$input" ]]; then
            printf -v "$out_var" '%s' "$default_phase"
            return 0
        fi

        # "all" by name or number
        if [[ "$input" == "all" || "$input" == "$((${#ALL_PHASE_IDS[@]} + 1))" ]]; then
            printf -v "$out_var" '%s' "all"
            return 0
        fi

        # "from" by name or number
        if [[ "$input" == "from" || "$input" == "$((${#ALL_PHASE_IDS[@]} + 2))" ]]; then
            printf -v "$out_var" '%s' "from"
            return 0
        fi

        # Number selection for individual phase
        if [[ "$input" =~ ^[0-9]+$ ]] && (( input >= 1 && input <= ${#ALL_PHASE_IDS[@]} )); then
            printf -v "$out_var" '%s' "${ALL_PHASE_IDS[$((input - 1))]}"
            return 0
        fi

        # Direct phase ID
        local phase_id
        for phase_id in "${ALL_PHASE_IDS[@]}"; do
            if [[ "$input" == "$phase_id" ]]; then
                printf -v "$out_var" '%s' "$phase_id"
                return 0
            fi
        done

        printf '  %b%bInvalid phase:%b %s\n' "$_RED" "$_BOLD" "$_RESET" "$input"
    done
}

prompt_from_phase() {
    local out_var="$1"
    local default_phase="${2:-1}"

    if [[ "$HAS_GUM" == "true" ]]; then
        local items=()
        for phase_id in "${ALL_PHASE_IDS[@]}"; do
            # Count remaining phases
            local count=0
            local collecting=false
            for p in "${ALL_PHASE_IDS[@]}"; do
                if [[ "$p" == "$phase_id" ]]; then collecting=true; fi
                if [[ "$collecting" == "true" ]]; then count=$((count + 1)); fi
            done
            local plural=""
            if [[ $count -gt 1 ]]; then plural="s"; fi
            items+=("$(printf '%-3s  %s  %s  (%d phase%s)' \
                "$phase_id" "$(_phase_icon "$phase_id")" "$(phase_title "$phase_id")" "$count" "$plural")")
        done

        local result
        result="$(printf '%s\n' "${items[@]}" | gum choose \
            --cursor "▸ " \
            --cursor.foreground 212 \
            --header "Choose starting phase (runs sequentially through phase $LAST_PHASE_ID):" \
            --header.foreground 255 \
            --header.bold \
            --item.foreground 250 \
            --height 12)" || { printf -v "$out_var" '%s' "$default_phase"; return 0; }

        local selected
        selected="$(printf '%s' "$result" | awk '{print $1}')"
        printf -v "$out_var" '%s' "$selected"
        return 0
    fi

    # ANSI fallback
    while true; do
        echo ""
        printf '  %b%bChoose starting phase%b %b(runs sequentially through phase %s)%b:\n\n' \
            "$_BOLD" "$_WHITE" "$_RESET" "$_DIM" "$LAST_PHASE_ID" "$_RESET"
        local i
        for i in "${!ALL_PHASE_IDS[@]}"; do
            local phase_id="${ALL_PHASE_IDS[$i]}"
            local icon
            icon="$(_phase_icon "$phase_id")"
            # Count remaining phases from this one
            local count=0
            local collecting=false
            for p in "${ALL_PHASE_IDS[@]}"; do
                if [[ "$p" == "$phase_id" ]]; then collecting=true; fi
                if [[ "$collecting" == "true" ]]; then count=$((count + 1)); fi
            done
            local marker=""
            if [[ "$phase_id" == "$default_phase" ]]; then
                marker=" ${_DIM}(default)${_RESET}"
            fi
            printf '    %b%2d)%b  %b%-3s%b  %s  %s  %b(%d phase%s)%b%b\n' \
                "$_CYAN" "$((i + 1))" "$_RESET" \
                "$_BOLD" "$phase_id" "$_RESET" \
                "$icon" "$(phase_title "$phase_id")" \
                "$_DIM" "$count" "$(if [[ $count -gt 1 ]]; then echo "s"; fi)" "$_RESET" \
                "$marker"
        done

        echo ""
        local input
        input="$(read_required_input "  ${_DIM}Select starting phase${_RESET} [${_CYAN}${default_phase}${_RESET}]: ")"
        if [[ -z "$input" ]]; then
            printf -v "$out_var" '%s' "$default_phase"
            return 0
        fi

        if [[ "$input" =~ ^[0-9]+$ ]] && (( input >= 1 && input <= ${#ALL_PHASE_IDS[@]} )); then
            printf -v "$out_var" '%s' "${ALL_PHASE_IDS[$((input - 1))]}"
            return 0
        fi

        local phase_id
        for phase_id in "${ALL_PHASE_IDS[@]}"; do
            if [[ "$input" == "$phase_id" ]]; then
                printf -v "$out_var" '%s' "$phase_id"
                return 0
            fi
        done

        printf '  %b%bInvalid phase:%b %s\n' "$_RED" "$_BOLD" "$_RESET" "$input"
    done
}

# ── Config Summary Display ───────────────────────────────────────────

_display_config_summary() {
    local phases_display=""
    if [[ ${#PHASES_TO_RUN[@]} -gt 1 ]]; then
        phases_display="$(printf '%s → ' "${PHASES_TO_RUN[@]}")"
        phases_display="${phases_display% → } (${#PHASES_TO_RUN[@]} phases)"
    else
        phases_display="${PHASES_TO_RUN[0]} ($(phase_title "${PHASES_TO_RUN[0]}"))"
    fi

    if [[ "$HAS_GUM" == "true" ]]; then
        local body=""
        body+="$(printf '  %-22s %s\n' "Phase(s)" "$phases_display")"
        body+="$(printf '\n  %-22s %s' "Impl agent" "$IMPL_AGENT")"
        if [[ -n "$IMPL_MODEL" ]]; then
            body+="$(printf '\n  %-22s %s' "Impl model" "$IMPL_MODEL")"
        elif [[ -n "$IMPL_MODEL_PRESET" ]]; then
            body+="$(printf '\n  %-22s %s (preset)' "Impl model" "$IMPL_MODEL_PRESET")"
        fi
        body+="$(printf '\n  %-22s %s' "Review mode" "$REVIEW_MODE")"
        if [[ "$REVIEW_MODE" != "none" ]]; then
            body+="$(printf '\n  %-22s %s' "Review agent" "$REVIEW_AGENT")"
            body+="$(printf '\n  %-22s %s' "Review concurrency" "$REVIEW_CONCURRENCY")"
            body+="$(printf '\n  %-22s %s' "Max review cycles" "$MAX_REVIEW_CYCLES")"
            body+="$(printf '\n  %-22s %s' "Fix agent" "$FIX_AGENT")"
        fi
        body+="$(printf '\n  %-22s %s' "Base branch" "$BASE_BRANCH")"
        body+="$(printf '\n  %-22s %s' "Sync base" "$SYNC_BASE")"

        local skips=""
        [[ "$SKIP_IMPLEMENT" == "true" ]] && skips+="impl "
        [[ "$SKIP_REVIEW" == "true" ]]    && skips+="review "
        [[ "$SKIP_FIX" == "true" ]]       && skips+="fix "
        [[ "$SKIP_PR" == "true" ]]        && skips+="pr "
        if [[ -n "$skips" ]]; then
            body+="$(printf '\n  %-22s %s' "Skip" "${skips% }")"
        fi
        [[ "$DRY_RUN" == "true" ]] && body+="$(printf '\n  %-22s %s' "Dry run" "yes")"

        echo ""
        gum style --border rounded --border-foreground 179 \
            --padding "1 2" --margin "0 2" \
            --bold --foreground 255 "Configuration" "" "$body"
    else
        echo ""
        printf '  %b╭─ %b%bConfiguration%b %b─────────────────────────────╮%b\n' \
            "$_YELLOW" "$_RESET" "$_BOLD$_WHITE" "$_RESET" "$_YELLOW" "$_RESET"
        printf '  %b│%b\n' "$_YELLOW" "$_RESET"
        printf '  %b│%b  %-20s  %b%s%b\n' "$_YELLOW" "$_RESET" "Phase(s)" "$_WHITE" "$phases_display" "$_RESET"
        printf '  %b│%b  %-20s  %b%s%b\n' "$_YELLOW" "$_RESET" "Impl agent" "$_MAGENTA" "$IMPL_AGENT" "$_RESET"
        if [[ -n "$IMPL_MODEL" ]]; then
            printf '  %b│%b  %-20s  %b%s%b\n' "$_YELLOW" "$_RESET" "Impl model" "$_CYAN" "$IMPL_MODEL" "$_RESET"
        elif [[ -n "$IMPL_MODEL_PRESET" ]]; then
            printf '  %b│%b  %-20s  %b%s (preset)%b\n' "$_YELLOW" "$_RESET" "Impl model" "$_CYAN" "$IMPL_MODEL_PRESET" "$_RESET"
        fi
        printf '  %b│%b  %-20s  %s\n' "$_YELLOW" "$_RESET" "Review mode" "$REVIEW_MODE"
        if [[ "$REVIEW_MODE" != "none" ]]; then
            printf '  %b│%b  %-20s  %s\n' "$_YELLOW" "$_RESET" "Review agent" "$REVIEW_AGENT"
            printf '  %b│%b  %-20s  %s\n' "$_YELLOW" "$_RESET" "Concurrency" "$REVIEW_CONCURRENCY"
            printf '  %b│%b  %-20s  %s\n' "$_YELLOW" "$_RESET" "Max review cycles" "$MAX_REVIEW_CYCLES"
            printf '  %b│%b  %-20s  %s\n' "$_YELLOW" "$_RESET" "Fix agent" "$FIX_AGENT"
        fi
        printf '  %b│%b  %-20s  %s\n' "$_YELLOW" "$_RESET" "Base branch" "$BASE_BRANCH"
        printf '  %b│%b  %-20s  %s\n' "$_YELLOW" "$_RESET" "Sync base" "$SYNC_BASE"

        local skips=""
        [[ "$SKIP_IMPLEMENT" == "true" ]] && skips+="impl "
        [[ "$SKIP_REVIEW" == "true" ]]    && skips+="review "
        [[ "$SKIP_FIX" == "true" ]]       && skips+="fix "
        [[ "$SKIP_PR" == "true" ]]        && skips+="pr "
        if [[ -n "$skips" ]]; then
            printf '  %b│%b  %-20s  %b%s%b\n' "$_YELLOW" "$_RESET" "Skip" "$_YELLOW" "${skips% }" "$_RESET"
        fi
        if [[ "$DRY_RUN" == "true" ]]; then
            printf '  %b│%b  %-20s  %b%s%b\n' "$_YELLOW" "$_RESET" "Dry run" "$_YELLOW" "yes" "$_RESET"
        fi
        printf '  %b│%b\n' "$_YELLOW" "$_RESET"
        printf '  %b╰──────────────────────────────────────────────╯%b\n' "$_YELLOW" "$_RESET"
    fi
}

# ── Interactive Wizard ───────────────────────────────────────────────

run_interactive_wizard() {
    echo ""
    if [[ "$HAS_GUM" == "true" ]]; then
        gum style --bold --foreground 255 --border double --border-foreground 179 \
            --padding "0 3" --margin "0 2" \
            "🌾  Raven Phase Pipeline Wizard"
    else
        printf '  %b╔══════════════════════════════════════╗%b\n' "$_YELLOW" "$_RESET"
        printf '  %b║%b  %b🌾  Raven Phase Pipeline Wizard%b    %b║%b\n' \
            "$_YELLOW" "$_RESET" "$_BOLD$_WHITE" "$_RESET" "$_YELLOW" "$_RESET"
        printf '  %b╚══════════════════════════════════════╝%b\n' "$_YELLOW" "$_RESET"
    fi

    prompt_phase_id PHASE_ID "${PHASE_ID:-1}"

    # Handle "from" selection: prompt for starting phase
    if [[ "$PHASE_ID" == "from" ]]; then
        prompt_from_phase FROM_PHASE "${FROM_PHASE:-1}"
        PHASE_ID=""  # Clear so resolve_phases_to_run uses FROM_PHASE
    fi

    prompt_choice IMPL_AGENT "Select implementation agent:" "$IMPL_AGENT" "codex" "claude"

    # Model selection for implementation agent
    local model_presets
    model_presets="$(get_model_presets_for_agent "$IMPL_AGENT")"
    if [[ -n "$model_presets" ]]; then
        local default_preset
        default_preset="$(get_default_model_preset "$IMPL_AGENT")"
        # shellcheck disable=SC2086  # word splitting is intentional
        prompt_choice IMPL_MODEL_PRESET "Select ${IMPL_AGENT} model:" "${IMPL_MODEL_PRESET:-$default_preset}" $model_presets
        resolve_impl_model
    fi

    prompt_choice REVIEW_MODE "Select review mode:" "$REVIEW_MODE" "all" "agent" "none"

    if [[ "$REVIEW_MODE" == "agent" ]]; then
        prompt_choice REVIEW_AGENT "Select review agent:" "$REVIEW_AGENT" "codex" "claude" "gemini"
    fi

    if [[ "$REVIEW_MODE" != "none" ]]; then
        prompt_number REVIEW_CONCURRENCY "Set review concurrency:" "$REVIEW_CONCURRENCY" '^[1-9][0-9]*$'
        prompt_number MAX_REVIEW_CYCLES "Set max review-fix cycles:" "$MAX_REVIEW_CYCLES" '^[0-9]+$'
        prompt_choice FIX_AGENT "Select fix agent for blocking review findings:" "$FIX_AGENT" "codex" "claude" "gemini"
    fi

    prompt_text BASE_BRANCH "Base branch:" "$BASE_BRANCH"
    prompt_yes_no SYNC_BASE "Sync base branch from origin before bootstrap?" "$SYNC_BASE"

    local execution_profile="full"
    prompt_choice execution_profile "Select execution profile:" "full" "full" "impl-only" "review-only" "custom"

    case "$execution_profile" in
        full)
            SKIP_IMPLEMENT="false"
            SKIP_REVIEW="false"
            SKIP_FIX="false"
            SKIP_PR="false"
            ;;
        impl-only)
            SKIP_IMPLEMENT="false"
            SKIP_REVIEW="true"
            SKIP_FIX="true"
            SKIP_PR="true"
            ;;
        review-only)
            SKIP_IMPLEMENT="true"
            SKIP_REVIEW="false"
            SKIP_FIX="false"
            SKIP_PR="false"
            ;;
        custom)
            prompt_yes_no SKIP_IMPLEMENT "Skip implementation stage?" "$SKIP_IMPLEMENT"
            prompt_yes_no SKIP_REVIEW "Skip review stage?" "$SKIP_REVIEW"
            prompt_yes_no SKIP_FIX "Skip review-fix cycles?" "$SKIP_FIX"
            prompt_yes_no SKIP_PR "Skip PR creation?" "$SKIP_PR"
            ;;
    esac

    prompt_yes_no DRY_RUN "Run in dry-run mode?" "$DRY_RUN"

    # Resolve phases early so we can display them in the summary
    resolve_phases_to_run

    _display_config_summary

    echo ""
    local proceed="false"
    prompt_yes_no proceed "Proceed with this configuration?" "true"
    if [[ "$proceed" != "true" ]]; then
        die "Pipeline cancelled from interactive wizard"
    fi
}

# ── Pipeline Logic ───────────────────────────────────────────────────

is_blocking_verdict() {
    local verdict="$1"
    [[ "$verdict" == "REQUEST_CHANGES" || "$verdict" == "NEEDS_FIXES" ]]
}

extract_implementation_block_reason() {
    local log_file="$1"

    if [[ ! -f "$log_file" ]]; then
        return 1
    fi

    local blocked_line=""
    blocked_line="$(grep -Eim1 'TASK_BLOCKED|blocked dependencies|No eligible tasks found' "$log_file" || true)"
    if [[ -z "$blocked_line" ]]; then
        return 1
    fi

    blocked_line="$(printf '%s' "$blocked_line" | sed -E 's/^\[[^]]+\][[:space:]]*//')"
    if [[ "$blocked_line" == *"TASK_BLOCKED:"* ]]; then
        blocked_line="${blocked_line#*TASK_BLOCKED: }"
    elif [[ "$blocked_line" == *"BLOCKED:"* ]]; then
        blocked_line="${blocked_line#*BLOCKED: }"
    fi
    blocked_line="$(printf '%s' "$blocked_line" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"
    if [[ -z "$blocked_line" ]]; then
        blocked_line="implementation reported blocked dependencies"
    fi

    printf '%s\n' "$blocked_line"
    return 0
}

assert_expected_branch() {
    if [[ "$DRY_RUN" == "true" ]]; then
        return 0
    fi

    if [[ -z "$EXPECTED_BRANCH" ]]; then
        return 0
    fi

    local current
    current="$(git rev-parse --abbrev-ref HEAD)"
    if [[ "$current" != "$EXPECTED_BRANCH" ]]; then
        die "Expected branch '$EXPECTED_BRANCH' but found '$current'"
    fi
}

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

persist_metadata() {
    cat > "$METADATA_FILE" <<EOF_META
run_id=$RUN_ID
phase=$PHASE_ID
branch=$EXPECTED_BRANCH
base_branch=$BASE_BRANCH
impl_agent=$IMPL_AGENT
impl_model=$IMPL_MODEL
impl_model_preset=$IMPL_MODEL_PRESET
review_mode=$REVIEW_MODE
review_agent=$REVIEW_AGENT
review_concurrency=$REVIEW_CONCURRENCY
max_review_cycles=$MAX_REVIEW_CYCLES
fix_agent=$FIX_AGENT
sync_base=$SYNC_BASE
dry_run=$DRY_RUN
skip_implement=$SKIP_IMPLEMENT
skip_review=$SKIP_REVIEW
skip_fix=$SKIP_FIX
skip_pr=$SKIP_PR
run_dir=$RUN_DIR
report_dir=$REPORT_DIR
pipeline_log=$PIPELINE_LOG
implementation_status=$IMPLEMENT_STATUS
implementation_reason=$IMPLEMENT_REASON
review_verdict=$REVIEW_VERDICT
review_cycles=$REVIEW_CYCLES
fix_cycles=$FIX_CYCLES
pr_status=$PR_STATUS
updated_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF_META
}

init_artifacts() {
    RUN_ID="phase-${PHASE_ID}-$(date -u +%Y%m%dT%H%M%SZ)"
    RUN_DIR="$PROJECT_ROOT/.review-workspace/phase-pipeline/$RUN_ID"
    REPORT_DIR="$PROJECT_ROOT/reports/review/$RUN_ID"
    mkdir -p "$RUN_DIR" "$REPORT_DIR"

    PIPELINE_LOG="$RUN_DIR/pipeline.log"
    METADATA_FILE="$RUN_DIR/metadata.env"
    : > "$PIPELINE_LOG"

    persist_metadata
}

ensure_clean_tree_before_bootstrap() {
    if [[ "$DRY_RUN" == "true" ]]; then
        return 0
    fi

    local dirty
    dirty="$(git status --porcelain)"
    if [[ -n "$dirty" ]]; then
        die "Working tree is dirty before branch bootstrap. Commit/stash first."
    fi
}

preflight() {
    for phase in "${PHASES_TO_RUN[@]}"; do
        if ! validate_single_phase "$phase"; then
            die "Invalid phase '$phase' in run list. Expected one of: ${ALL_PHASE_IDS[*]}"
        fi
    done

    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        die "Must run inside a git repository"
    fi

    if [[ "$SKIP_IMPLEMENT" != "true" ]]; then
        if [[ "$IMPL_AGENT" != "claude" && "$IMPL_AGENT" != "codex" ]]; then
            die "--impl-agent must be claude or codex"
        fi
        if [[ ! -f "$PROJECT_ROOT/scripts/ralph_${IMPL_AGENT}.sh" ]]; then
            die "Implementation script missing: scripts/ralph_${IMPL_AGENT}.sh"
        fi
    fi

    if [[ "$REVIEW_MODE" != "none" && "$REVIEW_MODE" != "agent" && "$REVIEW_MODE" != "all" ]]; then
        die "--review must be one of: none, agent, all"
    fi

    if [[ "$REVIEW_AGENT" != "claude" && "$REVIEW_AGENT" != "codex" && "$REVIEW_AGENT" != "gemini" ]]; then
        die "--review-agent must be one of: claude, codex, gemini"
    fi

    if [[ "$FIX_AGENT" != "claude" && "$FIX_AGENT" != "codex" && "$FIX_AGENT" != "gemini" ]]; then
        die "--fix-agent must be one of: claude, codex, gemini"
    fi

    if ! [[ "$REVIEW_CONCURRENCY" =~ ^[1-9][0-9]*$ ]]; then
        die "--review-concurrency must be a positive integer"
    fi

    if ! [[ "$MAX_REVIEW_CYCLES" =~ ^[0-9]+$ ]]; then
        die "--max-review-cycles must be a non-negative integer"
    fi

    if [[ "$SKIP_PR" != "true" ]] && [[ "$DRY_RUN" != "true" ]]; then
        if ! command -v gh >/dev/null 2>&1; then
            die "GitHub CLI (gh) is required for PR creation"
        fi
    fi

    if ! resolve_base_ref >/dev/null; then
        die "Could not resolve base branch '$BASE_BRANCH' locally or at origin/$BASE_BRANCH"
    fi

    ensure_clean_tree_before_bootstrap
}

sync_base_branch() {
    if [[ "$SYNC_BASE" != "true" ]]; then
        return 0
    fi

    log_step "$_SYM_RUNNING" "Syncing base branch ${_BOLD}'$BASE_BRANCH'${_RESET} from origin"
    run_cmd git fetch origin "$BASE_BRANCH"

    local base_ref
    if git show-ref --verify --quiet "refs/remotes/origin/$BASE_BRANCH"; then
        if git show-ref --verify --quiet "refs/heads/$BASE_BRANCH"; then
            run_cmd git checkout "$BASE_BRANCH"
            run_cmd git merge --ff-only "origin/$BASE_BRANCH"
        else
            run_cmd git checkout -b "$BASE_BRANCH" "origin/$BASE_BRANCH"
        fi
    else
        die "origin/$BASE_BRANCH not found after fetch"
    fi
    log_step "$_SYM_CHECK" "Base branch synced"
}

bootstrap_branch() {
    local slug
    slug="$(phase_slug "$PHASE_ID")"
    local branch
    branch="phase/${PHASE_ID}-${slug}"

    # In multi-phase mode, chain from the previous phase's branch
    local base_ref
    if [[ -n "$CHAIN_BASE" ]]; then
        base_ref="$CHAIN_BASE"
    else
        base_ref="$(resolve_base_ref)"
    fi

    log_step "$_SYM_RUNNING" "Bootstrapping branch ${_BOLD}'$branch'${_RESET} from ${_DIM}'$base_ref'${_RESET}"

    if git show-ref --verify --quiet "refs/heads/$branch"; then
        run_cmd git checkout "$branch"
    else
        run_cmd git checkout -b "$branch" "$base_ref"
    fi

    EXPECTED_BRANCH="$branch"
    assert_expected_branch
    log_step "$_SYM_CHECK" "On branch ${_BOLD}$branch${_RESET}"
}

run_implementation() {
    if [[ "$SKIP_IMPLEMENT" == "true" ]]; then
        IMPLEMENT_STATUS="skipped"
        IMPLEMENT_REASON=""
        log_step "${_DIM}⊘${_RESET}" "${_DIM}Implementation skipped${_RESET}"
        persist_metadata
        return 0
    fi

    assert_expected_branch

    local impl_script="$PROJECT_ROOT/scripts/ralph_${IMPL_AGENT}.sh"
    local impl_log="$RUN_DIR/implementation.log"
    local impl_rc=0
    local blocked_reason=""
    local model_display="${IMPL_MODEL:-default}"
    log_step "$_SYM_RUNNING" "Implementation starting ${_DIM}($IMPL_AGENT agent, model=$model_display, phase $PHASE_ID)${_RESET}"

    # Build implementation command arguments
    local impl_args=()
    impl_args+=(--phase "$PHASE_ID")
    if [[ -n "$IMPL_MODEL" ]]; then
        impl_args+=(--model "$IMPL_MODEL")
    fi

    if capture_cmd "$impl_log" "$impl_script" "${impl_args[@]}"; then
        impl_rc=0
    else
        impl_rc=$?
    fi

    if blocked_reason="$(extract_implementation_block_reason "$impl_log")"; then
        IMPLEMENT_STATUS="blocked"
        IMPLEMENT_REASON="$blocked_reason"
        log_step "$_SYM_WARN" "${_YELLOW}Implementation blocked:${_RESET} $blocked_reason"
        persist_metadata
        return 1
    fi

    if [[ "$impl_rc" -eq 2 ]]; then
        IMPLEMENT_STATUS="blocked"
        IMPLEMENT_REASON="implementation exited with code 2 (blocked/partial outcome)"
        log_step "$_SYM_WARN" "${_YELLOW}Implementation blocked:${_RESET} $IMPLEMENT_REASON"
        persist_metadata
        return 1
    fi

    if [[ "$impl_rc" -ne 0 ]]; then
        IMPLEMENT_STATUS="failed"
        IMPLEMENT_REASON="implementation exited with code $impl_rc (see ${impl_log#"$PROJECT_ROOT"/})"
        log_step "$_SYM_CROSS" "Implementation ${_RED}failed${_RESET}: $IMPLEMENT_REASON"
        persist_metadata
        return 1
    fi

    IMPLEMENT_STATUS="completed"
    IMPLEMENT_REASON=""
    assert_expected_branch
    log_step "$_SYM_CHECK" "Implementation ${_GREEN}completed${_RESET}"
    persist_metadata
}

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

extract_verdict_from_consolidated() {
    local consolidated_file="$PROJECT_ROOT/reports/review/latest/consolidated.json"

    if [[ ! -f "$consolidated_file" ]]; then
        echo "UNKNOWN"
        return 0
    fi

    local verdict
    verdict="$(jq -r '.verdict // "UNKNOWN"' "$consolidated_file" 2>/dev/null || echo "UNKNOWN")"

    case "$verdict" in
        REQUEST_CHANGES) echo "REQUEST_CHANGES" ;;
        NEEDS_FIXES) echo "NEEDS_FIXES" ;;
        APPROVE|APPROVED) echo "APPROVED" ;;
        COMMENT) echo "COMMENT" ;;
        *) echo "UNKNOWN" ;;
    esac
}

_verdict_styled() {
    local verdict="$1"
    case "$verdict" in
        APPROVED)         printf '%b%bAPPROVED%b'         "$_GREEN" "$_BOLD" "$_RESET" ;;
        REQUEST_CHANGES)  printf '%b%bREQUEST_CHANGES%b'  "$_RED"   "$_BOLD" "$_RESET" ;;
        NEEDS_FIXES)      printf '%b%bNEEDS_FIXES%b'      "$_RED"   "$_BOLD" "$_RESET" ;;
        COMMENT)          printf '%b%bCOMMENT%b'           "$_YELLOW" "$_BOLD" "$_RESET" ;;
        SKIPPED)          printf '%b%bSKIPPED%b'           "$_DIM"   "$_BOLD" "$_RESET" ;;
        *)                printf '%b%b%s%b'                "$_YELLOW" "$_BOLD" "$verdict" "$_RESET" ;;
    esac
}

run_review_once() {
    local cycle="$1"

    assert_expected_branch

    local review_script="$PROJECT_ROOT/scripts/review/review.sh"
    if [[ ! -f "$review_script" ]]; then
        die "Review script not found: scripts/review/review.sh"
    fi

    local review_log="$RUN_DIR/review-cycle-${cycle}.log"
    local review_args=()

    review_args+=(--base "$BASE_BRANCH")
    review_args+=(--concurrency "$REVIEW_CONCURRENCY")
    if [[ "$REVIEW_MODE" == "agent" ]]; then
        review_args+=(--agent "$REVIEW_AGENT")
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        review_args+=(--dry-run)
    fi

    log_step "$_SYM_RUNNING" "Review cycle $cycle ${_DIM}(mode=$REVIEW_MODE)${_RESET}"

    export RAVEN_PHASE_ID="$PHASE_ID"
    export RAVEN_REVIEW_MODE="$REVIEW_MODE"
    export RAVEN_REVIEW_AGENT="$REVIEW_AGENT"
    export RAVEN_REVIEW_CONCURRENCY="$REVIEW_CONCURRENCY"
    export RAVEN_REVIEW_REPORT_DIR="$REPORT_DIR"
    export RAVEN_REVIEW_RUN_DIR="$RUN_DIR"

    if [[ "$DRY_RUN" == "true" ]]; then
        if ! capture_cmd "$review_log" "$review_script" "${review_args[@]}"; then
            log "Review command returned non-zero in dry-run"
        fi
        REVIEW_VERDICT="UNKNOWN"
        log_step "${_DIM}⊘${_RESET}" "Review verdict: $(_verdict_styled "$REVIEW_VERDICT") ${_DIM}(dry-run)${_RESET}"
        persist_metadata
        return 0
    else
        local review_rc=0
        if capture_cmd "$review_log" "$review_script" "${review_args[@]}"; then
            review_rc=0
        else
            review_rc=$?
        fi

        if [[ "$review_rc" -ne 0 ]]; then
            die "Review command failed in cycle $cycle with exit code $review_rc (see ${review_log#"$PROJECT_ROOT"/})"
        fi
    fi

    REVIEW_VERDICT="$(extract_verdict_from_consolidated)"
    if [[ "$REVIEW_VERDICT" == "UNKNOWN" ]]; then
        REVIEW_VERDICT="$(extract_verdict "$review_log")"
    fi
    log_step "$_SYM_CHECK" "Review verdict: $(_verdict_styled "$REVIEW_VERDICT")"
    persist_metadata
}

run_fix_once() {
    local cycle="$1"

    assert_expected_branch

    local fix_script="$PROJECT_ROOT/scripts/review/review-fix.sh"
    if [[ ! -f "$fix_script" ]]; then
        die "Fix script not found: scripts/review/review-fix.sh"
    fi

    local fix_log="$RUN_DIR/fix-cycle-${cycle}.log"
    log_step "$_SYM_RUNNING" "Fix cycle $cycle ${_DIM}($FIX_AGENT agent)${_RESET}"

    export RAVEN_PHASE_ID="$PHASE_ID"
    export RAVEN_FIX_AGENT="$FIX_AGENT"
    export RAVEN_REVIEW_VERDICT="$REVIEW_VERDICT"
    export RAVEN_REVIEW_REPORT_DIR="$REPORT_DIR"
    export RAVEN_REVIEW_RUN_DIR="$RUN_DIR"

    local fix_args=()
    fix_args+=(--agent "$FIX_AGENT")
    if [[ "$DRY_RUN" == "true" ]]; then
        fix_args+=(--dry-run)
    fi

    if capture_cmd "$fix_log" "$fix_script" "${fix_args[@]}"; then
        log_step "$_SYM_CHECK" "Fix cycle $cycle ${_GREEN}completed${_RESET}"
    else
        die "Fix command failed in cycle $cycle (see ${fix_log#"$PROJECT_ROOT"/})"
    fi

    FIX_CYCLES=$((FIX_CYCLES + 1))
    persist_metadata
}

run_review_and_fix_cycles() {
    if [[ "$SKIP_REVIEW" == "true" || "$REVIEW_MODE" == "none" ]]; then
        REVIEW_VERDICT="SKIPPED"
        log_step "${_DIM}⊘${_RESET}" "${_DIM}Review skipped${_RESET}"
        persist_metadata
        return 0
    fi

    run_review_once 0
    REVIEW_CYCLES=0

    while is_blocking_verdict "$REVIEW_VERDICT"; do
        if [[ "$SKIP_FIX" == "true" ]]; then
            log_step "$_SYM_WARN" "Blocking review verdict but fix cycles are skipped"
            break
        fi

        if (( REVIEW_CYCLES >= MAX_REVIEW_CYCLES )); then
            log_step "$_SYM_WARN" "Max review cycles reached (${_BOLD}$MAX_REVIEW_CYCLES${_RESET}) with verdict $(_verdict_styled "$REVIEW_VERDICT")"
            break
        fi

        REVIEW_CYCLES=$((REVIEW_CYCLES + 1))
        run_fix_once "$REVIEW_CYCLES"
        run_review_once "$REVIEW_CYCLES"
    done

    if is_blocking_verdict "$REVIEW_VERDICT" && [[ "$SKIP_FIX" != "true" ]]; then
        die "Blocking review verdict remains after $REVIEW_CYCLES fix cycle(s): $REVIEW_VERDICT"
    fi

    persist_metadata
}

run_pr_creation() {
    if [[ "$SKIP_PR" == "true" ]]; then
        PR_STATUS="skipped"
        log_step "${_DIM}⊘${_RESET}" "${_DIM}PR creation skipped${_RESET}"
        persist_metadata
        return 0
    fi

    assert_expected_branch

    local verification_summary
    verification_summary="implementation=${IMPLEMENT_STATUS}; review_verdict=${REVIEW_VERDICT}; review_cycles=${REVIEW_CYCLES}; fix_cycles=${FIX_CYCLES}; artifacts=${RUN_DIR#"$PROJECT_ROOT"/}"

    local pr_script="$PROJECT_ROOT/scripts/review/create-pr.sh"
    if [[ ! -f "$pr_script" ]]; then
        die "PR script not found: scripts/review/create-pr.sh"
    fi

    log_step "$_SYM_RUNNING" "Creating pull request"

    local pr_args=()
    pr_args+=(--phase "$PHASE_ID")
    pr_args+=(--base "$BASE_BRANCH")
    pr_args+=(--review-verdict "$REVIEW_VERDICT")
    pr_args+=(--verification-summary "$verification_summary")
    pr_args+=(--artifacts-dir "$RUN_DIR")
    if [[ "$DRY_RUN" == "true" ]]; then
        pr_args+=(--dry-run)
    fi

    "$pr_script" "${pr_args[@]}"

    PR_STATUS="completed"
    log_step "$_SYM_CHECK" "Pull request ${_GREEN}created${_RESET}"
    persist_metadata
}

# ── Argument Parsing ─────────────────────────────────────────────────

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --phase)
                PHASE_ID="$2"
                shift 2
                ;;
            --from-phase)
                FROM_PHASE="$2"
                shift 2
                ;;
            --impl-agent)
                IMPL_AGENT="$2"
                shift 2
                ;;
            --impl-model)
                IMPL_MODEL_PRESET="$2"
                shift 2
                ;;
            --review)
                REVIEW_MODE="$2"
                shift 2
                ;;
            --review-agent)
                REVIEW_AGENT="$2"
                shift 2
                ;;
            --review-concurrency)
                REVIEW_CONCURRENCY="$2"
                shift 2
                ;;
            --max-review-cycles)
                MAX_REVIEW_CYCLES="$2"
                shift 2
                ;;
            --fix-agent)
                FIX_AGENT="$2"
                shift 2
                ;;
            --base)
                BASE_BRANCH="$2"
                shift 2
                ;;
            --sync-base)
                SYNC_BASE="true"
                shift
                ;;
            --skip-implement)
                SKIP_IMPLEMENT="true"
                shift
                ;;
            --skip-review)
                SKIP_REVIEW="true"
                shift
                ;;
            --skip-fix)
                SKIP_FIX="true"
                shift
                ;;
            --skip-pr)
                SKIP_PR="true"
                shift
                ;;
            --dry-run)
                DRY_RUN="true"
                shift
                ;;
            --interactive)
                INTERACTIVE="true"
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

}

resolve_interactive_mode() {
    if [[ "$INTERACTIVE" == "true" ]]; then
        run_interactive_wizard
        return 0
    fi

    if [[ -z "$PHASE_ID" && -z "$FROM_PHASE" ]] && is_interactive_terminal; then
        INTERACTIVE="true"
        run_interactive_wizard
        return 0
    fi

    if [[ -z "$PHASE_ID" && -z "$FROM_PHASE" ]]; then
        die "--phase or --from-phase is required (or run with --interactive)"
    fi
}

# ── Phase Execution ──────────────────────────────────────────────────

run_single_phase() {
    local phase="$1"

    # Reset per-phase state
    PHASE_ID="$phase"
    IMPLEMENT_STATUS="not-run"
    IMPLEMENT_REASON=""
    REVIEW_VERDICT="NOT_RUN"
    REVIEW_CYCLES=0
    FIX_CYCLES=0
    PR_STATUS="not-run"
    EXPECTED_BRANCH=""

    init_artifacts

    log_header "$(_phase_icon "$PHASE_ID") Phase $PHASE_ID: $(phase_title "$PHASE_ID")"

    # Show stage tracker at start
    local impl_st="pending" review_st="pending" fix_st="pending" pr_st="pending"
    [[ "$SKIP_IMPLEMENT" == "true" ]] && impl_st="skipped"
    [[ "$SKIP_REVIEW" == "true" || "$REVIEW_MODE" == "none" ]] && review_st="skipped"
    [[ "$SKIP_FIX" == "true" ]] && fix_st="skipped"
    [[ "$SKIP_PR" == "true" ]] && pr_st="skipped"

    ensure_clean_tree_before_bootstrap
    bootstrap_branch
    if run_implementation; then
        run_review_and_fix_cycles
        run_pr_creation
    else
        REVIEW_VERDICT="SKIPPED"
        PR_STATUS="skipped"
        log_step "$_SYM_WARN" "Skipping review/fix/PR because implementation status is ${_BOLD}${IMPLEMENT_STATUS}${_RESET}"
        if [[ -n "$IMPLEMENT_REASON" ]]; then
            log_step "$_SYM_WARN" "${_YELLOW}Reason:${_RESET} $IMPLEMENT_REASON"
        fi
        persist_metadata
    fi

    # Phase completion summary
    echo ""
    local verdict_display
    verdict_display="$(_verdict_styled "$REVIEW_VERDICT")"

    local impl_icon pr_icon
    case "$IMPLEMENT_STATUS" in
        completed) impl_icon="$_SYM_CHECK" ;;
        blocked)   impl_icon="$_SYM_WARN" ;;
        skipped)   impl_icon="${_DIM}⊘${_RESET}" ;;
        *)         impl_icon="$_SYM_CROSS" ;;
    esac
    case "$PR_STATUS" in
        completed) pr_icon="$_SYM_CHECK" ;;
        skipped)   pr_icon="${_DIM}⊘${_RESET}" ;;
        *)         pr_icon="$_SYM_CROSS" ;;
    esac

    local phase_summary_title="Phase $PHASE_ID Complete"
    local phase_summary_color="179"  # warm gold — matches framing
    local phase_summary_fg="255"     # white text
    if [[ "$IMPLEMENT_STATUS" == "blocked" ]]; then
        phase_summary_title="Phase $PHASE_ID Halted"
        phase_summary_color="214"
        phase_summary_fg="214"
    elif [[ "$IMPLEMENT_STATUS" == "failed" ]]; then
        phase_summary_title="Phase $PHASE_ID Halted"
        phase_summary_color="196"
        phase_summary_fg="196"
    fi

    if [[ "$HAS_GUM" == "true" ]]; then
        local summary=""
        summary+="$(printf '  impl=%s  review=%s  pr=%s' "$IMPLEMENT_STATUS" "$REVIEW_VERDICT" "$PR_STATUS")"
        if [[ "$REVIEW_CYCLES" -gt 0 ]]; then
            summary+="$(printf '  cycles=%d' "$REVIEW_CYCLES")"
        fi
        gum style --border rounded --border-foreground "$phase_summary_color" \
            --padding "0 2" --margin "0 2" \
            --bold --foreground "$phase_summary_fg" \
            "$phase_summary_title" "" "$summary"
    else
        local summary_color="$_CYAN"
        if [[ "$IMPLEMENT_STATUS" == "blocked" ]]; then
            summary_color="$_YELLOW"
        elif [[ "$IMPLEMENT_STATUS" == "failed" ]]; then
            summary_color="$_RED"
        fi
        printf '  %b╭─ %b%b%s%b %b──────────────────────╮%b\n' \
            "$summary_color" "$_RESET" "$_BOLD$_WHITE" "$phase_summary_title" "$_RESET" "$summary_color" "$_RESET"
        printf '  %b│%b  %b Implementation  %s\n' "$summary_color" "$_RESET" "$impl_icon" "$IMPLEMENT_STATUS"
        printf '  %b│%b  %b Review          %b\n' "$summary_color" "$_RESET" "$_SYM_CHECK" "$verdict_display"
        printf '  %b│%b  %b PR              %s\n' "$summary_color" "$_RESET" "$pr_icon" "$PR_STATUS"
        if [[ "$REVIEW_CYCLES" -gt 0 ]]; then
            printf '  %b│%b    Review cycles: %d  Fix cycles: %d\n' "$summary_color" "$_RESET" "$REVIEW_CYCLES" "$FIX_CYCLES"
        fi
        printf '  %b╰──────────────────────────────────────────╯%b\n' "$summary_color" "$_RESET"
    fi

    if is_blocking_verdict "$REVIEW_VERDICT"; then
        log_step "$_SYM_WARN" "${_YELLOW}Phase $PHASE_ID ended with blocking verdict: $(_verdict_styled "$REVIEW_VERDICT")${_RESET}"
    fi

    if [[ "$IMPLEMENT_STATUS" == "blocked" || "$IMPLEMENT_STATUS" == "failed" ]]; then
        return 1
    fi

    # Update chain base so next phase branches from this phase's branch
    CHAIN_BASE="$EXPECTED_BRANCH"
}

# ── Main ─────────────────────────────────────────────────────────────

main() {
    parse_args "$@"
    resolve_interactive_mode

    # resolve_phases_to_run is called in the wizard for interactive mode;
    # for non-interactive, call it here
    if [[ "$INTERACTIVE" != "true" ]]; then
        resolve_phases_to_run
        # Resolve model preset to full model ID for CLI usage
        resolve_impl_model
    fi

    preflight

    local total=${#PHASES_TO_RUN[@]}

    echo ""
    if [[ $total -gt 1 ]]; then
        local phases_display
        phases_display="$(printf '%s → ' "${PHASES_TO_RUN[@]}")"
        phases_display="${phases_display% → }"

        if [[ "$HAS_GUM" == "true" ]]; then
            gum style --bold --foreground 255 --border thick --border-foreground 179 \
                --padding "0 2" --margin "0 2" \
                "Pipeline: $phases_display ($total phases)"
        else
            printf '  %b━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%b\n' "$_YELLOW" "$_RESET"
            printf '  %b%b  Pipeline: %s (%d phases)%b\n' "$_BOLD" "$_YELLOW" "$phases_display" "$total" "$_RESET"
            printf '  %b━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%b\n' "$_YELLOW" "$_RESET"
        fi
        echo ""
        _draw_phase_progress 0 "$total"
        echo ""

        log "base=${_BOLD}$BASE_BRANCH${_RESET}  dry_run=${_BOLD}$DRY_RUN${_RESET}"
    else
        log_header "$(_phase_icon "${PHASES_TO_RUN[0]}") Phase ${PHASES_TO_RUN[0]}: $(phase_title "${PHASES_TO_RUN[0]}")"
        log "base=${_BOLD}$BASE_BRANCH${_RESET}  dry_run=${_BOLD}$DRY_RUN${_RESET}"
    fi

    sync_base_branch

    local idx=0
    CHAIN_BASE=""

    for phase in "${PHASES_TO_RUN[@]}"; do
        idx=$((idx + 1))

        if [[ $total -gt 1 ]]; then
            echo ""
            _draw_phase_progress "$((idx - 1))" "$total"
            printf '  %b%bPhase %d/%d:%b %s %b%s%b – %s\n\n' \
                "$_BOLD" "$_MAGENTA" "$idx" "$total" "$_RESET" \
                "$(_phase_icon "$phase")" "$_BOLD" "$phase" "$_RESET" \
                "$(phase_title "$phase")"
        fi

        if ! run_single_phase "$phase"; then
            local impl_reason_msg=""
            if [[ -n "$IMPLEMENT_REASON" ]]; then
                impl_reason_msg=" ($IMPLEMENT_REASON)"
            fi
            die "Phase $phase halted after implementation status '$IMPLEMENT_STATUS'${impl_reason_msg}"
        fi
    done

    # Final completion banner
    echo ""
    if [[ $total -gt 1 ]]; then
        _draw_phase_progress "$total" "$total"
        echo ""
    fi

    if [[ "$HAS_GUM" == "true" ]]; then
        if [[ $total -gt 1 ]]; then
            gum style --bold --foreground 255 --border double --border-foreground 179 \
                --padding "0 3" --margin "0 2" \
                "All $total phases complete!"
        else
            gum style --bold --foreground 255 --border double --border-foreground 179 \
                --padding "0 3" --margin "0 2" \
                "Pipeline complete!" "" \
                "  Metadata:  ${METADATA_FILE#"$PROJECT_ROOT"/}" \
                "  Artifacts: ${RUN_DIR#"$PROJECT_ROOT"/}"
        fi
    else
        if [[ $total -gt 1 ]]; then
            printf '  %b╔══════════════════════════════════════╗%b\n' "$_CYAN" "$_RESET"
            printf '  %b║%b  %b%b All %d phases complete! %b           %b║%b\n' \
                "$_CYAN" "$_RESET" "$_BOLD" "$_WHITE" "$total" "$_RESET" "$_CYAN" "$_RESET"
            printf '  %b╚══════════════════════════════════════╝%b\n' "$_CYAN" "$_RESET"
        else
            printf '  %b╔══════════════════════════════════════╗%b\n' "$_CYAN" "$_RESET"
            printf '  %b║%b  %b%b Pipeline complete! %b               %b║%b\n' \
                "$_CYAN" "$_RESET" "$_BOLD" "$_WHITE" "$_RESET" "$_CYAN" "$_RESET"
            printf '  %b╚══════════════════════════════════════╝%b\n' "$_CYAN" "$_RESET"
            echo ""
            printf '  %bMetadata:%b  %s\n' "$_DIM" "$_RESET" "${METADATA_FILE#"$PROJECT_ROOT"/}"
            printf '  %bArtifacts:%b %s\n' "$_DIM" "$_RESET" "${RUN_DIR#"$PROJECT_ROOT"/}"
        fi
    fi
    echo ""
}

main "$@"
