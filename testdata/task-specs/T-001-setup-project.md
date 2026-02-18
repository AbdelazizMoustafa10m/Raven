# T-001: Setup Project

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 1-3hrs |
| Dependencies | None |
| Blocked By | None |
| Blocks | T-002, T-003 |

## Goal
Initialize the Go project with module, directory structure, entry point,
and core dependency declarations.

## Acceptance Criteria
- [ ] go.mod exists with module path and Go 1.24+ directive
- [ ] go build ./cmd/raven/ succeeds
- [ ] go vet ./... passes with zero warnings
