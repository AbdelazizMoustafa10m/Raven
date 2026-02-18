package pipeline

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
)

// defaultBranchTemplate is the branch name pattern used when no template is
// configured in the project config.
const defaultBranchTemplate = "phase/{phase_id}-{slug}"

// nonAlphanumRE matches any sequence of characters that are not ASCII lowercase
// letters or digits. Used by slugify to replace unsafe characters.
var nonAlphanumRE = regexp.MustCompile(`[^a-z0-9]+`)

// BranchManager creates and switches git branches for pipeline phases using a
// configurable branch name template. It wraps a GitClient to perform all git
// operations and never modifies global state.
type BranchManager struct {
	gitClient      *git.GitClient
	branchTemplate string // e.g., "phase/{phase_id}-{slug}"
	baseBranch     string // e.g., "main"
	logger         *log.Logger
}

// NewBranchManager returns a BranchManager configured with the given git
// client, branch name template, and base branch name. If branchTemplate is
// empty the default template "phase/{phase_id}-{slug}" is used. If baseBranch
// is empty it defaults to "main".
func NewBranchManager(gitClient *git.GitClient, branchTemplate, baseBranch string) *BranchManager {
	if branchTemplate == "" {
		branchTemplate = defaultBranchTemplate
	}
	if baseBranch == "" {
		baseBranch = "main"
	}
	return &BranchManager{
		gitClient:      gitClient,
		branchTemplate: branchTemplate,
		baseBranch:     baseBranch,
	}
}

// WithLogger attaches a charmbracelet/log Logger to the BranchManager so that
// non-fatal warnings (e.g., remote-not-found during fetch) are emitted instead
// of being silently swallowed.
func (b *BranchManager) WithLogger(logger *log.Logger) *BranchManager {
	b.logger = logger
	return b
}

// PhaseBranchOpts configures a single call to CreatePhaseBranch or EnsureBranch.
type PhaseBranchOpts struct {
	// PhaseID is the numeric identifier for this phase.
	PhaseID int

	// PhaseName is the human-readable phase name used to build the slug.
	PhaseName string

	// ProjectName is substituted for the {project} template variable.
	ProjectName string

	// PreviousPhaseBranch is the branch to base the new branch on. When empty
	// the BranchManager's baseBranch is used (first phase behaviour).
	PreviousPhaseBranch string

	// SyncBase when true causes the manager to fetch from origin before
	// creating the branch. A missing remote is logged as a warning but does
	// not abort the operation.
	SyncBase bool
}

// ResolveBranchName applies template variable substitution to produce a branch
// name from the configured template. The following variables are replaced:
//
//   - {phase_id}  — the numeric phase identifier (e.g., "1")
//   - {slug}      — a kebab-case slug derived from phaseName
//   - {project}   — projectName as supplied by the caller
func (b *BranchManager) ResolveBranchName(phaseID int, phaseName string, projectName string) string {
	slug := slugify(phaseName)
	r := strings.NewReplacer(
		"{phase_id}", fmt.Sprintf("%d", phaseID),
		"{slug}", slug,
		"{project}", projectName,
	)
	return r.Replace(b.branchTemplate)
}

// CreatePhaseBranch creates a new git branch for the given phase. For the
// first phase (PreviousPhaseBranch empty) the branch is based on the
// BranchManager's baseBranch. Subsequent phases are based on
// PreviousPhaseBranch. If SyncBase is true the manager attempts to fetch from
// origin before creating the branch; a missing remote is treated as a warning.
// Returns the resolved branch name on success.
func (b *BranchManager) CreatePhaseBranch(ctx context.Context, opts PhaseBranchOpts) (string, error) {
	base := b.baseBranch
	if opts.PreviousPhaseBranch != "" {
		base = opts.PreviousPhaseBranch
	}

	if opts.SyncBase {
		if err := b.gitClient.Fetch(ctx, ""); err != nil {
			// A missing remote is non-fatal; log a warning and continue.
			b.logWarn("fetch from origin failed, proceeding without sync", "error", err)
		}
	}

	branchName := b.ResolveBranchName(opts.PhaseID, opts.PhaseName, opts.ProjectName)

	if err := b.gitClient.CreateBranch(ctx, branchName, base); err != nil {
		return "", fmt.Errorf("branch manager: create phase branch %q from %q: %w", branchName, base, err)
	}

	return branchName, nil
}

// SwitchToPhaseBranch checks out an existing phase branch. It returns an error
// if the branch does not exist locally.
func (b *BranchManager) SwitchToPhaseBranch(ctx context.Context, branchName string) error {
	exists, err := b.gitClient.BranchExists(ctx, branchName)
	if err != nil {
		return fmt.Errorf("branch manager: switch to %q: checking existence: %w", branchName, err)
	}
	if !exists {
		return fmt.Errorf("branch manager: switch to %q: branch does not exist", branchName)
	}
	if err := b.gitClient.Checkout(ctx, branchName); err != nil {
		return fmt.Errorf("branch manager: switch to %q: %w", branchName, err)
	}
	return nil
}

// BranchExists reports whether the named branch exists locally. It delegates
// directly to the underlying GitClient.
func (b *BranchManager) BranchExists(ctx context.Context, branchName string) (bool, error) {
	exists, err := b.gitClient.BranchExists(ctx, branchName)
	if err != nil {
		return false, fmt.Errorf("branch manager: branch exists %q: %w", branchName, err)
	}
	return exists, nil
}

// EnsureBranch creates the phase branch if it does not already exist, or
// switches to it if it does. This method is idempotent and is the primary
// entry point for resume logic. It returns the resolved branch name.
func (b *BranchManager) EnsureBranch(ctx context.Context, opts PhaseBranchOpts) (string, error) {
	branchName := b.ResolveBranchName(opts.PhaseID, opts.PhaseName, opts.ProjectName)

	exists, err := b.gitClient.BranchExists(ctx, branchName)
	if err != nil {
		return "", fmt.Errorf("branch manager: ensure branch %q: %w", branchName, err)
	}

	if exists {
		if err := b.gitClient.Checkout(ctx, branchName); err != nil {
			return "", fmt.Errorf("branch manager: ensure branch %q: checkout: %w", branchName, err)
		}
		return branchName, nil
	}

	// Branch does not exist yet — create it.
	name, err := b.CreatePhaseBranch(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("branch manager: ensure branch: %w", err)
	}
	return name, nil
}

// --- internal helpers ---

// slugify converts an arbitrary string into a URL-safe kebab-case slug. It
// lowercases all input, replaces any sequence of non-alphanumeric characters
// with a single hyphen, and trims leading/trailing hyphens.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = nonAlphanumRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// logWarn writes a warning message via the attached logger. It is a no-op when
// no logger is configured.
func (b *BranchManager) logWarn(msg string, kvs ...any) {
	if b.logger == nil {
		return
	}
	b.logger.Warn(msg, kvs...)
}
