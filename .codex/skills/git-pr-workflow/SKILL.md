---
name: git-pr-workflow
description: Review branch diff, derive a descriptive commit message, run Go checks, then commit, push, and open a PR with gh.
---

# Git PR Workflow

Use this skill when the user asks to prepare changes for git, commit, push, and open a PR.

## Required Sequence

1. Confirm branch context and inspect diffs.
2. Draft a descriptive commit message from actual code changes.
3. Run Go verification checks before staging/committing.
4. Only if checks pass: stage, commit, push.
5. Create PR with `gh`.

Do not skip or reorder these steps.

## 1. Branch and Diff Review

Run:

```bash
git branch --show-current
git status --short
git diff --stat
git diff
git diff --staged
```

Rules:

- If branch is `main` or `master`, stop and ask the user for confirmation before committing.
- If there are no changes, stop and report no-op.
- Use the diff to understand what changed before writing commit/PR text.

## 2. Commit Message Construction

Create a commit message from the diff, not from guesswork.

Use Conventional Commits style:

- Title format: `<type>(<scope>): <summary>`
- Common types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`
- Summary: imperative, specific, <= 72 chars
- Optional body: short bullets describing key changes and rationale

Examples:

- `feat(discovery): implement parallel file walker with errgroup`
- `fix(config): prevent panic on missing TOML key`
- `refactor(pipeline): extract FileDescriptor to shared types`
- `test(security): add redaction golden tests for AWS keys`

## 3. Mandatory Pre-Commit Checks

Before `git add` or `git commit`, run:

```bash
./run_pipeline_checks.sh
```

If checks fail:

1. Attempt to fix the issue
2. Re-run checks

Rules:

- Never commit when required checks fail.
- The script runs the following checks in order (fail-fast):
  - `gofmt -l .` — formatting check
  - `go vet ./...` — vetting
  - `golangci-lint run` — linting (skipped gracefully if not installed)
  - `go mod tidy` + diff check — module hygiene
  - `go test -race -count=1 ./...` — testing
  - `go build ./cmd/raven/` — build (optional with `--with-build` flag)

## 4. Stage and Commit

Only after all checks pass:

```bash
git add <specific-files>
git commit -m "<title>" -m "<body>"
```

Prefer staging specific files over `git add -A`.

## 5. Push Branch

```bash
branch="$(git branch --show-current)"
git push --set-upstream origin "$branch" || git push origin "$branch"
```

## 6. Create PR

```bash
branch="$(git branch --show-current)"
base="$(gh repo view --json defaultBranchRef -q .defaultBranchRef.name)"
gh pr create --base "$base" --head "$branch" --title "<pr-title>" --body "<pr-body>"
```

PR body should include:

```markdown
## Summary
- [Key changes in bullets]

## Task Reference
- [T-XXX if applicable]

## Verification
- `go build` pass
- `go vet` pass
- `go test` pass

## Test Plan
- [How to verify the changes]
```

## Final Report Format

Return:

1. Branch name
2. Commit title used
3. Verification check results
4. Push result
5. PR URL
