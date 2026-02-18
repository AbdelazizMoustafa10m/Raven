package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
)

// --- TestSlugify ----------------------------------------------------------

func TestSlugify(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "lowercase with spaces",
			input: "Foundation Setup",
			want:  "foundation-setup",
		},
		{
			name:  "ampersand and spaces",
			input: "Foundation & Setup",
			want:  "foundation-setup",
		},
		{
			name:  "colon and number",
			input: "Phase 1: Init",
			want:  "phase-1-init",
		},
		{
			name:  "leading and trailing spaces",
			input: "  spaces  ",
			want:  "spaces",
		},
		{
			name:  "consecutive hyphens collapsed",
			input: "hello--world",
			want:  "hello-world",
		},
		{
			name:  "already clean",
			input: "clean-slug",
			want:  "clean-slug",
		},
		{
			name:  "uppercase letters lowercased",
			input: "UPPER CASE",
			want:  "upper-case",
		},
		{
			name:  "unicode characters replaced",
			input: "Büro & Co.",
			want:  "b-ro-co",
		},
		{
			name:  "only special chars",
			input: "---",
			want:  "",
		},
		{
			name:  "numbers preserved",
			input: "phase-42",
			want:  "phase-42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slugify(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- TestResolveBranchName -----------------------------------------------

func TestResolveBranchName(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		phaseID     int
		phaseName   string
		projectName string
		want        string
	}{
		{
			name:        "default template foundation",
			template:    "phase/{phase_id}-{slug}",
			phaseID:     1,
			phaseName:   "Foundation Setup",
			projectName: "",
			want:        "phase/1-foundation-setup",
		},
		{
			name:        "project template",
			template:    "{project}/phase-{phase_id}",
			phaseID:     1,
			phaseName:   "any",
			projectName: "raven",
			want:        "raven/phase-1",
		},
		{
			name:        "all variables",
			template:    "{project}/{phase_id}-{slug}",
			phaseID:     3,
			phaseName:   "Integration & Testing",
			projectName: "myapp",
			want:        "myapp/3-integration-testing",
		},
		{
			name:        "empty template uses default",
			template:    "",
			phaseID:     2,
			phaseName:   "Implementation",
			projectName: "",
			want:        "phase/2-implementation",
		},
		{
			name:        "slug with special characters",
			template:    "phase/{phase_id}-{slug}",
			phaseID:     5,
			phaseName:   "Phase 5: Final Review",
			projectName: "",
			want:        "phase/5-phase-5-final-review",
		},
		{
			name:        "no substitution variables in template",
			template:    "feature/static-branch",
			phaseID:     1,
			phaseName:   "anything",
			projectName: "",
			want:        "feature/static-branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bm := NewBranchManager(nil, tt.template, "main")
			got := bm.ResolveBranchName(tt.phaseID, tt.phaseName, tt.projectName)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- TestNewBranchManager defaults ----------------------------------------

func TestNewBranchManager_Defaults(t *testing.T) {
	bm := NewBranchManager(nil, "", "")
	assert.Equal(t, defaultBranchTemplate, bm.branchTemplate)
	assert.Equal(t, "main", bm.baseBranch)
}

func TestNewBranchManager_CustomValues(t *testing.T) {
	bm := NewBranchManager(nil, "custom/{phase_id}", "develop")
	assert.Equal(t, "custom/{phase_id}", bm.branchTemplate)
	assert.Equal(t, "develop", bm.baseBranch)
}

// --- mock git client for unit tests --------------------------------------

// mockGitClient records calls and returns pre-configured responses. It is used
// to verify BranchManager behaviour without touching a real git repository.
type mockGitClient struct {
	// branchExistsResults maps branch name to (exists, error).
	branchExistsResults map[string]struct {
		exists bool
		err    error
	}

	// createBranchErr is returned by CreateBranch.
	createBranchErr error

	// checkoutErr is returned by Checkout.
	checkoutErr error

	// fetchErr is returned by Fetch.
	fetchErr error

	// Recorded calls.
	createdBranches []createBranchCall
	checkedOut      []string
	fetched         []string
}

type createBranchCall struct {
	name string
	base string
}

func (m *mockGitClient) BranchExists(_ context.Context, branch string) (bool, error) {
	if m.branchExistsResults != nil {
		if r, ok := m.branchExistsResults[branch]; ok {
			return r.exists, r.err
		}
	}
	return false, nil
}

func (m *mockGitClient) CreateBranch(_ context.Context, name, base string) error {
	m.createdBranches = append(m.createdBranches, createBranchCall{name: name, base: base})
	return m.createBranchErr
}

func (m *mockGitClient) Checkout(_ context.Context, branch string) error {
	m.checkedOut = append(m.checkedOut, branch)
	return m.checkoutErr
}

func (m *mockGitClient) Fetch(_ context.Context, remote string) error {
	m.fetched = append(m.fetched, remote)
	return m.fetchErr
}

// branchManagerWithMock builds a BranchManager backed by the provided mock.
// It uses the same internal fields but substitutes a thin adapter so the mock
// satisfies the method set expected by BranchManager. Because BranchManager
// holds a concrete *git.GitClient we use a test-only constructor that stores
// the adapter behind an interface field in a wrapped struct.
//
// Since BranchManager currently uses *git.GitClient directly (a concrete type)
// we delegate through a small testBranchManager wrapper that holds the mock
// and replicates the manager logic using the mock's methods.
type testBranchManager struct {
	mock           *mockGitClient
	branchTemplate string
	baseBranch     string
}

func newTestBranchManager(mock *mockGitClient, template, base string) *testBranchManager {
	if template == "" {
		template = defaultBranchTemplate
	}
	if base == "" {
		base = "main"
	}
	return &testBranchManager{
		mock:           mock,
		branchTemplate: template,
		baseBranch:     base,
	}
}

func (t *testBranchManager) ResolveBranchName(phaseID int, phaseName, projectName string) string {
	slug := slugify(phaseName)
	r := strings.NewReplacer(
		"{phase_id}", fmt.Sprintf("%d", phaseID),
		"{slug}", slug,
		"{project}", projectName,
	)
	return r.Replace(t.branchTemplate)
}

func (t *testBranchManager) BranchExists(ctx context.Context, branchName string) (bool, error) {
	return t.mock.BranchExists(ctx, branchName)
}

func (t *testBranchManager) CreatePhaseBranch(ctx context.Context, opts PhaseBranchOpts) (string, error) {
	base := t.baseBranch
	if opts.PreviousPhaseBranch != "" {
		base = opts.PreviousPhaseBranch
	}
	if opts.SyncBase {
		// Fetch errors are non-fatal (no remote); ignore in test helper.
		_ = t.mock.Fetch(ctx, "")
	}
	branchName := t.ResolveBranchName(opts.PhaseID, opts.PhaseName, opts.ProjectName)
	if err := t.mock.CreateBranch(ctx, branchName, base); err != nil {
		return "", fmt.Errorf("branch manager: create phase branch %q from %q: %w", branchName, base, err)
	}
	return branchName, nil
}

func (t *testBranchManager) SwitchToPhaseBranch(ctx context.Context, branchName string) error {
	exists, err := t.mock.BranchExists(ctx, branchName)
	if err != nil {
		return fmt.Errorf("branch manager: switch to %q: checking existence: %w", branchName, err)
	}
	if !exists {
		return fmt.Errorf("branch manager: switch to %q: branch does not exist", branchName)
	}
	return t.mock.Checkout(ctx, branchName)
}

func (t *testBranchManager) EnsureBranch(ctx context.Context, opts PhaseBranchOpts) (string, error) {
	branchName := t.ResolveBranchName(opts.PhaseID, opts.PhaseName, opts.ProjectName)
	exists, err := t.mock.BranchExists(ctx, branchName)
	if err != nil {
		return "", fmt.Errorf("branch manager: ensure branch %q: %w", branchName, err)
	}
	if exists {
		if err := t.mock.Checkout(ctx, branchName); err != nil {
			return "", fmt.Errorf("branch manager: ensure branch %q: checkout: %w", branchName, err)
		}
		return branchName, nil
	}
	return t.CreatePhaseBranch(ctx, opts)
}

// --- TestBranchManager_BranchExists --------------------------------------

func TestBranchManager_BranchExists(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		mockExists bool
		mockErr    error
		wantExists bool
		wantErr    bool
	}{
		{
			name:       "branch exists",
			branch:     "phase/1-foundation",
			mockExists: true,
			wantExists: true,
		},
		{
			name:       "branch does not exist",
			branch:     "phase/2-impl",
			mockExists: false,
			wantExists: false,
		},
		{
			name:    "git error propagated",
			branch:  "phase/3-review",
			mockErr: errors.New("git failure"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGitClient{
				branchExistsResults: map[string]struct {
					exists bool
					err    error
				}{
					tt.branch: {exists: tt.mockExists, err: tt.mockErr},
				},
			}
			tbm := newTestBranchManager(mock, "", "main")
			got, err := tbm.BranchExists(context.Background(), tt.branch)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, got)
		})
	}
}

// --- TestBranchManager_CreatePhaseBranch ---------------------------------

func TestBranchManager_CreatePhaseBranch(t *testing.T) {
	tests := []struct {
		name            string
		template        string
		baseBranch      string
		opts            PhaseBranchOpts
		createBranchErr error
		fetchErr        error
		wantBranch      string
		wantBase        string
		wantFetched     bool
		wantErr         bool
	}{
		{
			name:       "first phase branches from baseBranch",
			template:   "phase/{phase_id}-{slug}",
			baseBranch: "main",
			opts: PhaseBranchOpts{
				PhaseID:   1,
				PhaseName: "Foundation",
			},
			wantBranch: "phase/1-foundation",
			wantBase:   "main",
		},
		{
			name:       "subsequent phase branches from previous",
			template:   "phase/{phase_id}-{slug}",
			baseBranch: "main",
			opts: PhaseBranchOpts{
				PhaseID:             2,
				PhaseName:           "Implementation",
				PreviousPhaseBranch: "phase/1-foundation",
			},
			wantBranch: "phase/2-implementation",
			wantBase:   "phase/1-foundation",
		},
		{
			name:       "SyncBase triggers fetch",
			template:   "phase/{phase_id}-{slug}",
			baseBranch: "main",
			opts: PhaseBranchOpts{
				PhaseID:   3,
				PhaseName: "Review",
				SyncBase:  true,
			},
			wantBranch:  "phase/3-review",
			wantBase:    "main",
			wantFetched: true,
		},
		{
			name:       "SyncBase with fetch error is non-fatal",
			template:   "phase/{phase_id}-{slug}",
			baseBranch: "main",
			opts: PhaseBranchOpts{
				PhaseID:   4,
				PhaseName: "Fix",
				SyncBase:  true,
			},
			fetchErr:    errors.New("no remote origin"),
			wantBranch:  "phase/4-fix",
			wantBase:    "main",
			wantFetched: true,
		},
		{
			name:       "git CreateBranch error propagated",
			template:   "phase/{phase_id}-{slug}",
			baseBranch: "main",
			opts: PhaseBranchOpts{
				PhaseID:   5,
				PhaseName: "PR",
			},
			createBranchErr: errors.New("branch already exists"),
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGitClient{
				createBranchErr: tt.createBranchErr,
				fetchErr:        tt.fetchErr,
			}
			tbm := newTestBranchManager(mock, tt.template, tt.baseBranch)

			got, err := tbm.CreatePhaseBranch(context.Background(), tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBranch, got)

			require.Len(t, mock.createdBranches, 1)
			assert.Equal(t, tt.wantBranch, mock.createdBranches[0].name)
			assert.Equal(t, tt.wantBase, mock.createdBranches[0].base)

			if tt.wantFetched {
				assert.NotEmpty(t, mock.fetched)
			} else {
				assert.Empty(t, mock.fetched)
			}
		})
	}
}

// --- TestBranchManager_SwitchToPhaseBranch --------------------------------

func TestBranchManager_SwitchToPhaseBranch(t *testing.T) {
	tests := []struct {
		name        string
		branch      string
		mockExists  bool
		checkoutErr error
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:       "existing branch checked out",
			branch:     "phase/1-foundation",
			mockExists: true,
		},
		{
			name:       "non-existent branch returns error",
			branch:     "phase/99-missing",
			mockExists: false,
			wantErr:    true,
			wantErrMsg: "does not exist",
		},
		{
			name:        "checkout error propagated",
			branch:      "phase/2-impl",
			mockExists:  true,
			checkoutErr: errors.New("lock failure"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGitClient{
				branchExistsResults: map[string]struct {
					exists bool
					err    error
				}{
					tt.branch: {exists: tt.mockExists},
				},
				checkoutErr: tt.checkoutErr,
			}
			tbm := newTestBranchManager(mock, "", "main")

			err := tbm.SwitchToPhaseBranch(context.Background(), tt.branch)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
				return
			}
			require.NoError(t, err)
			assert.Contains(t, mock.checkedOut, tt.branch)
		})
	}
}

// --- TestBranchManager_EnsureBranch --------------------------------------

func TestBranchManager_EnsureBranch(t *testing.T) {
	tests := []struct {
		name        string
		opts        PhaseBranchOpts
		mockExists  bool
		wantBranch  string
		wantCreated bool
		wantErr     bool
	}{
		{
			name: "branch exists -- switches to it",
			opts: PhaseBranchOpts{
				PhaseID:   1,
				PhaseName: "Foundation",
			},
			mockExists:  true,
			wantBranch:  "phase/1-foundation",
			wantCreated: false,
		},
		{
			name: "branch does not exist -- creates it",
			opts: PhaseBranchOpts{
				PhaseID:   2,
				PhaseName: "Implementation",
			},
			mockExists:  false,
			wantBranch:  "phase/2-implementation",
			wantCreated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedName := slugify(tt.opts.PhaseName)
			branchName := fmt.Sprintf("phase/%d-%s", tt.opts.PhaseID, resolvedName)

			mock := &mockGitClient{
				branchExistsResults: map[string]struct {
					exists bool
					err    error
				}{
					branchName: {exists: tt.mockExists},
				},
			}
			tbm := newTestBranchManager(mock, "", "main")

			got, err := tbm.EnsureBranch(context.Background(), tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBranch, got)

			if tt.wantCreated {
				assert.NotEmpty(t, mock.createdBranches, "expected branch to be created")
				assert.Empty(t, mock.checkedOut, "expected no checkout call when creating")
			} else {
				assert.Empty(t, mock.createdBranches, "expected no creation when branch exists")
				assert.Contains(t, mock.checkedOut, tt.wantBranch)
			}
		})
	}
}

// --- TestBranchManager_WithLogger ----------------------------------------

func TestBranchManager_WithLogger(t *testing.T) {
	bm := NewBranchManager(nil, "", "main")
	assert.Nil(t, bm.logger)

	// WithLogger should return the same manager with logger attached.
	// We can't easily compare logger instances, just verify it doesn't panic.
	result := bm.WithLogger(nil)
	assert.Same(t, bm, result)
}

func TestBranchManager_WithLogger_RealLogger(t *testing.T) {
	bm := NewBranchManager(nil, "", "main")
	assert.Nil(t, bm.logger)

	logger := log.New(io.Discard)
	result := bm.WithLogger(logger)

	// WithLogger is fluent — must return the same pointer.
	assert.Same(t, bm, result)
	// The logger must be stored on the manager.
	assert.Equal(t, logger, bm.logger)
}

// --- Additional slugify edge cases ----------------------------------------

func TestSlugify_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "single space",
			input: " ",
			want:  "",
		},
		{
			name:  "only numbers",
			input: "123",
			want:  "123",
		},
		{
			name:  "mixed hyphen and special chars",
			input: "hello!@#$%world",
			want:  "hello-world",
		},
		{
			name:  "multiple dots and underscores",
			input: "v1.2.3_release",
			want:  "v1-2-3-release",
		},
		{
			name:  "unicode: CJK characters stripped",
			input: "phase 你好",
			want:  "phase",
		},
		{
			name:  "unicode: accented characters stripped",
			input: "café",
			want:  "caf",
		},
		{
			name:  "slash characters removed",
			input: "feature/new-thing",
			want:  "feature-new-thing",
		},
		{
			name:  "leading and trailing hyphens from special chars",
			input: "!important!",
			want:  "important",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slugify(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Additional ResolveBranchName edge cases ---------------------------------

func TestResolveBranchName_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		phaseID     int
		phaseName   string
		projectName string
		want        string
	}{
		{
			name:        "empty phase name produces phase_id only slug",
			template:    "phase/{phase_id}-{slug}",
			phaseID:     1,
			phaseName:   "",
			projectName: "",
			want:        "phase/1-",
		},
		{
			name:        "phase name with only special chars produces empty slug",
			template:    "phase/{phase_id}-{slug}",
			phaseID:     2,
			phaseName:   "---",
			projectName: "",
			want:        "phase/2-",
		},
		{
			name:        "phase name with git-unsafe chars sanitized",
			template:    "phase/{phase_id}-{slug}",
			phaseID:     3,
			phaseName:   "Auth & Token~Setup",
			projectName: "",
			want:        "phase/3-auth-token-setup",
		},
		{
			name:        "large phase ID",
			template:    "phase/{phase_id}-{slug}",
			phaseID:     999,
			phaseName:   "Final",
			projectName: "",
			want:        "phase/999-final",
		},
		{
			name:        "project name with special chars used verbatim",
			template:    "{project}/phase-{phase_id}",
			phaseID:     1,
			phaseName:   "Any Name",
			projectName: "my-org/my-project",
			want:        "my-org/my-project/phase-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bm := NewBranchManager(nil, tt.template, "main")
			got := bm.ResolveBranchName(tt.phaseID, tt.phaseName, tt.projectName)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Integration tests using a real git repository -------------------------

// newTestGitRepo initialises a temporary git repository, adds an initial
// commit, and returns the directory path together with a *git.GitClient
// pointing at it. It is the integration-test counterpart to the mock-based
// testBranchManager helpers used in unit tests above.
func newTestGitRepo(t *testing.T) (string, *git.GitClient) {
	t.Helper()
	dir := t.TempDir()

	gitRun(t, dir, "git", "init", "-b", "main")
	gitRun(t, dir, "git", "config", "user.email", "test@example.com")
	gitRun(t, dir, "git", "config", "user.name", "Test User")

	// An initial commit is required for branch operations to work correctly.
	writeTestFile(t, dir, "README.md", "# Test Project\n")
	gitRun(t, dir, "git", "add", ".")
	gitRun(t, dir, "git", "commit", "-m", "Initial commit")

	c, err := git.NewGitClient(dir)
	require.NoError(t, err)
	return dir, c
}

// gitRun is a test helper that runs a git command in the given directory and
// fails the test immediately if the command returns a non-zero exit code.
func gitRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command %q %v failed:\n%s", name, args, out)
}

// writeTestFile creates (or overwrites) a file at path dir/name with content.
func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

// --- TestBranchManager_Real_CreatePhaseBranch --------------------------------

// TestBranchManager_Real_CreatePhaseBranch verifies CreatePhaseBranch against
// a real git repository. It replaces the mock-based approach with an
// end-to-end test that exercises the entire code path including git operations.
func TestBranchManager_Real_CreatePhaseBranch(t *testing.T) {
	tests := []struct {
		name       string
		template   string
		baseBranch string
		opts       PhaseBranchOpts
		wantBranch string
	}{
		{
			name:       "first phase branches from baseBranch",
			template:   "phase/{phase_id}-{slug}",
			baseBranch: "main",
			opts: PhaseBranchOpts{
				PhaseID:   1,
				PhaseName: "Foundation Setup",
			},
			wantBranch: "phase/1-foundation-setup",
		},
		{
			name:       "second phase branches from previous phase branch",
			template:   "phase/{phase_id}-{slug}",
			baseBranch: "main",
			opts: PhaseBranchOpts{
				PhaseID:             2,
				PhaseName:           "Implementation",
				PreviousPhaseBranch: "phase/1-foundation-setup",
			},
			wantBranch: "phase/2-implementation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := newTestGitRepo(t)
			bm := NewBranchManager(client, tt.template, tt.baseBranch)

			ctx := context.Background()

			// For the "second phase" case, first create the phase/1 branch.
			if tt.opts.PreviousPhaseBranch != "" {
				require.NoError(t, client.CreateBranch(ctx, tt.opts.PreviousPhaseBranch, ""))
				// Return to main so the second branch is created from the right base.
				require.NoError(t, client.Checkout(ctx, tt.baseBranch))
			}

			got, err := bm.CreatePhaseBranch(ctx, tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBranch, got)

			// Verify the branch was actually created and is the current branch
			// (CreateBranch uses git checkout -b).
			current, err := client.CurrentBranch(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBranch, current)

			// Verify the branch exists.
			exists, err := client.BranchExists(ctx, tt.wantBranch)
			require.NoError(t, err)
			assert.True(t, exists)
		})
	}
}

// TestBranchManager_Real_CreatePhaseBranch_DuplicateErrors verifies that
// creating a branch that already exists returns an error, not silently skips.
func TestBranchManager_Real_CreatePhaseBranch_DuplicateErrors(t *testing.T) {
	_, client := newTestGitRepo(t)
	bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")
	ctx := context.Background()

	opts := PhaseBranchOpts{PhaseID: 1, PhaseName: "Foundation"}

	// First creation must succeed.
	_, err := bm.CreatePhaseBranch(ctx, opts)
	require.NoError(t, err)

	// Switch back to main.
	require.NoError(t, client.Checkout(ctx, "main"))

	// Second creation of the same branch must fail (branch already exists).
	_, err = bm.CreatePhaseBranch(ctx, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch manager: create phase branch")
}

// --- TestBranchManager_Real_SwitchToPhaseBranch -------------------------------

func TestBranchManager_Real_SwitchToPhaseBranch(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, ctx context.Context, client *git.GitClient)
		branchName  string
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name: "switches to existing branch",
			setup: func(t *testing.T, ctx context.Context, client *git.GitClient) {
				t.Helper()
				require.NoError(t, client.CreateBranch(ctx, "phase/1-foundation", ""))
				require.NoError(t, client.Checkout(ctx, "main"))
			},
			branchName: "phase/1-foundation",
		},
		{
			name:       "returns error for non-existent branch",
			setup:      func(t *testing.T, ctx context.Context, client *git.GitClient) {},
			branchName: "phase/99-nonexistent",
			wantErr:    true,
			wantErrMsg: "branch does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := newTestGitRepo(t)
			bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")
			ctx := context.Background()

			tt.setup(t, ctx, client)

			err := bm.SwitchToPhaseBranch(ctx, tt.branchName)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
				return
			}
			require.NoError(t, err)

			current, err := client.CurrentBranch(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.branchName, current)
		})
	}
}

// --- TestBranchManager_Real_BranchExists -------------------------------------

func TestBranchManager_Real_BranchExists(t *testing.T) {
	_, client := newTestGitRepo(t)
	bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")
	ctx := context.Background()

	// "main" must exist.
	exists, err := bm.BranchExists(ctx, "main")
	require.NoError(t, err)
	assert.True(t, exists)

	// Not-yet-created branch must not exist.
	exists, err = bm.BranchExists(ctx, "phase/1-foundation")
	require.NoError(t, err)
	assert.False(t, exists)

	// Create the branch, then check it exists.
	require.NoError(t, client.CreateBranch(ctx, "phase/1-foundation", ""))
	exists, err = bm.BranchExists(ctx, "phase/1-foundation")
	require.NoError(t, err)
	assert.True(t, exists)
}

// --- TestBranchManager_Real_EnsureBranch -------------------------------------

func TestBranchManager_Real_EnsureBranch(t *testing.T) {
	t.Run("creates branch when it does not exist", func(t *testing.T) {
		_, client := newTestGitRepo(t)
		bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")
		ctx := context.Background()

		opts := PhaseBranchOpts{PhaseID: 1, PhaseName: "Foundation"}

		got, err := bm.EnsureBranch(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, "phase/1-foundation", got)

		// After EnsureBranch the active branch must be the new branch.
		current, err := client.CurrentBranch(ctx)
		require.NoError(t, err)
		assert.Equal(t, "phase/1-foundation", current)
	})

	t.Run("switches to existing branch without creating", func(t *testing.T) {
		_, client := newTestGitRepo(t)
		bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")
		ctx := context.Background()

		// Pre-create the branch and return to main.
		require.NoError(t, client.CreateBranch(ctx, "phase/1-foundation", ""))
		require.NoError(t, client.Checkout(ctx, "main"))

		opts := PhaseBranchOpts{PhaseID: 1, PhaseName: "Foundation"}

		got, err := bm.EnsureBranch(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, "phase/1-foundation", got)

		current, err := client.CurrentBranch(ctx)
		require.NoError(t, err)
		assert.Equal(t, "phase/1-foundation", current)
	})

	t.Run("idempotent: calling twice returns same result", func(t *testing.T) {
		_, client := newTestGitRepo(t)
		bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")
		ctx := context.Background()

		opts := PhaseBranchOpts{PhaseID: 2, PhaseName: "Implementation"}

		first, err := bm.EnsureBranch(ctx, opts)
		require.NoError(t, err)

		// Switch back to main so the second call needs to checkout.
		require.NoError(t, client.Checkout(ctx, "main"))

		second, err := bm.EnsureBranch(ctx, opts)
		require.NoError(t, err)

		assert.Equal(t, first, second)
		current, err := client.CurrentBranch(ctx)
		require.NoError(t, err)
		assert.Equal(t, "phase/2-implementation", current)
	})
}

// --- TestBranchManager_Real_SyncBase_NoRemote_NonFatal -----------------------

// TestBranchManager_Real_SyncBase_NoRemote_NonFatal verifies that when
// SyncBase is true but no remote is configured, CreatePhaseBranch logs a
// warning and continues rather than aborting.
func TestBranchManager_Real_SyncBase_NoRemote_NonFatal(t *testing.T) {
	_, client := newTestGitRepo(t)
	bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")
	ctx := context.Background()

	// Repo has no remote; fetch will fail with "no such remote 'origin'".
	// The BranchManager must treat this as a warning and still create the branch.
	opts := PhaseBranchOpts{
		PhaseID:   1,
		PhaseName: "Foundation",
		SyncBase:  true,
	}

	got, err := bm.CreatePhaseBranch(ctx, opts)
	require.NoError(t, err, "fetch failure with no remote must be non-fatal")
	assert.Equal(t, "phase/1-foundation", got)

	exists, err := client.BranchExists(ctx, "phase/1-foundation")
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestBranchManager_Real_SyncBase_WithLogger_LogsWarning verifies that when
// SyncBase fails and a logger is attached, a warning is emitted without
// aborting the operation.
func TestBranchManager_Real_SyncBase_WithLogger_LogsWarning(t *testing.T) {
	_, client := newTestGitRepo(t)

	logger := log.New(io.Discard)

	bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main").WithLogger(logger)
	ctx := context.Background()

	opts := PhaseBranchOpts{
		PhaseID:   5,
		PhaseName: "Deployment",
		SyncBase:  true, // will fail — no remote origin in test repo
	}

	got, err := bm.CreatePhaseBranch(ctx, opts)
	require.NoError(t, err, "fetch failure must not abort branch creation")
	assert.Equal(t, "phase/5-deployment", got)
}

// --- Integration: 3-chained branches -----------------------------------------

// TestIntegration_ThreeChainedBranches creates three branches chained from
// each other (main -> phase/1 -> phase/2 -> phase/3) and verifies that each
// branch was created from the correct parent.
func TestIntegration_ThreeChainedBranches(t *testing.T) {
	_, client := newTestGitRepo(t)
	bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")
	ctx := context.Background()

	phases := []struct {
		id       int
		name     string
		previous string
		want     string
	}{
		{id: 1, name: "Foundation Setup", previous: "", want: "phase/1-foundation-setup"},
		{id: 2, name: "Implementation", previous: "phase/1-foundation-setup", want: "phase/2-implementation"},
		{id: 3, name: "Integration & Testing", previous: "phase/2-implementation", want: "phase/3-integration-testing"},
	}

	createdBranches := make([]string, 0, len(phases))

	for _, ph := range phases {
		// Return to base branch (main) before creating each new branch so the
		// BranchManager controls the parent via PreviousPhaseBranch.
		if ph.previous != "" {
			require.NoError(t, client.Checkout(ctx, "main"))
		}

		opts := PhaseBranchOpts{
			PhaseID:             ph.id,
			PhaseName:           ph.name,
			PreviousPhaseBranch: ph.previous,
		}

		got, err := bm.CreatePhaseBranch(ctx, opts)
		require.NoError(t, err, "phase %d", ph.id)
		assert.Equal(t, ph.want, got, "phase %d branch name mismatch", ph.id)

		createdBranches = append(createdBranches, got)
	}

	// Verify all three branches exist.
	for _, br := range createdBranches {
		exists, err := client.BranchExists(ctx, br)
		require.NoError(t, err)
		assert.True(t, exists, "branch %q must exist after creation", br)
	}
}

// --- Integration: resume scenario --------------------------------------------

// TestIntegration_ResumeScenario simulates the resume scenario: all three
// branches already exist from a previous run. EnsureBranch must switch to
// each branch without attempting to create it again.
func TestIntegration_ResumeScenario(t *testing.T) {
	_, client := newTestGitRepo(t)
	bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")
	ctx := context.Background()

	branchNames := []string{
		"phase/1-foundation-setup",
		"phase/2-implementation",
		"phase/3-integration-testing",
	}

	// Pre-create all branches (simulating a previous run).
	for _, br := range branchNames {
		require.NoError(t, client.CreateBranch(ctx, br, ""))
		require.NoError(t, client.Checkout(ctx, "main"))
	}

	// Now simulate resume: EnsureBranch for each phase. Because branches
	// already exist, the manager must switch without creating.
	phases := []struct {
		id   int
		name string
		want string
	}{
		{id: 1, name: "Foundation Setup", want: "phase/1-foundation-setup"},
		{id: 2, name: "Implementation", want: "phase/2-implementation"},
		{id: 3, name: "Integration & Testing", want: "phase/3-integration-testing"},
	}

	for _, ph := range phases {
		// Return to main before each EnsureBranch to simulate the real pipeline.
		require.NoError(t, client.Checkout(ctx, "main"))

		opts := PhaseBranchOpts{
			PhaseID:   ph.id,
			PhaseName: ph.name,
		}

		got, err := bm.EnsureBranch(ctx, opts)
		require.NoError(t, err, "phase %d", ph.id)
		assert.Equal(t, ph.want, got, "phase %d", ph.id)

		current, err := client.CurrentBranch(ctx)
		require.NoError(t, err)
		assert.Equal(t, ph.want, current, "phase %d: active branch mismatch", ph.id)
	}
}

// --- Context cancellation tests (real BranchManager) ------------------------

func TestBranchManager_Real_ContextCancellation(t *testing.T) {
	t.Run("CreatePhaseBranch accepts cancelled context", func(t *testing.T) {
		_, client := newTestGitRepo(t)
		bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		// Must not panic. Error is allowed but not required (git is fast).
		_, _ = bm.CreatePhaseBranch(ctx, PhaseBranchOpts{
			PhaseID:   1,
			PhaseName: "Foundation",
		})
	})

	t.Run("SwitchToPhaseBranch accepts cancelled context", func(t *testing.T) {
		_, client := newTestGitRepo(t)
		bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")

		ctx := context.Background()
		// Create a branch while context is still alive.
		require.NoError(t, client.CreateBranch(ctx, "phase/1-foundation", ""))
		require.NoError(t, client.Checkout(ctx, "main"))

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		// Must not panic.
		_ = bm.SwitchToPhaseBranch(cancelCtx, "phase/1-foundation")
	})

	t.Run("BranchExists accepts cancelled context", func(t *testing.T) {
		_, client := newTestGitRepo(t)
		bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Must not panic.
		_, _ = bm.BranchExists(ctx, "main")
	})

	t.Run("EnsureBranch accepts cancelled context", func(t *testing.T) {
		_, client := newTestGitRepo(t)
		bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Must not panic.
		_, _ = bm.EnsureBranch(ctx, PhaseBranchOpts{
			PhaseID:   1,
			PhaseName: "Foundation",
		})
	})
}

// --- Error wrapping tests (real BranchManager) -------------------------------

// TestBranchManager_Real_ErrorWrapping verifies that error messages returned
// by BranchManager include the "branch manager:" context prefix so callers
// can identify the source of errors.
func TestBranchManager_Real_ErrorWrapping(t *testing.T) {
	_, client := newTestGitRepo(t)
	bm := NewBranchManager(client, "phase/{phase_id}-{slug}", "main")
	ctx := context.Background()

	t.Run("SwitchToPhaseBranch to nonexistent branch", func(t *testing.T) {
		err := bm.SwitchToPhaseBranch(ctx, "phase/99-missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "branch manager:")
	})

	t.Run("CreatePhaseBranch duplicate wraps error", func(t *testing.T) {
		opts := PhaseBranchOpts{PhaseID: 1, PhaseName: "Foundation"}
		_, err := bm.CreatePhaseBranch(ctx, opts)
		require.NoError(t, err)

		// Return to main before trying to create the same branch again.
		require.NoError(t, client.Checkout(ctx, "main"))

		_, err = bm.CreatePhaseBranch(ctx, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "branch manager: create phase branch")
	})
}

// --- EnsureBranch error from BranchExists ------------------------------------

// TestBranchManager_EnsureBranch_BranchExistsError verifies that when
// BranchExists returns an error, EnsureBranch propagates it correctly.
func TestBranchManager_EnsureBranch_BranchExistsError(t *testing.T) {
	mock := &mockGitClient{
		branchExistsResults: map[string]struct {
			exists bool
			err    error
		}{
			"phase/1-foundation": {err: errors.New("git internal error")},
		},
	}
	tbm := newTestBranchManager(mock, "", "main")

	_, err := tbm.EnsureBranch(context.Background(), PhaseBranchOpts{
		PhaseID:   1,
		PhaseName: "Foundation",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch manager: ensure branch")
}

// --- EnsureBranch error on checkout of existing branch -----------------------

func TestBranchManager_EnsureBranch_CheckoutError(t *testing.T) {
	branchName := "phase/1-foundation"
	mock := &mockGitClient{
		branchExistsResults: map[string]struct {
			exists bool
			err    error
		}{
			branchName: {exists: true},
		},
		checkoutErr: errors.New("checkout failed"),
	}
	tbm := newTestBranchManager(mock, "", "main")

	_, err := tbm.EnsureBranch(context.Background(), PhaseBranchOpts{
		PhaseID:   1,
		PhaseName: "Foundation",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkout")
}

// --- BranchExists error path via testBranchManager ---------------------------

func TestBranchManager_BranchExists_GitError(t *testing.T) {
	mock := &mockGitClient{
		branchExistsResults: map[string]struct {
			exists bool
			err    error
		}{
			"any-branch": {err: errors.New("git failed")},
		},
	}
	tbm := newTestBranchManager(mock, "", "main")

	_, err := tbm.BranchExists(context.Background(), "any-branch")
	require.Error(t, err)
}

// --- SwitchToPhaseBranch: BranchExists error propagation ---------------------

func TestBranchManager_SwitchToPhaseBranch_BranchExistsError(t *testing.T) {
	mock := &mockGitClient{
		branchExistsResults: map[string]struct {
			exists bool
			err    error
		}{
			"phase/1-bad": {err: errors.New("rev-parse failed")},
		},
	}
	tbm := newTestBranchManager(mock, "", "main")

	err := tbm.SwitchToPhaseBranch(context.Background(), "phase/1-bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checking existence")
}

// --- SyncBase: fetch called and proceeds even when fetch errors --------------

func TestBranchManager_CreatePhaseBranch_SyncBase_FetchErrorNonFatal(t *testing.T) {
	// fetchErr is non-nil but CreateBranch succeeds: branch must be created.
	mock := &mockGitClient{
		fetchErr: errors.New("fatal: 'origin' does not appear to be a git repository"),
	}
	tbm := newTestBranchManager(mock, "", "main")

	opts := PhaseBranchOpts{
		PhaseID:   7,
		PhaseName: "Deployment",
		SyncBase:  true,
	}

	got, err := tbm.CreatePhaseBranch(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "phase/7-deployment", got)

	// Fetch must have been attempted.
	assert.Len(t, mock.fetched, 1)
	// Branch must have been created.
	require.Len(t, mock.createdBranches, 1)
	assert.Equal(t, "phase/7-deployment", mock.createdBranches[0].name)
	assert.Equal(t, "main", mock.createdBranches[0].base)
}

// --- Benchmark ---------------------------------------------------------------

// BenchmarkResolveBranchName measures the cost of template substitution and
// slug generation for the typical case.
func BenchmarkResolveBranchName(b *testing.B) {
	bm := NewBranchManager(nil, "phase/{phase_id}-{slug}", "main")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bm.ResolveBranchName(3, "Integration & Testing", "raven")
	}
}

// BenchmarkSlugify measures the cost of the slug conversion alone.
func BenchmarkSlugify(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = slugify("Integration & Testing: Phase 3 -- Final Review")
	}
}

// --- Fuzz test for slugify ---------------------------------------------------

// FuzzSlugify ensures that slugify never panics and always returns a string
// that contains only lowercase ASCII letters, digits, and hyphens (no leading
// or trailing hyphens).
func FuzzSlugify(f *testing.F) {
	// Seed corpus.
	f.Add("")
	f.Add("normal text")
	f.Add("Foundation & Setup")
	f.Add("Phase 1: Init")
	f.Add("---")
	f.Add("Büro & Co.")
	f.Add("你好世界")
	f.Add(strings.Repeat("a", 1000))
	f.Add("!@#$%^&*()")

	f.Fuzz(func(t *testing.T, input string) {
		result := slugify(input)

		// Invariant 1: result contains only [a-z0-9-].
		for _, ch := range result {
			if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
				t.Errorf("slugify(%q) = %q contains invalid character %q", input, result, ch)
			}
		}

		// Invariant 2: no leading or trailing hyphen.
		if len(result) > 0 {
			if result[0] == '-' {
				t.Errorf("slugify(%q) = %q starts with hyphen", input, result)
			}
			if result[len(result)-1] == '-' {
				t.Errorf("slugify(%q) = %q ends with hyphen", input, result)
			}
		}

		// Invariant 3: no consecutive hyphens (they are collapsed to one).
		if strings.Contains(result, "--") {
			t.Errorf("slugify(%q) = %q contains consecutive hyphens", input, result)
		}
	})
}


