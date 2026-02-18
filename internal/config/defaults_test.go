package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaults(t *testing.T) {
	t.Parallel()
	cfg := NewDefaults()
	require.NotNil(t, cfg)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "TasksDir", got: cfg.Project.TasksDir, want: "docs/tasks"},
		{name: "TaskStateFile", got: cfg.Project.TaskStateFile, want: "docs/tasks/task-state.conf"},
		{name: "PhasesConf", got: cfg.Project.PhasesConf, want: "docs/tasks/phases.conf"},
		{name: "ProgressFile", got: cfg.Project.ProgressFile, want: "docs/tasks/PROGRESS.md"},
		{name: "LogDir", got: cfg.Project.LogDir, want: "scripts/logs"},
		{name: "PromptDir", got: cfg.Project.PromptDir, want: "prompts"},
		{name: "BranchTemplate", got: cfg.Project.BranchTemplate, want: "phase/{phase_id}-{slug}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.got)
		})
	}

	// Project name and language should be empty (project-specific).
	assert.Empty(t, cfg.Project.Name, "project name should be empty by default")
	assert.Empty(t, cfg.Project.Language, "project language should be empty by default")
	assert.Nil(t, cfg.Project.VerificationCommands, "verification commands should be nil by default")
}

func TestNewDefaults_EmptyAgents(t *testing.T) {
	t.Parallel()
	cfg := NewDefaults()
	require.NotNil(t, cfg.Agents, "agents map should not be nil")
	assert.Empty(t, cfg.Agents, "agents map should be empty by default")
}

func TestNewDefaults_EmptyWorkflows(t *testing.T) {
	t.Parallel()
	cfg := NewDefaults()
	require.NotNil(t, cfg.Workflows, "workflows map should not be nil")
	assert.Empty(t, cfg.Workflows, "workflows map should be empty by default")
}

func TestNewDefaults_ZeroReview(t *testing.T) {
	t.Parallel()
	cfg := NewDefaults()
	assert.Empty(t, cfg.Review.Extensions, "review extensions should be empty by default")
	assert.Empty(t, cfg.Review.RiskPatterns, "review risk_patterns should be empty by default")
	assert.Empty(t, cfg.Review.PromptsDir, "review prompts_dir should be empty by default")
	assert.Empty(t, cfg.Review.RulesDir, "review rules_dir should be empty by default")
	assert.Empty(t, cfg.Review.ProjectBriefFile, "review project_brief_file should be empty by default")
}
