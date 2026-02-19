#!/usr/bin/env bash
# scripts/release-verify.sh
# Automated release verification for Raven.
#
# Usage:
#   ./scripts/release-verify.sh [version] [--quick]
#
# Options:
#   version   Semantic version to verify (default: git describe --tags or v0.0.0-dev)
#   --quick   Skip E2E tests and GoReleaser checks for faster developer runs
#
# Exit codes:
#   0  All checks passed
#   1  One or more checks failed
set -euo pipefail

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
VERSION="${1:-}"
QUICK=false

for arg in "$@"; do
    case "$arg" in
        --quick) QUICK=true ;;
    esac
done

if [[ -z "$VERSION" || "$VERSION" == "--quick" ]]; then
    VERSION="$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0-dev")"
fi

# ---------------------------------------------------------------------------
# Result tracking
# ---------------------------------------------------------------------------
PASS=0
FAIL=0
RESULTS=()

# check <name> <cmd> [args...]
# Runs the command, captures combined stdout+stderr, records PASS or FAIL.
check() {
    local name="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        RESULTS+=("PASS: $name")
        ((PASS++)) || true
    else
        RESULTS+=("FAIL: $name")
        ((FAIL++)) || true
    fi
}

# skip <name> <reason>
# Records a skipped check in the results without affecting PASS/FAIL counts.
skip() {
    local name="$1"
    local reason="$2"
    RESULTS+=("SKIP: $name ($reason)")
}

# ---------------------------------------------------------------------------
# Resolve the Go module path (needed for ldflags -X)
# ---------------------------------------------------------------------------
MODULE="$(go list -m 2>/dev/null || echo "unknown")"

# ---------------------------------------------------------------------------
# Header
# ---------------------------------------------------------------------------
echo "=== Raven Release Verification: $VERSION ==="
echo ""

# ---------------------------------------------------------------------------
# 1. Build Checks
# ---------------------------------------------------------------------------
echo "--- Build Checks ---"

check "go build" go build ./cmd/raven/

check "go vet" go vet ./...

# go mod tidy must produce no diff in go.mod / go.sum
check "go mod tidy (no diff)" bash -c "go mod tidy && git diff --exit-code go.mod go.sum"

echo ""

# ---------------------------------------------------------------------------
# 2. Test Checks
# ---------------------------------------------------------------------------
echo "--- Test Checks ---"

check "unit tests (race)" go test -race ./...

if [[ "$QUICK" == true ]]; then
    skip "e2e tests" "--quick flag set"
else
    check "e2e tests" go test -v ./tests/e2e/ -timeout 10m
fi

echo ""

# ---------------------------------------------------------------------------
# 3. Binary Size Checks  (5 platform targets)
# ---------------------------------------------------------------------------
echo "--- Binary Size Checks ---"

# Portable file-size helper: prefers GNU stat (-c%s), falls back to BSD stat (-f%z)
file_size() {
    local path="$1"
    stat -c%s "$path" 2>/dev/null || stat -f%z "$path"
}

PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
)

MAX_BYTES=$(( 25 * 1024 * 1024 ))   # 25 MiB

for platform in "${PLATFORMS[@]}"; do
    GOOS="${platform%%/*}"
    GOARCH="${platform##*/}"
    EXT=""
    if [[ "$GOOS" == "windows" ]]; then EXT=".exe"; fi

    TMP_BIN="/tmp/raven-${GOOS}-${GOARCH}${EXT}"

    # Build for the target platform
    if CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
        go build -ldflags="-s -w" -o "$TMP_BIN" ./cmd/raven/ 2>/dev/null; then

        SIZE="$(file_size "$TMP_BIN" 2>/dev/null || echo 0)"
        MB=$(( SIZE / 1024 / 1024 ))

        check "binary size ${GOOS}/${GOARCH} (${MB}MB < 25MB)" \
            test "$SIZE" -lt "$MAX_BYTES"

        rm -f "$TMP_BIN"
    else
        RESULTS+=("FAIL: binary build ${GOOS}/${GOARCH}")
        ((FAIL++)) || true
        rm -f "$TMP_BIN"
    fi
done

echo ""

# ---------------------------------------------------------------------------
# 4. GoReleaser Checks
# ---------------------------------------------------------------------------
echo "--- GoReleaser Checks ---"

if ! command -v goreleaser >/dev/null 2>&1; then
    echo "WARNING: goreleaser not installed -- skipping GoReleaser checks"
    skip "goreleaser check" "goreleaser not installed"
    skip "goreleaser build --snapshot" "goreleaser not installed"
elif [[ "$QUICK" == true ]]; then
    skip "goreleaser check" "--quick flag set"
    skip "goreleaser build --snapshot" "--quick flag set"
else
    check "goreleaser check" goreleaser check
    check "goreleaser build --snapshot" goreleaser build --snapshot --clean
fi

echo ""

# ---------------------------------------------------------------------------
# 5. Documentation Checks
# ---------------------------------------------------------------------------
echo "--- Documentation Checks ---"

check "README.md exists" test -f README.md
check "LICENSE exists" test -f LICENSE
check "CONTRIBUTING.md exists" test -f CONTRIBUTING.md

# Man pages: generate to a temp dir so we don't pollute the tree
MAN_TMP="$(mktemp -d)"
check "man pages generate" go run ./scripts/gen-manpages "$MAN_TMP"
rm -rf "$MAN_TMP"

# Completions: generate to a temp dir
COMP_TMP="/tmp/raven-completions-verify"
rm -rf "$COMP_TMP"
check "completions generate" go run ./scripts/gen-completions "$COMP_TMP"
rm -rf "$COMP_TMP"

echo ""

# ---------------------------------------------------------------------------
# 6. Version Injection Checks
# ---------------------------------------------------------------------------
echo "--- Version Injection Checks ---"

VERIFY_BIN="/tmp/raven-verify"

# Build with explicit version in ldflags
if CGO_ENABLED=0 go build \
    -ldflags="-s -w -X ${MODULE}/internal/buildinfo.Version=${VERSION}" \
    -o "$VERIFY_BIN" ./cmd/raven/ 2>/dev/null; then

    check "version output contains ${VERSION}" \
        bash -c "\"$VERIFY_BIN\" version | grep -qF '${VERSION}'"

    rm -f "$VERIFY_BIN"
else
    RESULTS+=("FAIL: build with version ldflags")
    ((FAIL++)) || true
    rm -f "$VERIFY_BIN"
fi

echo ""

# ---------------------------------------------------------------------------
# 7. CI Workflow Checks
# ---------------------------------------------------------------------------
echo "--- CI Checks ---"

check "ci.yml exists" test -f .github/workflows/ci.yml
check "release.yml exists" test -f .github/workflows/release.yml

echo ""

# ---------------------------------------------------------------------------
# Final Report
# ---------------------------------------------------------------------------
echo "=== Results ==="
for result in "${RESULTS[@]}"; do
    echo "  $result"
done
echo ""
echo "Passed: $PASS  Failed: $FAIL  Total: $(( PASS + FAIL ))"

if [[ $FAIL -gt 0 ]]; then
    echo ""
    echo "RELEASE BLOCKED: $FAIL check(s) failed"
    exit 1
fi

echo ""
echo "ALL CHECKS PASSED -- Ready to release $VERSION"
echo "Run: git tag -a $VERSION -m 'Release $VERSION' && git push origin $VERSION"
