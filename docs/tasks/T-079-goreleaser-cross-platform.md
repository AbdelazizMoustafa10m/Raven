# T-079: GoReleaser Configuration for Cross-Platform Builds

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-001, T-002, T-003 |
| Blocked By | T-003 |
| Blocks | T-080, T-081, T-087 |

## Goal
Create a GoReleaser v2 configuration file (`.goreleaser.yaml`) that builds Raven for all target platforms (macOS amd64/arm64, Linux amd64/arm64, Windows amd64) with CGO disabled, ldflags for version injection, and archive packaging. This is the foundation for automated release distribution.

## Background
Per PRD Section 7 (Phase 7) and PRD Section 4 (Success Metrics), Raven must compile to a single, zero-dependency binary under 25MB for macOS, Linux, and Windows. The PRD specifies CGO_ENABLED=0 for pure Go cross-compilation and lists `.goreleaser.yml` in the project structure (Section 6.2). GoReleaser v2 is the industry standard for Go project releases, used by projects like goreleaser itself, nfpm, and hundreds of other Go CLIs.

The `internal/buildinfo/buildinfo.go` package (T-003) provides version, commit, and build date variables that are injected via `-ldflags` at build time. GoReleaser must configure these ldflags to match the buildinfo package's expected variable names.

## Technical Specifications
### Implementation Approach
Create `.goreleaser.yaml` at the project root using GoReleaser v2 schema (`version: 2`). Configure a single build targeting five OS/arch combinations with CGO disabled. Use `ldflags` to inject version information into the `internal/buildinfo` package. Configure archives as `.tar.gz` for Unix platforms and `.zip` for Windows. Include a checksum file for integrity verification.

### Key Components
- **`.goreleaser.yaml`**: GoReleaser v2 configuration file at project root
- **Build configuration**: Single build entry for `cmd/raven` targeting 5 platform combinations
- **Archive configuration**: Platform-appropriate archive formats with consistent naming
- **Checksum configuration**: SHA256 checksums for all artifacts
- **Changelog configuration**: Auto-generated changelog from git history

### API/Interface Contracts
```yaml
# .goreleaser.yaml
version: 2

project_name: raven

before:
  hooks:
    - go mod tidy
    - go vet ./...

builds:
  - id: raven
    main: ./cmd/raven
    binary: raven
    env:
      - CGO_ENABLED=0
    goos:
      - darwin
      - linux
      - windows
    goarch:
      - amd64
      - arm64
    ignore:
      - goos: windows
        goarch: arm64
    ldflags:
      - -s -w
      - -X raven/internal/buildinfo.Version={{.Version}}
      - -X raven/internal/buildinfo.Commit={{.Commit}}
      - -X raven/internal/buildinfo.Date={{.Date}}

archives:
  - id: raven-archive
    builds:
      - raven
    format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^ci:"
      - "^chore:"

release:
  github:
    owner: "{{ .Env.GITHUB_OWNER }}"
    name: raven
  draft: false
  prerelease: auto
  name_template: "v{{ .Version }}"
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| GoReleaser | v2.6+ | Cross-platform build and release automation |
| Go | 1.24+ | Build toolchain |
| internal/buildinfo | - | Version/commit/date variables for ldflags injection |

## Acceptance Criteria
- [ ] `.goreleaser.yaml` exists at project root with `version: 2` schema
- [ ] `goreleaser check` passes without warnings
- [ ] `goreleaser build --snapshot --clean` produces binaries for all 5 targets (darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64)
- [ ] All produced binaries have CGO_ENABLED=0 (verified via `go version -m <binary>`)
- [ ] Binary size is under 25MB for all platforms (verify with `ls -lh`)
- [ ] Version information is correctly injected (run binary with `version` command to verify)
- [ ] Archives use `.tar.gz` for macOS/Linux and `.zip` for Windows
- [ ] SHA256 checksums file is generated
- [ ] `goreleaser release --snapshot --clean` completes successfully (local dry-run)
- [ ] Makefile updated with `release-snapshot` target that invokes `goreleaser build --snapshot --clean`

## Testing Requirements
### Unit Tests
- No new Go unit tests required (GoReleaser config is declarative YAML)

### Integration Tests
- Run `goreleaser check` in CI to validate config syntax
- Run `goreleaser build --snapshot --clean` and verify all 5 binaries exist
- Verify binary sizes are under 25MB threshold
- Run each binary's `version` command and verify ldflags injection

### Edge Cases to Handle
- Module path must match ldflags `-X` package paths exactly
- Windows binary must have `.exe` extension automatically (GoReleaser handles this)
- arm64 builds on macOS must work for both Apple Silicon native and Rosetta 2
- Snapshot builds should use a dev version tag (e.g., `0.0.0-SNAPSHOT`)

## Implementation Notes
### Recommended Approach
1. Verify the exact Go module path by checking `go.mod` (needed for ldflags `-X` paths)
2. Verify the exact variable names in `internal/buildinfo/buildinfo.go` (Version, Commit, Date)
3. Create `.goreleaser.yaml` with the configuration above, adjusting module paths as needed
4. Run `goreleaser check` to validate the configuration
5. Run `goreleaser build --snapshot --clean` to test a local build
6. Verify binary sizes with `du -sh dist/*/raven*`
7. Test version injection by running `dist/raven_darwin_arm64_v8.0/raven version`
8. Add a `release-snapshot` Makefile target
9. Add `.goreleaser.yaml` linting to the pre-commit or CI checks

### Potential Pitfalls
- The ldflags `-X` paths must use the full Go module path (e.g., `github.com/user/raven/internal/buildinfo.Version`), not a relative path. Check `go.mod` for the module name.
- GoReleaser v2 requires `version: 2` at the top of the YAML file; without it, GoReleaser will use v1 schema and may reject v2-only features.
- The `-s -w` ldflags strip debug symbols and DWARF tables, reducing binary size by approximately 25-30%. This is intentional for release builds but may complicate debugging -- keep a `build-debug` Makefile target without these flags.
- GoReleaser's `before.hooks` run before every build -- ensure `go mod tidy` and `go vet` do not fail in a clean checkout.

### Security Considerations
- Checksums file provides integrity verification for downloaded artifacts
- The `-s` ldflag strips symbol tables, making reverse engineering slightly harder
- Consider adding GPG signing in a future iteration (GoReleaser supports `signs` configuration)
- The `release.prerelease: auto` setting will mark releases with pre-release tags (e.g., `v2.0.0-rc.1`) as pre-releases on GitHub

## References
- [GoReleaser v2 Announcement](https://goreleaser.com/blog/goreleaser-v2/)
- [GoReleaser Build Customization](https://goreleaser.com/customization/builds/go/)
- [GoReleaser Archive Customization](https://goreleaser.com/customization/archive/)
- [GoReleaser Quick Start](https://goreleaser.com/quick-start/)
- [Go Binary Size Optimization](https://www.codingexplorations.com/blog/reducing-binary-size-in-go-strip-unnecessary-data-with-ldflags-w-s)
- [GoReleaser Example Repository](https://github.com/goreleaser/example/blob/master/.goreleaser.yaml)