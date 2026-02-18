# Raven Task Index

> **Total Tasks:** 89 across 8 phases | **Must Have:** 80 | **Should Have:** 9
>
> **Estimated Total Effort:** ~582-909 hours (~14 weeks at full pace)

This index organizes all implementation tasks for Raven -- a Go CLI tool that orchestrates AI-assisted software development workflows from PRD decomposition to implementation, review, fix, and PR creation.

---

## Quick Navigation

- [Phase 1: Foundation](#phase-1-foundation-t-001--t-015)
- [Phase 2: Task System & Agent Adapters](#phase-2-task-system--agent-adapters-t-016--t-030)
- [Phase 3: Review Pipeline](#phase-3-review-pipeline-t-031--t-042)
- [Phase 4: Workflow Engine & Pipeline](#phase-4-workflow-engine--pipeline-t-043--t-055)
- [Phase 5: PRD Decomposition](#phase-5-prd-decomposition-t-056--t-065)
- [Phase 6: TUI Command Center](#phase-6-tui-command-center-t-066--t-078)
- [Phase 7: Polish & Distribution](#phase-7-polish--distribution-t-079--t-087)

---

## Phase 1: Foundation (T-001 -- T-015)

> Go project scaffolding, CLI framework, configuration system, embedded templates, git client

| Task | Name | Priority | Effort | Dependencies |
|------|------|----------|--------|--------------|
| [T-001](T-001-go-project-init.md) | Go Project Initialization and Module Setup | Must Have | Medium (4-8hrs) | None |
| [T-002](T-002-makefile-setup.md) | Makefile with Build Targets and ldflags | Must Have | Small (2-4hrs) | T-001 |
| [T-003](T-003-buildinfo-package.md) | Build Info Package -- internal/buildinfo | Must Have | Small (1-2hrs) | T-001 |
| [T-004](T-004-central-data-types.md) | Central Data Types (WorkflowState, RunOpts, RunResult, Task, Phase) | Must Have | Medium (4-8hrs) | T-001 |
| [T-005](T-005-structured-logging.md) | Structured Logging with charmbracelet/log | Must Have | Small (2-4hrs) | T-001 |
| [T-006](T-006-cobra-root-command.md) | Cobra CLI Root Command and Global Flags | Must Have | Medium (4-8hrs) | T-001, T-005 |
| [T-007](T-007-version-command.md) | Version Command -- raven version | Must Have | Small (1-2hrs) | T-003, T-006 |
| [T-008](T-008-shell-completion.md) | Shell Completion Command -- raven completion | Must Have | Small (2-3hrs) | T-006 |
| [T-009](T-009-toml-config-loading.md) | TOML Configuration Types and Loading | Must Have | Medium (6-10hrs) | T-001 |
| [T-010](T-010-config-resolution.md) | Config Resolution -- CLI > env > file > defaults | Must Have | Medium (6-10hrs) | T-009 |
| [T-011](T-011-config-validation.md) | Configuration Validation and Unknown Key Detection | Must Have | Medium (4-6hrs) | T-009, T-010 |
| [T-012](T-012-config-debug-validate-commands.md) | Config Debug and Validate Commands | Must Have | Medium (4-6hrs) | T-006, T-009, T-010, T-011 |
| [T-013](T-013-embedded-project-templates.md) | Embedded Project Templates -- go-cli | Must Have | Medium (4-8hrs) | T-001 |
| [T-014](T-014-init-command.md) | Init Command -- raven init [template] | Must Have | Medium (4-6hrs) | T-006, T-013 |
| [T-015](T-015-git-client-wrapper.md) | Git Client Wrapper -- internal/git/client.go | Must Have | Medium (6-10hrs) | T-001 |

---

## Phase 2: Task System & Agent Adapters (T-016 -- T-030)

> Task parsing, state management, agent adapters, implementation loop engine

| Task | Name | Priority | Effort | Dependencies |
|------|------|----------|--------|--------------|
| [T-016](T-016-task-spec-parser.md) | Task Spec Markdown Parser | Must Have | Medium (6-10hrs) | T-004 |
| [T-017](T-017-task-state-management.md) | Task State Management (task-state.conf) | Must Have | Medium (8-12hrs) | T-004, T-016 |
| [T-018](T-018-phase-config-parser.md) | Phase Configuration Parser (phases.conf) | Must Have | Small (3-5hrs) | T-004 |
| [T-019](T-019-dependency-resolution.md) | Dependency Resolution & Next-Task Selection | Must Have | Medium (8-12hrs) | T-004, T-016, T-017, T-018 |
| [T-020](T-020-status-command.md) | Status Command -- raven status | Must Have | Medium (6-10hrs) | T-006, T-009, T-016, T-017, T-018, T-019 |
| [T-021](T-021-agent-interface-registry.md) | Agent Interface & Registry | Must Have | Medium (6-10hrs) | T-004, T-005, T-009 |
| [T-022](T-022-claude-agent-adapter.md) | Claude Agent Adapter | Must Have | Medium (8-12hrs) | T-021, T-025 |
| [T-023](T-023-codex-agent-adapter.md) | Codex Agent Adapter | Must Have | Medium (6-10hrs) | T-021, T-025 |
| [T-024](T-024-gemini-agent-stub.md) | Gemini Agent Stub | Should Have | Small (2-3hrs) | T-004, T-021 |
| [T-025](T-025-rate-limit-coordination.md) | Rate-Limit Detection & Coordination | Must Have | Medium (8-12hrs) | T-004, T-021 |
| [T-026](T-026-prompt-template-system.md) | Prompt Template System | Must Have | Medium (6-10hrs) | T-004, T-009, T-016, T-019 |
| [T-027](T-027-implementation-loop-runner.md) | Implementation Loop Runner | Must Have | Large (16-24hrs) | T-015, T-019, T-021, T-025, T-026 |
| [T-028](T-028-loop-recovery.md) | Loop Recovery (Rate-Limit Wait, Dirty-Tree) | Must Have | Medium (8-12hrs) | T-015, T-025, T-027 |
| [T-029](T-029-implement-cli-command.md) | Implementation CLI Command -- raven implement | Must Have | Medium (8-12hrs) | T-006, T-009, T-027, T-028 |
| [T-030](T-030-progress-file-generation.md) | Progress File Generation -- PROGRESS.md | Must Have | Small (4-6hrs) | T-004, T-016, T-017, T-018, T-019 |

---

## Phase 3: Review Pipeline (T-031 -- T-042)

> Multi-agent parallel review, diff generation, fix engine, PR creation

| Task | Name | Priority | Effort | Dependencies |
|------|------|----------|--------|--------------|
| [T-031](T-031-review-finding-types.md) | Review Finding Types and Schema | Must Have | Small (2-4hrs) | T-004 |
| [T-032](T-032-diff-generation-risk.md) | Git Diff Generation and Risk Classification | Must Have | Medium (6-10hrs) | T-009, T-015, T-031 |
| [T-033](T-033-review-prompt-synthesis.md) | Review Prompt Synthesis | Must Have | Medium (6-10hrs) | T-009, T-031, T-032 |
| [T-034](T-034-finding-consolidation.md) | Finding Consolidation and Deduplication | Must Have | Medium (6-10hrs) | T-031 |
| [T-035](T-035-review-orchestrator.md) | Multi-Agent Parallel Review Orchestrator | Must Have | Large (14-20hrs) | T-021, T-031, T-032, T-033, T-034, T-058 |
| [T-036](T-036-review-report-generation.md) | Review Report Generation (Markdown) | Must Have | Medium (6-10hrs) | T-031, T-034 |
| [T-037](T-037-verification-runner.md) | Verification Command Runner | Must Have | Medium (6-10hrs) | T-009 |
| [T-038](T-038-fix-engine.md) | Review Fix Engine | Must Have | Large (14-20hrs) | T-021, T-031, T-034, T-036, T-037 |
| [T-039](T-039-pr-body-generation.md) | PR Body Generation with AI Summary | Must Have | Medium (6-10hrs) | T-021, T-031, T-036, T-037 |
| [T-040](T-040-pr-creation-gh-cli.md) | PR Creation via gh CLI | Must Have | Medium (6-10hrs) | T-015, T-039 |
| [T-041](T-041-review-cli-command.md) | CLI Command -- raven review | Must Have | Medium (6-10hrs) | T-009, T-035 |
| [T-042](T-042-fix-pr-cli-commands.md) | CLI Commands -- raven fix and raven pr | Must Have | Medium (6-10hrs) | T-038, T-040, T-041 |

---

## Phase 4: Workflow Engine & Pipeline (T-043 -- T-055)

> State machine engine, checkpointing, resume, pipeline orchestrator, branch management

| Task | Name | Priority | Effort | Dependencies |
|------|------|----------|--------|--------------|
| [T-043](T-043-workflow-event-types.md) | Workflow Event Types and Constants | Must Have | Small (2-4hrs) | T-004 |
| [T-044](T-044-step-handler-registry.md) | Step Handler Registry | Must Have | Small (2-4hrs) | T-043 |
| [T-045](T-045-workflow-engine-core.md) | Workflow Engine Core -- State Machine Runner | Must Have | Large (14-20hrs) | T-004, T-043, T-044 |
| [T-046](T-046-workflow-state-checkpoint.md) | Workflow State Checkpointing and Persistence | Must Have | Medium (6-10hrs) | T-004, T-043, T-045 |
| [T-047](T-047-resume-command.md) | Resume Command -- List and Resume Interrupted Workflows | Must Have | Medium (6-10hrs) | T-006, T-045, T-046 |
| [T-048](T-048-workflow-definition-validation.md) | Workflow Definition Validation | Must Have | Medium (6-10hrs) | T-009, T-043, T-044 |
| [T-049](T-049-builtin-workflow-definitions.md) | Built-in Workflow Definitions and Step Handlers | Must Have | Large (14-20hrs) | T-027, T-029, T-043, T-044, T-045, T-048 |
| [T-050](T-050-pipeline-orchestrator-core.md) | Pipeline Orchestrator Core -- Multi-Phase Lifecycle | Must Have | Large (14-20hrs) | T-015, T-045, T-046, T-049 |
| [T-051](T-051-pipeline-branch-management.md) | Pipeline Branch Management | Must Have | Medium (6-10hrs) | T-009, T-015, T-050 |
| [T-052](T-052-pipeline-metadata-tracking.md) | Pipeline Metadata Tracking | Must Have | Small (2-4hrs) | T-050 |
| [T-053](T-053-pipeline-wizard.md) | Pipeline Interactive Wizard | Should Have | Medium (6-10hrs) | T-009, T-050 |
| [T-054](T-054-pipeline-dryrun.md) | Pipeline and Workflow Dry-Run Mode | Must Have | Medium (6-10hrs) | T-045, T-049, T-050 |
| [T-055](T-055-pipeline-command.md) | Pipeline CLI Command | Must Have | Medium (6-10hrs) | T-006, T-050, T-051, T-052, T-053, T-054 |

---

## Phase 5: PRD Decomposition (T-056 -- T-065)

> PRD shredding, parallel epic workers, merge pipeline, task file generation

| Task | Name | Priority | Effort | Dependencies |
|------|------|----------|--------|--------------|
| [T-056](T-056-epic-json-schema.md) | Epic JSON Schema and Types | Must Have | Small (2-4hrs) | T-004 |
| [T-057](T-057-prd-shredder.md) | PRD Shredder (Single Agent -> Epic JSON) | Must Have | Medium (8-12hrs) | T-021, T-056 |
| [T-058](T-058-json-extraction-utility.md) | JSON Extraction Utility | Must Have | Medium (6-10hrs) | T-056 |
| [T-059](T-059-parallel-epic-workers.md) | Parallel Epic Workers | Must Have | Medium (8-12hrs) | T-021, T-056, T-057, T-058 |
| [T-060](T-060-merge-global-id-assignment.md) | Merge -- Global ID Assignment | Must Have | Medium (6-10hrs) | T-056 |
| [T-061](T-061-merge-dependency-remapping.md) | Merge -- Dependency Remapping | Must Have | Medium (6-10hrs) | T-056, T-060 |
| [T-062](T-062-merge-title-deduplication.md) | Merge -- Title Deduplication | Must Have | Medium (6-10hrs) | T-056 |
| [T-063](T-063-merge-dag-validation.md) | Merge -- DAG Validation | Must Have | Medium (6-10hrs) | T-056, T-060, T-061, T-062 |
| [T-064](T-064-task-file-emitter.md) | Task File Emitter | Must Have | Medium (8-12hrs) | T-056, T-059, T-060, T-061, T-062, T-063 |
| [T-065](T-065-prd-cli-command.md) | PRD CLI Command -- raven prd | Must Have | Medium (8-12hrs) | T-006, T-009, T-057, T-059, T-064 |

---

## Phase 6: TUI Command Center (T-066 -- T-078, T-089)

> Bubble Tea dashboard, split-pane layout, agent monitoring, keyboard navigation

| Task | Name | Priority | Effort | Dependencies |
|------|------|----------|--------|--------------|
| [T-089](T-089-stream-integration.md) | Stream-JSON Integration -- Wire into Adapters & Loop | Must Have | Medium (8-12hrs) | T-022, T-027, T-088 |
| [T-066](T-066-bubbletea-app-scaffold.md) | Bubble Tea Application Scaffold and Elm Architecture | Must Have | Medium (8-12hrs) | T-004 |
| [T-067](T-067-tui-message-types.md) | TUI Message Types and Event System | Must Have | Medium (6-10hrs) | T-004, T-066, T-089 |
| [T-068](T-068-lipgloss-styles-theme.md) | Lipgloss Styles and Theme System | Must Have | Medium (6-8hrs) | T-066 |
| [T-069](T-069-split-pane-layout.md) | Split-Pane Layout Manager | Must Have | Medium (8-12hrs) | T-066, T-068 |
| [T-070](T-070-sidebar-workflow-list.md) | Sidebar -- Workflow List with Status Indicators | Must Have | Medium (6-8hrs) | T-067, T-068, T-069 |
| [T-071](T-071-sidebar-task-progress.md) | Sidebar -- Task Progress Bars and Phase Progress | Must Have | Medium (6-8hrs) | T-067, T-068, T-069, T-070 |
| [T-072](T-072-sidebar-rate-limit-status.md) | Sidebar -- Rate-Limit Status Display with Countdown | Must Have | Medium (6-8hrs) | T-067, T-068, T-069, T-070 |
| [T-073](T-073-agent-output-panel.md) | Agent Output Panel with Viewport and Tabbed View | Must Have | Large (16-24hrs) | T-066, T-067, T-068, T-069, T-089 |
| [T-074](T-074-event-log-panel.md) | Event Log Panel for Workflow Milestones | Must Have | Medium (6-10hrs) | T-066, T-067, T-068, T-069 |
| [T-075](T-075-status-bar.md) | Status Bar with Current State, Iteration, and Timer | Must Have | Small (4-6hrs) | T-066, T-067, T-068, T-069 |
| [T-076](T-076-keyboard-navigation-help.md) | Keyboard Navigation and Help Overlay | Must Have | Medium (8-12hrs) | T-066, T-067, T-069, T-070, T-073, T-074 |
| [T-077](T-077-pipeline-wizard-tui.md) | Pipeline Wizard TUI Integration (huh) | Should Have | Medium (8-12hrs) | T-066, T-068, T-069, T-076 |
| [T-078](T-078-dashboard-command-integration.md) | Raven Dashboard Command and TUI Integration Testing | Must Have | Large (16-24hrs) | T-006, T-066-T-077 |

---

## Phase 7: Polish & Distribution (T-079 -- T-087)

> Cross-platform builds, CI/CD, testing infrastructure, release automation

| Task | Name | Priority | Effort | Dependencies |
|------|------|----------|--------|--------------|
| [T-079](T-079-goreleaser-cross-platform.md) | GoReleaser Configuration for Cross-Platform Builds | Must Have | Medium (6-10hrs) | T-002, T-003 |
| [T-080](T-080-github-actions-release.md) | GitHub Actions Release Automation Workflow | Must Have | Medium (6-10hrs) | T-079 |
| [T-081](T-081-shell-completion-scripts.md) | Shell Completion Installation Scripts and Packaging | Should Have | Small (3-4hrs) | T-008 |
| [T-082](T-082-man-page-generation.md) | Man Page Generation Using cobra/doc | Should Have | Small (2-4hrs) | T-006 |
| [T-083](T-083-performance-benchmarks.md) | Performance Benchmarking Suite | Should Have | Medium (8-12hrs) | T-015, T-027 |
| [T-084](T-084-e2e-integration-tests.md) | End-to-End Integration Test Suite with Mock Agents | Must Have | Large (20-30hrs) | T-029, T-042, T-055, T-065 |
| [T-085](T-085-ci-cd-pipeline.md) | CI/CD Pipeline with GitHub Actions | Must Have | Medium (6-10hrs) | T-001 |
| [T-086](T-086-readme-documentation.md) | Comprehensive README and User Documentation | Must Have | Medium (8-12hrs) | T-006 |
| [T-087](T-087-release-verification.md) | Final Binary Verification and Release Checklist | Must Have | Medium (6-8hrs) | T-079, T-080, T-084, T-085 |

---

## Phase 8: Headless Observability (T-088)

> Stream-JSON event parsing for real-time agent observability in headless/automation mode

| Task | Name | Priority | Effort | Dependencies |
|------|------|----------|--------|--------------|
| [T-088](T-088-headless-observability.md) | Headless Observability -- Stream-JSON Event Parsing | Must Have | Medium (8-12hrs) | T-004, T-021 |

---

## Effort Summary

| Effort Level | Count | Hours Range |
|-------------|-------|-------------|
| Small (1-6hrs) | 15 | 30-64 hrs |
| Medium (4-12hrs) | 60 | 374-594 hrs |
| Large (14-30hrs) | 14 | 186-262 hrs |
| **Total** | **89** | **~590-920 hrs** |

---

## Priority Summary

| Priority | Count |
|----------|-------|
| Must Have | 80 |
| Should Have | 9 |

---

## PRD Section Mapping

| PRD Section | Tasks |
|-------------|-------|
| 5.1 Generic Workflow Engine | T-043, T-044, T-045, T-046, T-047, T-048, T-049 |
| 5.2 Agent Adapter System | T-021, T-022, T-023, T-024, T-025, T-088, T-089 |
| 5.3 Task Management System | T-016, T-017, T-018, T-019, T-020, T-030 |
| 5.4 Implementation Loop Engine | T-026, T-027, T-028, T-029 |
| 5.5 Multi-Agent Review Pipeline | T-031, T-032, T-033, T-034, T-035, T-036, T-041 |
| 5.6 Review Fix Engine | T-037, T-038 |
| 5.7 PR Creation | T-039, T-040, T-042 |
| 5.8 PRD Decomposition Pipeline | T-056, T-057, T-058, T-059, T-060, T-061, T-062, T-063, T-064, T-065 |
| 5.9 Phase Pipeline Orchestrator | T-050, T-051, T-052, T-053, T-054, T-055 |
| 5.10 Configuration System | T-009, T-010, T-011, T-012 |
| 5.11 CLI Interface | T-006, T-007, T-008, T-014, T-020, T-029, T-041, T-042, T-047, T-055, T-065 |
| 5.12 Interactive TUI Dashboard | T-066, T-067, T-068, T-069, T-070, T-071, T-072, T-073, T-074, T-075, T-076, T-077, T-078, T-089 |
| 5.13 Git Integration | T-015 |
| 6.x Architecture | T-001, T-002, T-003, T-004, T-005, T-013 |
| 7.x Development Roadmap | All phases |
| 9.x Testing & Quality | T-083, T-084 |
| 10.x Distribution | T-079, T-080, T-081, T-082, T-085, T-086, T-087 |

---

## Development Phase Dependency Graph

```
Phase 1: Foundation (Weeks 1-2)
  T-001 -> T-002, T-003, T-004, T-005, T-009, T-013, T-015
  T-005 -> T-006
  T-006 -> T-007, T-008, T-012, T-014
  T-003 -> T-007
  T-009 -> T-010 -> T-011 -> T-012
  T-013 -> T-014

Phase 2: Task System & Agent Adapters (Weeks 3-4)
  T-004 -> T-016, T-018, T-021, T-025
  T-016 -> T-017 -> T-019 -> T-026 -> T-027 -> T-028 -> T-029
  T-021 -> T-022, T-023, T-024, T-025
  T-019 -> T-020, T-030

Phase 3: Review Pipeline (Weeks 5-6)
  T-031 -> T-032, T-033, T-034, T-036
  T-032 -> T-033 -> T-035
  T-034 -> T-035, T-036, T-038
  T-037 -> T-038, T-039
  T-039 -> T-040 -> T-042
  T-038 -> T-042

Phase 4: Workflow Engine & Pipeline (Weeks 7-8)
  T-043 -> T-044 -> T-045 -> T-046, T-049
  T-045 -> T-050 -> T-051, T-052, T-053, T-054
  T-049 -> T-050
  T-051 + T-052 + T-053 + T-054 -> T-055

Phase 5: PRD Decomposition (Weeks 9-10)
  T-056 -> T-057, T-058, T-060, T-062
  T-057 + T-058 -> T-059
  T-060 -> T-061 -> T-063
  T-059 + T-063 -> T-064 -> T-065

Phase 6: TUI Command Center (Weeks 11-13)
  T-089 (first -- blocks T-067, T-073; depends on T-088 from Phase 8)
  T-066 -> T-067, T-068
  T-089 -> T-067, T-073
  T-068 -> T-069
  T-069 -> T-070, T-073, T-074, T-075
  T-070 -> T-071, T-072, T-076
  T-076 -> T-077, T-078

Phase 7: Polish & Distribution (Week 14)
  T-079 -> T-080 -> T-087
  T-084, T-085 -> T-087
  T-081, T-082, T-083, T-086 (parallel)

Phase 8: Headless Observability (T-088 only -- completed)
  T-088 (decoder/types, consumed by T-089 in Phase 6)
```

---

_Last updated: 2026-02-17_
