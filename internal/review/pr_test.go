package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- extractPRURL -----------------------------------------------------------

func TestExtractPRURL(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "URL on last line",
			output: "https://github.com/owner/repo/pull/42\n",
			want:   "https://github.com/owner/repo/pull/42",
		},
		{
			name:   "URL with preceding status lines",
			output: "Creating pull request for feature-branch into main in owner/repo\n\nhttps://github.com/owner/repo/pull/99\n",
			want:   "https://github.com/owner/repo/pull/99",
		},
		{
			name:   "empty output",
			output: "",
			want:   "",
		},
		{
			name:   "only whitespace",
			output: "   \n  \n",
			want:   "",
		},
		{
			name:   "single line no newline",
			output: "https://github.com/owner/repo/pull/7",
			want:   "https://github.com/owner/repo/pull/7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPRURL(tt.output)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- extractPRNumber --------------------------------------------------------

func TestExtractPRNumber(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want int
	}{
		{
			name: "standard GitHub URL",
			url:  "https://github.com/owner/repo/pull/42",
			want: 42,
		},
		{
			name: "large PR number",
			url:  "https://github.com/owner/repo/pull/1234",
			want: 1234,
		},
		{
			name: "empty URL",
			url:  "",
			want: 0,
		},
		{
			name: "non-PR URL",
			url:  "https://github.com/owner/repo/issues/10",
			want: 0,
		},
		{
			name: "URL without number",
			url:  "https://github.com/owner/repo/pull/",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPRNumber(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- buildCommandString -----------------------------------------------------

func TestBuildCommandString(t *testing.T) {
	tests := []struct {
		name string
		bin  string
		args []string
		want string
	}{
		{
			name: "simple command",
			bin:  "gh",
			args: []string{"pr", "create", "--draft"},
			want: "gh pr create --draft",
		},
		{
			name: "argument with spaces is quoted",
			bin:  "gh",
			args: []string{"pr", "create", "--title", "My PR Title"},
			want: "gh pr create --title 'My PR Title'",
		},
		{
			name: "no arguments",
			bin:  "gh",
			args: []string{},
			want: "gh",
		},
		{
			name: "multiple labels",
			bin:  "gh",
			args: []string{"pr", "create", "--label", "bug", "--label", "enhancement"},
			want: "gh pr create --label bug --label enhancement",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCommandString(tt.bin, tt.args)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- validBranchNameRe ------------------------------------------------------

func TestValidBranchNameRe(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{name: "simple name", input: "main", valid: true},
		{name: "name with slash", input: "feature/my-branch", valid: true},
		{name: "name with dots", input: "release.1.0", valid: true},
		{name: "name with underscore", input: "my_branch", valid: true},
		{name: "name with hyphen", input: "fix-123", valid: true},
		{name: "alphanumeric", input: "abc123", valid: true},
		{name: "semicolon injection", input: "main; rm -rf /", valid: false},
		{name: "backtick injection", input: "main`id`", valid: false},
		{name: "dollar injection", input: "main$(id)", valid: false},
		{name: "ampersand injection", input: "main && rm -rf /", valid: false},
		{name: "empty string", input: "", valid: false},
		{name: "spaces", input: "main branch", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validBranchNameRe.MatchString(tt.input)
			assert.Equal(t, tt.valid, got)
		})
	}
}

// --- NewPRCreator -----------------------------------------------------------

func TestNewPRCreator(t *testing.T) {
	pc := NewPRCreator("/some/workdir", nil)
	require.NotNil(t, pc)
	assert.Equal(t, "/some/workdir", pc.workDir)
	assert.Nil(t, pc.logger)
}

// --- Create dry-run ---------------------------------------------------------

func TestCreate_DryRun(t *testing.T) {
	tests := []struct {
		name        string
		opts        PRCreateOpts
		wantCreated bool
		wantInCmd   []string
	}{
		{
			name: "basic dry run",
			opts: PRCreateOpts{
				Title:      "My Feature",
				Body:       "PR body here",
				BaseBranch: "main",
				DryRun:     true,
			},
			wantCreated: false,
			wantInCmd:   []string{"gh", "pr", "create", "--title", "--base", "main"},
		},
		{
			name: "dry run with draft flag",
			opts: PRCreateOpts{
				Title:      "Draft Feature",
				Body:       "body",
				BaseBranch: "main",
				Draft:      true,
				DryRun:     true,
			},
			wantCreated: false,
			wantInCmd:   []string{"--draft"},
		},
		{
			name: "dry run with labels and assignees",
			opts: PRCreateOpts{
				Title:      "Labelled PR",
				Body:       "body",
				BaseBranch: "develop",
				Labels:     []string{"bug", "enhancement"},
				Assignees:  []string{"alice", "bob"},
				DryRun:     true,
			},
			wantCreated: false,
			wantInCmd:   []string{"--label", "bug", "--label", "enhancement", "--assignee", "alice", "--assignee", "bob"},
		},
		{
			name: "dry run defaults base branch to main",
			opts: PRCreateOpts{
				Title:  "Feature",
				Body:   "body",
				DryRun: true,
			},
			wantCreated: false,
			wantInCmd:   []string{"--base", "main"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewPRCreator("", nil)
			result, err := pc.Create(context.Background(), tt.opts)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantCreated, result.Created)
			assert.Equal(t, tt.opts.Draft, result.Draft)

			for _, substr := range tt.wantInCmd {
				assert.Contains(t, result.Command, substr, "expected %q in command", substr)
			}
		})
	}
}

func TestCreate_DryRun_InvalidBaseBranch(t *testing.T) {
	pc := NewPRCreator("", nil)
	_, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "Test",
		Body:       "body",
		BaseBranch: "main; rm -rf /",
		DryRun:     true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid base branch name")
}

// --- dryRun helper -----------------------------------------------------------

func TestDryRun_CommandContainsBodyFile(t *testing.T) {
	pc := NewPRCreator("", nil)
	result := pc.dryRun(PRCreateOpts{
		Title:      "Test PR",
		Body:       "some body",
		BaseBranch: "main",
	})

	require.NotNil(t, result)
	assert.False(t, result.Created)
	assert.Contains(t, result.Command, "<body-tempfile>")
	assert.Contains(t, result.Command, "--body-file")
}

// --- Additional pure-logic tests --------------------------------------------

func TestExtractPRURL_AdditionalCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "multiple blank lines at end",
			output: "https://github.com/owner/repo/pull/10\n\n\n",
			want:   "https://github.com/owner/repo/pull/10",
		},
		{
			name:   "trailing spaces on URL line",
			output: "  https://github.com/owner/repo/pull/5  \n",
			want:   "https://github.com/owner/repo/pull/5",
		},
		{
			name:   "multiple non-empty lines returns last",
			output: "line one\nline two\nhttps://github.com/owner/repo/pull/77",
			want:   "https://github.com/owner/repo/pull/77",
		},
		{
			name:   "CRLF line endings",
			output: "Creating PR\r\nhttps://github.com/owner/repo/pull/3\r\n",
			want:   "https://github.com/owner/repo/pull/3",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractPRURL(tt.output)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractPRNumber_AdditionalCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want int
	}{
		{
			name: "PR number 1",
			url:  "https://github.com/owner/repo/pull/1",
			want: 1,
		},
		{
			name: "URL with query params after number",
			url:  "https://github.com/owner/repo/pull/99?notification_referrer_id=xyz",
			want: 99,
		},
		{
			name: "URL is just /pull/ with no digits",
			url:  "https://github.com/owner/repo/pull/abc",
			want: 0,
		},
		{
			name: "GitHub Enterprise URL",
			url:  "https://github.example.com/org/repo/pull/55",
			want: 55,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractPRNumber(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildCommandString_AdditionalCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		bin  string
		args []string
		want string
	}{
		{
			name: "argument with tab is quoted",
			bin:  "gh",
			args: []string{"pr", "create", "--title", "My\tTitle"},
			want: "gh pr create --title 'My\tTitle'",
		},
		{
			name: "argument with newline is quoted",
			bin:  "gh",
			args: []string{"pr", "create", "--body", "line1\nline2"},
			want: "gh pr create --body 'line1\nline2'",
		},
		{
			name: "mixed quoted and unquoted args",
			bin:  "gh",
			args: []string{"pr", "create", "--title", "My Feature Branch", "--draft"},
			want: "gh pr create --title 'My Feature Branch' --draft",
		},
		{
			name: "nil args slice",
			bin:  "gh",
			args: nil,
			want: "gh",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildCommandString(tt.bin, tt.args)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreate_DryRun_TitleWithSpaces(t *testing.T) {
	t.Parallel()

	pc := NewPRCreator("", nil)
	result, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "My Feature Title With Spaces",
		Body:       "some body",
		BaseBranch: "main",
		DryRun:     true,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Created)
	// Title has spaces, so it must be single-quoted in the command string.
	assert.Contains(t, result.Command, "'My Feature Title With Spaces'")
}

func TestCreate_DryRun_NoURL(t *testing.T) {
	t.Parallel()

	pc := NewPRCreator("", nil)
	result, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "Draft PR",
		Body:       "body",
		BaseBranch: "develop",
		DryRun:     true,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	// Dry run never creates a PR so URL and Number must be zero values.
	assert.Empty(t, result.URL)
	assert.Equal(t, 0, result.Number)
}

// --- fake binary helpers ----------------------------------------------------

// writeFakeScript creates an executable shell script at dir/name with the
// given content. The script must start with #!/bin/sh.
func writeFakeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	err := os.WriteFile(p, []byte(content), 0755)
	require.NoError(t, err)
	return p
}

// withFakePath prepends dir to PATH for the duration of the test and restores
// the original value via t.Cleanup.
func withFakePath(t *testing.T, dir string) {
	t.Helper()
	old := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", old) })
	os.Setenv("PATH", dir+":"+old)
}

// fakeGHScript builds a gh script body that handles common gh subcommands.
//
// versionExitCode controls the exit code for `gh --version`.
// authExitCode    controls the exit code for `gh auth status`.
// prCreateOutput  is printed to stdout when `gh pr create` is invoked (exit 0).
// prCreateExitCode overrides the exit code for `gh pr create`.
// prCreateStderr  is written to stderr for `gh pr create` when prCreateExitCode != 0.
type fakeGHConfig struct {
	versionExitCode  int
	authExitCode     int
	authStderr       string
	prCreateOutput   string
	prCreateExitCode int
	prCreateStderr   string
}

func buildFakeGHScript(cfg fakeGHConfig) string {
	authStderr := cfg.authStderr
	if authStderr == "" {
		authStderr = "not logged in to any GitHub host. Run gh auth login to authenticate."
	}
	prOutput := cfg.prCreateOutput
	if prOutput == "" {
		prOutput = "https://github.com/owner/repo/pull/42"
	}

	return fmt.Sprintf(`#!/bin/sh
case "$1" in
  --version)
    echo "gh version 2.40.0"
    exit %d
    ;;
  auth)
    if [ "$2" = "status" ]; then
      echo "%s" >&2
      exit %d
    fi
    ;;
  pr)
    if [ "$2" = "create" ]; then
      if [ %d -ne 0 ]; then
        echo "%s" >&2
        exit %d
      fi
      echo "%s"
      exit 0
    fi
    ;;
esac
exit 0
`,
		cfg.versionExitCode,
		authStderr,
		cfg.authExitCode,
		cfg.prCreateExitCode,
		cfg.prCreateStderr,
		cfg.prCreateExitCode,
		prOutput,
	)
}

// buildFakeGitScript creates a git script that responds to the subcommands
// used by PRCreator.
//
// branch          is returned by `git rev-parse --abbrev-ref HEAD`.
// remoteExitCode  is the exit code for `git rev-parse --verify origin/<branch>`.
// pushExitCode    is the exit code for `git push ...`.
// pushStderr      is written to stderr when push fails.
type fakeGitConfig struct {
	branch         string
	remoteExitCode int
	pushExitCode   int
	pushStderr     string
}

func buildFakeGitScript(cfg fakeGitConfig) string {
	branch := cfg.branch
	if branch == "" {
		branch = "feature/my-pr"
	}
	pushStderr := cfg.pushStderr
	if pushStderr == "" {
		pushStderr = "failed to push some refs"
	}

	return fmt.Sprintf(`#!/bin/sh
case "$1" in
  rev-parse)
    case "$2" in
      --abbrev-ref)
        echo "%s"
        exit 0
        ;;
      --verify)
        exit %d
        ;;
    esac
    ;;
  push)
    if [ %d -ne 0 ]; then
      echo "%s" >&2
      exit %d
    fi
    exit 0
    ;;
esac
exit 0
`,
		branch,
		cfg.remoteExitCode,
		cfg.pushExitCode,
		pushStderr,
		cfg.pushExitCode,
	)
}

// --- CheckPrerequisites tests (exec-based) ----------------------------------

func TestCheckPrerequisites_Success(t *testing.T) {
	dir := t.TempDir()

	writeFakeScript(t, dir, "gh", buildFakeGHScript(fakeGHConfig{
		versionExitCode: 0,
		authExitCode:    0,
	}))
	writeFakeScript(t, dir, "git", buildFakeGitScript(fakeGitConfig{
		branch: "feature/cool-thing",
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	err := pc.CheckPrerequisites(context.Background(), "main")
	require.NoError(t, err)
}

func TestCheckPrerequisites_GHNotInstalled(t *testing.T) {
	// Use a directory with no gh binary -- only git is present.
	// We replace PATH entirely so the real gh (if any) cannot be found.
	dir := t.TempDir()
	writeFakeScript(t, dir, "git", buildFakeGitScript(fakeGitConfig{branch: "feature/x"}))

	old := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", old) })
	os.Setenv("PATH", dir) // only our dir, no gh

	pc := NewPRCreator("", nil)
	err := pc.CheckPrerequisites(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestCheckPrerequisites_NotAuthenticated(t *testing.T) {
	dir := t.TempDir()

	writeFakeScript(t, dir, "gh", buildFakeGHScript(fakeGHConfig{
		versionExitCode: 0,
		authExitCode:    1,
		authStderr:      "not logged in to any GitHub host",
	}))
	writeFakeScript(t, dir, "git", buildFakeGitScript(fakeGitConfig{
		branch: "feature/auth-test",
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	err := pc.CheckPrerequisites(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authenticated")
}

func TestCheckPrerequisites_OnBaseBranch(t *testing.T) {
	dir := t.TempDir()

	writeFakeScript(t, dir, "gh", buildFakeGHScript(fakeGHConfig{
		versionExitCode: 0,
		authExitCode:    0,
	}))
	// Git reports we're on "main" which is also the base branch.
	writeFakeScript(t, dir, "git", buildFakeGitScript(fakeGitConfig{
		branch: "main",
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	err := pc.CheckPrerequisites(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same as the base branch")
	assert.Contains(t, err.Error(), "main")
}

func TestCheckPrerequisites_OnBaseBranch_CustomBase(t *testing.T) {
	dir := t.TempDir()

	writeFakeScript(t, dir, "gh", buildFakeGHScript(fakeGHConfig{
		versionExitCode: 0,
		authExitCode:    0,
	}))
	writeFakeScript(t, dir, "git", buildFakeGitScript(fakeGitConfig{
		branch: "develop",
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	// Current branch == base branch == "develop"
	err := pc.CheckPrerequisites(context.Background(), "develop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same as the base branch")
	assert.Contains(t, err.Error(), "develop")
}

func TestCheckPrerequisites_DetachedHead(t *testing.T) {
	dir := t.TempDir()

	writeFakeScript(t, dir, "gh", buildFakeGHScript(fakeGHConfig{
		versionExitCode: 0,
		authExitCode:    0,
	}))
	// Git returns "HEAD" to simulate detached HEAD state.
	writeFakeScript(t, dir, "git", buildFakeGitScript(fakeGitConfig{
		branch: "HEAD",
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	err := pc.CheckPrerequisites(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "detached HEAD")
}

func TestCheckPrerequisites_DefaultBaseBranch(t *testing.T) {
	// When baseBranch == "" it defaults to "main". This test verifies that a
	// feature branch passes even when no explicit baseBranch is provided.
	dir := t.TempDir()

	writeFakeScript(t, dir, "gh", buildFakeGHScript(fakeGHConfig{
		versionExitCode: 0,
		authExitCode:    0,
	}))
	writeFakeScript(t, dir, "git", buildFakeGitScript(fakeGitConfig{
		branch: "feature/my-feature",
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	err := pc.CheckPrerequisites(context.Background(), "")
	require.NoError(t, err)
}

// --- EnsureBranchPushed tests (exec-based) ----------------------------------

func TestEnsureBranchPushed_AlreadyPushed(t *testing.T) {
	dir := t.TempDir()

	// git rev-parse --verify returns 0 => remote branch exists, no push needed.
	writeFakeScript(t, dir, "git", buildFakeGitScript(fakeGitConfig{
		branch:         "feature/already-pushed",
		remoteExitCode: 0,
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	err := pc.EnsureBranchPushed(context.Background())
	require.NoError(t, err)
}

func TestEnsureBranchPushed_NotPushed_PushSucceeds(t *testing.T) {
	dir := t.TempDir()

	// git rev-parse --verify returns 1 => remote branch absent. push exits 0.
	writeFakeScript(t, dir, "git", buildFakeGitScript(fakeGitConfig{
		branch:         "feature/new-branch",
		remoteExitCode: 1,
		pushExitCode:   0,
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	err := pc.EnsureBranchPushed(context.Background())
	require.NoError(t, err)
}

func TestEnsureBranchPushed_NotPushed_PushFails(t *testing.T) {
	dir := t.TempDir()

	writeFakeScript(t, dir, "git", buildFakeGitScript(fakeGitConfig{
		branch:         "feature/push-fail",
		remoteExitCode: 1,
		pushExitCode:   1,
		pushStderr:     "error: failed to push some refs to origin",
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	err := pc.EnsureBranchPushed(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git push")
}

func TestEnsureBranchPushed_GitNotInstalled(t *testing.T) {
	// Replace PATH with an empty directory so git cannot be found.
	dir := t.TempDir()
	old := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", old) })
	os.Setenv("PATH", dir)

	pc := NewPRCreator("", nil)
	err := pc.EnsureBranchPushed(context.Background())
	// EnsureBranchPushed calls git rev-parse first; with no git in PATH it
	// should fail with exit code -1 which surfaces as an error.
	require.Error(t, err)
}

// --- Create (live exec) tests -----------------------------------------------

func TestCreate_Success(t *testing.T) {
	dir := t.TempDir()

	writeFakeScript(t, dir, "gh", buildFakeGHScript(fakeGHConfig{
		versionExitCode:  0,
		authExitCode:     0,
		prCreateOutput:   "https://github.com/owner/repo/pull/42",
		prCreateExitCode: 0,
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	result, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "My Feature PR",
		Body:       "## Summary\n\nThis is the body.",
		BaseBranch: "main",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Created)
	assert.Equal(t, "https://github.com/owner/repo/pull/42", result.URL)
	assert.Equal(t, 42, result.Number)
	assert.False(t, result.Draft)
	assert.NotEmpty(t, result.Command)
}

func TestCreate_WithDraft(t *testing.T) {
	dir := t.TempDir()

	// Record the args that gh receives so we can inspect them.
	ghScript := `#!/bin/sh
case "$1" in
  pr)
    # Scan for --draft flag in the full argument list.
    draft_found=0
    for arg in "$@"; do
      if [ "$arg" = "--draft" ]; then
        draft_found=1
      fi
    done
    if [ "$draft_found" = "1" ]; then
      echo "DRAFT_FLAG_PRESENT" > ` + filepath.Join(dir, "gh_args.txt") + `
    fi
    echo "https://github.com/owner/repo/pull/7"
    exit 0
    ;;
esac
exit 0
`
	writeFakeScript(t, dir, "gh", ghScript)
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	result, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "Draft PR",
		Body:       "Draft body",
		BaseBranch: "main",
		Draft:      true,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Created)
	assert.True(t, result.Draft)

	// Verify --draft was actually passed to the fake gh binary.
	argsData, err := os.ReadFile(filepath.Join(dir, "gh_args.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(argsData), "DRAFT_FLAG_PRESENT")
}

func TestCreate_WithLabels(t *testing.T) {
	dir := t.TempDir()

	labelLogFile := filepath.Join(dir, "labels.txt")
	ghScript := fmt.Sprintf(`#!/bin/sh
case "$1" in
  pr)
    # Record all --label values.
    prev=""
    for arg in "$@"; do
      if [ "$prev" = "--label" ]; then
        echo "$arg" >> %s
      fi
      prev="$arg"
    done
    echo "https://github.com/owner/repo/pull/8"
    exit 0
    ;;
esac
exit 0
`, labelLogFile)
	writeFakeScript(t, dir, "gh", ghScript)
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	result, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "Labelled PR",
		Body:       "body",
		BaseBranch: "main",
		Labels:     []string{"bug", "enhancement"},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Created)

	// Confirm both labels were passed to gh.
	labelsData, err := os.ReadFile(labelLogFile)
	require.NoError(t, err)
	labelsStr := string(labelsData)
	assert.Contains(t, labelsStr, "bug")
	assert.Contains(t, labelsStr, "enhancement")
}

func TestCreate_WithAssignees(t *testing.T) {
	dir := t.TempDir()

	assigneeLogFile := filepath.Join(dir, "assignees.txt")
	ghScript := fmt.Sprintf(`#!/bin/sh
case "$1" in
  pr)
    prev=""
    for arg in "$@"; do
      if [ "$prev" = "--assignee" ]; then
        echo "$arg" >> %s
      fi
      prev="$arg"
    done
    echo "https://github.com/owner/repo/pull/9"
    exit 0
    ;;
esac
exit 0
`, assigneeLogFile)
	writeFakeScript(t, dir, "gh", ghScript)
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	result, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "Assigned PR",
		Body:       "body",
		BaseBranch: "main",
		Assignees:  []string{"alice", "bob"},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Created)

	assigneesData, err := os.ReadFile(assigneeLogFile)
	require.NoError(t, err)
	assigneesStr := string(assigneesData)
	assert.Contains(t, assigneesStr, "alice")
	assert.Contains(t, assigneesStr, "bob")
}

func TestCreate_AlreadyExists(t *testing.T) {
	dir := t.TempDir()

	// gh exits 1 with "already exists" in stderr.
	ghScript := `#!/bin/sh
case "$1" in
  pr)
    echo "a pull request already exists for feature/my-branch" >&2
    exit 1
    ;;
esac
exit 0
`
	writeFakeScript(t, dir, "gh", ghScript)
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	_, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "Duplicate PR",
		Body:       "body",
		BaseBranch: "main",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCreate_GHError(t *testing.T) {
	dir := t.TempDir()

	// gh exits 1 with a generic error.
	ghScript := `#!/bin/sh
case "$1" in
  pr)
    echo "GraphQL: FORBIDDEN" >&2
    exit 1
    ;;
esac
exit 0
`
	writeFakeScript(t, dir, "gh", ghScript)
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	_, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "Failing PR",
		Body:       "body",
		BaseBranch: "main",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gh pr create exited")
}

func TestCreate_BodyWrittenToTempFile(t *testing.T) {
	// Verify that the body written to the temp file matches the Body field.
	// We do this by reading the --body-file argument from our fake gh script.
	dir := t.TempDir()

	bodyFileLog := filepath.Join(dir, "body_file_path.txt")
	ghScript := fmt.Sprintf(`#!/bin/sh
case "$1" in
  pr)
    # Find --body-file argument value and log its path.
    prev=""
    for arg in "$@"; do
      if [ "$prev" = "--body-file" ]; then
        echo "$arg" > %s
        # Also dump the content so we can verify it.
        cat "$arg" >> %s
      fi
      prev="$arg"
    done
    echo "https://github.com/owner/repo/pull/100"
    exit 0
    ;;
esac
exit 0
`, bodyFileLog, bodyFileLog)
	writeFakeScript(t, dir, "gh", ghScript)
	withFakePath(t, dir)

	expectedBody := "## Changes\n\nThis PR fixes a critical bug.\n"
	pc := NewPRCreator("", nil)
	result, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "Body Verification PR",
		Body:       expectedBody,
		BaseBranch: "main",
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	logData, err := os.ReadFile(bodyFileLog)
	require.NoError(t, err)

	logStr := string(logData)
	// The log contains the body-file path on first line and body content after.
	assert.Contains(t, logStr, expectedBody)
}

func TestCreate_InvalidBaseBranch_Exec(t *testing.T) {
	t.Parallel()

	// Even in non-dry-run mode, invalid base branch is rejected before gh runs.
	pc := NewPRCreator("", nil)
	_, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "Test",
		Body:       "body",
		BaseBranch: "main`whoami`",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid base branch name")
}

func TestCreate_DefaultBaseBranch_Exec(t *testing.T) {
	dir := t.TempDir()

	baseLogFile := filepath.Join(dir, "base.txt")
	ghScript := fmt.Sprintf(`#!/bin/sh
case "$1" in
  pr)
    prev=""
    for arg in "$@"; do
      if [ "$prev" = "--base" ]; then
        echo "$arg" > %s
      fi
      prev="$arg"
    done
    echo "https://github.com/owner/repo/pull/11"
    exit 0
    ;;
esac
exit 0
`, baseLogFile)
	writeFakeScript(t, dir, "gh", ghScript)
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	result, err := pc.Create(context.Background(), PRCreateOpts{
		Title: "Default Base PR",
		Body:  "body",
		// BaseBranch intentionally omitted to test default.
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	baseData, err := os.ReadFile(baseLogFile)
	require.NoError(t, err)
	assert.Equal(t, "main", strings.TrimSpace(string(baseData)))
}

func TestCreate_CommandStringPopulated(t *testing.T) {
	dir := t.TempDir()

	writeFakeScript(t, dir, "gh", buildFakeGHScript(fakeGHConfig{
		prCreateOutput: "https://github.com/owner/repo/pull/55",
	}))
	withFakePath(t, dir)

	pc := NewPRCreator("", nil)
	result, err := pc.Create(context.Background(), PRCreateOpts{
		Title:      "Command Test",
		Body:       "body",
		BaseBranch: "main",
		Draft:      true,
		Labels:     []string{"feature"},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	// Command string must contain key substrings for observability.
	assert.Contains(t, result.Command, "gh")
	assert.Contains(t, result.Command, "pr")
	assert.Contains(t, result.Command, "create")
	assert.Contains(t, result.Command, "--draft")
	assert.Contains(t, result.Command, "--label")
	assert.Contains(t, result.Command, "feature")
}

// --- Context cancellation ---------------------------------------------------

func TestCreate_ContextCancelled(t *testing.T) {
	dir := t.TempDir()

	// A gh script that sleeps so context cancellation takes effect.
	ghScript := `#!/bin/sh
case "$1" in
  pr)
    sleep 30
    echo "https://github.com/owner/repo/pull/1"
    exit 0
    ;;
esac
exit 0
`
	writeFakeScript(t, dir, "gh", ghScript)
	withFakePath(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	pc := NewPRCreator("", nil)
	_, err := pc.Create(ctx, PRCreateOpts{
		Title:      "Cancelled PR",
		Body:       "body",
		BaseBranch: "main",
	})
	// Expect an error due to context cancellation.
	require.Error(t, err)
}

// --- Benchmark tests --------------------------------------------------------

// BenchmarkBuildCommandString measures the overhead of assembling a command
// string with a realistic set of gh pr create arguments.
func BenchmarkBuildCommandString(b *testing.B) {
	args := []string{
		"pr", "create",
		"--title", "My Big Feature Branch PR",
		"--body-file", "/tmp/raven-pr-body-123456789.md",
		"--base", "main",
		"--draft",
		"--label", "feature",
		"--label", "review-needed",
		"--assignee", "alice",
		"--assignee", "bob",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildCommandString("gh", args)
	}
}

// BenchmarkExtractPRURL measures URL extraction from typical gh output.
func BenchmarkExtractPRURL(b *testing.B) {
	output := "Creating pull request for feature/my-branch into main in owner/repo\n\nhttps://github.com/owner/repo/pull/42\n"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractPRURL(output)
	}
}

// BenchmarkExtractPRNumber measures PR number extraction from a URL.
func BenchmarkExtractPRNumber(b *testing.B) {
	url := "https://github.com/owner/repo/pull/1234"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractPRNumber(url)
	}
}
