# T-087: Final Binary Verification and Release Checklist

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-8hrs |
| Dependencies | T-079, T-080, T-081, T-082, T-083, T-084, T-085, T-086 |
| Blocked By | T-079, T-080, T-081, T-082, T-083, T-084, T-085, T-086 |
| Blocks | None |

## Goal
Perform the final verification of all release artifacts, validate cross-platform binaries, run the complete test suite, verify documentation accuracy, and execute a structured release checklist that ensures Raven v2.0.0 meets all PRD quality targets before publishing. This is the gatekeeper task that confirms everything works together.

## Background
Per PRD Section 4 (Success Metrics), Raven has quantitative targets that must be verified before release: binary size < 25MB, zero runtime dependencies, TUI responsiveness < 100ms per frame, resume < 5 seconds, and time from install to first implement < 3 minutes. Per PRD Section 7 (Phase 7), the deliverable is "Published v2.0.0 with signed binaries for all platforms."

This task is the final convergence point for all Phase 7 work. It depends on every other Phase 7 task and produces the release tag that triggers the automated release workflow (T-080).

## Technical Specifications
### Implementation Approach
Create a `scripts/release-verify.sh` script that automates the verification of all release criteria. Create a `docs/RELEASE_CHECKLIST.md` with the manual verification steps. The verification script checks binary sizes, runs the test suite, validates documentation, and produces a pass/fail report. After all checks pass, the release is created by pushing a git tag.

### Key Components
- **`scripts/release-verify.sh`**: Automated release verification script
- **`docs/RELEASE_CHECKLIST.md`**: Manual release checklist with sign-off fields
- **Makefile `release-verify` target**: Convenience target for running verification
- **Makefile `release` target**: Creates and pushes the version tag

### API/Interface Contracts
```bash
#!/usr/bin/env bash
# scripts/release-verify.sh
# Automated release verification for Raven
# Usage: ./scripts/release-verify.sh [version]
# Example: ./scripts/release-verify.sh v2.0.0
set -euo pipefail

VERSION="${1:-$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0-dev")}"
PASS=0
FAIL=0
RESULTS=()

check() {
    local name="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        RESULTS+=("PASS: $name")
        ((PASS++))
    else
        RESULTS+=("FAIL: $name")
        ((FAIL++))
    fi
}

echo "=== Raven Release Verification: $VERSION ==="
echo ""

# 1. Build verification
echo "--- Build Checks ---"
check "go build" go build ./cmd/raven/
check "go vet" go vet ./...
check "go mod tidy (no diff)" bash -c "go mod tidy && git diff --exit-code go.mod go.sum"

# 2. Test verification
echo "--- Test Checks ---"
check "unit tests" go test -race ./...
check "e2e tests" go test -v ./tests/e2e/ -timeout 10m

# 3. Binary size verification
echo "--- Binary Size Checks ---"
for GOOS in darwin linux windows; do
    for GOARCH in amd64 arm64; do
        if [[ "$GOOS" == "windows" && "$GOARCH" == "arm64" ]]; then
            continue
        fi
        BINARY="/tmp/raven-${GOOS}-${GOARCH}"
        CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -o "$BINARY" ./cmd/raven
        SIZE=$(stat -c%s "$BINARY" 2>/dev/null || stat -f%z "$BINARY")
        MAX=$((25 * 1024 * 1024))
        check "binary size ${GOOS}/${GOARCH} ($(( SIZE / 1024 / 1024 ))MB < 25MB)" \
            test "$SIZE" -lt "$MAX"
        rm -f "$BINARY"
    done
done

# 4. GoReleaser verification
echo "--- GoReleaser Checks ---"
check "goreleaser check" goreleaser check
check "goreleaser build --snapshot" goreleaser build --snapshot --clean

# 5. Documentation verification
echo "--- Documentation Checks ---"
check "README.md exists" test -f README.md
check "LICENSE exists" test -f LICENSE
check "CONTRIBUTING.md exists" test -f CONTRIBUTING.md
check "man pages generate" go run scripts/gen-manpages/main.go /tmp/raven-man
check "completions generate" go run scripts/gen-completions/main.go /tmp/raven-completions

# 6. Version injection verification
echo "--- Version Injection Checks ---"
BINARY="/tmp/raven-verify"
go build -ldflags="-s -w -X raven/internal/buildinfo.Version=${VERSION}" -o "$BINARY" ./cmd/raven
check "version output contains version" bash -c "$BINARY version | grep -q '${VERSION}'"
rm -f "$BINARY"

# 7. CI workflow verification
echo "--- CI Checks ---"
check "ci.yml exists" test -f .github/workflows/ci.yml
check "release.yml exists" test -f .github/workflows/release.yml

# Report
echo ""
echo "=== Results ==="
for result in "${RESULTS[@]}"; do
    echo "  $result"
done
echo ""
echo "Passed: $PASS  Failed: $FAIL  Total: $((PASS + FAIL))"

if [[ $FAIL -gt 0 ]]; then
    echo ""
    echo "RELEASE BLOCKED: $FAIL check(s) failed"
    exit 1
fi

echo ""
echo "ALL CHECKS PASSED -- Ready to release $VERSION"
echo "Run: git tag -a $VERSION -m 'Release $VERSION' && git push origin $VERSION"
```

```markdown
# docs/RELEASE_CHECKLIST.md
# Raven Release Checklist

## Pre-Release Verification

### Automated Checks
- [ ] `make release-verify VERSION=vX.Y.Z` passes all checks
- [ ] All CI checks pass on the main branch
- [ ] GoReleaser snapshot build produces all 5 platform binaries

### Binary Verification
- [ ] All binaries are under 25MB
- [ ] `raven version` shows correct version on each platform
- [ ] `raven --help` displays all commands
- [ ] CGO_ENABLED=0 verified (no C dependencies)

### Functional Verification (Manual)
- [ ] `raven init go-cli` creates valid project structure
- [ ] `raven config debug` shows resolved configuration
- [ ] `raven implement --task T-001 --dry-run` shows prompt
- [ ] `raven review --dry-run` shows review plan
- [ ] `raven pipeline --phase 1 --dry-run` shows pipeline steps
- [ ] `raven dashboard` launches TUI without errors
- [ ] Shell completions work for bash/zsh/fish
- [ ] Man pages display correctly with `man raven`

### Documentation Verification
- [ ] README.md is complete and accurate
- [ ] All command examples work as documented
- [ ] Installation instructions are correct
- [ ] Quick start guide works for a new user
- [ ] CONTRIBUTING.md has development setup instructions
- [ ] LICENSE file is present (MIT)

### Performance Verification
- [ ] `make bench` results within acceptable ranges
- [ ] Startup time < 200ms
- [ ] TUI frame render < 100ms at 120x40

## Release Process

1. Ensure main branch is clean and all CI passes
2. Run `make release-verify VERSION=vX.Y.Z`
3. Create annotated tag: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
4. Push tag: `git push origin vX.Y.Z`
5. Wait for GitHub Actions release workflow to complete
6. Verify GitHub Release page:
   - [ ] Release notes are correct
   - [ ] All 5 platform archives are present
   - [ ] Checksums file is present
   - [ ] Completions archive is present
7. Download and verify one binary from the release page
8. Update any external references (blog posts, announcements)

## Post-Release

- [ ] Verify `go install` works with the tagged version
- [ ] Create GitHub Discussion or announcement
- [ ] Update project roadmap for next version
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| GoReleaser | v2.6+ | Release build verification |
| Go | 1.24+ | Build toolchain |
| bash | 4.0+ | Verification script |
| git | 2.0+ | Tag creation |

## Acceptance Criteria
- [ ] `scripts/release-verify.sh` exists and runs all verification checks
- [ ] `docs/RELEASE_CHECKLIST.md` exists with complete pre-release, release, and post-release sections
- [ ] Verification script checks binary size for all 5 platforms against 25MB limit
- [ ] Verification script runs full test suite (unit + E2E)
- [ ] Verification script validates GoReleaser configuration
- [ ] Verification script checks documentation files exist
- [ ] Verification script verifies version injection via ldflags
- [ ] Verification script produces clear pass/fail report
- [ ] `make release-verify` target invokes the verification script
- [ ] `make release` target creates and pushes a git tag (with confirmation prompt)
- [ ] All verification checks pass on the current main branch
- [ ] Release checklist covers manual verification steps not in the script

## Testing Requirements
### Unit Tests
- No Go unit tests (shell script and documentation)

### Integration Tests
- Run `scripts/release-verify.sh v0.0.1-test` and verify all checks pass
- Verify the script correctly reports failures (introduce a deliberate failure)
- Verify Makefile targets work (`make release-verify`, `make release`)

### Edge Cases to Handle
- Running verification without GoReleaser installed (skip GoReleaser checks with warning)
- Running on macOS vs Linux (different `stat` syntax for file size)
- Module path differences between development and CI environments
- Version string format validation (must match `vX.Y.Z` or `vX.Y.Z-qualifier`)
- Git tag already exists (error with clear message)

## Implementation Notes
### Recommended Approach
1. Create `scripts/release-verify.sh` with all automated checks
2. Create `docs/RELEASE_CHECKLIST.md` with manual steps
3. Add Makefile targets:
   - `release-verify`: `./scripts/release-verify.sh $(VERSION)`
   - `release`: Prompt for confirmation, then `git tag -a $(VERSION) -m "Release $(VERSION)" && git push origin $(VERSION)`
4. Run the verification script and fix any failing checks
5. Walk through the manual checklist items
6. Perform a dry run of the full release process with a test tag (e.g., `v0.0.1-test`)
7. Delete the test release and tag
8. Document the process in CONTRIBUTING.md

### Potential Pitfalls
- The `stat` command differs between macOS (`stat -f%z`) and Linux (`stat -c%s`). The script handles both.
- GoReleaser snapshot builds create artifacts in a `dist/` directory -- ensure this is in `.gitignore`.
- The ldflags `-X` path must match the module path exactly. If the module is renamed, the verification will fail silently (version will show empty).
- E2E tests in the verification script may be slow (up to 5 minutes). Consider a `--quick` flag that skips E2E.
- The `goreleaser build --snapshot` command requires GoReleaser to be installed. CI environments may need to install it first.

### Security Considerations
- The release process should require a signed git tag (consider GPG signing for v2.1)
- Verification script does not access external networks (except `go mod tidy` which may download modules)
- Release binaries should be verified by consumers using the checksums file
- The release workflow (T-080) uses `GITHUB_TOKEN` with minimal permissions

## References
- [GoReleaser Quick Start](https://goreleaser.com/quick-start/)
- [Semantic Versioning](https://semver.org/)
- [Git Tagging](https://git-scm.com/book/en/v2/Git-Basics-Tagging)
- [Go Module Release Best Practices](https://go.dev/doc/modules/release-workflow)
- [PRD Section 4: Success Metrics](docs/prd/PRD-Raven.md)
- [PRD Section 7: Phase 7 Deliverable](docs/prd/PRD-Raven.md)