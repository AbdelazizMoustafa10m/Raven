package internal_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectRoot returns the absolute path to the project root directory.
// It walks up from the current file's directory until it finds go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()

	// Start from the working directory (tests run from the package directory).
	dir, err := os.Getwd()
	require.NoError(t, err, "failed to get working directory")

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (no go.mod found in any parent directory)")
		}
		dir = parent
	}
}

// readFileContent reads a file and returns its content as a string.
func readFileContent(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read file: %s", path)
	return string(data)
}

func TestInternalSubpackages_Exist(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)

	expectedPackages := []struct {
		name    string
		pkgDecl string
	}{
		{name: "cli", pkgDecl: "package cli"},
		{name: "config", pkgDecl: "package config"},
		{name: "workflow", pkgDecl: "package workflow"},
		{name: "agent", pkgDecl: "package agent"},
		{name: "task", pkgDecl: "package task"},
		{name: "loop", pkgDecl: "package loop"},
		{name: "review", pkgDecl: "package review"},
		{name: "prd", pkgDecl: "package prd"},
		{name: "pipeline", pkgDecl: "package pipeline"},
		{name: "git", pkgDecl: "package git"},
		{name: "tui", pkgDecl: "package tui"},
		{name: "buildinfo", pkgDecl: "package buildinfo"},
	}

	for _, pkg := range expectedPackages {
		t.Run(pkg.name, func(t *testing.T) {
			t.Parallel()

			pkgDir := filepath.Join(root, "internal", pkg.name)

			// Verify directory exists.
			info, err := os.Stat(pkgDir)
			require.NoError(t, err, "internal/%s directory does not exist", pkg.name)
			assert.True(t, info.IsDir(), "internal/%s is not a directory", pkg.name)

			// Verify doc.go exists and has valid package declaration.
			docPath := filepath.Join(pkgDir, "doc.go")
			content := readFileContent(t, docPath)
			assert.Contains(t, content, pkg.pkgDecl,
				"doc.go in internal/%s must contain %q", pkg.name, pkg.pkgDecl)
		})
	}
}

func TestInternalSubpackages_Count(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	internalDir := filepath.Join(root, "internal")

	entries, err := os.ReadDir(internalDir)
	require.NoError(t, err, "failed to read internal/ directory")

	// Count only directories (exclude files like project_test.go).
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}

	assert.Len(t, dirs, 12,
		"expected exactly 12 internal subpackages, got: %v", dirs)
}

func TestInternalSubpackages_DocGoHasComment(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)

	packages := []string{
		"cli", "config", "workflow", "agent", "task", "loop",
		"review", "prd", "pipeline", "git", "tui", "buildinfo",
	}

	for _, pkg := range packages {
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()

			docPath := filepath.Join(root, "internal", pkg, "doc.go")
			content := readFileContent(t, docPath)

			// doc.go should have a doc comment starting with "// Package <name>"
			expectedComment := "// Package " + pkg
			assert.Contains(t, content, expectedComment,
				"doc.go in internal/%s should have a doc comment starting with %q", pkg, expectedComment)
		})
	}
}

func TestGoMod_Exists(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	goModPath := filepath.Join(root, "go.mod")

	_, err := os.Stat(goModPath)
	require.NoError(t, err, "go.mod does not exist at project root")
}

func TestGoMod_ModulePath(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	content := readFileContent(t, filepath.Join(root, "go.mod"))

	assert.Contains(t, content, "module github.com/AbdelazizMoustafa10m/Raven",
		"go.mod must declare module path as github.com/AbdelazizMoustafa10m/Raven")
}

func TestGoMod_GoDirective(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	content := readFileContent(t, filepath.Join(root, "go.mod"))

	// The go directive should specify 1.24 or higher.
	// It may be "go 1.24", "go 1.24.0", "go 1.24.2", etc.
	assert.Contains(t, content, "go 1.24",
		"go.mod must have a Go 1.24+ directive")
}

func TestGoMod_DirectDependencies(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	content := readFileContent(t, filepath.Join(root, "go.mod"))

	expectedDeps := []struct {
		name       string
		modulePath string
	}{
		{name: "cobra", modulePath: "github.com/spf13/cobra"},
		{name: "bubbletea", modulePath: "github.com/charmbracelet/bubbletea"},
		{name: "lipgloss", modulePath: "github.com/charmbracelet/lipgloss"},
		{name: "bubbles", modulePath: "github.com/charmbracelet/bubbles"},
		{name: "huh", modulePath: "github.com/charmbracelet/huh"},
		{name: "log", modulePath: "github.com/charmbracelet/log"},
		{name: "toml", modulePath: "github.com/BurntSushi/toml"},
		{name: "sync", modulePath: "golang.org/x/sync"},
		{name: "doublestar", modulePath: "github.com/bmatcuk/doublestar"},
		{name: "testify", modulePath: "github.com/stretchr/testify"},
		{name: "xxhash", modulePath: "github.com/cespare/xxhash"},
	}

	for _, dep := range expectedDeps {
		t.Run(dep.name, func(t *testing.T) {
			t.Parallel()
			assert.Contains(t, content, dep.modulePath,
				"go.mod must declare direct dependency on %s (%s)", dep.name, dep.modulePath)
		})
	}
}

func TestGoMod_NoReplaceDirectives(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	content := readFileContent(t, filepath.Join(root, "go.mod"))

	assert.NotContains(t, content, "replace ",
		"go.mod must not contain replace directives")
}

func TestGoSum_Exists(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	goSumPath := filepath.Join(root, "go.sum")

	info, err := os.Stat(goSumPath)
	require.NoError(t, err, "go.sum does not exist at project root")
	assert.Greater(t, info.Size(), int64(0),
		"go.sum must not be empty (should contain dependency checksums)")
}

func TestGoSum_ContainsDependencyChecksums(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	content := readFileContent(t, filepath.Join(root, "go.sum"))

	// go.sum entries look like: module version h1:hash=
	// Verify some key direct dependencies have checksums.
	checksumDeps := []string{
		"github.com/spf13/cobra",
		"github.com/BurntSushi/toml",
		"github.com/stretchr/testify",
	}

	for _, dep := range checksumDeps {
		assert.Contains(t, content, dep,
			"go.sum should contain checksums for %s", dep)
	}
}

func TestTestdata_DirectoryExists(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	testdataDir := filepath.Join(root, "testdata")

	info, err := os.Stat(testdataDir)
	require.NoError(t, err, "testdata/ directory does not exist")
	assert.True(t, info.IsDir(), "testdata/ is not a directory")
}

func TestTemplates_DirectoryExists(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	templatesDir := filepath.Join(root, "templates", "go-cli")

	info, err := os.Stat(templatesDir)
	require.NoError(t, err, "templates/go-cli/ directory does not exist")
	assert.True(t, info.IsDir(), "templates/go-cli/ is not a directory")
}

func TestGitignore_Exists(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	gitignorePath := filepath.Join(root, ".gitignore")

	_, err := os.Stat(gitignorePath)
	require.NoError(t, err, ".gitignore does not exist at project root")
}

func TestGitignore_RequiredEntries(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	content := readFileContent(t, filepath.Join(root, ".gitignore"))

	requiredEntries := []struct {
		name    string
		pattern string
	}{
		{name: "compiled binaries (exe)", pattern: "*.exe"},
		{name: "raven state directory", pattern: ".raven/"},
		{name: "dist directory", pattern: "dist/"},
		{name: "vendor directory", pattern: "vendor/"},
		{name: "IDE files (idea)", pattern: ".idea/"},
		{name: "IDE files (vscode)", pattern: ".vscode/"},
	}

	for _, entry := range requiredEntries {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			assert.Contains(t, content, entry.pattern,
				".gitignore must include pattern %q for %s", entry.pattern, entry.name)
		})
	}
}

func TestSourceFiles_NoInitFunctions(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)

	// Per CLAUDE.md: "No init() functions except for cobra command registration"
	// For T-001 there should be no init() functions at all since Cobra is not yet wired up.
	goFiles := []string{
		filepath.Join(root, "cmd", "raven", "main.go"),
	}

	// Also check all doc.go files.
	packages := []string{
		"cli", "config", "workflow", "agent", "task", "loop",
		"review", "prd", "pipeline", "git", "tui", "buildinfo",
	}
	for _, pkg := range packages {
		goFiles = append(goFiles, filepath.Join(root, "internal", pkg, "doc.go"))
	}

	for _, file := range goFiles {
		t.Run(filepath.Base(filepath.Dir(file))+"/"+filepath.Base(file), func(t *testing.T) {
			t.Parallel()

			content := readFileContent(t, file)
			assert.NotContains(t, content, "func init()",
				"file %s must not contain init() functions per project conventions", file)
		})
	}
}

func TestMainGo_Exists(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	mainPath := filepath.Join(root, "cmd", "raven", "main.go")

	_, err := os.Stat(mainPath)
	require.NoError(t, err, "cmd/raven/main.go does not exist")
}

func TestMainGo_PackageMain(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	content := readFileContent(t, filepath.Join(root, "cmd", "raven", "main.go"))

	assert.Contains(t, content, "package main",
		"cmd/raven/main.go must declare package main")
}

func TestMainGo_HasMainFunction(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	content := readFileContent(t, filepath.Join(root, "cmd", "raven", "main.go"))

	assert.Contains(t, content, "func main()",
		"cmd/raven/main.go must define a main function")
}

func TestToolsGo_Exists(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	toolsPath := filepath.Join(root, "tools.go")

	_, err := os.Stat(toolsPath)
	require.NoError(t, err, "tools.go does not exist at project root")
}

func TestToolsGo_HasBuildTag(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	content := readFileContent(t, filepath.Join(root, "tools.go"))

	assert.Contains(t, content, "//go:build tools",
		"tools.go must have //go:build tools build tag")
}

func TestProjectStructure_CmdRavenDir(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	cmdDir := filepath.Join(root, "cmd", "raven")

	info, err := os.Stat(cmdDir)
	require.NoError(t, err, "cmd/raven/ directory does not exist")
	assert.True(t, info.IsDir(), "cmd/raven/ is not a directory")
}

func TestProjectStructure_InternalDir(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	internalDir := filepath.Join(root, "internal")

	info, err := os.Stat(internalDir)
	require.NoError(t, err, "internal/ directory does not exist")
	assert.True(t, info.IsDir(), "internal/ is not a directory")
}

func TestGoMod_DependencyVersions(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	content := readFileContent(t, filepath.Join(root, "go.mod"))

	// Verify minimum version requirements from the task spec.
	versionChecks := []struct {
		name       string
		dep        string
		minVersion string
	}{
		{name: "toml v1.5.0", dep: "github.com/BurntSushi/toml", minVersion: "v1.5.0"},
		{name: "cobra v1.10+", dep: "github.com/spf13/cobra", minVersion: "v1.10"},
		{name: "doublestar v4.10+", dep: "github.com/bmatcuk/doublestar/v4", minVersion: "v4.10"},
		{name: "sync v0.19+", dep: "golang.org/x/sync", minVersion: "v0.19"},
	}

	for _, vc := range versionChecks {
		t.Run(vc.name, func(t *testing.T) {
			t.Parallel()
			assert.Contains(t, content, vc.dep,
				"go.mod must contain dependency %s", vc.dep)
			// Extract the version line for this dependency.
			scanner := bufio.NewScanner(strings.NewReader(content))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.Contains(line, vc.dep) && !strings.HasPrefix(line, "//") {
					assert.Contains(t, line, vc.minVersion,
						"dependency %s must be at least version %s", vc.dep, vc.minVersion)
					break
				}
			}
		})
	}
}
