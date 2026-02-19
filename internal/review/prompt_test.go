package review

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ContextLoader tests
// ---------------------------------------------------------------------------

func TestNewContextLoader(t *testing.T) {
	t.Parallel()

	cl := NewContextLoader("/some/brief.md", "/some/rules")
	require.NotNil(t, cl)
	assert.Equal(t, "/some/brief.md", cl.briefPath)
	assert.Equal(t, "/some/rules", cl.rulesDir)
}

func TestContextLoader_Load_EmptyPaths(t *testing.T) {
	t.Parallel()

	cl := NewContextLoader("", "")
	ctx, err := cl.Load()
	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.Empty(t, ctx.Brief)
	assert.Empty(t, ctx.Rules)
}

func TestContextLoader_Load_MissingBriefIsNotError(t *testing.T) {
	t.Parallel()

	cl := NewContextLoader("/nonexistent/path/brief.md", "")
	ctx, err := cl.Load()
	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.Empty(t, ctx.Brief)
}

func TestContextLoader_Load_MissingRulesDirIsNotError(t *testing.T) {
	t.Parallel()

	cl := NewContextLoader("", "/nonexistent/rules/dir")
	ctx, err := cl.Load()
	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.Empty(t, ctx.Rules)
}

func TestContextLoader_Load_ReadsProjectBrief(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	briefPath := filepath.Join(dir, "brief.md")
	require.NoError(t, os.WriteFile(briefPath, []byte("# Project Brief\nThis is the brief."), 0644))

	cl := NewContextLoader(briefPath, "")
	ctx, err := cl.Load()
	require.NoError(t, err)
	assert.Equal(t, "# Project Brief\nThis is the brief.", ctx.Brief)
}

func TestContextLoader_Load_ReadsRuleFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0755))

	// Create rule files in non-alphabetical order to verify sorting.
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "02-style.md"), []byte("Style rules"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "01-security.md"), []byte("Security rules"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "03-testing.md"), []byte("Testing rules"), 0644))
	// Non-markdown file should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "ignore.txt"), []byte("Ignore this"), 0644))

	cl := NewContextLoader("", rulesDir)
	ctx, err := cl.Load()
	require.NoError(t, err)
	require.Len(t, ctx.Rules, 3)
	assert.Equal(t, "Security rules", ctx.Rules[0])
	assert.Equal(t, "Style rules", ctx.Rules[1])
	assert.Equal(t, "Testing rules", ctx.Rules[2])
}

func TestContextLoader_Load_SkipsSubdirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(rulesDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "rule.md"), []byte("A rule"), 0644))
	// File inside subdir should not be included (we only read the top-level dir).
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "subdir", "nested.md"), []byte("Nested"), 0644))

	cl := NewContextLoader("", rulesDir)
	ctx, err := cl.Load()
	require.NoError(t, err)
	require.Len(t, ctx.Rules, 1)
	assert.Equal(t, "A rule", ctx.Rules[0])
}

func TestContextLoader_Load_PathTraversalRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		briefPath string
		rulesDir  string
	}{
		{
			name:      "brief path with traversal",
			briefPath: "../../../etc/passwd",
		},
		{
			name:     "rules dir with traversal",
			rulesDir: "some/../../etc",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cl := NewContextLoader(tt.briefPath, tt.rulesDir)
			_, err := cl.Load()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "path traversal rejected")
		})
	}
}

// ---------------------------------------------------------------------------
// validatePath tests
// ---------------------------------------------------------------------------

func TestValidatePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"absolute path", "/usr/local/bin/raven", false},
		{"relative path no traversal", "internal/review", false},
		{"single dot", ".", false},
		{"parent traversal", "../etc/passwd", true},
		{"double parent traversal", "../../etc", true},
		// Note: "/tmp/some/../../etc" cleans to "/etc" (no ".." left after Clean)
		// so it is NOT rejected — absolute paths resolve without traversal components.
		{"absolute path with traversal cleaned away", "/tmp/some/../../etc", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validatePath(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "path traversal rejected")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatFileList tests
// ---------------------------------------------------------------------------

func TestFormatFileList_Empty(t *testing.T) {
	t.Parallel()

	list, highRisk := formatFileList(nil)
	assert.Empty(t, list)
	assert.Empty(t, highRisk)

	list, highRisk = formatFileList([]ChangedFile{})
	assert.Empty(t, list)
	assert.Empty(t, highRisk)
}

func TestFormatFileList_HighRiskAnnotation(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "internal/auth/handler.go", ChangeType: ChangeModified, Risk: RiskHigh, LinesAdded: 42, LinesDeleted: 10},
		{Path: "internal/config/load.go", ChangeType: ChangeAdded, Risk: RiskNormal, LinesAdded: 150},
		{Path: "README.md", ChangeType: ChangeModified, Risk: RiskNormal, LinesAdded: 5, LinesDeleted: 2},
	}

	list, highRisk := formatFileList(files)

	assert.Contains(t, list, "[HIGH RISK] internal/auth/handler.go")
	assert.Contains(t, list, "+42/-10")
	assert.Contains(t, list, "internal/config/load.go")
	assert.Contains(t, list, "+150")
	assert.Contains(t, list, "README.md")
	assert.Contains(t, list, "+5/-2")

	require.Len(t, highRisk, 1)
	assert.Equal(t, "internal/auth/handler.go", highRisk[0])
}

func TestFormatFileList_RenamedFile(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "new_name.go", ChangeType: ChangeRenamed, Risk: RiskNormal, OldPath: "old_name.go", LinesAdded: 5, LinesDeleted: 2},
	}

	list, _ := formatFileList(files)
	assert.Contains(t, list, "renamed from old_name.go")
	assert.Contains(t, list, "+5/-2")
}

func TestFormatFileList_DeletedFileOnlyLines(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "removed.go", ChangeType: ChangeDeleted, Risk: RiskNormal, LinesDeleted: 30},
	}

	list, _ := formatFileList(files)
	assert.Contains(t, list, "deleted")
	assert.Contains(t, list, "-30")
}

func TestFormatFileList_TruncatesAt500(t *testing.T) {
	t.Parallel()

	files := make([]ChangedFile, 600)
	for i := range files {
		files[i] = ChangedFile{Path: "file.go", ChangeType: ChangeModified, Risk: RiskNormal}
	}

	list, _ := formatFileList(files)
	assert.Contains(t, list, "... and 100 more files (showing 500 of 600)")
}

func TestFormatFileList_ExactlyAtLimit_NoTruncation(t *testing.T) {
	t.Parallel()

	files := make([]ChangedFile, maxFilesInList)
	for i := range files {
		files[i] = ChangedFile{Path: "file.go", ChangeType: ChangeModified, Risk: RiskNormal}
	}

	list, _ := formatFileList(files)
	assert.NotContains(t, list, "more files")
}

// ---------------------------------------------------------------------------
// buildChangeSummary tests
// ---------------------------------------------------------------------------

func TestBuildChangeSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file ChangedFile
		want string
	}{
		{
			name: "added and deleted lines",
			file: ChangedFile{ChangeType: ChangeModified, LinesAdded: 42, LinesDeleted: 10},
			want: "modified, +42/-10",
		},
		{
			name: "only added lines",
			file: ChangedFile{ChangeType: ChangeAdded, LinesAdded: 150},
			want: "added, +150",
		},
		{
			name: "only deleted lines",
			file: ChangedFile{ChangeType: ChangeDeleted, LinesDeleted: 30},
			want: "deleted, -30",
		},
		{
			name: "no line counts",
			file: ChangedFile{ChangeType: ChangeModified},
			want: "modified",
		},
		{
			name: "renamed with old path",
			file: ChangedFile{ChangeType: ChangeRenamed, OldPath: "old.go", LinesAdded: 5, LinesDeleted: 2},
			want: "renamed from old.go, +5/-2",
		},
		{
			name: "renamed without old path",
			file: ChangedFile{ChangeType: ChangeRenamed, LinesAdded: 3},
			want: "renamed, +3",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildChangeSummary(tt.file)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// PromptBuilder.Build tests
// ---------------------------------------------------------------------------

func TestNewPromptBuilder(t *testing.T) {
	t.Parallel()

	cfg := ReviewConfig{
		PromptsDir:       "/prompts",
		RulesDir:         "/rules",
		ProjectBriefFile: "/brief.md",
	}
	pb := NewPromptBuilder(cfg, nil)
	require.NotNil(t, pb)
	assert.NotNil(t, pb.loader)
	assert.Equal(t, cfg, pb.cfg)
}

func TestPromptBuilder_Build_UsesEmbeddedTemplate(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:  "claude",
		ReviewMode: ReviewModeAll,
		Stats:      DiffStats{TotalFiles: 3, FilesModified: 2, FilesAdded: 1},
		JSONSchema: jsonSchemaExample,
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, result, "claude")
	assert.Contains(t, result, "APPROVED|CHANGES_NEEDED|BLOCKING")
	assert.Contains(t, result, "Total files changed: 3")
}

func TestPromptBuilder_Build_UsesCustomTemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	customTmpl := `Agent: [[ .AgentName ]], Files: [[ .Stats.TotalFiles ]]`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review.tmpl"), []byte(customTmpl), 0644))

	pb := NewPromptBuilder(ReviewConfig{PromptsDir: dir}, nil)
	data := PromptData{
		AgentName: "codex",
		Stats:     DiffStats{TotalFiles: 5},
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Equal(t, "Agent: codex, Files: 5", result)
}

func TestPromptBuilder_Build_FallsBackToMDTemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Only review.md exists (no review.tmpl).
	customTmpl := `MD template: [[ .AgentName ]]`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review.md"), []byte(customTmpl), 0644))

	pb := NewPromptBuilder(ReviewConfig{PromptsDir: dir}, nil)
	data := PromptData{AgentName: "gemini"}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Equal(t, "MD template: gemini", result)
}

func TestPromptBuilder_Build_FallsBackToEmbeddedWhenNeitherExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Empty directory — neither review.tmpl nor review.md.

	pb := NewPromptBuilder(ReviewConfig{PromptsDir: dir}, nil)
	data := PromptData{
		AgentName:  "claude",
		JSONSchema: jsonSchemaExample,
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	// Should have rendered using the embedded template.
	assert.Contains(t, result, "claude")
	assert.Contains(t, result, "APPROVED|CHANGES_NEEDED|BLOCKING")
}

func TestPromptBuilder_Build_InvalidTemplateSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Template with invalid syntax (mismatched delimiter).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review.tmpl"), []byte(`[[ .Unclosed `), 0644))

	pb := NewPromptBuilder(ReviewConfig{PromptsDir: dir}, nil)
	_, err := pb.Build(context.Background(), PromptData{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse template")
	assert.Contains(t, err.Error(), "review.tmpl")
}

func TestPromptBuilder_Build_PathTraversalInPromptsDir(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{PromptsDir: "../../../etc"}, nil)
	_, err := pb.Build(context.Background(), PromptData{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal rejected")
}

func TestPromptBuilder_Build_ProjectBriefIncluded(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:    "claude",
		ProjectBrief: "This is a Go CLI tool.",
		JSONSchema:   jsonSchemaExample,
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, result, "This is a Go CLI tool.")
}

func TestPromptBuilder_Build_RulesIncluded(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:  "claude",
		Rules:      []string{"Rule 1: No global state.", "Rule 2: Always wrap errors."},
		JSONSchema: jsonSchemaExample,
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, result, "Rule 1: No global state.")
	assert.Contains(t, result, "Rule 2: Always wrap errors.")
}

func TestPromptBuilder_Build_DiffIncluded(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:  "claude",
		Diff:       "diff --git a/main.go b/main.go\n+func main() {}",
		JSONSchema: jsonSchemaExample,
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, result, "diff --git a/main.go b/main.go")
}

// ---------------------------------------------------------------------------
// PromptBuilder.Build — large diff truncation
// ---------------------------------------------------------------------------

func TestPromptBuilder_Build_DiffTruncatedAt100KB(t *testing.T) {
	t.Parallel()

	// The truncation is done in BuildForAgent, so we test it directly via data.
	largeDiff := strings.Repeat("a", maxDiffBytes+100)

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:  "claude",
		Diff:       largeDiff[:maxDiffBytes] + "\n... [diff truncated at 100KB] ...",
		JSONSchema: jsonSchemaExample,
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, result, "[diff truncated at 100KB]")
}

// ---------------------------------------------------------------------------
// PromptBuilder.BuildForAgent tests
// ---------------------------------------------------------------------------

func TestPromptBuilder_BuildForAgent_Basic(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)

	diff := &DiffResult{
		Files: []ChangedFile{
			{Path: "main.go", ChangeType: ChangeModified, Risk: RiskNormal, LinesAdded: 5, LinesDeleted: 2},
		},
		FullDiff:   "diff output",
		BaseBranch: "main",
		Stats: DiffStats{
			TotalFiles:    1,
			FilesModified: 1,
		},
	}

	result, err := pb.BuildForAgent(
		context.Background(),
		"claude",
		diff,
		diff.Files,
		ReviewModeAll,
	)
	require.NoError(t, err)
	assert.Contains(t, result, "claude")
	assert.Contains(t, result, "main.go")
	assert.Contains(t, result, "diff output")
}

func TestPromptBuilder_BuildForAgent_SplitModeUsesSubsetStats(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)

	allFiles := []ChangedFile{
		{Path: "a.go", ChangeType: ChangeAdded, Risk: RiskNormal, LinesAdded: 100},
		{Path: "b.go", ChangeType: ChangeModified, Risk: RiskNormal, LinesAdded: 10, LinesDeleted: 5},
	}

	// The agent only reviews b.go.
	agentFiles := []ChangedFile{allFiles[1]}

	diff := &DiffResult{
		Files:      allFiles,
		FullDiff:   "full diff",
		BaseBranch: "main",
		Stats: DiffStats{
			TotalFiles:      2,
			FilesAdded:      1,
			FilesModified:   1,
			TotalLinesAdded: 110,
		},
	}

	result, err := pb.BuildForAgent(
		context.Background(),
		"claude",
		diff,
		agentFiles,
		ReviewModeSplit,
	)
	require.NoError(t, err)
	// In split mode the stats reflect only the agent's subset (1 file, 10+5 lines).
	assert.Contains(t, result, "Total files changed: 1")
}

func TestPromptBuilder_BuildForAgent_TruncatesLargeDiff(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)

	largeDiff := strings.Repeat("x", maxDiffBytes+1000)
	diff := &DiffResult{
		Files:      []ChangedFile{{Path: "big.go", ChangeType: ChangeModified, Risk: RiskNormal}},
		FullDiff:   largeDiff,
		BaseBranch: "main",
	}

	result, err := pb.BuildForAgent(context.Background(), "claude", diff, diff.Files, ReviewModeAll)
	require.NoError(t, err)
	assert.Contains(t, result, "[diff truncated at 100KB]")
}

func TestPromptBuilder_BuildForAgent_HighRiskFilesHighlighted(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)

	diff := &DiffResult{
		Files: []ChangedFile{
			{Path: "internal/auth/handler.go", ChangeType: ChangeModified, Risk: RiskHigh, LinesAdded: 42, LinesDeleted: 10},
			{Path: "README.md", ChangeType: ChangeModified, Risk: RiskNormal, LinesAdded: 5},
		},
		FullDiff:   "diff output",
		BaseBranch: "main",
	}

	result, err := pb.BuildForAgent(
		context.Background(),
		"claude",
		diff,
		diff.Files,
		ReviewModeAll,
	)
	require.NoError(t, err)
	assert.Contains(t, result, "[HIGH RISK]")
	assert.Contains(t, result, "internal/auth/handler.go")
}

func TestPromptBuilder_BuildForAgent_WithProjectContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	briefPath := filepath.Join(dir, "brief.md")
	rulesDir := filepath.Join(dir, "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0755))
	require.NoError(t, os.WriteFile(briefPath, []byte("# Test Project"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "001-rules.md"), []byte("No global state"), 0644))

	pb := NewPromptBuilder(ReviewConfig{
		ProjectBriefFile: briefPath,
		RulesDir:         rulesDir,
	}, nil)

	diff := &DiffResult{
		Files:    []ChangedFile{{Path: "main.go", ChangeType: ChangeModified, Risk: RiskNormal}},
		FullDiff: "diff",
	}

	result, err := pb.BuildForAgent(context.Background(), "claude", diff, diff.Files, ReviewModeAll)
	require.NoError(t, err)
	assert.Contains(t, result, "# Test Project")
	assert.Contains(t, result, "No global state")
}

func TestPromptBuilder_BuildForAgent_WithLogger(t *testing.T) {
	t.Parallel()

	logger := log.New(io.Discard)
	pb := NewPromptBuilder(ReviewConfig{}, logger)

	diff := &DiffResult{
		Files:    []ChangedFile{{Path: "main.go", ChangeType: ChangeModified, Risk: RiskNormal}},
		FullDiff: "diff output",
	}

	result, err := pb.BuildForAgent(context.Background(), "claude", diff, diff.Files, ReviewModeAll)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestPromptBuilder_BuildForAgent_ContextCancellation(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)

	diff := &DiffResult{
		Files:    []ChangedFile{{Path: "main.go", ChangeType: ChangeModified}},
		FullDiff: "diff",
	}

	// Even with a cancelled context, Build does not block and should complete.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Build doesn't actually check context yet (future feature), so it should
	// still succeed.
	result, err := pb.BuildForAgent(ctx, "claude", diff, diff.Files, ReviewModeAll)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// Embedded template smoke test
// ---------------------------------------------------------------------------

func TestDefaultReviewTemplate_NotEmpty(t *testing.T) {
	t.Parallel()

	assert.NotEmpty(t, defaultReviewTemplate)
}

func TestDefaultReviewTemplate_ContainsJSONSchemaPlaceholder(t *testing.T) {
	t.Parallel()

	// The template must reference .JSONSchema.
	assert.Contains(t, defaultReviewTemplate, ".JSONSchema")
}

func TestDefaultReviewTemplate_UsesCustomDelimiters(t *testing.T) {
	t.Parallel()

	// Verify the template does not contain standard Go template delimiters.
	assert.NotContains(t, defaultReviewTemplate, "{{")
	assert.NotContains(t, defaultReviewTemplate, "}}")
	assert.Contains(t, defaultReviewTemplate, "[[")
	assert.Contains(t, defaultReviewTemplate, "]]")
}

// ---------------------------------------------------------------------------
// loadTemplateText — PromptsDir prefers review.tmpl over review.md
// ---------------------------------------------------------------------------

func TestLoadTemplateText_PrefersReviewTmplOverMD(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review.tmpl"), []byte("tmpl content"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review.md"), []byte("md content"), 0644))

	pb := NewPromptBuilder(ReviewConfig{PromptsDir: dir}, nil)
	text, path, err := pb.loadTemplateText()
	require.NoError(t, err)
	assert.Equal(t, "tmpl content", text)
	assert.Contains(t, path, "review.tmpl")
}

// ---------------------------------------------------------------------------
// testdata/prompts/review.tmpl fixture test
// ---------------------------------------------------------------------------

func TestPromptBuilder_Build_TestdataTemplate(t *testing.T) {
	t.Parallel()

	// Use the checked-in testdata template fixture.
	pb := NewPromptBuilder(ReviewConfig{PromptsDir: "testdata/prompts"}, nil)
	data := PromptData{
		AgentName:  "claude",
		ReviewMode: ReviewModeAll,
		Stats:      DiffStats{TotalFiles: 2},
		JSONSchema: jsonSchemaExample,
		Diff:       "diff --git a/main.go b/main.go",
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, result, "claude")
	assert.Contains(t, result, "Files: 2")
	assert.Contains(t, result, "Mode: all")
	assert.Contains(t, result, "diff --git a/main.go b/main.go")
}

// ---------------------------------------------------------------------------
// loadRuleFiles edge cases
// ---------------------------------------------------------------------------

func TestLoadRuleFiles_EmptyDirectory(t *testing.T) {
	t.Parallel()

	// A directory that exists but has no .md files must return nil, not an error.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a rule"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "script.sh"), []byte("#!/bin/sh"), 0644))

	rules, err := loadRuleFiles(dir)
	require.NoError(t, err)
	assert.Empty(t, rules)
}

func TestLoadRuleFiles_NonExistentDirectory(t *testing.T) {
	t.Parallel()

	rules, err := loadRuleFiles("/this/path/does/not/exist")
	require.NoError(t, err)
	assert.Nil(t, rules)
}

func TestLoadRuleFiles_CaseInsensitiveMDExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rule1.MD"), []byte("Upper MD"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rule2.md"), []byte("Lower md"), 0644))

	rules, err := loadRuleFiles(dir)
	require.NoError(t, err)
	// Both files should be picked up regardless of extension case.
	require.Len(t, rules, 2)
}

// ---------------------------------------------------------------------------
// ContextLoader.Load — additional branches
// ---------------------------------------------------------------------------

func TestContextLoader_Load_BothPathsSet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	briefPath := filepath.Join(dir, "brief.md")
	rulesDir := filepath.Join(dir, "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0755))
	require.NoError(t, os.WriteFile(briefPath, []byte("Brief content"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "a-rule.md"), []byte("Rule A"), 0644))

	cl := NewContextLoader(briefPath, rulesDir)
	ctx, err := cl.Load()
	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.Equal(t, "Brief content", ctx.Brief)
	require.Len(t, ctx.Rules, 1)
	assert.Equal(t, "Rule A", ctx.Rules[0])
}

func TestContextLoader_Load_UnicodeBriefContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	unicodeContent := "# 프로젝트 설명\n日本語テスト\nCafé résumé naïve"
	briefPath := filepath.Join(dir, "brief.md")
	require.NoError(t, os.WriteFile(briefPath, []byte(unicodeContent), 0644))

	cl := NewContextLoader(briefPath, "")
	ctx, err := cl.Load()
	require.NoError(t, err)
	assert.Equal(t, unicodeContent, ctx.Brief)
}

func TestContextLoader_Load_PathTraversalInBriefReturnsError(t *testing.T) {
	t.Parallel()

	cl := NewContextLoader("../../etc/passwd", "")
	_, err := cl.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project brief path")
	assert.Contains(t, err.Error(), "path traversal rejected")
}

func TestContextLoader_Load_PathTraversalInRulesDirReturnsError(t *testing.T) {
	t.Parallel()

	cl := NewContextLoader("", "../../etc/shadow")
	_, err := cl.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rules dir path")
	assert.Contains(t, err.Error(), "path traversal rejected")
}

// ---------------------------------------------------------------------------
// formatFileList — additional change type and risk coverage
// ---------------------------------------------------------------------------

func TestFormatFileList_MultipleHighRiskFiles(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "internal/auth/login.go", ChangeType: ChangeModified, Risk: RiskHigh, LinesAdded: 10},
		{Path: "internal/auth/token.go", ChangeType: ChangeAdded, Risk: RiskHigh, LinesAdded: 50},
		{Path: "README.md", ChangeType: ChangeModified, Risk: RiskNormal, LinesAdded: 1},
	}

	list, highRisk := formatFileList(files)

	assert.Contains(t, list, "[HIGH RISK] internal/auth/login.go")
	assert.Contains(t, list, "[HIGH RISK] internal/auth/token.go")
	require.Len(t, highRisk, 2)
	assert.Contains(t, highRisk, "internal/auth/login.go")
	assert.Contains(t, highRisk, "internal/auth/token.go")
}

func TestFormatFileList_NoLineChanges(t *testing.T) {
	t.Parallel()

	// A file with ChangeType but zero line counts should show only the change type.
	files := []ChangedFile{
		{Path: "binary.dat", ChangeType: ChangeModified, Risk: RiskNormal},
	}

	list, highRisk := formatFileList(files)

	assert.Contains(t, list, "binary.dat")
	assert.Contains(t, list, "modified")
	// No plus/minus signs because there are no line counts.
	assert.NotContains(t, list, "+")
	assert.NotContains(t, list, "-")
	assert.Empty(t, highRisk)
}

func TestFormatFileList_AddedFileOnlyLines(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "new_feature.go", ChangeType: ChangeAdded, Risk: RiskNormal, LinesAdded: 200},
	}

	list, _ := formatFileList(files)
	assert.Contains(t, list, "added")
	assert.Contains(t, list, "+200")
	assert.NotContains(t, list, "-")
}

// ---------------------------------------------------------------------------
// PromptBuilder.Build — special characters in diff
// ---------------------------------------------------------------------------

func TestPromptBuilder_Build_DiffWithGoTemplateChars(t *testing.T) {
	t.Parallel()

	// Diff containing {{ and }} must not cause template parse/execute errors
	// because the builder uses [[ ]] delimiters, not {{ }}.
	diffWithBraces := `diff --git a/main.go b/main.go
+func example() {
+    m := map[string]string{"key": "value"}
+    fmt.Printf("{{.Field}}")
+}`

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:  "claude",
		Diff:       diffWithBraces,
		JSONSchema: jsonSchemaExample,
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err, "curly braces in diff must not break the template")
	assert.Contains(t, result, `{{.Field}}`)
}

// ---------------------------------------------------------------------------
// PromptBuilder.BuildForAgent — additional branches
// ---------------------------------------------------------------------------

func TestPromptBuilder_BuildForAgent_EmptyFilesList(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	diff := &DiffResult{
		Files:      []ChangedFile{},
		FullDiff:   "",
		BaseBranch: "main",
		Stats:      DiffStats{},
	}

	result, err := pb.BuildForAgent(context.Background(), "claude", diff, []ChangedFile{}, ReviewModeAll)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	// Template should handle empty file list gracefully.
	assert.Contains(t, result, "claude")
}

func TestPromptBuilder_BuildForAgent_DiffExactlyAtLimit_NotTruncated(t *testing.T) {
	t.Parallel()

	// A diff of exactly maxDiffBytes should not be truncated.
	exactDiff := strings.Repeat("x", maxDiffBytes)
	pb := NewPromptBuilder(ReviewConfig{}, nil)
	diff := &DiffResult{
		Files:    []ChangedFile{{Path: "a.go", ChangeType: ChangeModified, Risk: RiskNormal}},
		FullDiff: exactDiff,
	}

	result, err := pb.BuildForAgent(context.Background(), "claude", diff, diff.Files, ReviewModeAll)
	require.NoError(t, err)
	assert.NotContains(t, result, "[diff truncated at 100KB]")
}

func TestPromptBuilder_BuildForAgent_AllModeAllFilesListed(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	files := []ChangedFile{
		{Path: "pkg/a.go", ChangeType: ChangeAdded, Risk: RiskNormal, LinesAdded: 10},
		{Path: "pkg/b.go", ChangeType: ChangeModified, Risk: RiskNormal, LinesAdded: 5, LinesDeleted: 2},
		{Path: "pkg/c.go", ChangeType: ChangeDeleted, Risk: RiskNormal, LinesDeleted: 20},
	}
	diff := &DiffResult{
		Files:    files,
		FullDiff: "diff content",
	}

	result, err := pb.BuildForAgent(context.Background(), "claude", diff, files, ReviewModeAll)
	require.NoError(t, err)
	// In "all" mode all three files must appear in the rendered prompt.
	assert.Contains(t, result, "pkg/a.go")
	assert.Contains(t, result, "pkg/b.go")
	assert.Contains(t, result, "pkg/c.go")
}

func TestPromptBuilder_BuildForAgent_SplitModeOnlyAssignedFilesListed(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	allFiles := []ChangedFile{
		{Path: "pkg/a.go", ChangeType: ChangeAdded, Risk: RiskNormal, LinesAdded: 10},
		{Path: "pkg/b.go", ChangeType: ChangeModified, Risk: RiskNormal, LinesAdded: 5},
	}
	// This agent is only assigned pkg/a.go.
	agentFiles := []ChangedFile{allFiles[0]}

	diff := &DiffResult{
		Files:    allFiles,
		FullDiff: "full diff content",
	}

	result, err := pb.BuildForAgent(context.Background(), "claude", diff, agentFiles, ReviewModeSplit)
	require.NoError(t, err)
	assert.Contains(t, result, "pkg/a.go")
	assert.NotContains(t, result, "pkg/b.go")
}

func TestPromptBuilder_BuildForAgent_ProjectContextLoadError(t *testing.T) {
	t.Parallel()

	// Using a traversal path in briefPath triggers a load error.
	pb := NewPromptBuilder(ReviewConfig{
		ProjectBriefFile: "../../etc/passwd",
	}, nil)
	diff := &DiffResult{
		Files:    []ChangedFile{{Path: "main.go", ChangeType: ChangeModified}},
		FullDiff: "diff",
	}

	_, err := pb.BuildForAgent(context.Background(), "claude", diff, diff.Files, ReviewModeAll)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading project context")
}

// ---------------------------------------------------------------------------
// loadTemplateText — additional branches
// ---------------------------------------------------------------------------

func TestLoadTemplateText_ReadErrorOnExistingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "review.tmpl")
	require.NoError(t, os.WriteFile(tmplPath, []byte("content"), 0644))
	// Remove read permission so ReadFile returns a non-not-exist error.
	require.NoError(t, os.Chmod(tmplPath, 0000))
	t.Cleanup(func() { _ = os.Chmod(tmplPath, 0644) })

	// Skip on platforms where permission enforcement may not apply (e.g. root).
	if os.Getuid() == 0 {
		t.Skip("running as root; permission test not meaningful")
	}

	pb := NewPromptBuilder(ReviewConfig{PromptsDir: dir}, nil)
	_, _, err := pb.loadTemplateText()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading template")
}

func TestLoadTemplateText_LogsWhenCustomTemplateLoaded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review.tmpl"), []byte("[[ .AgentName ]]"), 0644))

	logger := log.New(io.Discard)
	pb := NewPromptBuilder(ReviewConfig{PromptsDir: dir}, logger)

	text, path, err := pb.loadTemplateText()
	require.NoError(t, err)
	assert.Equal(t, "[[ .AgentName ]]", text)
	assert.Contains(t, path, "review.tmpl")
}

// ---------------------------------------------------------------------------
// PromptBuilder.Build — full embedded template section coverage
// ---------------------------------------------------------------------------

func TestPromptBuilder_Build_EmbeddedTemplate_RulesSectionPresent(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:  "claude",
		Rules:      []string{"Security rule 1", "Performance rule 2"},
		JSONSchema: jsonSchemaExample,
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, result, "Review Rules and Conventions")
	assert.Contains(t, result, "Security rule 1")
	assert.Contains(t, result, "Performance rule 2")
}

func TestPromptBuilder_Build_EmbeddedTemplate_HighRiskSection(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:     "claude",
		HighRiskFiles: []string{"internal/auth/handler.go"},
		JSONSchema:    jsonSchemaExample,
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, result, "High-Risk Files")
	assert.Contains(t, result, "internal/auth/handler.go")
}

func TestPromptBuilder_Build_EmbeddedTemplate_NoBriefOmitsBriefSection(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:  "claude",
		JSONSchema: jsonSchemaExample,
		// No ProjectBrief set.
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	// The "Project Context" section header should be absent when brief is empty.
	assert.NotContains(t, result, "## Project Context")
}

func TestPromptBuilder_Build_EmbeddedTemplate_NoRulesOmitsRulesSection(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:  "claude",
		JSONSchema: jsonSchemaExample,
		// No Rules set.
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.NotContains(t, result, "## Review Rules and Conventions")
}

func TestPromptBuilder_Build_EmbeddedTemplate_EmptyDiffFallback(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:  "claude",
		JSONSchema: jsonSchemaExample,
		// No Diff set.
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	// The template should still be valid and note no diff content.
	assert.Contains(t, result, "No diff content available")
}

func TestPromptBuilder_Build_EmbeddedTemplate_EmptyFileListFallback(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:  "claude",
		JSONSchema: jsonSchemaExample,
		// No FileList set.
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, result, "No files to review")
}

// ---------------------------------------------------------------------------
// JSON schema content verification
// ---------------------------------------------------------------------------

func TestJSONSchemaExample_ContainsRequiredFields(t *testing.T) {
	t.Parallel()

	// The embedded schema must contain all fields required by the PRD.
	assert.Contains(t, jsonSchemaExample, `"findings"`)
	assert.Contains(t, jsonSchemaExample, `"severity"`)
	assert.Contains(t, jsonSchemaExample, `"category"`)
	assert.Contains(t, jsonSchemaExample, `"file"`)
	assert.Contains(t, jsonSchemaExample, `"line"`)
	assert.Contains(t, jsonSchemaExample, `"description"`)
	assert.Contains(t, jsonSchemaExample, `"suggestion"`)
	assert.Contains(t, jsonSchemaExample, `"verdict"`)
	assert.Contains(t, jsonSchemaExample, "APPROVED|CHANGES_NEEDED|BLOCKING")
}

// ---------------------------------------------------------------------------
// Default embedded template — complete prompt validation
// ---------------------------------------------------------------------------

func TestDefaultEmbeddedTemplate_ProducesCompletePrompt(t *testing.T) {
	t.Parallel()

	// Verify that a prompt built with no custom configuration is still
	// complete: it must contain all major sections expected by a reviewing agent.
	pb := NewPromptBuilder(ReviewConfig{}, nil)
	data := PromptData{
		AgentName:     "claude",
		ReviewMode:    ReviewModeAll,
		ProjectBrief:  "A CLI tool written in Go.",
		Rules:         []string{"No global state.", "Always wrap errors with context."},
		Diff:          "diff --git a/main.go b/main.go\n+func main() {}",
		FileList:      "            main.go (added, +1)",
		HighRiskFiles: []string{},
		Stats: DiffStats{
			TotalFiles:      1,
			FilesAdded:      1,
			TotalLinesAdded: 1,
		},
		JSONSchema: jsonSchemaExample,
	}

	result, err := pb.Build(context.Background(), data)
	require.NoError(t, err)

	// All major sections must be present.
	assert.Contains(t, result, "claude")
	assert.Contains(t, result, "Project Context")
	assert.Contains(t, result, "A CLI tool written in Go.")
	assert.Contains(t, result, "Review Rules and Conventions")
	assert.Contains(t, result, "No global state.")
	assert.Contains(t, result, "Diff Statistics")
	assert.Contains(t, result, "Total files changed: 1")
	assert.Contains(t, result, "Files to Review")
	assert.Contains(t, result, "main.go")
	assert.Contains(t, result, "Code Diff")
	assert.Contains(t, result, "func main()")
	assert.Contains(t, result, "Output Format")
	assert.Contains(t, result, "APPROVED")
	assert.Contains(t, result, "CHANGES_NEEDED")
	assert.Contains(t, result, "BLOCKING")
	assert.Contains(t, result, jsonSchemaExample)
}

// ---------------------------------------------------------------------------
// computeStats helper (indirectly via BuildForAgent)
// ---------------------------------------------------------------------------

func TestPromptBuilder_BuildForAgent_AllModeUsesFullStats(t *testing.T) {
	t.Parallel()

	pb := NewPromptBuilder(ReviewConfig{}, nil)

	files := []ChangedFile{
		{Path: "a.go", ChangeType: ChangeAdded, Risk: RiskNormal, LinesAdded: 20},
		{Path: "b.go", ChangeType: ChangeModified, Risk: RiskHigh, LinesAdded: 5, LinesDeleted: 3},
		{Path: "c.go", ChangeType: ChangeDeleted, Risk: RiskNormal, LinesDeleted: 15},
		{Path: "d.go", ChangeType: ChangeRenamed, Risk: RiskNormal, OldPath: "old_d.go", LinesAdded: 2},
	}
	diff := &DiffResult{
		Files:    files,
		FullDiff: "diff text",
		Stats: DiffStats{
			TotalFiles:        4,
			FilesAdded:        1,
			FilesModified:     1,
			FilesDeleted:      1,
			FilesRenamed:      1,
			TotalLinesAdded:   27,
			TotalLinesDeleted: 18,
			HighRiskFiles:     1,
		},
	}

	result, err := pb.BuildForAgent(context.Background(), "claude", diff, files, ReviewModeAll)
	require.NoError(t, err)
	// In "all" mode the diff's own Stats are passed through unchanged.
	assert.Contains(t, result, "Total files changed: 4")
}
