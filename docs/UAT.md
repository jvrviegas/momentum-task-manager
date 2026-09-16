# Momentum UAT

Date: 2026-09-16  
Host: macOS arm64 (`Darwin Mac.lan`), Go 1.27.1, Taskwarrior 3.5.0  
Linux runtime: Debian sid arm64 container, Go 1.27.1, Taskwarrior 3.5.0 (baseline v1 UAT)  
Scope: local-only, isolated Taskwarrior workflows, and Settings → Projects; no sync server was configured.

## Automated gates

| Gate | Result | Evidence |
|---|---|---|
| Format | PASS | `test -z "$(gofmt -l .)"` |
| Unit | PASS | `go test ./... -count=1`: 8 packages, 377 top-level tests / 423 passing actions, 0 skips |
| Vet | PASS | `go vet ./...` |
| Build | PASS | `go build ./cmd/momentum` |
| Race | PASS | `go test -race ./...` |
| Linux amd64 build | PASS | `GOOS=linux GOARCH=amd64 go build ./cmd/momentum` |
| macOS arm64 build | PASS | `GOOS=darwin GOARCH=arm64 go build ./cmd/momentum` |
| Linux full/race gates | PASS | Prior Debian sid arm64 container UAT: format, unit, vet, build, race |
| Real Taskwarrior integration (macOS) | PASS | `go test ./internal/taskwarrior -run Integration -v -count=1`: 11 isolated scenarios |
| Real Taskwarrior integration (Linux) | PASS | Prior Debian sid arm64 container run: 8 isolated scenarios |
| Final working-tree diff check | PASS | `git diff --check` after the documentation commit |

## Runtime workflows

All real Taskwarrior checks create a temporary `TASKRC` and `TASKDATA` directory under `t.TempDir()`. The integration guard rejects `~/.taskrc` and `~/.task` before any command is started.

| Workflow | Result | Evidence |
|---|---|---|
| Empty local-only launch and quit (macOS) | PASS | Prior temporary Taskwarrior data, `[sync].enabled = false`, expect smoke run |
| Quick add with project and tag (macOS) | PASS | Prior temporary TUI run and isolated export verification |
| Empty/local-only launch and quick add (Linux) | PASS | Prior Debian sid arm64 container, actual Taskwarrior 3.5.0, temporary data, expect TTY run |
| Linux edit/search/details/start-stop/delete-confirm/quit routing | PASS | Prior Debian sid arm64 container, expect input sequence and isolated export verification |
| Add/export/UUID edit/clear | PASS | `TestIntegrationAddAndExport`, `TestIntegrationModifyByUUIDAndClearFields` |
| Complete/start/stop/delete/undo | PASS | `TestIntegrationStartStopDoneLifecycle`, `TestIntegrationDeleteAndUndo` |
| Search and match count | PASS | `internal/ui/search_test.go`, `internal/app/composition_test.go` |
| Offline/retry/undo/shutdown state behavior | PASS | `internal/app/sync_state_test.go`, `internal/app/sync_test.go` |
| Wide terminal (120 columns) | PASS | Responsive composition/component tests |
| Compact terminal (79 columns) | PASS | Responsive composition/component tests |
| Narrow terminal (49 columns) | PASS | Metadata suppression and Settings/rename bounds tests |
| Minimum terminal (20x5 / 28x8) | PASS | Minimum-size and Settings/rename bounds tests |
| Unicode/Nerd Font/ASCII icon modes | PASS automated | `internal/ui/theme_test.go`; live font inspection remains maintainer work |

## Settings → Projects acceptance evidence

| Requirement | Evidence | Result |
|---|---|---|
| SP-01 | `TestSettingsRouteIsExplicitAndCannotMutateHiddenTask`, `TestSettingsMouseNavigationUsesActualCompactTabBoundary`; `3` keyboard route and three-entry sidebar/tabs | PASS automated; live visual check pending |
| SP-02 | `TestProjectSettingsRendersConfiguredHierarchyAndEmptyState`, long-catalog visibility, immediate catalog save workflow | PASS automated; live visual check pending |
| SP-03 | Domain draft composition tests and `TestValueEditCatalogOnlySavesAfterExplicitPreviewConfirmation` | PASS automated |
| SP-04 | Domain duplicate/cycle validation and `TestProjectSettingsRejectsInvalidHierarchyAndKeepsDraft` | PASS automated |
| SP-05 | `TestPlanRemoveProjectHonorsSubtreeChoiceWithoutMutatingInput`, `TestProjectSettingsRemoveAsksAboutConfiguredChildren` | PASS automated |
| SP-06 | Config snapshot/save fixtures plus async app save success/failure and separate discovery tests | PASS automated |
| SP-07 | Label-only plans have no task mappings; label-only Settings workflow remains catalog-only | PASS automated |
| SP-08 | Rename preview defaults to Catalog only; pending mode requires option and confirmation; mode resets on each preview | PASS automated |
| SP-09 | `TestIntegrationProjectMigrationHonorsActiveContextWithoutChangingIt`; guarded pending export/modify integration | PASS automated |
| SP-10 | Exact/boundary/descendant domain tests; separate catalog/task counts; isolated migration with `workshop` lookalike | PASS automated |
| SP-11 | Preview context/count/old→new/merge/history/undo text; coordinator catalog-first tests | PASS automated |
| SP-12 | Exact-set preflight, stale completion, stale preview, and explicit retry tests | PASS automated |
| SP-13 | Comment/newline/mode/symlink/conflict/read-only config-store tests; no environment-derived serialization | PASS automated |
| SP-14 | Typed async commands/results; migration gate blocks writes/sync/undo and releases after refresh | PASS automated |
| SP-15 | Catalog-first failure/partial/ambiguous/zero-match tests; migration undo suppression and ordinary undo restoration | PASS automated |
| SP-16 | Temporary config/TASKDATA guards, hook rejection, invalid/read-only/conflict coverage, race gate | PASS automated |

## Settings test counts and named evidence

The pre-feature baseline at `b5c567a` was 8 packages, 285 top-level tests / 308 passing actions, and 0 skips. The final repository has 8 packages, 377 top-level tests / 423 passing actions, and 0 skips: a delta of +92 top-level tests / +115 passing actions. The final command output used `-count=1`; the race gate also passed.

Key added suites:

- Domain: `TestComposeProjectValueAndDraftResolution`, `TestPlanUpdateProjectClassifiesChangesAndMapsConfiguredDescendants`, `TestProjectCatalogPlanMapsOnlyEligiblePendingTasks`, and collision/removal/no-match cases.
- Config: `TestProjectCatalogStorePreservesUnrelatedTOMLAndArrayTableComments`, `TestProjectCatalogStorePreservesCRLFForNewArrayTables`, external-edit/read-only/symlink cases, and missing/array/array-table round trips.
- UI: Project Settings and rename preview interaction/rendering tests at supported widths.
- App: Settings routing/save/discovery tests, coordinator fault injection, migration gate/sync/undo lifecycle, and end-to-end Catalog-only/pending/stale/partial/zero-match workflows.
- Taskwarrior: 45 adapter tests and 11 isolated `Integration` tests passed against Taskwarrior 3.5.0, including active-context, pending exact/descendant, hook rejection, and stale-completion cases.

## Acceptance evidence for the original v1 surface

| # | Criterion | Result |
|---:|---|---|
| 1 | Taskwarrior 3.x launch | PASS on macOS and prior Linux runtime |
| 2 | Inbox/Today overlapping semantics | PASS — domain table tests |
| 3 | Today grouping and no duplicates | PASS — domain and composition tests |
| 4 | Safe quick add and five triggers | PASS — parser, adapter, and runtime smoke tests |
| 5 | Structured edit and unsupported-field preservation | PASS — domain/UI/app/integration tests |
| 6 | UUID mutations and undo | PASS — exact argv and isolated integration tests |
| 7 | Search, navigation, mouse, details, help, responsive layouts | PASS — app/UI tests |
| 8 | Async refresh/sync without blocking Update | PASS — typed command/update-loop tests |
| 9 | Offline/retry/undo-delay/unsynced quit | PASS — pure sync state and app flow tests |
| 10 | Local-only mode | PASS — config, doctor, and runtime smoke tests |
| 11 | No direct Taskwarrior database access | PASS — adapter uses `exec.CommandContext`; isolation guard |
| 12 | Secret-safe diagnostics/logging | PASS — redaction and doctor tests |
| 13 | Isolated automated tests | PASS — full and race gates; integration guard |
| 14 | Installation/configuration/control/sync documentation | PASS — README, CONTRIBUTING, config example, and sync guide |

## Manual Settings UAT status

Automated input and rendering tests cover wide (120), compact (79), narrow (49), and minimum (28) layouts, long catalogs, empty states, prompts, and input routing. A human-driven live terminal session for the new Settings flow on macOS/Linux was not completed in this non-interactive coding-agent harness; visual spacing, actual terminal font rendering, and hands-on restart/keyboard confirmation remain to be checked by a maintainer. No automated result is represented as a substitute for that visual inspection.

Suggested maintainer smoke pass:

1. Launch with a temporary `--config` and temporary `TASKRC`/`TASKDATA`; press `3` and inspect Settings → Projects at wide/compact/narrow widths.
2. Add, reparent, rename-label, remove (No/Yes/Cancel), save, quit/restart, and verify quick-add/editor suggestions immediately and after restart.
3. Seed isolated pending, completed, deleted, waiting, recurring-template/instance, child, and prefix-lookalike tasks. Preview a value rename with descendants off/on; verify only confirmed active-context pending tasks change and historical values/unrelated fields remain.
4. Exercise catalog-save failure, hook rejection, stale preview/retry, partial task failure, task-only destination warning, no-active-context, and disabled-sync states.
5. Repeat with Unicode, ASCII, and Nerd icon modes and inspect the final status/undo wording.

## Reproduction notes

The prior Linux checks ran in an ephemeral Debian sid arm64 container. The production Taskwarrior database was never mounted; each run created a disposable `TASKRC`/`TASKDATA` pair. The amd64 artifact was compiled separately as the release target; runtime UAT was executed on Linux arm64 because that was the available container architecture. Sync-network behavior is covered with pure state tests because no self-hosted sync server is provisioned for local UAT.

## Overall status

Automated implementation and safety gates pass. Release readiness still requires the maintainer’s live visual/keyboard UAT for the new Settings flow; this document deliberately records that limitation instead of claiming an interactive pass that was not observed.
