# Release Checklist

This checklist guides the release engineer through every step required to
publish a new Raven version.  Work through each section top-to-bottom.
Check every item before proceeding to the next section.

---

## Pre-Release Verification

### Automated Checks

- [ ] All CI checks pass on the `main` branch (green status badge)
- [ ] `make release-verify VERSION=vX.Y.Z` exits with code 0 and prints
      `ALL CHECKS PASSED`
- [ ] GoReleaser snapshot build (`goreleaser build --snapshot --clean`)
      produces all five platform archives without errors

### Binary Verification

- [ ] All five platform binaries are under 25 MB (enforced by `release-verify`)
- [ ] `raven version` shows the expected version string on at least one platform
- [ ] `raven --help` displays all top-level commands without errors
- [ ] `CGO_ENABLED=0` confirmed -- `ldd ./dist/raven` (Linux) or
      `otool -L ./dist/raven` (macOS) shows no external shared libraries

### Functional Verification (Manual)

Run these commands against a locally built binary and confirm expected output:

- [ ] `raven init go-cli` creates a valid project structure in a temp directory
- [ ] `raven config debug` shows the resolved configuration with source annotations
- [ ] `raven implement --task T-001 --dry-run` prints the prompt that would be
      sent to the agent without actually invoking it
- [ ] `raven review --dry-run` shows the review plan (agents, tasks, parallelism)
- [ ] `raven pipeline --phase 1 --dry-run` lists the pipeline steps in order
- [ ] `raven dashboard` launches the TUI without errors; press `q` to exit
- [ ] Shell completions work: `source completions/raven.bash && raven <TAB>`
- [ ] Man pages display correctly: `man ./man/man1/raven.1`

### Documentation Verification

- [ ] `README.md` is complete, accurate, and matches the release version
- [ ] All command examples in the README execute successfully
- [ ] Installation instructions are correct for Homebrew, direct download, and
      `go install`
- [ ] Quick Start guide produces the expected output for a new user
- [ ] `CONTRIBUTING.md` has up-to-date development setup instructions
- [ ] `LICENSE` file is present (MIT)
- [ ] Configuration reference reflects any new or changed config keys
- [ ] Workflows reference reflects the current built-in workflow definitions
- [ ] Agents reference reflects the current agent adapter capabilities

### Performance Verification

- [ ] `make bench` completes successfully and results are within acceptable ranges
- [ ] Binary startup time is under 200 ms
  (`time raven version` -- wall time minus prompt rendering)
- [ ] TUI frame render time is under 100 ms at 120x40 terminal size
  (verified by benchmark suite in `internal/tui/`)

---

## Release Process

Follow these steps in order.  Do not proceed if any step fails.

1. **Verify CI is green**
   - Open the repository on GitHub.
   - Confirm the latest commit on `main` has a green checkmark on all CI jobs.

2. **Run the verification script**
   ```bash
   make release-verify VERSION=vX.Y.Z
   ```
   All checks must pass before continuing.

3. **Create an annotated tag**
   ```bash
   git tag -a vX.Y.Z -m "Release vX.Y.Z"
   ```
   The tag message should be a brief summary of the release highlights.
   For a pre-release suffix use `vX.Y.Z-rc.1` etc.

4. **Push the tag to GitHub**
   ```bash
   git push origin vX.Y.Z
   ```
   Alternatively, use the Makefile convenience target (includes a confirmation
   prompt):
   ```bash
   make release VERSION=vX.Y.Z
   ```

5. **Wait for the GitHub Actions release workflow**
   - Navigate to the repository's **Actions** tab.
   - Watch the `release` workflow triggered by the tag push.
   - The workflow builds all platform binaries, generates checksums, and
     creates the GitHub Release with all assets attached.
   - Typical duration: 5-10 minutes.

6. **Verify the GitHub Release page**
   - [ ] Release title matches the tag (`vX.Y.Z`)
   - [ ] Release notes are accurate and complete
   - [ ] All five platform archives are present:
     - `raven_vX.Y.Z_darwin_amd64.tar.gz`
     - `raven_vX.Y.Z_darwin_arm64.tar.gz`
     - `raven_vX.Y.Z_linux_amd64.tar.gz`
     - `raven_vX.Y.Z_linux_arm64.tar.gz`
     - `raven_vX.Y.Z_windows_amd64.zip`
   - [ ] `raven_vX.Y.Z_checksums.txt` is present
   - [ ] Shell completions archive is present

7. **Download and verify a release binary**
   ```bash
   # Example for macOS arm64
   curl -L https://github.com/AbdelazizMoustafa10m/Raven/releases/download/vX.Y.Z/raven_vX.Y.Z_darwin_arm64.tar.gz \
       | tar -xz
   ./raven version
   ```
   - [ ] Binary executes without errors
   - [ ] `raven version` output shows `vX.Y.Z`
   - [ ] Checksum matches the value in `raven_vX.Y.Z_checksums.txt`

8. **Update any external references**
   - [ ] Homebrew formula (if applicable) points to the new release URL and checksum
   - [ ] Project website / landing page version badge updated
   - [ ] Any pinned Docker image tags updated

---

## Post-Release

- [ ] Verify `go install` works with the published tag:
  ```bash
  go install github.com/AbdelazizMoustafa10m/Raven/cmd/raven@vX.Y.Z
  raven version
  ```
- [ ] Create a GitHub Discussion or announcement post describing the release
      highlights, breaking changes (if any), and upgrade instructions
- [ ] Update the project roadmap (`docs/tasks/PROGRESS.md`) to reflect the
      released version and outline the scope of the next version
- [ ] Close or milestone-move any GitHub Issues addressed by this release
- [ ] Tag the release in any external tracking systems (Jira, Linear, etc.)

---

## Sign-Off

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Release Engineer | | | |
| Reviewer | | | |

Release `vX.Y.Z` is approved for publication: **YES / NO**
