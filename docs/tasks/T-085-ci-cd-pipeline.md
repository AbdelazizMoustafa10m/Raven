# T-085: CI/CD Pipeline with GitHub Actions

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-001, T-002 |
| Blocked By | T-001 |
| Blocks | T-080 |

## Goal
Create a GitHub Actions CI/CD workflow that runs on every pull request and push to the main branch, performing build verification, static analysis (`go vet`), linting (`golangci-lint`), unit tests, and integration tests. This ensures code quality gates are enforced before merging and provides the foundation that the release workflow (T-080) depends on.

## Background
Per PRD Section 7 (Phase 7), Raven requires CI/CD automation as part of the distribution infrastructure. The PRD specifies verification commands (`go build ./cmd/raven/`, `go vet ./...`, `go test ./...`, `go mod tidy`) in the CLAUDE.md conventions. The CI pipeline codifies these checks as required status checks for pull requests.

The pipeline uses `golangci-lint` (the de facto Go linting aggregator) which bundles `govet`, `staticcheck`, `gosec`, `errcheck`, and dozens of other linters into a single tool. The official `golangci/golangci-lint-action@v6` (latest stable as of 2025) is recommended for GitHub Actions integration.

## Technical Specifications
### Implementation Approach
Create `.github/workflows/ci.yml` that triggers on pull requests and pushes to `main`. The workflow runs three jobs: (1) `lint` using golangci-lint, (2) `test` running unit and integration tests across Go versions, and (3) `build` verifying the binary compiles for all target platforms. Create a `.golangci.yml` configuration file at the project root for linter settings.

### Key Components
- **`.github/workflows/ci.yml`**: Main CI workflow for PRs and main branch
- **`.golangci.yml`**: golangci-lint configuration file with enabled linters and settings
- **Lint job**: Runs golangci-lint with project-specific configuration
- **Test job**: Runs `go test` with race detection and coverage reporting
- **Build job**: Verifies cross-compilation for all 5 target platforms
- **Module hygiene job**: Verifies `go mod tidy` produces no diff

### API/Interface Contracts
```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

permissions:
  contents: read

jobs:
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v1.63
          args: --timeout=5m

  test:
    name: Test
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
        go-version: ["1.24"]
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go-version }}
          cache: true

      - name: Run go vet
        run: go vet ./...

      - name: Run tests
        run: go test -race -coverprofile=coverage.out -covermode=atomic ./...

      - name: Check coverage
        run: |
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
          echo "Total coverage: ${COVERAGE}%"
          # Fail if coverage drops below threshold (adjust as project matures)
          # awk "BEGIN {exit ($COVERAGE < 60) ? 1 : 0}"

      - name: Upload coverage
        if: matrix.os == 'ubuntu-latest'
        uses: actions/upload-artifact@v4
        with:
          name: coverage-report
          path: coverage.out

  build:
    name: Build
    runs-on: ubuntu-latest
    strategy:
      matrix:
        goos: [darwin, linux, windows]
        goarch: [amd64, arm64]
        exclude:
          - goos: windows
            goarch: arm64
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Build
        env:
          CGO_ENABLED: "0"
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: |
          go build -ldflags="-s -w" -o /dev/null ./cmd/raven

  mod-tidy:
    name: Module Hygiene
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Check go mod tidy
        run: |
          go mod tidy
          git diff --exit-code go.mod go.sum
```

```yaml
# .golangci.yml
run:
  timeout: 5m
  modules-download-mode: readonly

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gofmt
    - goimports
    - misspell
    - unconvert
    - unparam
    - gosec
    - bodyclose
    - noctx
    - errorlint
    - exhaustive
    - prealloc

linters-settings:
  errcheck:
    check-type-assertions: true
  govet:
    enable-all: true
  goimports:
    local-prefixes: raven
  misspell:
    locale: US
  errorlint:
    errorf: true
    asserts: true
    comparison: true
  gosec:
    excludes:
      - G104  # Unhandled errors (errcheck covers this)

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gosec
        - errcheck
    - path: mock
      linters:
        - unused
  max-issues-per-linter: 50
  max-same-issues: 10
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| golangci/golangci-lint-action | v6 | Official golangci-lint GitHub Action |
| golangci-lint | v1.63+ | Go linter aggregator |
| actions/checkout | v4 | Repository checkout |
| actions/setup-go | v5 | Go toolchain setup |
| actions/upload-artifact | v4 | Coverage report upload |

## Acceptance Criteria
- [ ] `.github/workflows/ci.yml` exists and is valid YAML
- [ ] CI triggers on PRs to `main` and pushes to `main`
- [ ] Lint job runs golangci-lint with project-specific `.golangci.yml`
- [ ] Test job runs on both Ubuntu and macOS
- [ ] Tests run with `-race` flag for race condition detection
- [ ] Coverage report is generated and uploaded as artifact
- [ ] Build job verifies compilation for all 5 target platforms (CGO_ENABLED=0)
- [ ] Module hygiene check verifies `go mod tidy` is clean
- [ ] `.golangci.yml` enables relevant linters (errcheck, gosec, staticcheck, etc.)
- [ ] Test files are excluded from security linting (gosec)
- [ ] All jobs pass on a clean checkout of the main branch
- [ ] CI runs complete in under 10 minutes total
- [ ] Workflow has read-only `contents` permission (principle of least privilege)

## Testing Requirements
### Unit Tests
- No Go unit tests (workflow is declarative YAML)

### Integration Tests
- Push a PR and verify all CI checks run
- Introduce a deliberate `go vet` warning and verify lint job catches it
- Introduce a failing test and verify test job fails
- Verify cross-compilation builds complete for all 5 platforms
- Run `go mod tidy` and verify module hygiene check passes on clean state

### Edge Cases to Handle
- Go module cache: `actions/setup-go` with `cache: true` caches `~/go/pkg/mod` and `~/.cache/go-build`
- golangci-lint version pinning: use a specific version (e.g., `v1.63`) rather than `latest` for reproducibility
- macOS runners are slower and more expensive; consider running only critical tests there
- Windows testing: excluded for now (mock agents are bash scripts); add in v2.1 if needed
- Flaky tests: use `-count=1` to disable test caching in CI if flakiness is observed

## Implementation Notes
### Recommended Approach
1. Create `.github/workflows/` directory
2. Write `ci.yml` with the configuration above
3. Create `.golangci.yml` at project root
4. Run `golangci-lint run` locally to verify configuration and fix any existing issues
5. Push to a branch and open a PR to verify CI runs
6. Iterate on linter configuration until all jobs pass
7. Set CI status checks as required for PRs to `main`
8. Add a CI status badge to the README (reference in T-086)

### Potential Pitfalls
- golangci-lint action recommends running in a separate job from `go test` because linting and testing have different failure modes and resource requirements
- The `go-version-file: go.mod` approach requires the `go` directive in `go.mod` to be correct (e.g., `go 1.24`); if it says `go 1.24.0`, the action will look for that exact patch version
- Race detection (`-race`) doubles memory usage and significantly slows tests. For very large test suites, consider splitting into parallel shards.
- golangci-lint version should be pinned to avoid surprise breaking changes. Update quarterly via Dependabot or manual PRs.
- macOS runners in GitHub Actions are more expensive (10x cost multiplier). Consider running macOS tests only on main branch, not on every PR.

### Security Considerations
- CI workflow has read-only `contents` permission -- minimum required for checkout and test
- golangci-lint includes `gosec` for security-focused static analysis
- No secrets or tokens required for CI (unlike the release workflow)
- Dependencies are verified via `go mod tidy` check (ensures no unauthorized dependency changes)
- Consider adding `go vuln check` (Go vulnerability scanning) as an additional step

## References
- [golangci-lint GitHub Action](https://github.com/golangci/golangci-lint-action)
- [golangci-lint Configuration](https://golangci-lint.run/usage/configuration/)
- [GitHub Actions for Go Projects](https://docs.github.com/en/actions/automating-builds-and-tests/building-and-testing-go)
- [Go Linting Best Practices for CI/CD](https://medium.com/@tedious/go-linting-best-practices-for-ci-cd-with-github-actions-aa6d96e0c509)
- [actions/setup-go Documentation](https://github.com/actions/setup-go)