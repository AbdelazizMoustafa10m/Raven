# T-009: TOML Configuration Types and Loading with BurntSushi/toml

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-001 |
| Blocked By | T-001 |
| Blocks | T-010, T-011, T-012, T-032, T-033, T-037, T-041, T-048, T-051, T-053 |

## Goal
Define the complete TOML configuration type hierarchy for `raven.toml` and implement the config file loading logic including auto-detection (walking up from CWD to find `raven.toml`). This is the foundation of Raven's configuration system, consumed by nearly every other subsystem.

## Background
Per PRD Section 5.10, Raven's configuration lives in `raven.toml` at the project root. The file is auto-detected by walking up directories from CWD (same pattern as `git` looking for `.git/`). Configuration sections include `[project]`, `[agents.*]`, `[review]`, and `[workflows.*]`. Per PRD Section 6.1, `BurntSushi/toml` v1.5.0 is used for parsing, chosen for its simpler API and `MetaData.Undecoded()` for unknown-key detection.

The config types must map exactly to the TOML structure shown in the PRD, with Go struct tags for `toml:"field_name"`. The loading function returns both the parsed config and the TOML metadata (for unknown-key detection in T-011).

## Technical Specifications
### Implementation Approach
Create `internal/config/config.go` with the full configuration type hierarchy using Go structs with `toml` struct tags. Create `internal/config/load.go` with the `FindConfigFile()` function (walks up from CWD) and `LoadFromFile(path)` function that parses the TOML file. Return both the config struct and `toml.MetaData` for downstream validation. Create `internal/config/defaults.go` with the `NewDefaults()` function that returns a fully populated config with sensible defaults.

### Key Components
- **Config**: Top-level config struct mapping to `raven.toml` root
- **ProjectConfig**: `[project]` section (name, language, paths, verification commands)
- **AgentConfig**: `[agents.*]` section (command, model, effort, prompt template, allowed tools)
- **ReviewConfig**: `[review]` section (extensions, risk patterns, prompt/rules dirs)
- **WorkflowConfig**: `[workflows.*]` section (description, steps, transitions)
- **FindConfigFile()**: Walks up from CWD to find `raven.toml`
- **LoadFromFile()**: Parses TOML file and returns config + metadata
- **NewDefaults()**: Returns config with all default values populated

### API/Interface Contracts
```go
// internal/config/config.go

// Package config provides TOML configuration loading, defaults, resolution, and validation.
package config

// Config is the top-level configuration structure mapping to raven.toml.
type Config struct {
    Project   ProjectConfig            `toml:"project"`
    Agents    map[string]AgentConfig   `toml:"agents"`
    Review    ReviewConfig             `toml:"review"`
    Workflows map[string]WorkflowConfig `toml:"workflows"`
}

// ProjectConfig maps to the [project] section in raven.toml.
type ProjectConfig struct {
    Name                 string   `toml:"name"`
    Language             string   `toml:"language"`
    TasksDir             string   `toml:"tasks_dir"`
    TaskStateFile        string   `toml:"task_state_file"`
    PhasesConf           string   `toml:"phases_conf"`
    ProgressFile         string   `toml:"progress_file"`
    LogDir               string   `toml:"log_dir"`
    PromptDir            string   `toml:"prompt_dir"`
    BranchTemplate       string   `toml:"branch_template"`
    VerificationCommands []string `toml:"verification_commands"`
}

// AgentConfig maps to an [agents.<name>] section in raven.toml.
type AgentConfig struct {
    Command        string `toml:"command"`
    Model          string `toml:"model"`
    Effort         string `toml:"effort"`
    PromptTemplate string `toml:"prompt_template"`
    AllowedTools   string `toml:"allowed_tools"`
}

// ReviewConfig maps to the [review] section in raven.toml.
type ReviewConfig struct {
    Extensions       string `toml:"extensions"`
    RiskPatterns     string `toml:"risk_patterns"`
    PromptsDir       string `toml:"prompts_dir"`
    RulesDir         string `toml:"rules_dir"`
    ProjectBriefFile string `toml:"project_brief_file"`
}

// WorkflowConfig maps to a [workflows.<name>] section in raven.toml.
type WorkflowConfig struct {
    Description string                         `toml:"description"`
    Steps       []string                       `toml:"steps"`
    Transitions map[string]map[string]string   `toml:"transitions"`
}
```

```go
// internal/config/load.go

import "github.com/BurntSushi/toml"

// FindConfigFile walks up from the given directory to find raven.toml.
// Returns the absolute path to the config file, or an empty string if not found.
// Stops at the filesystem root.
func FindConfigFile(startDir string) (string, error)

// LoadFromFile parses the TOML file at the given path and returns the
// configuration and TOML metadata. The metadata can be used to detect
// unknown keys via MetaData.Undecoded().
func LoadFromFile(path string) (*Config, toml.MetaData, error)
```

```go
// internal/config/defaults.go

// NewDefaults returns a Config populated with all default values.
// These defaults match the PRD-specified defaults for a Go CLI project.
func NewDefaults() *Config
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| github.com/BurntSushi/toml | v1.5.0 | TOML parsing with MetaData.Undecoded() |
| path/filepath | stdlib | Directory walking and path resolution |
| os | stdlib | File existence checking |

## Acceptance Criteria
- [ ] Config struct hierarchy matches all `raven.toml` sections from PRD Section 5.10
- [ ] All struct fields have `toml:"field_name"` tags
- [ ] `FindConfigFile("/some/deep/path")` walks up directories until it finds `raven.toml`
- [ ] `FindConfigFile` returns empty string (not error) when no config found
- [ ] `FindConfigFile` stops at filesystem root (no infinite loop)
- [ ] `LoadFromFile("raven.toml")` parses the sample config from PRD Section 5.10 without error
- [ ] `LoadFromFile` returns `toml.MetaData` for unknown-key detection
- [ ] `LoadFromFile` returns descriptive error for malformed TOML
- [ ] `LoadFromFile` returns error for non-existent file
- [ ] `NewDefaults()` returns config with all default values populated:
  - Project.TasksDir = "docs/tasks"
  - Project.TaskStateFile = "docs/tasks/task-state.conf"
  - Project.PhasesConf = "docs/tasks/phases.conf"
  - Project.ProgressFile = "docs/tasks/PROGRESS.md"
  - Project.LogDir = "scripts/logs"
  - Project.PromptDir = "prompts"
  - Project.BranchTemplate = "phase/{phase_id}-{slug}"
- [ ] `Agents` map is empty by default (agents are project-specific)
- [ ] Unit tests achieve 95% coverage
- [ ] `go vet ./...` passes

## Testing Requirements
### Unit Tests
- LoadFromFile with valid TOML parses all sections correctly
- LoadFromFile with `[project]` only section (partial config)
- LoadFromFile with multiple `[agents.*]` sections populates Agents map
- LoadFromFile returns error for malformed TOML (missing closing bracket)
- LoadFromFile returns error for non-existent file
- LoadFromFile returns MetaData with Undecoded() for unknown keys
- FindConfigFile in a directory containing raven.toml returns that path
- FindConfigFile in a child directory finds raven.toml in parent
- FindConfigFile in a directory tree with no raven.toml returns empty string
- FindConfigFile at filesystem root with no config returns empty string
- NewDefaults returns config with all expected default values
- Config struct round-trip: marshal to TOML, unmarshal back, values match

### Integration Tests
- Load the actual `raven.toml` at project root and verify it parses correctly

### Edge Cases to Handle
- Empty `raven.toml` file: should parse successfully, all fields zero/empty
- `raven.toml` with only comments: should parse successfully
- Symlinked raven.toml: FindConfigFile should follow symlinks
- Very deeply nested directory (20+ levels): FindConfigFile should not stack overflow
- Config file with UTF-8 characters in string values
- Config file with TOML multi-line strings
- Agent names with hyphens or dots: `[agents.claude-3]` or `[agents.gpt.4]`

## Implementation Notes
### Recommended Approach
1. Create `internal/config/config.go` with all type definitions
2. Create `internal/config/defaults.go` with `NewDefaults()`
3. Create `internal/config/load.go` with `FindConfigFile()` and `LoadFromFile()`
4. For `FindConfigFile`: start at startDir, check for `raven.toml`, if not found get parent dir, repeat until parent == current (root reached)
5. For `LoadFromFile`: use `toml.DecodeFile(path, &config)` which returns `(MetaData, error)`
6. Create comprehensive test fixtures in `testdata/` (valid configs, partial configs, malformed configs)
7. Verify: `go build ./... && go vet ./... && go test ./internal/config/...`

### Potential Pitfalls
- `BurntSushi/toml` v1.5.0 uses `toml.DecodeFile` (not `toml.Decode`). Ensure the import path is `github.com/BurntSushi/toml` (note capital B in BurntSushi).
- The `Agents` and `Workflows` fields are maps, not slices. TOML tables like `[agents.claude]` naturally map to `map[string]AgentConfig`.
- `WorkflowConfig.Transitions` is a nested map: `map[string]map[string]string`. In TOML this looks like `[workflows.myflow.transitions.step_name]` with key-value pairs for event->next_step.
- Do not set defaults in `LoadFromFile` -- that is the responsibility of T-010 (config resolution). `LoadFromFile` returns exactly what is in the file.
- `filepath.Abs` should be used to normalize the start directory before walking up.

### Security Considerations
- Config files may contain agent command paths -- validate they are not absolute paths pointing to unexpected locations (defensive, not critical for a dev tool)
- Do not log the full config at info level -- it may contain agent configuration details

## References
- [BurntSushi/toml Documentation](https://pkg.go.dev/github.com/BurntSushi/toml)
- [BurntSushi/toml MetaData.Undecoded()](https://pkg.go.dev/github.com/BurntSushi/toml#MetaData.Undecoded)
- [PRD Section 5.10 - TOML Configuration System](docs/prd/PRD-Raven.md)
- [PRD Section 6.1 - BurntSushi/toml v1.5.0](docs/prd/PRD-Raven.md)
- [TOML Specification](https://toml.io/en/)