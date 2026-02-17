# T-010: Config Resolution -- CLI > env > file > defaults

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-009 |
| Blocked By | T-009 |
| Blocks | T-011, T-012, T-083, T-084 |

## Goal
Implement the four-layer configuration resolution system that merges values from CLI flags, environment variables, the TOML config file, and built-in defaults in strict priority order: CLI flags > environment variables > raven.toml > defaults. This produces a single, fully-resolved `Config` struct that all subsystems consume.

## Background
Per PRD Section 5.10, Raven's config resolution order is: "CLI flags always override config file values" and "Environment variable overrides with RAVEN_ prefix." Per PRD Section 10, Open Question #7 confirms: "CLI always overrides config. Resolution order: CLI flags > env vars > raven.toml > defaults."

The resolver takes the raw config from `LoadFromFile()` (T-009), the defaults from `NewDefaults()` (T-009), CLI flag values, and environment variables, then produces a fully-resolved config. Each field in the resolved config can be traced back to its source (CLI, env, file, or default) -- this source tracking is used by `raven config debug` (T-012).

## Technical Specifications
### Implementation Approach
Create `internal/config/resolve.go` with a `Resolve()` function that takes CLI overrides, environment reader, file config, and defaults, and returns a resolved config with source annotations. The resolution is field-by-field: for each config field, check CLI override first, then env var, then file value, then default. Use a `ResolvedConfig` wrapper that pairs each value with its source.

### Key Components
- **Resolve()**: Main resolution function that merges all four layers
- **ResolvedConfig**: Config with source annotations for every field
- **ConfigSource**: Enum for value source (CLI, Env, File, Default)
- **CLIOverrides**: Struct capturing CLI flag values that can override config
- **envLookup**: Function type for reading environment variables (testable)

### API/Interface Contracts
```go
// internal/config/resolve.go

// ConfigSource identifies where a configuration value came from.
type ConfigSource string

const (
    SourceDefault ConfigSource = "default"
    SourceFile    ConfigSource = "file"
    SourceEnv     ConfigSource = "env"
    SourceCLI     ConfigSource = "cli"
)

// ResolvedValue pairs a config value with its source.
type ResolvedValue struct {
    Value  interface{}
    Source ConfigSource
}

// ResolvedConfig holds the fully-resolved configuration with source tracking.
// The Config field contains the merged values; Sources tracks where each came from.
type ResolvedConfig struct {
    Config  *Config
    Sources map[string]ConfigSource // key is dotted path, e.g., "project.name"
    Path    string                  // path to the config file used (empty if none)
}

// CLIOverrides captures flag values that can override configuration.
// Nil/zero values mean "not set" (do not override).
type CLIOverrides struct {
    ProjectName *string
    LogDir      *string
    TasksDir    *string
    Verbose     *bool
    Quiet       *bool
    // Agent-specific overrides
    AgentModel  *string
    AgentEffort *string
}

// EnvFunc is a function that looks up environment variables.
// Default implementation is os.LookupEnv. Injected for testability.
type EnvFunc func(key string) (string, bool)

// Resolve merges configuration from all sources in priority order:
// CLI flags > environment variables > config file > defaults.
//
// Parameters:
//   - defaults: built-in default config (from NewDefaults())
//   - fileConfig: parsed config from raven.toml (nil if no file found)
//   - envFn: function to look up environment variables
//   - overrides: CLI flag values (nil fields mean "not set")
//
// Returns the fully-resolved config with source annotations.
func Resolve(defaults *Config, fileConfig *Config, envFn EnvFunc, overrides *CLIOverrides) *ResolvedConfig

// Environment variable mapping:
// RAVEN_PROJECT_NAME       -> project.name
// RAVEN_TASKS_DIR          -> project.tasks_dir
// RAVEN_LOG_DIR            -> project.log_dir
// RAVEN_PROMPT_DIR         -> project.prompt_dir
// RAVEN_BRANCH_TEMPLATE    -> project.branch_template
// RAVEN_AGENT_MODEL        -> agents.*.model (applies to all agents)
// RAVEN_AGENT_EFFORT       -> agents.*.effort (applies to all agents)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| os | stdlib | Environment variable reading (default EnvFunc) |
| strings | stdlib | Environment variable name mapping |

## Acceptance Criteria
- [ ] `Resolve()` correctly applies priority: CLI > env > file > defaults
- [ ] CLI override for project.name takes precedence over env, file, and default
- [ ] Environment variable RAVEN_PROJECT_NAME overrides file and default values
- [ ] File value takes precedence over default
- [ ] Default value is used when no other source provides a value
- [ ] Source tracking correctly identifies the source of each resolved value
- [ ] ResolvedConfig.Sources map contains entries for all resolved fields
- [ ] Nil/zero CLIOverrides fields do not override anything
- [ ] Nil fileConfig (no raven.toml found) falls back to defaults
- [ ] Agent config is merged correctly (file agents merged with defaults)
- [ ] All RAVEN_* environment variables listed in PRD are supported
- [ ] Unit tests achieve 95% coverage
- [ ] `go vet ./...` passes

## Testing Requirements
### Unit Tests
- Resolve with only defaults: all values from defaults, all sources are "default"
- Resolve with file overriding one field: that field source is "file", others are "default"
- Resolve with env overriding file: env takes precedence, source is "env"
- Resolve with CLI overriding env: CLI takes precedence, source is "cli"
- Resolve with all four layers providing different values: CLI wins
- Resolve with nil fileConfig: falls back to defaults
- Resolve with nil CLIOverrides: CLI layer skipped
- Resolve with empty CLIOverrides (all nil fields): CLI layer skipped
- Environment variable RAVEN_PROJECT_NAME sets project.name
- Environment variable RAVEN_TASKS_DIR sets project.tasks_dir
- Environment variable RAVEN_LOG_DIR sets project.log_dir
- Agent config merging: file agents preserved, env overrides applied
- Source map contains correct source for each field

### Integration Tests
- Resolve with real os.LookupEnv and actual raven.toml at project root

### Edge Cases to Handle
- Config file with empty string values (should override default, source is "file")
- Environment variable set to empty string (should it override? Document: yes, empty string is a valid value)
- Multiple agents in config file with env override applying to all
- RAVEN_ env var that maps to a non-existent config field (should be ignored)
- Very long environment variable values (no artificial limit)

## Implementation Notes
### Recommended Approach
1. Create `internal/config/resolve.go`
2. Define `ConfigSource`, `ResolvedValue`, `ResolvedConfig`, `CLIOverrides`, `EnvFunc`
3. Implement `Resolve()` with a clear four-step merge:
   a. Start with `defaults` as the base config
   b. Merge `fileConfig` on top (non-zero values override)
   c. Merge environment variables on top (set values override)
   d. Merge `CLIOverrides` on top (non-nil values override)
4. Track sources for each field as you merge
5. For step (b), use reflection or explicit field-by-field merge. Explicit is preferred for type safety and clarity.
6. Create comprehensive tests with a mock `EnvFunc` that returns predetermined values
7. Verify: `go build ./... && go vet ./... && go test ./internal/config/...`

### Potential Pitfalls
- Using reflection for merging is fragile and hard to maintain. Prefer explicit field-by-field merging, even though it is more verbose. Each field is merged exactly once with clear source tracking.
- The `CLIOverrides` uses pointer types (`*string`, `*bool`) to distinguish "not set" from "set to zero value." A `*string` that is nil means "not overridden"; a `*string` pointing to "" means "override to empty string."
- The `Agents` map requires special handling: file agents should be preserved, not replaced. If the file defines `[agents.claude]` and the default is empty, the resolved config should have claude. If both file and default define claude, file wins.
- Environment variables are ALL_CAPS with RAVEN_ prefix. Map them to the correct dotted config path.

### Security Considerations
- Environment variables may contain sensitive values -- do not log the resolved config at debug level without masking sensitive fields
- The `EnvFunc` injection enables testing without polluting the real environment

## References
- [PRD Section 5.10 - Config Resolution Order](docs/prd/PRD-Raven.md)
- [PRD Section 10 - Open Question #7 (CLI overrides config)](docs/prd/PRD-Raven.md)
- [12-Factor App Configuration](https://12factor.net/config)
- [Viper Configuration Precedence (for reference, Raven does not use Viper)](https://github.com/spf13/viper#why-viper)