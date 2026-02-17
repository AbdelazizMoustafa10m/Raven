# T-080: GitHub Actions Release Automation Workflow

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-079, T-085 |
| Blocked By | T-079 |
| Blocks | T-087 |

## Goal
Create a GitHub Actions workflow that automatically builds and publishes Raven releases to GitHub Releases when a version tag is pushed. The workflow triggers on `v*` tags, runs GoReleaser to produce cross-platform binaries, generates checksums, and publishes all artifacts as a GitHub Release with an auto-generated changelog.

## Background
Per PRD Section 7 (Phase 7), Raven requires "GitHub Release automation" to produce a published v2.0.0 release with binaries for all platforms. The GoReleaser configuration from T-079 defines the build matrix and archive formats. This task creates the GitHub Actions workflow that invokes GoReleaser in CI, handling authentication, artifact upload, and release creation automatically.

The workflow uses `goreleaser/goreleaser-action@v6` (the official GitHub Action) which supports GoReleaser v2. The action requires full git history (`fetch-depth: 0`) for changelog generation and a `GITHUB_TOKEN` for creating releases and uploading artifacts.

## Technical Specifications
### Implementation Approach
Create `.github/workflows/release.yml` that triggers on tag pushes matching `v*`. The workflow checks out the repository with full history, sets up Go 1.24+, and runs GoReleaser via the official action. The `GITHUB_TOKEN` is passed for GitHub Release creation. The workflow also verifies binary sizes are under 25MB before publishing.

### Key Components
- **`.github/workflows/release.yml`**: Release automation workflow triggered by version tags
- **GoReleaser step**: Invokes GoReleaser v2 with the project's `.goreleaser.yaml`
- **Size verification step**: Checks all produced binaries are under 25MB
- **Artifact upload**: GoReleaser handles uploading to GitHub Releases

### API/Interface Contracts
```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Run tests
        run: |
          go vet ./...
          go test ./...

      - name: Verify binary sizes
        run: |
          CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /tmp/raven-check ./cmd/raven
          SIZE=$(stat -c%s /tmp/raven-check 2>/dev/null || stat -f%z /tmp/raven-check)
          MAX_SIZE=$((25 * 1024 * 1024))
          if [ "$SIZE" -gt "$MAX_SIZE" ]; then
            echo "ERROR: Binary size ${SIZE} bytes exceeds 25MB limit"
            exit 1
          fi
          echo "Binary size check passed: ${SIZE} bytes"

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| goreleaser/goreleaser-action | v6 | Official GoReleaser GitHub Action |
| actions/checkout | v4 | Repository checkout with full history |
| actions/setup-go | v5 | Go toolchain setup |
| GoReleaser | ~> v2 | Cross-platform build and release |

## Acceptance Criteria
- [ ] `.github/workflows/release.yml` exists and is valid YAML
- [ ] Workflow triggers only on `v*` tag pushes (not branches, not PRs)
- [ ] Workflow checks out with `fetch-depth: 0` for changelog generation
- [ ] Go version is read from `go.mod` (not hardcoded)
- [ ] Tests and vet pass before release build
- [ ] Binary size verification runs before GoReleaser
- [ ] GoReleaser produces artifacts for all 5 target platforms
- [ ] GitHub Release is created with auto-generated changelog
- [ ] Release includes archives (`.tar.gz` for Unix, `.zip` for Windows) and checksum file
- [ ] Workflow has `contents: write` permission for release creation
- [ ] A manual test tag push (e.g., `v0.0.1-test`) successfully creates a pre-release

## Testing Requirements
### Unit Tests
- No Go unit tests (workflow is declarative YAML)

### Integration Tests
- Push a test tag (`v0.0.1-test`) and verify the workflow runs successfully
- Verify all 5 platform binaries are present in the GitHub Release
- Verify checksums file is present and correct
- Verify the release changelog is auto-generated from commit messages
- Verify pre-release tags (e.g., `v2.0.0-rc.1`) are marked as pre-releases

### Edge Cases to Handle
- Tag format validation: only `v*` tags trigger the workflow (not arbitrary tags)
- Failed tests should prevent release publication
- Binary size exceeding 25MB should fail the workflow before GoReleaser runs
- Network issues during artifact upload (GoReleaser retries internally)
- Concurrent tag pushes should not conflict (GitHub Actions handles job isolation)

## Implementation Notes
### Recommended Approach
1. Create the `.github/workflows/` directory structure
2. Write `release.yml` with the configuration above
3. Ensure `.goreleaser.yaml` from T-079 is committed and passes `goreleaser check`
4. Test locally with `act` (GitHub Actions local runner) if available, or push a test tag
5. Push a test tag like `v0.0.1-test` to verify the full pipeline
6. Delete the test release and tag after verification
7. Document the release process in the project README (T-086)

### Potential Pitfalls
- The `GITHUB_TOKEN` provided by GitHub Actions has limited permissions (only the current repository). If the project moves to an organization, the token may need additional scopes.
- The `fetch-depth: 0` is critical -- without it, GoReleaser cannot generate the changelog from git history and will fail.
- The `go-version-file: go.mod` approach reads the Go version from the `go` directive in `go.mod`. Ensure `go.mod` has a valid `go 1.24` (or higher) directive.
- GoReleaser `~> v2` version constraint ensures compatibility with the v2 config schema. Pinning to an exact version (e.g., `v2.6.1`) is more reproducible but requires manual updates.
- Binary size check uses Linux `stat` format; the workflow runs on `ubuntu-latest` so this is fine.

### Security Considerations
- The `GITHUB_TOKEN` is automatically provided by GitHub Actions with scoped permissions; no need for personal access tokens
- The `contents: write` permission is the minimum required for creating releases
- GoReleaser checksums provide integrity verification for consumers
- Consider adding Sigstore cosign signing in a future iteration for supply-chain security
- Dependabot should be configured to keep action versions updated (security patches)

## References
- [GoReleaser GitHub Actions Documentation](https://goreleaser.com/ci/actions/)
- [goreleaser/goreleaser-action Repository](https://github.com/goreleaser/goreleaser-action)
- [GitHub Actions: Automatic Token Authentication](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication)
- [actions/setup-go Documentation](https://github.com/actions/setup-go)
- [GoReleaser Changelog Customization](https://goreleaser.com/customization/changelog/)