package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validConfig returns a Config that passes all validation checks.
// Directories and files are set to empty to avoid filesystem-dependent warnings.
func validConfig() *Config {
	return &Config{
		Project: ProjectConfig{
			Name:     "my-project",
			Language: "go",
		},
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "claude",
				Model:   "claude-opus-4-6",
				Effort:  "high",
			},
		},
		Review: ReviewConfig{},
		Workflows: map[string]WorkflowConfig{
			"default": {
				Description: "default workflow",
				Steps:       []string{"implement", "review"},
				Transitions: map[string]map[string]string{
					"implement": {"success": "review"},
				},
			},
		},
	}
}

// decodeMetadata parses TOML content and returns the metadata, useful for
// testing unknown key detection.
func decodeMetadata(t *testing.T, content string) toml.MetaData {
	t.Helper()
	var cfg Config
	md, err := toml.Decode(content, &cfg)
	require.NoError(t, err)
	return md
}

// --- ValidationResult method tests ---

func TestValidationResult_HasErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		issues []ValidationIssue
		want   bool
	}{
		{
			name:   "no issues",
			issues: nil,
			want:   false,
		},
		{
			name: "only warnings",
			issues: []ValidationIssue{
				{Severity: SeverityWarning, Field: "a", Message: "warn"},
			},
			want: false,
		},
		{
			name: "has error",
			issues: []ValidationIssue{
				{Severity: SeverityWarning, Field: "a", Message: "warn"},
				{Severity: SeverityError, Field: "b", Message: "err"},
			},
			want: true,
		},
		{
			name: "only errors",
			issues: []ValidationIssue{
				{Severity: SeverityError, Field: "x", Message: "err"},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vr := &ValidationResult{Issues: tt.issues}
			assert.Equal(t, tt.want, vr.HasErrors())
		})
	}
}

func TestValidationResult_HasWarnings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		issues []ValidationIssue
		want   bool
	}{
		{
			name:   "no issues",
			issues: nil,
			want:   false,
		},
		{
			name: "only errors",
			issues: []ValidationIssue{
				{Severity: SeverityError, Field: "a", Message: "err"},
			},
			want: false,
		},
		{
			name: "has warning",
			issues: []ValidationIssue{
				{Severity: SeverityWarning, Field: "a", Message: "warn"},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vr := &ValidationResult{Issues: tt.issues}
			assert.Equal(t, tt.want, vr.HasWarnings())
		})
	}
}

func TestValidationResult_Errors(t *testing.T) {
	t.Parallel()
	vr := &ValidationResult{
		Issues: []ValidationIssue{
			{Severity: SeverityWarning, Field: "a", Message: "warn1"},
			{Severity: SeverityError, Field: "b", Message: "err1"},
			{Severity: SeverityWarning, Field: "c", Message: "warn2"},
			{Severity: SeverityError, Field: "d", Message: "err2"},
		},
	}
	errs := vr.Errors()
	require.Len(t, errs, 2)
	assert.Equal(t, "b", errs[0].Field)
	assert.Equal(t, "d", errs[1].Field)
}

func TestValidationResult_Warnings(t *testing.T) {
	t.Parallel()
	vr := &ValidationResult{
		Issues: []ValidationIssue{
			{Severity: SeverityWarning, Field: "a", Message: "warn1"},
			{Severity: SeverityError, Field: "b", Message: "err1"},
			{Severity: SeverityWarning, Field: "c", Message: "warn2"},
		},
	}
	warns := vr.Warnings()
	require.Len(t, warns, 2)
	assert.Equal(t, "a", warns[0].Field)
	assert.Equal(t, "c", warns[1].Field)
}

func TestValidationResult_EmptyResult(t *testing.T) {
	t.Parallel()
	vr := &ValidationResult{}
	assert.False(t, vr.HasErrors())
	assert.False(t, vr.HasWarnings())
	assert.Nil(t, vr.Errors())
	assert.Nil(t, vr.Warnings())
}

// --- Validate: nil config ---

func TestValidate_NilConfig(t *testing.T) {
	t.Parallel()
	vr := Validate(nil, nil)
	require.True(t, vr.HasErrors())
	require.Len(t, vr.Errors(), 1)
	assert.Contains(t, vr.Errors()[0].Message, "configuration is nil")
}

// --- Validate: valid config ---

func TestValidate_ValidConfig_NoErrors(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	vr := Validate(cfg, nil)
	assert.False(t, vr.HasErrors(), "expected no errors for valid config, got: %v", vr.Errors())
}

func TestValidate_ValidConfig_NilMeta(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	// Nil meta should not panic.
	vr := Validate(cfg, nil)
	assert.False(t, vr.HasErrors())
}

func TestValidate_DefaultsOnly_NoErrors(t *testing.T) {
	t.Parallel()
	// Defaults with a name set should validate (name is required).
	cfg := NewDefaults()
	cfg.Project.Name = "test-project"
	vr := Validate(cfg, nil)
	assert.False(t, vr.HasErrors(), "expected defaults with name to have no errors, got: %v", vr.Errors())
}

// --- Validate: project section errors ---

func TestValidate_EmptyProjectName(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Project.Name = ""
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())
	errs := vr.Errors()
	found := false
	for _, e := range errs {
		if e.Field == "project.name" {
			found = true
			assert.Contains(t, e.Message, "must not be empty")
		}
	}
	assert.True(t, found, "expected error on project.name")
}

func TestValidate_InvalidProjectLanguage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		language string
		wantErr  bool
	}{
		{name: "empty is valid", language: "", wantErr: false},
		{name: "go", language: "go", wantErr: false},
		{name: "typescript", language: "typescript", wantErr: false},
		{name: "python", language: "python", wantErr: false},
		{name: "rust", language: "rust", wantErr: false},
		{name: "java", language: "java", wantErr: false},
		{name: "invalid csharp", language: "csharp", wantErr: true},
		{name: "invalid Go uppercase", language: "Go", wantErr: true},
		{name: "invalid JavaScript", language: "JavaScript", wantErr: true},
		{name: "invalid c++", language: "c++", wantErr: true},
		{name: "invalid unknown", language: "unknown", wantErr: true},
		{name: "invalid random", language: "brainfuck", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := validConfig()
			cfg.Project.Language = tt.language
			vr := Validate(cfg, nil)
			hasLangErr := false
			for _, e := range vr.Errors() {
				if e.Field == "project.language" {
					hasLangErr = true
				}
			}
			assert.Equal(t, tt.wantErr, hasLangErr,
				"language=%q: expected error=%v", tt.language, tt.wantErr)
		})
	}
}

func TestValidate_EmptyVerificationCommand(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Project.VerificationCommands = []string{"go build ./...", "", "go vet ./..."}
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())
	found := false
	for _, e := range vr.Errors() {
		if strings.Contains(e.Field, "verification_commands") {
			found = true
			assert.Contains(t, e.Field, "[1]")
			assert.Contains(t, e.Message, "empty string")
		}
	}
	assert.True(t, found, "expected error on empty verification command")
}

func TestValidate_EmptyVerificationCommandsArray(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Project.VerificationCommands = []string{}
	vr := Validate(cfg, nil)
	// Empty array is valid (no commands to run).
	hasVCErr := false
	for _, e := range vr.Errors() {
		if strings.Contains(e.Field, "verification_commands") {
			hasVCErr = true
		}
	}
	assert.False(t, hasVCErr, "empty verification_commands array should not be an error")
}

// --- Validate: agent section errors ---

func TestValidate_EmptyAgentCommand(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agents["badagent"] = AgentConfig{
		Command: "",
		Model:   "some-model",
	}
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())
	found := false
	for _, e := range vr.Errors() {
		if e.Field == "agents.badagent.command" {
			found = true
			assert.Contains(t, e.Message, "must not be empty")
		}
	}
	assert.True(t, found, "expected error on agents.badagent.command")
}

func TestValidate_InvalidAgentEffort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		effort  string
		wantErr bool
	}{
		{name: "empty is valid", effort: "", wantErr: false},
		{name: "low", effort: "low", wantErr: false},
		{name: "medium", effort: "medium", wantErr: false},
		{name: "high", effort: "high", wantErr: false},
		{name: "invalid extreme", effort: "extreme", wantErr: true},
		{name: "invalid Low uppercase", effort: "Low", wantErr: true},
		{name: "invalid HIGH all caps", effort: "HIGH", wantErr: true},
		{name: "invalid High uppercase", effort: "High", wantErr: true},
		{name: "invalid max", effort: "max", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := validConfig()
			cfg.Agents["claude"] = AgentConfig{
				Command: "claude",
				Effort:  tt.effort,
			}
			vr := Validate(cfg, nil)
			hasEffortErr := false
			for _, e := range vr.Errors() {
				if e.Field == "agents.claude.effort" {
					hasEffortErr = true
				}
			}
			assert.Equal(t, tt.wantErr, hasEffortErr,
				"effort=%q: expected error=%v", tt.effort, tt.wantErr)
		})
	}
}

func TestValidate_NoAgentsDefined(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agents = nil
	vr := Validate(cfg, nil)
	// No agents is valid.
	hasAgentErr := false
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "agents.") {
			hasAgentErr = true
		}
	}
	assert.False(t, hasAgentErr, "no agents should not produce an error")
}

func TestValidate_AgentSpecialCharacterName(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agents["claude-3.5"] = AgentConfig{
		Command: "claude",
		Effort:  "high",
	}
	vr := Validate(cfg, nil)
	// Should not crash or produce unexpected errors.
	for _, e := range vr.Errors() {
		if strings.Contains(e.Field, "claude-3.5") {
			t.Errorf("unexpected error for agent with special chars: %v", e)
		}
	}
}

// --- Validate: review section errors ---

func TestValidate_InvalidReviewExtensionsRegex(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.Extensions = "[invalid("
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())
	found := false
	for _, e := range vr.Errors() {
		if e.Field == "review.extensions" {
			found = true
			assert.Contains(t, e.Message, "[invalid(")
			assert.Contains(t, e.Message, "invalid regex")
		}
	}
	assert.True(t, found, "expected error on review.extensions")
}

func TestValidate_ValidReviewExtensionsRegex(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.Extensions = `(\.go$|go\.mod$|go\.sum$)`
	vr := Validate(cfg, nil)
	hasExtErr := false
	for _, e := range vr.Errors() {
		if e.Field == "review.extensions" {
			hasExtErr = true
		}
	}
	assert.False(t, hasExtErr, "valid regex should not produce an error")
}

func TestValidate_InvalidReviewRiskPatternsRegex(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.RiskPatterns = "**bad"
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())
	found := false
	for _, e := range vr.Errors() {
		if e.Field == "review.risk_patterns" {
			found = true
			assert.Contains(t, e.Message, "**bad")
			assert.Contains(t, e.Message, "invalid regex")
		}
	}
	assert.True(t, found, "expected error on review.risk_patterns")
}

func TestValidate_ValidReviewRiskPatternsRegex(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.RiskPatterns = `^(cmd/|internal/|scripts/)`
	vr := Validate(cfg, nil)
	hasRPErr := false
	for _, e := range vr.Errors() {
		if e.Field == "review.risk_patterns" {
			hasRPErr = true
		}
	}
	assert.False(t, hasRPErr, "valid regex should not produce an error")
}

func TestValidate_EmptyReviewRegexFields(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.Extensions = ""
	cfg.Review.RiskPatterns = ""
	vr := Validate(cfg, nil)
	hasReviewErr := false
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "review.") {
			hasReviewErr = true
		}
	}
	assert.False(t, hasReviewErr, "empty regex fields should not produce errors")
}

func TestValidate_ComplexRegexPattern(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.Extensions = `(?:\.(?:go|ts|tsx|js|jsx|py|rs|java|kt|swift|c|cpp|h|hpp))$`
	vr := Validate(cfg, nil)
	hasExtErr := false
	for _, e := range vr.Errors() {
		if e.Field == "review.extensions" {
			hasExtErr = true
		}
	}
	assert.False(t, hasExtErr, "complex valid regex should not produce an error")
}

// --- Validate: workflow section errors ---

func TestValidate_WorkflowEmptySteps(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows["empty"] = WorkflowConfig{
		Description: "empty workflow",
		Steps:       []string{},
	}
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())
	found := false
	for _, e := range vr.Errors() {
		if e.Field == "workflows.empty.steps" {
			found = true
			assert.Contains(t, e.Message, "must not be empty")
		}
	}
	assert.True(t, found, "expected error on workflows.empty.steps")
}

func TestValidate_WorkflowNilSteps(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows["nilsteps"] = WorkflowConfig{
		Description: "nil steps workflow",
		Steps:       nil,
	}
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())
	found := false
	for _, e := range vr.Errors() {
		if e.Field == "workflows.nilsteps.steps" {
			found = true
		}
	}
	assert.True(t, found, "expected error on workflows.nilsteps.steps")
}

func TestValidate_WorkflowTransitionReferencesUndefinedStep(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows["bad-trans"] = WorkflowConfig{
		Description: "bad transitions",
		Steps:       []string{"implement", "review"},
		Transitions: map[string]map[string]string{
			"deploy": {"success": "review"}, // "deploy" is not a defined step
		},
	}
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())
	found := false
	for _, e := range vr.Errors() {
		if e.Field == "workflows.bad-trans.transitions.deploy" {
			found = true
			assert.Contains(t, e.Message, "undefined step")
			assert.Contains(t, e.Message, "deploy")
		}
	}
	assert.True(t, found, "expected error for undefined transition key step")
}

func TestValidate_WorkflowTransitionTargetUndefined(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows["bad-target"] = WorkflowConfig{
		Description: "bad target",
		Steps:       []string{"implement", "review"},
		Transitions: map[string]map[string]string{
			"implement": {"success": "deploy"}, // "deploy" is not a defined step
		},
	}
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())
	found := false
	for _, e := range vr.Errors() {
		if e.Field == "workflows.bad-target.transitions.implement.success" {
			found = true
			assert.Contains(t, e.Message, "deploy")
			assert.Contains(t, e.Message, "not a defined step")
		}
	}
	assert.True(t, found, "expected error for undefined transition target")
}

func TestValidate_WorkflowValidTransitions(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows["valid"] = WorkflowConfig{
		Description: "valid",
		Steps:       []string{"implement", "review", "fix"},
		Transitions: map[string]map[string]string{
			"implement": {"success": "review", "failure": "implement"},
			"review":    {"success": "fix"},
		},
	}
	vr := Validate(cfg, nil)
	hasWFErr := false
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "workflows.valid") {
			hasWFErr = true
		}
	}
	assert.False(t, hasWFErr, "valid workflow should not produce errors")
}

func TestValidate_NoWorkflowsDefined(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows = nil
	vr := Validate(cfg, nil)
	hasWFErr := false
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "workflows.") {
			hasWFErr = true
		}
	}
	assert.False(t, hasWFErr, "no workflows should not produce errors")
}

// --- Validate: unknown keys ---

func TestValidate_UnknownKeysDetected(t *testing.T) {
	t.Parallel()
	content := `
[project]
name = "test"
unknown_key = "oops"

[unknown_section]
foo = "bar"
`
	md := decodeMetadata(t, content)
	cfg := &Config{
		Project: ProjectConfig{Name: "test"},
	}
	vr := Validate(cfg, &md)

	require.True(t, vr.HasWarnings())
	warns := vr.Warnings()

	// Collect warning fields.
	fields := make([]string, 0, len(warns))
	for _, w := range warns {
		if w.Message == "unknown configuration key" {
			fields = append(fields, w.Field)
		}
	}
	assert.Contains(t, fields, "project.unknown_key")
	assert.Contains(t, fields, "unknown_section.foo")
}

func TestValidate_NoUnknownKeys(t *testing.T) {
	t.Parallel()
	content := `
[project]
name = "test"
language = "go"
`
	md := decodeMetadata(t, content)
	cfg := &Config{
		Project: ProjectConfig{
			Name:     "test",
			Language: "go",
		},
	}
	vr := Validate(cfg, &md)

	// No unknown key warnings.
	for _, w := range vr.Warnings() {
		if w.Message == "unknown configuration key" {
			t.Errorf("unexpected unknown key warning: %s", w.Field)
		}
	}
}

func TestValidate_NilMetadata_NoUnknownKeyCheck(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	// Should not panic and should not produce unknown key warnings.
	vr := Validate(cfg, nil)
	for _, w := range vr.Warnings() {
		if w.Message == "unknown configuration key" {
			t.Errorf("unexpected unknown key warning with nil metadata: %s", w.Field)
		}
	}
}

// --- Validate: filesystem warnings ---

func TestValidate_NonExistentTasksDir(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Project.TasksDir = "/nonexistent/tasks/dir/that/does/not/exist"
	vr := Validate(cfg, nil)

	found := false
	for _, w := range vr.Warnings() {
		if w.Field == "project.tasks_dir" {
			found = true
			assert.Contains(t, w.Message, "does not exist")
		}
	}
	assert.True(t, found, "expected warning on non-existent tasks_dir")

	// Should NOT be an error.
	for _, e := range vr.Errors() {
		if e.Field == "project.tasks_dir" {
			t.Error("tasks_dir non-existence should be a warning, not an error")
		}
	}
}

func TestValidate_NonExistentLogDir(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Project.LogDir = "/nonexistent/log/dir"
	vr := Validate(cfg, nil)

	found := false
	for _, w := range vr.Warnings() {
		if w.Field == "project.log_dir" {
			found = true
			assert.Contains(t, w.Message, "does not exist")
		}
	}
	assert.True(t, found, "expected warning on non-existent log_dir")
}

func TestValidate_ExistingTasksDir_NoWarning(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := validConfig()
	cfg.Project.TasksDir = dir
	vr := Validate(cfg, nil)

	for _, w := range vr.Warnings() {
		if w.Field == "project.tasks_dir" {
			t.Error("existing tasks_dir should not produce a warning")
		}
	}
}

func TestValidate_EmptyTasksDir_NoWarning(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Project.TasksDir = ""
	vr := Validate(cfg, nil)

	for _, w := range vr.Warnings() {
		if w.Field == "project.tasks_dir" {
			t.Error("empty tasks_dir should not produce a warning")
		}
	}
}

func TestValidate_NonExistentPromptTemplate(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agents["claude"] = AgentConfig{
		Command:        "claude",
		PromptTemplate: "/nonexistent/prompt.md",
	}
	vr := Validate(cfg, nil)

	found := false
	for _, w := range vr.Warnings() {
		if w.Field == "agents.claude.prompt_template" {
			found = true
			assert.Contains(t, w.Message, "does not exist")
		}
	}
	assert.True(t, found, "expected warning on non-existent prompt_template")
}

func TestValidate_ExistingPromptTemplate_NoWarning(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "prompt.md")
	require.NoError(t, os.WriteFile(promptFile, []byte("# Prompt"), 0o644))

	cfg := validConfig()
	cfg.Agents["claude"] = AgentConfig{
		Command:        "claude",
		PromptTemplate: promptFile,
	}
	vr := Validate(cfg, nil)

	for _, w := range vr.Warnings() {
		if w.Field == "agents.claude.prompt_template" {
			t.Error("existing prompt_template should not produce a warning")
		}
	}
}

func TestValidate_EmptyPromptTemplate_NoWarning(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agents["claude"] = AgentConfig{
		Command:        "claude",
		PromptTemplate: "",
	}
	vr := Validate(cfg, nil)

	for _, w := range vr.Warnings() {
		if w.Field == "agents.claude.prompt_template" {
			t.Error("empty prompt_template should not produce a warning")
		}
	}
}

func TestValidate_NonExistentReviewPromptsDir(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.PromptsDir = "/nonexistent/review/prompts"
	vr := Validate(cfg, nil)

	found := false
	for _, w := range vr.Warnings() {
		if w.Field == "review.prompts_dir" {
			found = true
			assert.Contains(t, w.Message, "does not exist")
		}
	}
	assert.True(t, found, "expected warning on non-existent review.prompts_dir")
}

func TestValidate_NonExistentProjectBriefFile(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.ProjectBriefFile = "/nonexistent/brief.md"
	vr := Validate(cfg, nil)

	found := false
	for _, w := range vr.Warnings() {
		if w.Field == "review.project_brief_file" {
			found = true
			assert.Contains(t, w.Message, "does not exist")
		}
	}
	assert.True(t, found, "expected warning on non-existent review.project_brief_file")
}

// --- Validate: multiple errors collected ---

func TestValidate_MultipleErrorsCollected(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: ProjectConfig{
			Name:                 "",        // error: empty
			Language:             "haskell", // error: unrecognized
			VerificationCommands: []string{"", ""},
		},
		Agents: map[string]AgentConfig{
			"bad": {Command: "", Effort: "extreme"},
		},
		Review: ReviewConfig{
			Extensions:   "[bad(",
			RiskPatterns: "**bad",
		},
		Workflows: map[string]WorkflowConfig{
			"empty": {Steps: []string{}},
		},
	}
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())

	// We should have at minimum:
	// 1. project.name empty
	// 2. project.language invalid
	// 3. verification_commands[0] empty
	// 4. verification_commands[1] empty
	// 5. agents.bad.command empty
	// 6. agents.bad.effort invalid
	// 7. review.extensions invalid regex
	// 8. review.risk_patterns invalid regex
	// 9. workflows.empty.steps empty
	errs := vr.Errors()
	assert.GreaterOrEqual(t, len(errs), 9,
		"expected at least 9 errors, got %d: %v", len(errs), errs)
}

// --- Validate: zero-value config ---

func TestValidate_ZeroValueConfig(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	// Should not panic, but will have project.name error.
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())
	found := false
	for _, e := range vr.Errors() {
		if e.Field == "project.name" {
			found = true
		}
	}
	assert.True(t, found, "zero-value config should report project.name error")
}

// --- Integration: validate testdata/valid-full.toml ---

func TestValidate_FullTestdataConfig(t *testing.T) {
	t.Parallel()
	cfg, md, err := LoadFromFile(testdataPath(t, "valid-full.toml"))
	require.NoError(t, err)

	vr := Validate(cfg, &md)
	assert.False(t, vr.HasErrors(),
		"valid-full.toml should have no validation errors, got: %v", vr.Errors())
	// Unknown keys check should also be clean.
	for _, w := range vr.Warnings() {
		if w.Message == "unknown configuration key" {
			t.Errorf("unexpected unknown key in valid-full.toml: %s", w.Field)
		}
	}
}

// --- Integration: validate testdata/valid-unknown-keys.toml ---

func TestValidate_UnknownKeysTestdataConfig(t *testing.T) {
	t.Parallel()
	cfg, md, err := LoadFromFile(testdataPath(t, "valid-unknown-keys.toml"))
	require.NoError(t, err)

	vr := Validate(cfg, &md)
	require.True(t, vr.HasWarnings())

	fields := make([]string, 0)
	for _, w := range vr.Warnings() {
		if w.Message == "unknown configuration key" {
			fields = append(fields, w.Field)
		}
	}
	assert.Contains(t, fields, "project.unknown_key")
	assert.Contains(t, fields, "unknown_section.foo")
}

// --- Validate: issue message quality ---

func TestValidate_IssueMessagesIncludeFieldPath(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Project.Name = ""
	cfg.Review.Extensions = "[bad("
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())

	for _, e := range vr.Errors() {
		assert.NotEmpty(t, e.Field, "every issue should have a field path")
		assert.NotEmpty(t, e.Message, "every issue should have a message")
	}
}

func TestValidate_RegexErrorIncludesPattern(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.Extensions = `(?P<bad`
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())

	found := false
	for _, e := range vr.Errors() {
		if e.Field == "review.extensions" {
			found = true
			assert.Contains(t, e.Message, `(?P<bad`, "error message should include the invalid pattern")
		}
	}
	assert.True(t, found)
}

// --- Additional tests: combined ValidationResult method assertions ---

func TestValidationResult_MethodsMixed(t *testing.T) {
	t.Parallel()

	vr := &ValidationResult{
		Issues: []ValidationIssue{
			{Severity: SeverityError, Field: "project.name", Message: "must not be empty"},
			{Severity: SeverityWarning, Field: "project.tasks_dir", Message: "directory does not exist"},
			{Severity: SeverityError, Field: "agents.claude.command", Message: "must not be empty"},
			{Severity: SeverityWarning, Field: "review.prompts_dir", Message: "directory does not exist"},
		},
	}

	assert.True(t, vr.HasErrors())
	assert.True(t, vr.HasWarnings())

	errors := vr.Errors()
	require.Len(t, errors, 2, "expected exactly 2 errors")
	assert.Equal(t, "project.name", errors[0].Field)
	assert.Equal(t, "agents.claude.command", errors[1].Field)
	for _, e := range errors {
		assert.Equal(t, SeverityError, e.Severity)
	}

	warnings := vr.Warnings()
	require.Len(t, warnings, 2, "expected exactly 2 warnings")
	assert.Equal(t, "project.tasks_dir", warnings[0].Field)
	assert.Equal(t, "review.prompts_dir", warnings[1].Field)
	for _, w := range warnings {
		assert.Equal(t, SeverityWarning, w.Severity)
	}
}

// --- Additional tests: valid config has no warnings either ---

func TestValidate_ValidConfig_NoWarnings(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	vr := Validate(cfg, nil)
	assert.False(t, vr.HasErrors(), "valid config should have no errors, got: %v", vr.Errors())
	assert.False(t, vr.HasWarnings(), "valid config should have no warnings, got: %v", vr.Warnings())
}

// --- Additional tests: verification_commands nil slice is valid ---

func TestValidate_NilVerificationCommandsValid(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Project.VerificationCommands = nil
	vr := Validate(cfg, nil)
	hasVCErr := false
	for _, e := range vr.Errors() {
		if strings.Contains(e.Field, "verification_commands") {
			hasVCErr = true
		}
	}
	assert.False(t, hasVCErr, "nil verification_commands should not produce errors")
}

// --- Additional tests: multiple agents with mixed validity ---

func TestValidate_MultipleAgentsMixed(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agents["good"] = AgentConfig{
		Command: "good-cmd",
		Effort:  "low",
	}
	cfg.Agents["bad"] = AgentConfig{
		Command: "",
		Effort:  "cosmic",
	}
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())

	// bad agent should have errors.
	cmdFound := false
	effortFound := false
	for _, e := range vr.Errors() {
		if e.Field == "agents.bad.command" {
			cmdFound = true
		}
		if e.Field == "agents.bad.effort" {
			effortFound = true
		}
	}
	assert.True(t, cmdFound, "expected error on agents.bad.command")
	assert.True(t, effortFound, "expected error on agents.bad.effort")

	// good agent should have no errors.
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "agents.good") {
			t.Errorf("good agent should have no errors, got: %v", e)
		}
	}
}

// --- Additional tests: both transition key and value undefined ---

func TestValidate_WorkflowTransitionBothKeyAndValueUndefined(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows["wf"] = WorkflowConfig{
		Description: "test",
		Steps:       []string{"a", "b"},
		Transitions: map[string]map[string]string{
			"x": {"success": "y"},
		},
	}
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())

	foundX := false
	foundY := false
	for _, e := range vr.Errors() {
		if strings.Contains(e.Message, "x") {
			foundX = true
		}
		if strings.Contains(e.Message, "y") {
			foundY = true
		}
	}
	assert.True(t, foundX, "expected error about undefined transition key 'x'")
	assert.True(t, foundY, "expected error about undefined transition target 'y'")
}

// --- Additional tests: workflow with nil transitions is still valid ---

func TestValidate_WorkflowNilTransitions(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows["simple"] = WorkflowConfig{
		Description: "simple workflow",
		Steps:       []string{"build", "test"},
		Transitions: nil,
	}
	vr := Validate(cfg, nil)
	hasWFErr := false
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "workflows.simple") {
			hasWFErr = true
		}
	}
	assert.False(t, hasWFErr, "workflow with steps but nil transitions should not produce errors")
}

// --- Additional tests: existing directories produce no warning ---

func TestValidate_ExistingDirectories_NoWarning(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := validConfig()
	cfg.Project.TasksDir = dir
	cfg.Project.LogDir = dir
	vr := Validate(cfg, nil)

	for _, w := range vr.Warnings() {
		if w.Field == "project.tasks_dir" || w.Field == "project.log_dir" {
			t.Errorf("existing directory should not produce a warning: %v", w)
		}
	}
}

// --- Additional tests: nil agents and workflows maps ---

func TestValidate_NilAgentsMap(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agents = nil
	vr := Validate(cfg, nil)
	require.NotNil(t, vr)
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "agents.") {
			t.Errorf("nil agents map should not produce agent errors: %v", e)
		}
	}
}

func TestValidate_EmptyAgentsMap(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agents = map[string]AgentConfig{}
	vr := Validate(cfg, nil)
	require.NotNil(t, vr)
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "agents.") {
			t.Errorf("empty agents map should not produce agent errors: %v", e)
		}
	}
}

func TestValidate_NilWorkflowsMap(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows = nil
	vr := Validate(cfg, nil)
	require.NotNil(t, vr)
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "workflows.") {
			t.Errorf("nil workflows map should not produce workflow errors: %v", e)
		}
	}
}

func TestValidate_EmptyWorkflowsMap(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows = map[string]WorkflowConfig{}
	vr := Validate(cfg, nil)
	require.NotNil(t, vr)
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "workflows.") {
			t.Errorf("empty workflows map should not produce workflow errors: %v", e)
		}
	}
}

// --- Additional tests: agent with hyphen and dot in name ---

func TestValidate_AgentNameWithHyphensAndDots(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agents["my-agent.v2"] = AgentConfig{
		Command: "myagent",
		Model:   "model-v2",
		Effort:  "high",
	}
	vr := Validate(cfg, nil)
	require.NotNil(t, vr, "validation must not crash with special characters in agent name")

	// The agent is valid so no agent-related errors should appear for it.
	for _, e := range vr.Errors() {
		if strings.Contains(e.Field, "my-agent.v2") {
			t.Errorf("valid agent with special chars should not produce errors: %v", e)
		}
	}
}

// --- Additional tests: multiple empty verification commands ---

func TestValidate_MultipleEmptyVerificationCommands(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Project.VerificationCommands = []string{"", "go build", "", "go test"}
	vr := Validate(cfg, nil)
	require.True(t, vr.HasErrors())

	foundIdx0 := false
	foundIdx2 := false
	for _, e := range vr.Errors() {
		if strings.Contains(e.Field, "verification_commands[0]") {
			foundIdx0 = true
		}
		if strings.Contains(e.Field, "verification_commands[2]") {
			foundIdx2 = true
		}
	}
	assert.True(t, foundIdx0, "expected error for verification_commands[0]")
	assert.True(t, foundIdx2, "expected error for verification_commands[2]")
}

// --- Additional tests: zero-value config does not panic ---

func TestValidate_ZeroValueConfig_NoPanic(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	vr := Validate(cfg, nil)
	require.NotNil(t, vr)
	require.True(t, vr.HasErrors(), "zero-value config should have at least project.name error")
}

// --- Additional tests: valid workflow transitions produce no errors ---

func TestValidate_WorkflowValidTransitions_Full(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workflows["pipeline"] = WorkflowConfig{
		Description: "valid pipeline",
		Steps:       []string{"build", "test", "deploy"},
		Transitions: map[string]map[string]string{
			"build": {"success": "test", "failure": "build"},
			"test":  {"success": "deploy", "failure": "build"},
		},
	}
	vr := Validate(cfg, nil)
	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "workflows.pipeline") {
			t.Errorf("valid workflow should not produce errors: %v", e)
		}
	}
}

// --- Additional tests: unknown keys from testdata fixture ---

func TestValidate_UnknownKeysFromTestdataFixture(t *testing.T) {
	t.Parallel()
	fixturePath := testdataPath(t, "valid-unknown-keys.toml")

	var cfg Config
	md, err := toml.DecodeFile(fixturePath, &cfg)
	require.NoError(t, err)

	// Set the project name so we do not get an error for it.
	cfg.Project.Name = "test"

	vr := Validate(&cfg, &md)
	require.True(t, vr.HasWarnings(), "expected warnings for unknown keys in fixture")

	fields := make([]string, 0)
	for _, w := range vr.Warnings() {
		fields = append(fields, w.Field)
	}

	foundProjectUnknown := false
	foundUnknownSection := false
	for _, f := range fields {
		if strings.Contains(f, "project.unknown_key") {
			foundProjectUnknown = true
		}
		if strings.Contains(f, "unknown_section") {
			foundUnknownSection = true
		}
	}
	assert.True(t, foundProjectUnknown, "expected warning about 'project.unknown_key', got fields: %v", fields)
	assert.True(t, foundUnknownSection, "expected warning about 'unknown_section', got fields: %v", fields)
}

// --- Additional tests: every error and warning has field and message ---

func TestValidate_AllIssuesHaveFieldAndMessage(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Project.Name = ""
	cfg.Project.Language = "INVALID"
	cfg.Project.TasksDir = "/nonexistent/tasks"
	cfg.Review.Extensions = "[bad"

	vr := Validate(cfg, nil)
	require.NotEmpty(t, vr.Issues)

	for _, iss := range vr.Issues {
		assert.NotEmpty(t, iss.Field, "every issue should have a non-empty Field, got issue: %v", iss)
		assert.NotEmpty(t, iss.Message, "every issue should have a non-empty Message, got issue: %v", iss)
		assert.True(t, iss.Severity == SeverityError || iss.Severity == SeverityWarning,
			"every issue should have a valid severity, got: %q", iss.Severity)
	}
}

// --- Additional tests: complex but valid regex patterns ---

func TestValidate_ComplexValidRegexPatterns(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.Extensions = `(?i)\.(go|ts|tsx|py|rs|java|kt|swift|rb|php|c|cpp|h|hpp)$`
	cfg.Review.RiskPatterns = `^(cmd/|internal/(?:config|workflow)/|scripts/)`
	vr := Validate(cfg, nil)

	for _, e := range vr.Errors() {
		if e.Field == "review.extensions" || e.Field == "review.risk_patterns" {
			t.Errorf("complex but valid regex should not produce error: %v", e)
		}
	}
}

// --- Additional tests: empty review regex fields are valid ---

func TestValidate_EmptyReviewFields_Valid(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review = ReviewConfig{}
	vr := Validate(cfg, nil)

	for _, e := range vr.Errors() {
		if strings.HasPrefix(e.Field, "review.") {
			t.Errorf("empty review config should not produce errors: %v", e)
		}
	}
}

// --- Additional tests: empty prompt_template and prompts_dir produce no warnings ---

func TestValidate_EmptyPathFields_NoWarning(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Review.PromptsDir = ""
	cfg.Review.ProjectBriefFile = ""
	cfg.Agents["claude"] = AgentConfig{
		Command:        "claude",
		PromptTemplate: "",
	}
	cfg.Project.TasksDir = ""
	cfg.Project.LogDir = ""
	vr := Validate(cfg, nil)

	for _, w := range vr.Warnings() {
		switch w.Field {
		case "project.tasks_dir", "project.log_dir",
			"agents.claude.prompt_template",
			"review.prompts_dir", "review.project_brief_file":
			t.Errorf("empty path field should not produce a warning: %v", w)
		}
	}
}
