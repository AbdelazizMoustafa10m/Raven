# T-086: Comprehensive README and User Documentation

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-006, T-079, T-080, T-081, T-082, T-085 |
| Blocked By | T-079 |
| Blocks | T-087 |

## Goal
Create a comprehensive README.md and supporting documentation that covers installation, quick start, all CLI commands with examples, configuration reference, architecture overview, and contribution guidelines. The README is the primary entry point for new users and must enable the PRD's target of "Time from install to first `raven implement` < 3 minutes."

## Background
Per PRD Section 4 (Success Metrics), Raven targets "Time from install to first `raven implement` < 3 minutes" and "Time to configure a new project < 5 minutes with `raven init`." Per PRD Section 7 (Phase 7), the project requires a "Comprehensive README with usage examples." The README must serve multiple audiences: power users who want the full reference, new users who want to get started quickly, and CI integrators who want headless automation examples.

The documentation should follow the pattern of well-regarded Go CLI projects like `gh` (GitHub CLI), `lazygit`, and `cobra` itself, which provide clear installation instructions, usage examples, and configuration references.

## Technical Specifications
### Implementation Approach
Create `README.md` at the project root with structured sections following the diataxis documentation framework (tutorials, how-to guides, reference, explanation). Create a `docs/` subdirectory for extended documentation that the README links to. Include CI badges, a screencast/screenshot placeholder, and copy-pasteable examples for every major workflow.

### Key Components
- **`README.md`**: Main project documentation (primary deliverable)
- **`docs/configuration.md`**: Full `raven.toml` configuration reference
- **`docs/workflows.md`**: Workflow engine documentation and custom workflow guide
- **`docs/agents.md`**: Agent adapter documentation and configuration
- **`CONTRIBUTING.md`**: Contribution guidelines, development setup, code standards
- **`LICENSE`**: MIT license file

### API/Interface Contracts
```markdown
# README.md structure

# Raven

> Your AI command center.

[![CI](https://github.com/OWNER/raven/actions/workflows/ci.yml/badge.svg)](...)
[![Release](https://github.com/OWNER/raven/actions/workflows/release.yml/badge.svg)](...)
[![Go Report Card](https://goreportcard.com/badge/github.com/OWNER/raven)](...)

[Screenshot/GIF of TUI dashboard]

## What is Raven?

[2-3 paragraph overview matching PRD executive summary]

## Features

- [Bullet list of key features with brief descriptions]

## Installation

### Binary Downloads
### Homebrew (future)
### From Source

## Quick Start

### 1. Initialize a Project
### 2. Implement Your First Task
### 3. Run a Review
### 4. Create a PR

## Commands

### raven implement
### raven review
### raven fix
### raven pr
### raven pipeline
### raven prd
### raven status
### raven resume
### raven init
### raven config
### raven dashboard
### raven version
### raven completion

## Configuration

### raven.toml Reference
### Agent Configuration
### Workflow Configuration

## Architecture

[Brief overview with link to extended docs]

## Shell Completions

### Bash
### Zsh
### Fish
### PowerShell

## Man Pages

## CI/CD Integration

## Contributing

## License
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| N/A (documentation only) | - | - |

## Acceptance Criteria
- [ ] `README.md` exists at project root with all sections populated
- [ ] Installation section includes binary download, source build, and completion setup
- [ ] Quick Start section enables a new user to go from install to `raven implement` in under 3 minutes (as per PRD target)
- [ ] Every CLI command has a dedicated section with synopsis, description, flags, and at least one example
- [ ] Configuration section documents all `raven.toml` fields with defaults and descriptions
- [ ] CI badges are included (CI workflow, release workflow, Go report card)
- [ ] A placeholder for TUI screenshot/GIF is included (to be filled after TUI is complete)
- [ ] Shell completion installation instructions are provided for all 4 shells
- [ ] Man page installation instructions are included
- [ ] CI/CD integration section shows how to use Raven in GitHub Actions
- [ ] `CONTRIBUTING.md` documents development setup, testing, and code standards
- [ ] `LICENSE` file contains MIT license text
- [ ] `docs/configuration.md` provides exhaustive `raven.toml` reference
- [ ] All code examples in README are copy-pasteable and correct
- [ ] No broken links in any documentation files
- [ ] Documentation passes a spell checker (run `aspell` or equivalent)

## Testing Requirements
### Unit Tests
- No Go unit tests (documentation is prose)

### Integration Tests
- Verify all command examples in the README actually work against the built binary
- Verify installation instructions produce a working binary
- Verify quick start steps can be followed by a new user in under 3 minutes (manual test)
- Verify all internal links in documentation resolve correctly
- Run `markdownlint` on all markdown files to verify formatting

### Edge Cases to Handle
- Users on different platforms (macOS, Linux, Windows) need platform-specific instructions
- Shell completion instructions differ per shell and OS
- GitHub owner/repo placeholders must be updated when the repository URL is finalized
- Configuration examples must match the current `raven.toml` schema exactly

## Implementation Notes
### Recommended Approach
1. Start with the README skeleton (headers and section structure)
2. Write the "What is Raven?" section from the PRD executive summary
3. Write installation instructions (binary downloads from GitHub Releases, from source)
4. Write the Quick Start tutorial (most important for user adoption)
5. Document each CLI command with `raven <command> --help` output and examples
6. Write the configuration reference from the config struct definitions
7. Add CI badges (placeholder URLs, update when repo is public)
8. Write CONTRIBUTING.md with development setup instructions
9. Create LICENSE file with MIT license text
10. Create extended docs (configuration.md, workflows.md, agents.md)
11. Review all examples by running them against the actual binary
12. Add screenshot/GIF placeholder for TUI dashboard

### Potential Pitfalls
- README should not be too long -- link to extended docs for detailed references
- Code examples must be tested against the current version (they will rot otherwise)
- Platform-specific instructions (macOS vs Linux vs Windows) should use tabs or collapsible sections
- GitHub badge URLs depend on the repository owner/name being finalized
- Configuration reference must stay in sync with code changes (consider generating from Go struct tags in v2.1)
- Shell completion instructions vary by OS: macOS zsh vs Linux bash defaults

### Security Considerations
- No API keys, tokens, or secrets should appear in documentation examples
- Example configurations should use placeholder values (e.g., `your-project-name`)
- Installation instructions should verify checksums when downloading binaries
- Document the security model: Raven shells out to external tools, does not store credentials

## References
- [GitHub CLI (gh) README](https://github.com/cli/cli#readme) -- Excellent example of CLI project documentation
- [Cobra Project README](https://github.com/spf13/cobra#readme) -- Go CLI framework documentation
- [Diataxis Documentation Framework](https://diataxis.fr/) -- Tutorial/how-to/reference/explanation structure
- [Make a README](https://www.makeareadme.com/) -- README best practices
- [PRD Section 4: Success Metrics](docs/prd/PRD-Raven.md)