package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stringPtr returns a pointer to the given string value.
func stringPtr(s string) *string {
	return &s
}

// boolPtr returns a pointer to the given bool value.
func boolPtr(b bool) *bool {
	return &b
}

// mockEnvFunc creates an EnvFunc backed by a map.
func mockEnvFunc(vars map[string]string) EnvFunc {
	return func(key string) (string, bool) {
		val, ok := vars[key]
		return val, ok
	}
}

// noEnv is an EnvFunc that returns no environment variables.
func noEnv(_ string) (string, bool) {
	return "", false
}

// --- Resolve with only defaults ---

func TestResolve_OnlyDefaults(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()

	rc := Resolve(defaults, nil, noEnv, nil)

	require.NotNil(t, rc)
	require.NotNil(t, rc.Config)

	// All values should come from defaults.
	assert.Equal(t, "docs/tasks", rc.Config.Project.TasksDir)
	assert.Equal(t, "docs/tasks/task-state.conf", rc.Config.Project.TaskStateFile)
	assert.Equal(t, "docs/tasks/phases.conf", rc.Config.Project.PhasesConf)
	assert.Equal(t, "docs/tasks/PROGRESS.md", rc.Config.Project.ProgressFile)
	assert.Equal(t, "scripts/logs", rc.Config.Project.LogDir)
	assert.Equal(t, "prompts", rc.Config.Project.PromptDir)
	assert.Equal(t, "phase/{phase_id}-{slug}", rc.Config.Project.BranchTemplate)

	// Name and language are empty in defaults.
	assert.Empty(t, rc.Config.Project.Name)
	assert.Empty(t, rc.Config.Project.Language)

	// All sources should be "default".
	assert.Equal(t, SourceDefault, rc.Sources["project.tasks_dir"])
	assert.Equal(t, SourceDefault, rc.Sources["project.task_state_file"])
	assert.Equal(t, SourceDefault, rc.Sources["project.phases_conf"])
	assert.Equal(t, SourceDefault, rc.Sources["project.progress_file"])
	assert.Equal(t, SourceDefault, rc.Sources["project.log_dir"])
	assert.Equal(t, SourceDefault, rc.Sources["project.prompt_dir"])
	assert.Equal(t, SourceDefault, rc.Sources["project.branch_template"])
	assert.Equal(t, SourceDefault, rc.Sources["project.name"])
	assert.Equal(t, SourceDefault, rc.Sources["project.language"])
}

// --- Resolve with file overriding one field ---

func TestResolve_FileOverridesOneField(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Project: ProjectConfig{
			Name: "my-project",
		},
	}

	rc := Resolve(defaults, fileConfig, noEnv, nil)

	// project.name should come from file.
	assert.Equal(t, "my-project", rc.Config.Project.Name)
	assert.Equal(t, SourceFile, rc.Sources["project.name"])

	// Other fields remain from defaults.
	assert.Equal(t, "docs/tasks", rc.Config.Project.TasksDir)
	assert.Equal(t, SourceDefault, rc.Sources["project.tasks_dir"])
}

// --- Resolve with env overriding file ---

func TestResolve_EnvOverridesFile(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Project: ProjectConfig{
			Name: "file-project",
		},
	}
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_PROJECT_NAME": "env-project",
	})

	rc := Resolve(defaults, fileConfig, envFn, nil)

	assert.Equal(t, "env-project", rc.Config.Project.Name)
	assert.Equal(t, SourceEnv, rc.Sources["project.name"])
}

// --- Resolve with CLI overriding env ---

func TestResolve_CLIOverridesEnv(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Project: ProjectConfig{
			Name: "file-project",
		},
	}
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_PROJECT_NAME": "env-project",
	})
	overrides := &CLIOverrides{
		ProjectName: stringPtr("cli-project"),
	}

	rc := Resolve(defaults, fileConfig, envFn, overrides)

	assert.Equal(t, "cli-project", rc.Config.Project.Name)
	assert.Equal(t, SourceCLI, rc.Sources["project.name"])
}

// --- All four layers providing different values: CLI wins ---

func TestResolve_AllFourLayers_CLIWins(t *testing.T) {
	t.Parallel()
	defaults := &Config{
		Project: ProjectConfig{
			Name:   "default-name",
			LogDir: "default-logs",
		},
		Agents:    map[string]AgentConfig{},
		Workflows: map[string]WorkflowConfig{},
	}
	fileConfig := &Config{
		Project: ProjectConfig{
			Name:   "file-name",
			LogDir: "file-logs",
		},
	}
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_PROJECT_NAME": "env-name",
		"RAVEN_LOG_DIR":      "env-logs",
	})
	overrides := &CLIOverrides{
		ProjectName: stringPtr("cli-name"),
		LogDir:      stringPtr("cli-logs"),
	}

	rc := Resolve(defaults, fileConfig, envFn, overrides)

	assert.Equal(t, "cli-name", rc.Config.Project.Name)
	assert.Equal(t, SourceCLI, rc.Sources["project.name"])
	assert.Equal(t, "cli-logs", rc.Config.Project.LogDir)
	assert.Equal(t, SourceCLI, rc.Sources["project.log_dir"])
}

// --- Resolve with nil fileConfig falls back to defaults ---

func TestResolve_NilFileConfig(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()

	rc := Resolve(defaults, nil, noEnv, nil)

	assert.Equal(t, "docs/tasks", rc.Config.Project.TasksDir)
	assert.Equal(t, SourceDefault, rc.Sources["project.tasks_dir"])
}

// --- Resolve with nil CLIOverrides: CLI layer skipped ---

func TestResolve_NilCLIOverrides(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Project: ProjectConfig{
			Name: "file-project",
		},
	}

	rc := Resolve(defaults, fileConfig, noEnv, nil)

	assert.Equal(t, "file-project", rc.Config.Project.Name)
	assert.Equal(t, SourceFile, rc.Sources["project.name"])
}

// --- Resolve with empty CLIOverrides (all nil fields): CLI layer skipped ---

func TestResolve_EmptyCLIOverrides(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Project: ProjectConfig{
			Name: "file-project",
		},
	}
	overrides := &CLIOverrides{}

	rc := Resolve(defaults, fileConfig, noEnv, overrides)

	assert.Equal(t, "file-project", rc.Config.Project.Name)
	assert.Equal(t, SourceFile, rc.Sources["project.name"])
}

// --- Environment variable tests ---

func TestResolve_EnvProjectName(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_PROJECT_NAME": "env-name",
	})

	rc := Resolve(defaults, nil, envFn, nil)

	assert.Equal(t, "env-name", rc.Config.Project.Name)
	assert.Equal(t, SourceEnv, rc.Sources["project.name"])
}

func TestResolve_EnvTasksDir(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_TASKS_DIR": "custom/tasks",
	})

	rc := Resolve(defaults, nil, envFn, nil)

	assert.Equal(t, "custom/tasks", rc.Config.Project.TasksDir)
	assert.Equal(t, SourceEnv, rc.Sources["project.tasks_dir"])
}

func TestResolve_EnvLogDir(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_LOG_DIR": "custom/logs",
	})

	rc := Resolve(defaults, nil, envFn, nil)

	assert.Equal(t, "custom/logs", rc.Config.Project.LogDir)
	assert.Equal(t, SourceEnv, rc.Sources["project.log_dir"])
}

func TestResolve_EnvPromptDir(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_PROMPT_DIR": "custom/prompts",
	})

	rc := Resolve(defaults, nil, envFn, nil)

	assert.Equal(t, "custom/prompts", rc.Config.Project.PromptDir)
	assert.Equal(t, SourceEnv, rc.Sources["project.prompt_dir"])
}

func TestResolve_EnvBranchTemplate(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_BRANCH_TEMPLATE": "feat/{task_id}",
	})

	rc := Resolve(defaults, nil, envFn, nil)

	assert.Equal(t, "feat/{task_id}", rc.Config.Project.BranchTemplate)
	assert.Equal(t, SourceEnv, rc.Sources["project.branch_template"])
}

// --- Agent config merging ---

func TestResolve_AgentConfig_FileAgentsPreserved(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "claude",
				Model:   "claude-opus-4-6",
				Effort:  "high",
			},
		},
	}

	rc := Resolve(defaults, fileConfig, noEnv, nil)

	require.Len(t, rc.Config.Agents, 1)
	claude, ok := rc.Config.Agents["claude"]
	require.True(t, ok)
	assert.Equal(t, "claude", claude.Command)
	assert.Equal(t, "claude-opus-4-6", claude.Model)
	assert.Equal(t, "high", claude.Effort)
	assert.Equal(t, SourceFile, rc.Sources["agents.claude.model"])
}

func TestResolve_AgentConfig_EnvOverridesAllAgents(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "claude",
				Model:   "claude-opus-4-6",
				Effort:  "high",
			},
			"codex": {
				Command: "codex",
				Model:   "gpt-4",
				Effort:  "medium",
			},
		},
	}
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_AGENT_MODEL":  "env-model",
		"RAVEN_AGENT_EFFORT": "low",
	})

	rc := Resolve(defaults, fileConfig, envFn, nil)

	require.Len(t, rc.Config.Agents, 2)

	claude := rc.Config.Agents["claude"]
	assert.Equal(t, "env-model", claude.Model)
	assert.Equal(t, "low", claude.Effort)
	assert.Equal(t, "claude", claude.Command) // command unchanged
	assert.Equal(t, SourceEnv, rc.Sources["agents.claude.model"])
	assert.Equal(t, SourceEnv, rc.Sources["agents.claude.effort"])

	codex := rc.Config.Agents["codex"]
	assert.Equal(t, "env-model", codex.Model)
	assert.Equal(t, "low", codex.Effort)
	assert.Equal(t, "codex", codex.Command) // command unchanged
	assert.Equal(t, SourceEnv, rc.Sources["agents.codex.model"])
	assert.Equal(t, SourceEnv, rc.Sources["agents.codex.effort"])
}

func TestResolve_AgentConfig_CLIOverridesAllAgents(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "claude",
				Model:   "claude-opus-4-6",
				Effort:  "high",
			},
		},
	}
	overrides := &CLIOverrides{
		AgentModel:  stringPtr("cli-model"),
		AgentEffort: stringPtr("cli-effort"),
	}

	rc := Resolve(defaults, fileConfig, noEnv, overrides)

	claude := rc.Config.Agents["claude"]
	assert.Equal(t, "cli-model", claude.Model)
	assert.Equal(t, "cli-effort", claude.Effort)
	assert.Equal(t, SourceCLI, rc.Sources["agents.claude.model"])
	assert.Equal(t, SourceCLI, rc.Sources["agents.claude.effort"])
}

func TestResolve_AgentConfig_FileOverridesDefault(t *testing.T) {
	t.Parallel()
	defaults := &Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "default-claude",
				Model:   "default-model",
			},
		},
		Workflows: map[string]WorkflowConfig{},
	}
	fileConfig := &Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "file-claude",
				Model:   "file-model",
				Effort:  "high",
			},
		},
	}

	rc := Resolve(defaults, fileConfig, noEnv, nil)

	require.Len(t, rc.Config.Agents, 1)
	claude := rc.Config.Agents["claude"]
	assert.Equal(t, "file-claude", claude.Command)
	assert.Equal(t, "file-model", claude.Model)
	assert.Equal(t, "high", claude.Effort)
	assert.Equal(t, SourceFile, rc.Sources["agents.claude.command"])
}

func TestResolve_AgentConfig_MultipleAgentsFromDefaults(t *testing.T) {
	t.Parallel()
	defaults := &Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "claude",
				Model:   "default-claude-model",
			},
			"codex": {
				Command: "codex",
				Model:   "default-codex-model",
			},
		},
		Workflows: map[string]WorkflowConfig{},
	}

	rc := Resolve(defaults, nil, noEnv, nil)

	require.Len(t, rc.Config.Agents, 2)
	assert.Equal(t, "default-claude-model", rc.Config.Agents["claude"].Model)
	assert.Equal(t, "default-codex-model", rc.Config.Agents["codex"].Model)
	assert.Equal(t, SourceDefault, rc.Sources["agents.claude.model"])
	assert.Equal(t, SourceDefault, rc.Sources["agents.codex.model"])
}

// --- Edge cases ---

func TestResolve_EnvEmptyString_OverridesDefault(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_LOG_DIR": "",
	})

	rc := Resolve(defaults, nil, envFn, nil)

	// Empty string IS a valid value and should override the default.
	assert.Equal(t, "", rc.Config.Project.LogDir)
	assert.Equal(t, SourceEnv, rc.Sources["project.log_dir"])
}

func TestResolve_CLIEmptyString_OverridesDefault(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	overrides := &CLIOverrides{
		LogDir: stringPtr(""),
	}

	rc := Resolve(defaults, nil, noEnv, overrides)

	// Empty string via CLI pointer means "override to empty string".
	assert.Equal(t, "", rc.Config.Project.LogDir)
	assert.Equal(t, SourceCLI, rc.Sources["project.log_dir"])
}

func TestResolve_EnvOnlyModelSet(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "claude",
				Model:   "claude-opus-4-6",
				Effort:  "high",
			},
		},
	}
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_AGENT_MODEL": "env-model",
	})

	rc := Resolve(defaults, fileConfig, envFn, nil)

	claude := rc.Config.Agents["claude"]
	assert.Equal(t, "env-model", claude.Model)
	assert.Equal(t, "high", claude.Effort) // effort not overridden
	assert.Equal(t, SourceEnv, rc.Sources["agents.claude.model"])
	assert.Equal(t, SourceFile, rc.Sources["agents.claude.effort"])
}

func TestResolve_NoAgents_EnvAgentModelIgnored(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults() // no agents in defaults
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_AGENT_MODEL": "env-model",
	})

	rc := Resolve(defaults, nil, envFn, nil)

	// With no agents defined, the env var has nothing to apply to.
	assert.Empty(t, rc.Config.Agents)
}

func TestResolve_NilDefaults(t *testing.T) {
	t.Parallel()

	rc := Resolve(nil, nil, noEnv, nil)

	require.NotNil(t, rc)
	require.NotNil(t, rc.Config)
	assert.Empty(t, rc.Config.Project.Name)
	assert.NotNil(t, rc.Config.Agents)
	assert.NotNil(t, rc.Config.Workflows)
}

func TestResolve_NilEnvFunc(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()

	rc := Resolve(defaults, nil, nil, nil)

	require.NotNil(t, rc)
	assert.Equal(t, "docs/tasks", rc.Config.Project.TasksDir)
}

func TestResolve_FileVerificationCommands(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Project: ProjectConfig{
			VerificationCommands: []string{"go build ./...", "go test ./..."},
		},
	}

	rc := Resolve(defaults, fileConfig, noEnv, nil)

	assert.Equal(t, []string{"go build ./...", "go test ./..."}, rc.Config.Project.VerificationCommands)
	assert.Equal(t, SourceFile, rc.Sources["project.verification_commands"])
}

func TestResolve_DefaultVerificationCommands(t *testing.T) {
	t.Parallel()
	defaults := &Config{
		Project: ProjectConfig{
			VerificationCommands: []string{"make test"},
		},
		Agents:    map[string]AgentConfig{},
		Workflows: map[string]WorkflowConfig{},
	}

	rc := Resolve(defaults, nil, noEnv, nil)

	assert.Equal(t, []string{"make test"}, rc.Config.Project.VerificationCommands)
	assert.Equal(t, SourceDefault, rc.Sources["project.verification_commands"])
}

func TestResolve_ReviewConfig_FromFile(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Review: ReviewConfig{
			Extensions:       `(\.go$)`,
			RiskPatterns:     `^internal/`,
			PromptsDir:       "review/prompts",
			RulesDir:         "review/rules",
			ProjectBriefFile: "BRIEF.md",
		},
	}

	rc := Resolve(defaults, fileConfig, noEnv, nil)

	assert.Equal(t, `(\.go$)`, rc.Config.Review.Extensions)
	assert.Equal(t, `^internal/`, rc.Config.Review.RiskPatterns)
	assert.Equal(t, "review/prompts", rc.Config.Review.PromptsDir)
	assert.Equal(t, "review/rules", rc.Config.Review.RulesDir)
	assert.Equal(t, "BRIEF.md", rc.Config.Review.ProjectBriefFile)
	assert.Equal(t, SourceFile, rc.Sources["review.extensions"])
	assert.Equal(t, SourceFile, rc.Sources["review.risk_patterns"])
	assert.Equal(t, SourceFile, rc.Sources["review.prompts_dir"])
	assert.Equal(t, SourceFile, rc.Sources["review.rules_dir"])
	assert.Equal(t, SourceFile, rc.Sources["review.project_brief_file"])
}

func TestResolve_WorkflowConfig_FromFile(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Workflows: map[string]WorkflowConfig{
			"test-wf": {
				Description: "Test workflow",
				Steps:       []string{"step1", "step2"},
				Transitions: map[string]map[string]string{
					"step1": {"success": "step2"},
				},
			},
		},
	}

	rc := Resolve(defaults, fileConfig, noEnv, nil)

	require.Len(t, rc.Config.Workflows, 1)
	wf, ok := rc.Config.Workflows["test-wf"]
	require.True(t, ok)
	assert.Equal(t, "Test workflow", wf.Description)
	assert.Equal(t, []string{"step1", "step2"}, wf.Steps)
	assert.Equal(t, "step2", wf.Transitions["step1"]["success"])
}

func TestResolve_SourcesMap_Complete(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()

	rc := Resolve(defaults, nil, noEnv, nil)

	// Verify all expected project fields are tracked.
	expectedKeys := []string{
		"project.name",
		"project.language",
		"project.tasks_dir",
		"project.task_state_file",
		"project.phases_conf",
		"project.progress_file",
		"project.log_dir",
		"project.prompt_dir",
		"project.branch_template",
		"project.verification_commands",
		"review.extensions",
		"review.risk_patterns",
		"review.prompts_dir",
		"review.rules_dir",
		"review.project_brief_file",
	}
	for _, key := range expectedKeys {
		_, ok := rc.Sources[key]
		assert.True(t, ok, "expected Sources to contain key %q", key)
	}
}

func TestResolve_CLITasksDir(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	overrides := &CLIOverrides{
		TasksDir: stringPtr("cli/tasks"),
	}

	rc := Resolve(defaults, nil, noEnv, overrides)

	assert.Equal(t, "cli/tasks", rc.Config.Project.TasksDir)
	assert.Equal(t, SourceCLI, rc.Sources["project.tasks_dir"])
}

func TestResolve_DeepCopy_AgentsNotShared(t *testing.T) {
	t.Parallel()
	defaults := &Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "claude",
				Model:   "default-model",
			},
		},
		Workflows: map[string]WorkflowConfig{},
	}

	rc := Resolve(defaults, nil, noEnv, nil)

	// Modify the resolved config's agent; should not affect defaults.
	agent := rc.Config.Agents["claude"]
	agent.Model = "modified"
	rc.Config.Agents["claude"] = agent

	assert.Equal(t, "default-model", defaults.Agents["claude"].Model, "defaults should not be mutated")
}

func TestResolve_DeepCopy_WorkflowsNotShared(t *testing.T) {
	t.Parallel()
	defaults := &Config{
		Agents: map[string]AgentConfig{},
		Workflows: map[string]WorkflowConfig{
			"wf": {
				Steps: []string{"step1"},
				Transitions: map[string]map[string]string{
					"step1": {"success": "done"},
				},
			},
		},
	}

	rc := Resolve(defaults, nil, noEnv, nil)

	// Modify the resolved config's workflow; should not affect defaults.
	wf := rc.Config.Workflows["wf"]
	wf.Steps = append(wf.Steps, "step2")
	rc.Config.Workflows["wf"] = wf

	assert.Equal(t, []string{"step1"}, defaults.Workflows["wf"].Steps, "defaults should not be mutated")
}

func TestResolve_FileAddsNewAgent(t *testing.T) {
	t.Parallel()
	defaults := &Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "claude",
				Model:   "default-model",
			},
		},
		Workflows: map[string]WorkflowConfig{},
	}
	fileConfig := &Config{
		Agents: map[string]AgentConfig{
			"gemini": {
				Command: "gemini",
				Model:   "gemini-pro",
			},
		},
	}

	rc := Resolve(defaults, fileConfig, noEnv, nil)

	// Both agents should be present.
	require.Len(t, rc.Config.Agents, 2)
	assert.Equal(t, "default-model", rc.Config.Agents["claude"].Model)
	assert.Equal(t, "gemini-pro", rc.Config.Agents["gemini"].Model)
	assert.Equal(t, SourceDefault, rc.Sources["agents.claude.model"])
	assert.Equal(t, SourceFile, rc.Sources["agents.gemini.model"])
}

func TestResolve_AllEnvVarsMapped(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_PROJECT_NAME":    "env-name",
		"RAVEN_TASKS_DIR":       "env-tasks",
		"RAVEN_LOG_DIR":         "env-logs",
		"RAVEN_PROMPT_DIR":      "env-prompts",
		"RAVEN_BRANCH_TEMPLATE": "env-template",
	})

	rc := Resolve(defaults, nil, envFn, nil)

	assert.Equal(t, "env-name", rc.Config.Project.Name)
	assert.Equal(t, "env-tasks", rc.Config.Project.TasksDir)
	assert.Equal(t, "env-logs", rc.Config.Project.LogDir)
	assert.Equal(t, "env-prompts", rc.Config.Project.PromptDir)
	assert.Equal(t, "env-template", rc.Config.Project.BranchTemplate)

	assert.Equal(t, SourceEnv, rc.Sources["project.name"])
	assert.Equal(t, SourceEnv, rc.Sources["project.tasks_dir"])
	assert.Equal(t, SourceEnv, rc.Sources["project.log_dir"])
	assert.Equal(t, SourceEnv, rc.Sources["project.prompt_dir"])
	assert.Equal(t, SourceEnv, rc.Sources["project.branch_template"])
}

func TestResolve_EnvAgentEmptyString_OverridesModel(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "claude",
				Model:   "claude-opus-4-6",
			},
		},
	}
	envFn := mockEnvFunc(map[string]string{
		"RAVEN_AGENT_MODEL": "",
	})

	rc := Resolve(defaults, fileConfig, envFn, nil)

	// Empty string is a valid override.
	assert.Equal(t, "", rc.Config.Agents["claude"].Model)
	assert.Equal(t, SourceEnv, rc.Sources["agents.claude.model"])
}

func TestResolve_VerboseAndQuiet_CLIOverrides(t *testing.T) {
	t.Parallel()
	// Verbose and Quiet are tracked in CLIOverrides but are not fields
	// on Config. They are used by the CLI layer. Verify they can be set
	// without affecting the config resolution.
	defaults := NewDefaults()
	overrides := &CLIOverrides{
		Verbose: boolPtr(true),
		Quiet:   boolPtr(false),
	}

	rc := Resolve(defaults, nil, noEnv, overrides)

	// The config itself should be unaffected.
	require.NotNil(t, rc.Config)
	assert.Equal(t, "docs/tasks", rc.Config.Project.TasksDir)
}

func TestResolve_PriorityOrder_AllLayers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		defaults   *Config
		fileConfig *Config
		envVars    map[string]string
		overrides  *CLIOverrides
		wantName   string
		wantSource ConfigSource
	}{
		{
			name: "default only",
			defaults: &Config{
				Project:   ProjectConfig{Name: "default"},
				Agents:    map[string]AgentConfig{},
				Workflows: map[string]WorkflowConfig{},
			},
			wantName:   "default",
			wantSource: SourceDefault,
		},
		{
			name: "file overrides default",
			defaults: &Config{
				Project:   ProjectConfig{Name: "default"},
				Agents:    map[string]AgentConfig{},
				Workflows: map[string]WorkflowConfig{},
			},
			fileConfig: &Config{
				Project: ProjectConfig{Name: "file"},
			},
			wantName:   "file",
			wantSource: SourceFile,
		},
		{
			name: "env overrides file",
			defaults: &Config{
				Project:   ProjectConfig{Name: "default"},
				Agents:    map[string]AgentConfig{},
				Workflows: map[string]WorkflowConfig{},
			},
			fileConfig: &Config{
				Project: ProjectConfig{Name: "file"},
			},
			envVars:    map[string]string{"RAVEN_PROJECT_NAME": "env"},
			wantName:   "env",
			wantSource: SourceEnv,
		},
		{
			name: "cli overrides all",
			defaults: &Config{
				Project:   ProjectConfig{Name: "default"},
				Agents:    map[string]AgentConfig{},
				Workflows: map[string]WorkflowConfig{},
			},
			fileConfig: &Config{
				Project: ProjectConfig{Name: "file"},
			},
			envVars:    map[string]string{"RAVEN_PROJECT_NAME": "env"},
			overrides:  &CLIOverrides{ProjectName: stringPtr("cli")},
			wantName:   "cli",
			wantSource: SourceCLI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			envFn := noEnv
			if tt.envVars != nil {
				envFn = mockEnvFunc(tt.envVars)
			}
			rc := Resolve(tt.defaults, tt.fileConfig, envFn, tt.overrides)
			assert.Equal(t, tt.wantName, rc.Config.Project.Name)
			assert.Equal(t, tt.wantSource, rc.Sources["project.name"])
		})
	}
}

func TestResolve_Path_EmptyByDefault(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()

	rc := Resolve(defaults, nil, noEnv, nil)

	assert.Empty(t, rc.Path, "Path should be empty when no config file is used")
}

func TestResolve_FileEmpty_KeepsDefaults(t *testing.T) {
	t.Parallel()
	defaults := NewDefaults()
	fileConfig := &Config{} // empty config from an empty toml file

	rc := Resolve(defaults, fileConfig, noEnv, nil)

	// All defaults should be preserved since file has empty strings.
	assert.Equal(t, "docs/tasks", rc.Config.Project.TasksDir)
	assert.Equal(t, SourceDefault, rc.Sources["project.tasks_dir"])
	assert.Equal(t, "scripts/logs", rc.Config.Project.LogDir)
	assert.Equal(t, SourceDefault, rc.Sources["project.log_dir"])
}
