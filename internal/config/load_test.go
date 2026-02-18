package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdataPath returns the absolute path to a file in the repo-root testdata/ directory.
func testdataPath(t *testing.T, name string) string {
	t.Helper()
	// The test binary runs in the package directory; testdata is at repo root.
	// Walk up from the package dir to find the repo root.
	wd, err := os.Getwd()
	require.NoError(t, err)
	// internal/config -> repo root is ../../
	return filepath.Join(wd, "..", "..", "testdata", name)
}

// --- LoadFromFile tests ---

func TestLoadFromFile_ValidFull(t *testing.T) {
	t.Parallel()
	cfg, md, err := LoadFromFile(testdataPath(t, "valid-full.toml"))
	require.NoError(t, err)

	// Project section.
	assert.Equal(t, "my-project", cfg.Project.Name)
	assert.Equal(t, "go", cfg.Project.Language)
	assert.Equal(t, "docs/tasks", cfg.Project.TasksDir)
	assert.Equal(t, "docs/tasks/task-state.conf", cfg.Project.TaskStateFile)
	assert.Equal(t, "docs/tasks/phases.conf", cfg.Project.PhasesConf)
	assert.Equal(t, "docs/tasks/PROGRESS.md", cfg.Project.ProgressFile)
	assert.Equal(t, "scripts/logs", cfg.Project.LogDir)
	assert.Equal(t, "prompts", cfg.Project.PromptDir)
	assert.Equal(t, "phase/{phase_id}-{slug}", cfg.Project.BranchTemplate)
	assert.Equal(t, []string{"go build ./...", "go test ./...", "go vet ./..."}, cfg.Project.VerificationCommands)

	// Agents section.
	require.Len(t, cfg.Agents, 2)
	claude, ok := cfg.Agents["claude"]
	require.True(t, ok, "expected agents.claude to exist")
	assert.Equal(t, "claude", claude.Command)
	assert.Equal(t, "claude-opus-4-6", claude.Model)
	assert.Equal(t, "high", claude.Effort)
	assert.Equal(t, "prompts/implement-claude.md", claude.PromptTemplate)
	assert.Equal(t, "Edit,Write,Read,Glob,Grep,Bash(go*),Bash(git*)", claude.AllowedTools)

	codex, ok := cfg.Agents["codex"]
	require.True(t, ok, "expected agents.codex to exist")
	assert.Equal(t, "codex", codex.Command)
	assert.Equal(t, "gpt-5.3-codex", codex.Model)

	// Review section.
	assert.Equal(t, `(\.go$|go\.mod$|go\.sum$)`, cfg.Review.Extensions)
	assert.Equal(t, `^(cmd/|internal/|scripts/)`, cfg.Review.RiskPatterns)
	assert.Equal(t, ".github/review/prompts", cfg.Review.PromptsDir)
	assert.Equal(t, ".github/review/rules", cfg.Review.RulesDir)
	assert.Equal(t, ".github/review/PROJECT_BRIEF.md", cfg.Review.ProjectBriefFile)

	// Workflows section.
	require.Len(t, cfg.Workflows, 1)
	wf, ok := cfg.Workflows["implement-review-pr"]
	require.True(t, ok, "expected workflows.implement-review-pr to exist")
	assert.Equal(t, "Full implementation cycle", wf.Description)
	assert.Equal(t, []string{"implement", "review", "fix", "pr"}, wf.Steps)
	require.Len(t, wf.Transitions, 2)
	assert.Equal(t, "review", wf.Transitions["implement"]["success"])
	assert.Equal(t, "implement", wf.Transitions["implement"]["failure"])

	// Metadata should have no undecoded keys for a fully valid config.
	assert.Empty(t, md.Undecoded(), "expected no undecoded keys for valid-full.toml")
}

func TestLoadFromFile_PartialConfig(t *testing.T) {
	t.Parallel()
	cfg, _, err := LoadFromFile(testdataPath(t, "valid-partial.toml"))
	require.NoError(t, err)

	assert.Equal(t, "partial-project", cfg.Project.Name)
	assert.Equal(t, "python", cfg.Project.Language)

	// Fields not in file should be zero-valued.
	assert.Empty(t, cfg.Project.TasksDir)
	assert.Empty(t, cfg.Project.LogDir)
	assert.Nil(t, cfg.Agents)
	assert.Nil(t, cfg.Workflows)
	assert.Empty(t, cfg.Review.Extensions)
}

func TestLoadFromFile_MultipleAgents(t *testing.T) {
	t.Parallel()
	cfg, _, err := LoadFromFile(testdataPath(t, "valid-full.toml"))
	require.NoError(t, err)
	require.Len(t, cfg.Agents, 2)

	_, hasClaude := cfg.Agents["claude"]
	_, hasCodex := cfg.Agents["codex"]
	assert.True(t, hasClaude, "expected agents map to contain claude")
	assert.True(t, hasCodex, "expected agents map to contain codex")
}

func TestLoadFromFile_MalformedTOML(t *testing.T) {
	t.Parallel()
	_, _, err := LoadFromFile(testdataPath(t, "invalid-malformed.toml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestLoadFromFile_NonExistentFile(t *testing.T) {
	t.Parallel()
	_, _, err := LoadFromFile("/nonexistent/path/raven.toml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestLoadFromFile_ReturnsMetadata(t *testing.T) {
	t.Parallel()
	_, md, err := LoadFromFile(testdataPath(t, "valid-unknown-keys.toml"))
	require.NoError(t, err)

	undecoded := md.Undecoded()
	require.NotEmpty(t, undecoded, "expected undecoded keys for config with unknown keys")

	// Collect undecoded key strings for assertion.
	keys := make([]string, 0, len(undecoded))
	for _, k := range undecoded {
		keys = append(keys, k.String())
	}
	assert.Contains(t, keys, "project.unknown_key")
	assert.Contains(t, keys, "unknown_section.foo")
}

func TestLoadFromFile_EmptyFile(t *testing.T) {
	t.Parallel()
	cfg, _, err := LoadFromFile(testdataPath(t, "valid-empty.toml"))
	require.NoError(t, err)

	// All fields should be zero values.
	assert.Empty(t, cfg.Project.Name)
	assert.Nil(t, cfg.Agents)
	assert.Nil(t, cfg.Workflows)
	assert.Empty(t, cfg.Review.Extensions)
}

func TestLoadFromFile_CommentsOnly(t *testing.T) {
	t.Parallel()
	cfg, _, err := LoadFromFile(testdataPath(t, "valid-comments-only.toml"))
	require.NoError(t, err)

	// Same as empty: all fields should be zero values.
	assert.Empty(t, cfg.Project.Name)
	assert.Nil(t, cfg.Agents)
}

func TestLoadFromFile_UTF8(t *testing.T) {
	t.Parallel()
	cfg, _, err := LoadFromFile(testdataPath(t, "valid-utf8.toml"))
	require.NoError(t, err)

	assert.Equal(t, "prøject-naïve", cfg.Project.Name)
	assert.Equal(t, "gö", cfg.Project.Language)
}

func TestLoadFromFile_MultilineStrings(t *testing.T) {
	t.Parallel()
	cfg, _, err := LoadFromFile(testdataPath(t, "valid-multiline.toml"))
	require.NoError(t, err)

	wf, ok := cfg.Workflows["test"]
	require.True(t, ok)
	assert.Contains(t, wf.Description, "multi-line")
	assert.Contains(t, wf.Description, "workflow description")
	assert.Equal(t, []string{"step1", "step2"}, wf.Steps)
}

func TestLoadFromFile_SpecialAgentNames(t *testing.T) {
	t.Parallel()
	cfg, _, err := LoadFromFile(testdataPath(t, "valid-special-agent-names.toml"))
	require.NoError(t, err)

	require.Len(t, cfg.Agents, 2)

	claude3, ok := cfg.Agents["claude-3"]
	require.True(t, ok, "expected agents with hyphen in name")
	assert.Equal(t, "claude", claude3.Command)
	assert.Equal(t, "claude-3-opus", claude3.Model)

	gpt4, ok := cfg.Agents["gpt.4"]
	require.True(t, ok, "expected agents with dot in name")
	assert.Equal(t, "gpt", gpt4.Command)
	assert.Equal(t, "gpt-4", gpt4.Model)
}

// --- FindConfigFile tests ---

func TestFindConfigFile_InCurrentDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, ConfigFileName)
	require.NoError(t, os.WriteFile(configPath, []byte("# test\n"), 0o644))

	found, err := FindConfigFile(dir)
	require.NoError(t, err)
	assert.Equal(t, configPath, found)
}

func TestFindConfigFile_InParentDir(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	child := filepath.Join(parent, "sub", "deep")
	require.NoError(t, os.MkdirAll(child, 0o755))

	configPath := filepath.Join(parent, ConfigFileName)
	require.NoError(t, os.WriteFile(configPath, []byte("# test\n"), 0o644))

	found, err := FindConfigFile(child)
	require.NoError(t, err)
	assert.Equal(t, configPath, found)
}

func TestFindConfigFile_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	found, err := FindConfigFile(dir)
	require.NoError(t, err)
	assert.Empty(t, found, "expected empty string when config not found")
}

func TestFindConfigFile_AtRoot(t *testing.T) {
	t.Parallel()
	// Start from filesystem root -- should not infinite loop, returns empty.
	found, err := FindConfigFile("/")
	require.NoError(t, err)
	// Unless someone has /raven.toml on their machine, this should be empty.
	// We just verify no error or infinite loop.
	_ = found
}

func TestFindConfigFile_DeeplyNested(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Create a 25-level deep directory tree.
	deepPath := root
	for i := 0; i < 25; i++ {
		deepPath = filepath.Join(deepPath, "level")
	}
	require.NoError(t, os.MkdirAll(deepPath, 0o755))

	// Place config at root.
	configPath := filepath.Join(root, ConfigFileName)
	require.NoError(t, os.WriteFile(configPath, []byte("# deep test\n"), 0o644))

	found, err := FindConfigFile(deepPath)
	require.NoError(t, err)
	assert.Equal(t, configPath, found)
}

func TestFindConfigFile_ReturnsAbsolutePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, ConfigFileName)
	require.NoError(t, os.WriteFile(configPath, []byte("# test\n"), 0o644))

	found, err := FindConfigFile(dir)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(found), "expected absolute path, got %s", found)
}
