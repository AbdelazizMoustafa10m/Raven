# T-002: Config Loading

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 4-8hrs |
| Dependencies | None |
| Blocked By | None |
| Blocks | T-003 |

## Goal
Load and parse TOML configuration files into strongly-typed Go structs.

## Acceptance Criteria
- [ ] Loads raven.toml from the working directory
- [ ] Falls back to ~/.config/raven/raven.toml
- [ ] Returns structured errors for malformed TOML
