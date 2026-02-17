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

// TestListTemplates verifies that ListTemplates returns the expected set of
// templates embedded in the binary.
func TestListTemplates(t *testing.T) {
	names, err := ListTemplates()
	require.NoError(t, err)
	assert.Contains(t, names, "go-cli", "go-cli template must be listed")
}

// TestTemplateExists_known verifies that TemplateExists returns true for the
// embedded go-cli template.
func TestTemplateExists_known(t *testing.T) {
	assert.True(t, TemplateExists("go-cli"))
}

// TestTemplateExists_unknown verifies that TemplateExists returns false for a
// non-existent template.
func TestTemplateExists_unknown(t *testing.T) {
	assert.False(t, TemplateExists("nonexistent"))
	assert.False(t, TemplateExists(""))
	assert.False(t, TemplateExists("../etc"))
}

// TestRenderTemplate_invalidName verifies that RenderTemplate returns an error
// when the requested template does not exist.
func TestRenderTemplate_invalidName(t *testing.T) {
	dir := t.TempDir()
	_, err := RenderTemplate("nonexistent", dir, TemplateVars{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestRenderTemplate_createsDestDir verifies that RenderTemplate creates the
// destination directory when it does not yet exist.
func TestRenderTemplate_createsDestDir(t *testing.T) {
	dir := t.TempDir()
	newDir := filepath.Join(dir, "newproject")

	_, err := RenderTemplate("go-cli", newDir, TemplateVars{
		ProjectName: "myproject",
		Language:    "go",
		ModulePath:  "github.com/example/myproject",
	})
	require.NoError(t, err)

	info, err := os.Stat(newDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestRenderTemplate_createsRavenToml verifies that the .tmpl file is rendered
// and the extension is stripped (raven.toml.tmpl -> raven.toml).
func TestRenderTemplate_createsRavenToml(t *testing.T) {
	dir := t.TempDir()
	vars := TemplateVars{
		ProjectName: "test-project",
		Language:    "go",
		ModulePath:  "github.com/example/test-project",
	}

	created, err := RenderTemplate("go-cli", dir, vars)
	require.NoError(t, err)

	tomlPath := filepath.Join(dir, "raven.toml")
	assert.FileExists(t, tomlPath, "raven.toml must be created (extension stripped from .tmpl)")

	// The .tmpl source must NOT appear.
	assert.NoFileExists(t, filepath.Join(dir, "raven.toml.tmpl"))

	// Confirm it's in the created list.
	assert.Contains(t, created, tomlPath)
}

// TestRenderTemplate_substitutesVars verifies that TemplateVars fields are
// correctly substituted into .tmpl files.
func TestRenderTemplate_substitutesVars(t *testing.T) {
	tests := []struct {
		name        string
		vars        TemplateVars
		wantInToml  []string
	}{
		{
			name: "project name and language appear in raven.toml",
			vars: TemplateVars{
				ProjectName: "awesome-cli",
				Language:    "go",
				ModulePath:  "github.com/org/awesome-cli",
			},
			wantInToml: []string{
				`name = "awesome-cli"`,
				`language = "go"`,
			},
		},
		{
			name: "different project name",
			vars: TemplateVars{
				ProjectName: "another-tool",
				Language:    "go",
				ModulePath:  "github.com/org/another-tool",
			},
			wantInToml: []string{
				`name = "another-tool"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := RenderTemplate("go-cli", dir, tt.vars)
			require.NoError(t, err)

			content, err := os.ReadFile(filepath.Join(dir, "raven.toml"))
			require.NoError(t, err)

			for _, want := range tt.wantInToml {
				assert.Contains(t, string(content), want, "raven.toml must contain %q", want)
			}
		})
	}
}

// TestRenderTemplate_renderedTomlIsValidTOML verifies that the rendered
// raven.toml can be parsed by the BurntSushi/toml decoder.
func TestRenderTemplate_renderedTomlIsValidTOML(t *testing.T) {
	dir := t.TempDir()
	vars := TemplateVars{
		ProjectName: "integration-test",
		Language:    "go",
		ModulePath:  "github.com/example/integration-test",
	}

	_, err := RenderTemplate("go-cli", dir, vars)
	require.NoError(t, err)

	tomlPath := filepath.Join(dir, "raven.toml")
	var cfg Config
	_, tomlErr := toml.DecodeFile(tomlPath, &cfg)
	require.NoError(t, tomlErr, "rendered raven.toml must be valid TOML")
	assert.Equal(t, "integration-test", cfg.Project.Name)
	assert.Equal(t, "go", cfg.Project.Language)
}

// TestRenderTemplate_createsPromptsDir verifies that the prompts/ directory and
// its files are created.
func TestRenderTemplate_createsPromptsDir(t *testing.T) {
	dir := t.TempDir()
	_, err := RenderTemplate("go-cli", dir, TemplateVars{
		ProjectName: "p",
		Language:    "go",
	})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, "prompts", "implement-claude.md"))
	assert.FileExists(t, filepath.Join(dir, "prompts", "implement-codex.md"))
}

// TestRenderTemplate_createsGitHubReviewDirs verifies that the .github/review/
// directory structure is created including dotfile directories.
func TestRenderTemplate_createsGitHubReviewDirs(t *testing.T) {
	dir := t.TempDir()
	_, err := RenderTemplate("go-cli", dir, TemplateVars{
		ProjectName: "p",
		Language:    "go",
	})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".github", "review", "prompts", "review-prompt.md"))
	assert.FileExists(t, filepath.Join(dir, ".github", "review", "rules", ".gitkeep"))
	assert.FileExists(t, filepath.Join(dir, ".github", "review", "PROJECT_BRIEF.md"))
}

// TestRenderTemplate_createsDocsDirs verifies that docs/tasks/ and docs/prd/
// directories are scaffolded via .gitkeep files.
func TestRenderTemplate_createsDocsDirs(t *testing.T) {
	dir := t.TempDir()
	_, err := RenderTemplate("go-cli", dir, TemplateVars{
		ProjectName: "p",
		Language:    "go",
	})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, "docs", "tasks", ".gitkeep"))
	assert.FileExists(t, filepath.Join(dir, "docs", "prd", ".gitkeep"))
}

// TestRenderTemplate_projectBriefSubstitution verifies that PROJECT_BRIEF.md.tmpl
// is rendered with variable substitution and written as PROJECT_BRIEF.md.
func TestRenderTemplate_projectBriefSubstitution(t *testing.T) {
	dir := t.TempDir()
	vars := TemplateVars{
		ProjectName: "my-brief-project",
		Language:    "go",
		ModulePath:  "github.com/example/my-brief-project",
	}

	_, err := RenderTemplate("go-cli", dir, vars)
	require.NoError(t, err)

	briefPath := filepath.Join(dir, ".github", "review", "PROJECT_BRIEF.md")
	assert.FileExists(t, briefPath)

	content, err := os.ReadFile(briefPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "my-brief-project")
	assert.Contains(t, string(content), "go")

	// The .tmpl source must NOT appear.
	assert.NoFileExists(t, filepath.Join(dir, ".github", "review", "PROJECT_BRIEF.md.tmpl"))
}

// TestRenderTemplate_doesNotOverwriteExistingFiles verifies that RenderTemplate
// skips files that already exist in the destination directory.
func TestRenderTemplate_doesNotOverwriteExistingFiles(t *testing.T) {
	dir := t.TempDir()

	// Pre-create raven.toml with known content.
	tomlPath := filepath.Join(dir, "raven.toml")
	originalContent := "# original content\n"
	err := os.WriteFile(tomlPath, []byte(originalContent), 0o644)
	require.NoError(t, err)

	// RenderTemplate must not overwrite the existing file.
	_, err = RenderTemplate("go-cli", dir, TemplateVars{
		ProjectName: "should-not-appear",
		Language:    "go",
	})
	require.NoError(t, err)

	content, err := os.ReadFile(tomlPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(content),
		"existing raven.toml must not be overwritten")
	assert.NotContains(t, string(content), "should-not-appear")
}

// TestRenderTemplate_filePermissions verifies that created files have 0644
// permissions and created directories have 0755 permissions.
func TestRenderTemplate_filePermissions(t *testing.T) {
	dir := t.TempDir()
	_, err := RenderTemplate("go-cli", dir, TemplateVars{
		ProjectName: "perm-test",
		Language:    "go",
	})
	require.NoError(t, err)

	// Check file permission.
	tomlInfo, err := os.Stat(filepath.Join(dir, "raven.toml"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), tomlInfo.Mode().Perm(),
		"raven.toml must have 0644 permissions")

	// Check directory permission.
	promptsInfo, err := os.Stat(filepath.Join(dir, "prompts"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), promptsInfo.Mode().Perm(),
		"prompts/ directory must have 0755 permissions")
}

// TestRenderTemplate_staticFilesNotModified verifies that static (non-.tmpl)
// prompt files are copied as-is without template processing.
func TestRenderTemplate_staticFilesNotModified(t *testing.T) {
	dir := t.TempDir()
	_, err := RenderTemplate("go-cli", dir, TemplateVars{
		ProjectName: "static-test",
		Language:    "go",
	})
	require.NoError(t, err)

	// The prompt files should be static markdown; they must exist and be non-empty.
	claudePath := filepath.Join(dir, "prompts", "implement-claude.md")
	content, err := os.ReadFile(claudePath)
	require.NoError(t, err)
	assert.True(t, len(content) > 0, "implement-claude.md must not be empty")
	// Verify the file does not contain unprocessed Go template syntax (since it
	// has no .tmpl extension it is copied as-is, and our static content has none).
	assert.False(t, strings.Contains(string(content), "{{"), "static file must not contain unresolved template syntax")
}

// TestRenderTemplate_allExpectedFiles verifies the complete set of files created.
func TestRenderTemplate_allExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	created, err := RenderTemplate("go-cli", dir, TemplateVars{
		ProjectName: "count-test",
		Language:    "go",
	})
	require.NoError(t, err)

	// Build a set of relative paths for easy lookup.
	relPaths := make(map[string]bool, len(created))
	for _, p := range created {
		rel, err := filepath.Rel(dir, p)
		require.NoError(t, err)
		relPaths[filepath.ToSlash(rel)] = true
	}

	expected := []string{
		"raven.toml",
		"prompts/implement-claude.md",
		"prompts/implement-codex.md",
		".github/review/prompts/review-prompt.md",
		".github/review/rules/.gitkeep",
		".github/review/PROJECT_BRIEF.md",
		"docs/tasks/.gitkeep",
		"docs/prd/.gitkeep",
	}

	for _, want := range expected {
		assert.True(t, relPaths[want], "expected file %q to be in created list", want)
	}

	assert.Equal(t, len(expected), len(created),
		"number of created files must match expected count")
}

// TestRenderTemplate_returnedPathsAreAbsolute verifies that RenderTemplate
// returns absolute file paths.
func TestRenderTemplate_returnedPathsAreAbsolute(t *testing.T) {
	dir := t.TempDir()
	created, err := RenderTemplate("go-cli", dir, TemplateVars{
		ProjectName: "abs-test",
		Language:    "go",
	})
	require.NoError(t, err)
	require.NotEmpty(t, created)

	for _, p := range created {
		assert.True(t, filepath.IsAbs(p), "created path %q must be absolute", p)
	}
}
