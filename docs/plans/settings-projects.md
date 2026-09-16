# Implementation plan — Settings → Projects and pending-only renames

**Created:** 2026-09-16  
**Status:** T10 complete; T11 next. All product policies are recorded and implementation is proceeding sequentially.  
**Baseline:** `b5c567a` on `main` (catalog: `d8c1728`; quick capture: `84bb513`). Recheck HEAD and working tree before starting.  
**Requirements:** [SP-01–SP-16](../spec/settings-projects.md)  
**Decision record:** [ADR 0001 / O1–O6](../adr/0001-project-settings-and-pending-task-renames.md)

## Next-agent instructions

1. Read the specification and ADR, then this plan, `.notebook/INDEX.md`, `CONTRIBUTING.md`, and relevant source/tests.
2. Follow the recorded O1–O6 choices, including resolved O2-R/O3-T, without reopening them. T00 now focuses on technical feasibility checks and documenting any implementation constraint that conflicts with an accepted policy.
3. Work through the checklist. Mark a task complete only after its co-located tests/gate pass; record a commit and concrete evidence below. Keep each implementation commit focused and update this tracker as work advances.
4. Use existing pure domain functions, Bubble Tea typed messages/async commands, injected Taskwarrior runner, and isolated test patterns. No shell-built task commands, direct Taskwarrior file edits, or production data migrations during development.
5. Keep changes scoped. The user's 16 project entries are already in local configuration; do not hard-code them or commit the local config. Do not use `docs/issues-from-linear.csv` as test data or automatically import it.
6. Each task should be an independently verified commit. Do not push or publish without instruction. Leave unrelated changes alone.

**Suggested skills:** `codenavi` for repository investigation, `tlc-spec-driven` for task execution/verification, `diagnose` only for failures, `handoff` when transferring unfinished work. No Linear issue or external tracker was requested: this checklist is the tracking artifact.

## Existing implementation map (verified at planning time)

| Area | Source / evidence | Implication |
|---|---|---|
| Project entity | `internal/domain/project.go`: `Project`, `ProjectCatalog.Validate/Merge/Labels/Values` | Flat entries; dotted values encode hierarchy. No separate parent field or stable ID is needed. Existing tests: `project_test.go`. |
| Config load | `internal/config/config.go`: `LoadWithOptions`, `ResolvePathFor`, strict `decode` | Loader returns resolved path; no save API exists. `Config` contains a slice; use `IsZero`, not direct struct equality. |
| CLI wiring | `cmd/momentum/main.go`: `run` calls `LoadWithOptions` | `resolvedPath` reaches diagnostics/logging but is not passed to `app.NewModel`. Pass the actual path into an injected persistence dependency. |
| Catalog suggestions | `internal/app/model.go`: `NewModel`, `Update(ProjectsMsg/TagsMsg)`; `internal/ui/quickadd.go` / `edit.go`: `SetProjectCatalog` | Both overlays are seeded before discovery. Preserve labels across tag updates and preserve configured values when discovery fails. Track discovered values separately if required for safe live re-merge after removal; do not re-merge against a list already containing the old catalog. |
| Fixed two-view routing | `internal/app/model.go`: `normalizeView`, `tasksFor`, `SwitchView`, `updateKey`; `view.go`: `renderBase`; `update.go`: `handleMouse` | Unknown views currently normalize to Inbox; mouse tabs split into two halves. Adding Settings needs explicit non-task routing, not just a new enum/nav label. |
| Navigation components | `internal/ui/sidebar.go`: `NavItem`, `RenderSidebar`, `RenderTabs`; `help.go` | Every nav item currently renders a task count. Settings should not misleadingly display a task count. Keep keyboard access at narrow widths. |
| Task discovery | `internal/taskwarrior/client.go`: `ExportPending`, `Projects` | Taskwarrior 3.5.0 deliberately leaves machine-readable `export` unencumbered by the active context, so the existing `ExportPending` call does not prove O1. Migration needs a dedicated export with the active `rc.context.<name>.read` filter supplied explicitly. Do not use UI Today/search subsets or a global override. |
| Task mutation adapter | `internal/taskwarrior/mutations.go`: `Modify/ModifyArgs`, `Undo`; `client.go`: `run`, `Runner` | Existing modification targets a UUID without an explicit pending/old-value guard. Migration needs one UUID command guarded by `status:pending recur.none: project.is:<old>` plus the captured context filter, with post-command reconciliation. |
| Mutation orchestration | `internal/app/model.go`: `beginMutation`, `applyMutation`; `commands.go`, `messages.go` | One mutation in flight; each success enters sync/undo bookkeeping and refreshes. Do not call this per task and claim it is an atomic batch. |
| Sync/undo | `internal/app/sync.go`, `sync_state.go`, `actions.go`: `UndoLast`, `beginSync`, `requestQuit` | Sync has independent timers. Gate all sync paths while a migration owns the task-write gate, not only keyboard sync. Native undo cannot be assumed to reverse multiple modifies or TOML. |
| Integration safety | `internal/taskwarrior/integration_test.go`: `newIsolatedEnvironment` | Temporary TASKRC/TASKDATA, production-path rejection, no `t.Parallel` for process environment tests. Do not bypass this guard. |

## Proposed technical boundaries

These are implementation guidance, not already-existing APIs. Keep names consistent with repository conventions.

- **Domain:** pure catalog edit/subtree planning and pure task-migration mapping. Inputs are snapshots; outputs contain proposed catalog and explicit UUID/old/new task mappings. Separate display-name changes from stored-value changes.
- **Config store:** source-aware read/patch/save of the active file, with optimistic conflict detection and atomic replacement. Preserve unrelated text; a whole-file TOML encode is not sufficient for the comment-preservation requirement. Choose/verify a parser or safe source-edit approach before implementing it; do not regex-match arbitrary TOML table boundaries.
- **UI:** a Projects settings component that owns drafts/validation/help, plus an explicit rename preview/confirmation component. Components emit intents and render state; no subprocesses or filesystem writes inside render/update code.
- **App:** owns screen routing, injected dependencies, saved catalog, discovered project snapshot, operation state, and typed command results. Apply live catalog changes only after confirmed persistence results; ignore stale async results using operation identity as needed.
- **Taskwarrior adapter:** read the active context name and its read filter without changing it; pass a parenthesized copy of that filter explicitly to the migration export and each guarded UUID assignment. Guard assignments with `status:pending recur.none: project.is:<old>`, preserve hooks, and reconcile by UUID after every success or ambiguous result. Exit code alone is not proof that exactly one eligible task changed.
- **Migration coordinator:** preview → explicit confirmation → revalidation → chosen cross-store execution → result/reconciliation. Never expand the confirmed UUID set during execution. Report config outcome and task successes/skips/failures separately; no automatic retry or rollback.

**Confirmed policies:** active Taskwarrior context only; ask whether to remove configured children when deleting a parent; reject duplicate catalog values; preflight then save catalog first and migrate tasks with explicit partial results/retry; no batch undo; preserve symlinks and safely update their targets. These were selected by the user on 2026-09-16. No global override, catalog merge, automatic rollback, or blanket refusal of symlink saving should be introduced.

**Completed follow-ups:** O2-R and O3-T are accepted: Include subprojects covers configured descendants and eligible pending-task descendants as applicable, with separate preview counts; task-only destinations are allowed after explicit effective-merge warning/confirmation.

**T00 gate:** Technical feasibility is recorded below. Product decisions remain fixed; implementation must honor the explicit context-export and source-preserving-save constraints.

## T00 feasibility record — 2026-09-16

### Taskwarrior 3.5.0

All commands below used a fresh temporary `TASKRC` with `data.location` under a fresh temporary directory; no user task data was read or changed.

- `task --version` reported `3.5.0` on Darwin/arm64.
- `task status:pending recur.none: export` selected ordinary pending tasks, excluded a future-`wait` task (which exports with raw `status:pending` but is currently `WAITING`), and excluded both a recurring template (`status:recurring`) and its pending recurring instance (`recur` present). A past wait date became ordinary pending again. `project.is:<value>` matched the exact value; `project:<value>` matched descendants and prefix lookalikes, so the latter is unsafe for exact migration mapping.
- The guarded command shape `task <uuid> status:pending recur.none: project.is:<old> modify project:<new>` changed only the exact eligible task. A child task required its own explicit old value; a `workshop` task did not match `work`; completed, future-waiting, deleted, recurring-template, and recurring-instance records returned exit `1` with `No tasks specified.` and retained their old values. A successful command returned exit `0` with no output under `verbose=no`, so every result requires UUID re-export/reconciliation rather than exit-code counting.
- Completing a task after preview and before the guarded command produced the same no-match result and left the completed task unchanged. This proves the in-command pending/old-value guard is required; preview export plus an unguarded UUID modify is not sufficient.
- Taskwarrior `export` intentionally ignores the active context (also confirmed by the official export/context documentation and an isolated context fixture). `_get rc.context` returned the active name and `_get rc.context.<name>.read` returned its filter. Supplying `(<read-filter>)` as one argv element made export and modify scope obey that captured filter. With no active context, `_get rc.context` was empty and no context filter was added. The existing `ExportPending` invocation therefore cannot be reused as proof of O1 without this explicit filter step.
- Hooks remained enabled. An `on-modify` hook received two JSON lines, could reject the operation with a non-zero result, and could alter another field before Taskwarrior saved the task. The adapter must preserve hooks, send only the project modification itself, reconcile the saved task, and report hook-induced or ambiguous outcomes; disabling hooks is not an acceptable shortcut.
- Two separate one-UUID modifies followed by one `task undo` reverted only the last task change; it did not touch configuration or the earlier task. The coordinator must not advertise native `u` as batch undo.

The exact official references consulted were Taskwarrior [hooks](https://taskwarrior.org/docs/hooks/), [modify](https://taskwarrior.org/docs/commands/modify/), [context](https://taskwarrior.org/docs/context/), [filter](https://taskwarrior.org/docs/filter/), and [export](https://taskwarrior.org/docs/commands/export/).

### TOML persistence and filesystem safety

- `BurntSushi/toml v1.6.0` has no lossless source-editing API. Its encoder re-emitted a complete document, dropped comments, changed quoting/indentation, and omitted fields not represented in the decode target. `MetaData` exposes keys/types/undecoded keys, not public source spans suitable for safe edits.
- A temporary probe against `github.com/smm-h/go-toml-edit v0.4.3` passed that module's full test suite and demonstrated editing `[[projects]]` entries across interleaved `[sync]`/other tables while retaining unrelated comments and layout. Its lossless AST supports value edits, array-table creation, and indexed deletion without regex-matching arbitrary table boundaries. A resulting fixture also loaded through Momentum's existing strict loader. T02 should use it for source editing while retaining that BurntSushi decode/strict validation path; verify the dependency in the repository before adding it.
- Do not call that package's generic `WriteFile` for the active config path: its implementation renames a temporary file over the supplied path, which would replace a symlink. Resolve the active path first, snapshot `Lstat`/link text, resolved target identity, target bytes/hash, mode, and parent; recheck those values immediately before replacement; write and `fsync` a same-directory temporary file with the target mode; rename it over the resolved regular target; and optionally sync the directory. A symlink remains intact. Reject broken/changed links, changed target bytes/mode, read-only targets, and write/parse/conflict failures without replacing the original. Missing regular paths may create their parent and file deliberately.
- Validate edited bytes both with the source editor's parser and the existing strict config loader before the final rename. Patch only `projects`, so environment-derived `MOMENTUM_ICONS` is never serialized. Optimistic checks cannot eliminate a final external-edit race; document that limitation and check as late as practicable.

### Coordinator guarantee

Catalog-first execution is feasible only as an explicit sequence, not a transaction: preflight/revalidate, replace the config, then run one guarded UUID command per confirmed task. A config failure runs zero task commands. A quit, timeout, hook rejection, or later task failure can leave a saved catalog and earlier successful task changes; retain and report those outcomes separately, stop without expanding the confirmed UUID set, and require a fresh preview/confirmation for retry. No automatic rollback, retry, durable journal, or batch undo is claimed.

### Baseline verification

On the pre-feature working tree, all of these passed:

- `go test ./... -count=1` — 8 packages passed.
- `go test ./internal/taskwarrior -run Integration -v -count=1` — 8 isolated integration tests passed against Taskwarrior 3.5.0.
- `test -z "$(gofmt -l .)"`, `go vet ./...`, and `go build ./cmd/momentum` — passed.

No repository feature code or user configuration was changed by the probes.

## Tracker

Change `[ ]` to `[x]` only when the task's tests and completion criteria pass. Put `blocked: O#` or current progress in the Evidence column when appropriate.

| Done | ID | Deliverable | Depends on | Evidence / commit |
|---|---|---|---|---|
| [x] | T00 | Verify adapter/config feasibility for accepted policies | — | Taskwarrior 3.5.0 and isolated config probes passed; explicit context filter, `status:pending recur.none:` + `project.is:` guard, hook/reconciliation, lossless TOML, symlink, atomic-write, partial-failure, and undo constraints recorded below. Documentation remains uncommitted per handoff. |
| [x] | T01 | Pure catalog edit and task-mapping planner | T00 | `go test ./internal/domain -count=1` passed; table-driven edit/mapping tests added; commit `feat(projects): plan catalog edits and pending renames` |
| [x] | T02 | Safe project-catalog config store | T00 | `go test ./internal/config -count=1` passed; source-preserving, conflict-checked, symlink-safe store and isolated filesystem tests added; commit `feat(config): safely persist project catalog edits` |
| [x] | T03 | Carry active config path and inject store | T02 | Resolved path is passed to `FileProjectCatalogStore` and `ModelOptions`; constructor/path-isolation tests pass; commit `feat(app): inject the active project config store` |
| [x] | T04 | Pure Projects settings editor component | T01 | `go test ./internal/ui -count=1` passed; pure draft/list/prompt component and width/input tests added; commit `feat(ui): add the project catalog settings editor` |
| [x] | T05 | Settings routing and live catalog saves | T03, T04 | Targeted app/UI/config tests and `go vet ./...` pass; Settings route/save/discovery integration committed as `feat(settings): connect project editing and live suggestions` |
| [x] | T06 | Guarded pending-project migration adapter | T01 | `go test ./internal/taskwarrior -count=1` and all 11 isolated Integration tests passed; guarded context export/reconciliation adapter committed as `feat(taskwarrior): guard pending project reassignments` |
| [x] | T07 | Rename preview/confirmation component | T01 | `go test ./internal/ui -count=1` passed; explicit scope/mode/merge preview and confirmation tests added; commit `feat(ui): preview pending project renames explicitly` |
| [x] | T08 | Explicit migration coordinator with partial outcomes | T02, T06 | Coordinator fault-injection tests pass for preflight/catalog-first/partial/ambiguous/cancel paths; commit `feat(app): coordinate explicit project migrations` |
| [x] | T09 | Migration-specific serialization/sync/undo lifecycle | T08 | Migration gate/sync/undo lifecycle tests pass, including race gate; commit `feat(sync): serialize project migrations safely` |
| [x] | T10 | Connect settings rename intent to migration lifecycle | T05, T07, T09 | End-to-end settings/preview/catalog-only/pending/stale/partial/zero-match workflow tests pass; commit `feat(settings): connect pending-only project renames` |
| [ ] | T11 | End-to-end evidence, documentation, final regression gate | T10 | — |

**Suggested execution order:** T00, T01, T02, T03, T04, T05, T06, T07, T08, T09, T10, T11. This is a topological sequence; it does not add dependencies. Default to sequential execution because several tasks touch app state and environment-based integration tests must not run in parallel. Catalog-only Settings is demonstrable at T05, but the requested feature is incomplete until T11.

## Task details

### T00 — Resolve policy and prove feasibility

**Where:** ADR 0001, feature spec, this plan; focused isolated experiments if necessary.  
**Depends on:** none. **Requirements:** SP-05, SP-09–SP-15.  
**Reuses:** current Taskwarrior runner/integration guards and strict TOML loader.  
**Tests:** isolated executable probes for Taskwarrior semantics; documentation/fixture inspection for config round-tripping.

- [x] Record all user choices (2026-09-16): active context; ask about child removal; reject catalog duplicates; Include subprojects covers catalog/task descendants as applicable; warn and confirm task-only destination merges; catalog-first partial-failure policy; no batch undo; preserve symlinks.
- [x] Verify guarded UUID/status/exact-project modification, context behavior, recurrence/hook implications, and detecting zero affected tasks in supported Taskwarrior 3.x. A re-export followed by an unguarded UUID modify is insufficient protection against a task being completed between commands.
- [x] Establish safe source-preserving TOML save strategy and conflict detection while preserving symlinks and updating their targets. Check current dependency capabilities instead of assuming the encoder preserves comments.
- [x] Verify the chosen catalog-first failure outcomes, mid-operation quit behavior, and no-batch-undo wording. If guarantees cannot be met, mark dependent tasks blocked and return to the user; do not silently change the agreed policies.
- [x] Update ADR/spec/plan with the agreed policies and test cases; capture baseline test names/counts.

**Gate/evidence:** record exact isolated commands, tested Taskwarrior version, observed outputs, chosen config technique, and policy answers. Never run these probes on the user's task data. No product code is considered done in this task.  
**Commit:** `docs(settings): resolve project rename policies`

### T01 — Pure catalog edits and migration mappings

**Where:** `internal/domain/project.go`; proposed `project_edit.go`, `project_migration.go` and matching tests.  
**Depends on:** T00. **Requirements:** SP-03–SP-05, SP-07–SP-12.  
**Reuses:** `ProjectCatalog.Validate`, `Labels`, immutable-copy patterns.  
**Tests:** table-driven domain unit tests.

- [x] Add/update/remove drafts without mutating input slices; implement the agreed catalog descendant/collision policies. Include subprojects moves configured descendants while preserving suffixes; duplicate resulting catalog values are rejected.
- [x] Distinguish label-only from value changes; compose parent + local segment into one dotted value; reject moving a parent under its own descendant where applicable.
- [x] Map only exact pending matches; optionally map `old + "."` descendants using `new + preserved suffix`. Exclude prefix lookalikes, historical/other statuses, and unrelated fields.
- [x] Return enough explicit source/destination data for preview, validation, and stale checks; unchanged drafts are no-ops.
- [x] Test zero matches, grandchildren, parent moves, missing configured ancestors, duplicate targets, source ownership, and hierarchy/name-only behavior.

**Gate:** `go test ./internal/domain -count=1 -v` passed: 35 top-level tests and 63 passing test actions including subtests across the domain package, including `TestComposeProjectValueAndDraftResolution`, `TestPlanAddProjectCopiesCatalogAndRejectsDuplicates`, `TestPlanUpdateProjectClassifiesChangesAndMapsConfiguredDescendants`, `TestPlanUpdateAllowsMissingConfiguredAncestorsAndPreservesSuffixes`, `TestPlanUpdateRejectsOwnDescendantAndResultingCollisions`, `TestPlanUpdateRequiresExactSourceOwnership`, `TestPlanRemoveProjectHonorsSubtreeChoiceWithoutMutatingInput`, `TestProjectCatalogPlanMapsOnlyEligiblePendingTasks`, `TestMapPendingProjectTasksHonorsExactBoundaryAndSubprojectOption`, `TestMapPendingProjectTasksReturnsNoMatchesWithoutBroadeningScope`, and `TestProjectCatalogPlanDoesNotOfferTaskMappingsForLabelOnlyOrNoOp`. `go test ./... -count=1` also passed (8 packages).  
**Commit:** `feat(projects): plan catalog edits and pending renames`

### T02 — Safe config catalog persistence

**Where:** proposed `internal/config/projects_store.go`, its tests; loader changes only if required.  
**Depends on:** T00. **Requirements:** SP-06, SP-13, SP-16.  
**Reuses:** config validation, XDG/path rules, temp-directory test helpers.  
**Tests:** filesystem unit/integration tests using temporary configs only.

- [x] Read a source snapshot/revision and replace only catalog content at an explicit path; create missing config directories/file deliberately.
- [x] Preserve unrelated fields/comments, newline handling, and permissions; retain symlinks while updating their resolved targets with link/target conflict checks. Handle valid TOML shapes, including comments, quoted strings, and interleaved top-level tables; do not serialize effective environment overrides.
- [x] Validate generated TOML through the existing strict loader before replacement; reject a changed source file rather than overwrite it.
- [x] Write via a temporary file in the destination directory and atomic replacement; clean temporary artifacts on failure. State any concurrency limitations honestly; detect conflicts as late as practicable.
- [x] Test existing/missing/empty config, no projects, multiple entries, clearing all entries, comments around catalog tables, unrelated sync settings, invalid source, read-only/write failure, external edits, and symlinks per O6. Failures retain original file bytes.

**Gate:** `go test ./internal/config -count=1` passed: 27 top-level tests and 35 passing actions including subtests; `go test ./... -count=1`, `go vet ./...`, and the formatting check also passed. The store uses direct `go-toml-edit v0.4.3` source editing and does not call its generic file writer.  
**Commit:** `feat(config): safely persist project catalog edits`

### T03 — Active-path/store dependency wiring

**Where:** `cmd/momentum/main.go`, `internal/app/model.go`, CLI/model tests.  
**Depends on:** T02. **Requirements:** SP-06, SP-13.  
**Reuses:** `LoadWithOptions`'s returned `resolvedPath`, `ModelOptions`, fake dependency patterns.  
**Tests:** unit tests with injected in-memory/temp stores.

- [x] Pass the resolved active config path/store into the app rather than recompute a default path during save.
- [x] Explicit `--config`, XDG config, and fallback path all target the same file used for loading.
- [x] Rendering-only models with no persistence dependency remain safe; surface unavailable persistence instead of silently touching the user's default config.
- [x] No filesystem side effect at model construction; errors are asynchronous typed results.

**Gate:** `go test ./cmd/momentum ./internal/app ./internal/config -count=1` passed; `go vet ./...`, formatting, and `go test ./... -count=1` also passed. `TestNewAppModelUsesTheResolvedConfigPathForEveryPathMode`, `TestNewModelInjectsProjectStoreWithoutConstructionTimeIO`, and `TestNewModelWithoutProjectStoreDoesNotCreatePersistenceDependency` passed.  
**Commit:** `feat(app): inject the active project config store`

### T04 — Projects settings editor component

**Where:** proposed `internal/ui/projectsettings.go` and tests; reuse existing text inputs/styles.  
**Depends on:** T01. **Requirements:** SP-02–SP-07, SP-16.  
**Tests:** pure UI interaction and width-bound rendering tests.

- [x] Render configured hierarchy/value list, empty state, and keyboard help; keep selection visible for a long catalog.
- [x] Support add/edit/remove drafts, name/value/parent fields, full-value preview, inline errors, and explicit save/cancel intents.
- [x] For a parent with configured descendants, ask whether to remove the children too; show counts and test No (parent only), Yes (configured subtree), and Cancel (no changes). Emit no task deletion/modify intent for removal or label-only edits.
- [x] Unsaved changes require explicit save/discard before leaving; keep draft on save errors and clear it only after success/discard.
- [x] Test wide/compact/narrow/minimum layouts, long names, blank/duplicate/invalid values, parent changes, navigation, and absence of I/O from the component.

**Gate:** `go test ./internal/ui -count=1` passed: 86 UI tests; the new coverage includes `TestProjectSettingsRendersConfiguredHierarchyAndEmptyState`, `TestProjectSettingsKeepsSelectedLongCatalogEntryVisible`, `TestProjectSettingsAddComposesParentAndPublishesOnlyAfterSave`, `TestProjectSettingsLabelOnlyEditDoesNotPlanTaskMappings`, `TestProjectSettingsRejectsInvalidHierarchyAndKeepsDraft`, `TestProjectSettingsRemoveAsksAboutConfiguredChildren`, `TestProjectSettingsRemoveWithoutChildrenEmitsSaveIntent`, `TestProjectSettingsDirtyCancelRequiresExplicitDiscard`, `TestProjectSettingsSaveFailureRetainsDraft`, `TestProjectSettingsSupportsResponsiveWidths`, and `TestProjectSettingsSetProjectsCopiesInput`. Full tests, vet, and formatting also passed.  
**Commit:** `feat(ui): add the project catalog settings editor`

### T05 — Settings navigation and catalog-only save integration

**Where:** `internal/app/model.go`, `view.go`, `update.go`, `messages.go`, proposed `settings.go`; `internal/ui/sidebar.go`, `help.go`; co-located app/UI tests.  
**Depends on:** T03, T04. **Requirements:** SP-01, SP-02, SP-05–SP-07, SP-13, SP-14, SP-16.  
**Tests:** model integration with fake store/client and navigation rendering/input tests.

- [x] Add Settings without coercing it through task-only view normalization, counters, selection restoration, or task rendering. Adjust tab/sidebar mouse hit targets for the actual navigation entries.
- [x] Provide discoverable keyboard access at every usable width; preserve Inbox/Today selection/search state when returning. Do not let Settings keys mutate a previously selected hidden task.
- [x] Save catalog-only intents asynchronously; publish the new catalog only after persistence success, preserve draft on failure, and route dirty-draft quit safely.
- [x] Re-merge the new catalog with a separately maintained discovered snapshot; prevent deleted/renamed configured values from being reintroduced as false discovery. Existing real Taskwarrior values may remain.
- [x] Refresh quick-capture/editor suggestions immediately. Guard against stale asynchronous save/discovery responses. Pure catalog saves never mutate tasks or trigger task sync/undo state.
- [x] Demonstrate create/edit/remove/restart with a temp config and no tasks.

**Gate:** `go test ./cmd/momentum ./internal/app ./internal/ui ./internal/config -count=1` passed; `go vet ./...`, formatting, and `go test ./... -count=1` also passed. The app package reported 104 passing tests, including Settings routing, actual compact-tab hit targets, selection round trips, async success/failure, separate discovery snapshots, stale results, and dirty-quit handling.  
**Commit:** `feat(settings): connect project editing and live suggestions`

### T06 — Guarded pending-project adapter

**Where:** proposed `internal/taskwarrior/project_migration.go`, runner tests and isolated integration tests; client interface changes/fakes as needed.  
**Depends on:** T01. **Requirements:** SP-09–SP-12, SP-16.  
**Reuses:** `CommandClient.run`, injected `Runner`, `newIsolatedEnvironment`.  
**Tests:** argv unit tests plus real Taskwarrior integration, not parallel.

- [x] Export pending tasks in the active Taskwarrior context without UI search/Today filtering or global override; no persistent context changes. Bind preview to that context, invalidate on context change, and test an explicit no-active-context case.
- [x] Apply only the planned UUID's project assignment, guarded by pending status and expected old value. Never execute an unrestricted `project:old modify` bulk command.
- [x] Return verified outcomes (changed/skipped/error) rather than assuming every successful process changed a task; handle timeouts/ambiguous results with reconciliation.
- [x] Preserve descendant suffixes from the domain mapping and all unrelated fields; commands use argument vectors, not a shell.
- [x] Test matching/mismatching old value, pending→completed changes, child boundary matching, context scope, hook errors, and non-pending records. Retain production-path guards; absence of Taskwarrior is a recorded blocked integration gate, not full success.

**Gate:** `go test ./internal/taskwarrior -count=1` passed: 45 adapter tests; `go test ./internal/taskwarrior -run Integration -v -count=1` executed and passed all 11 isolated integration tests against Taskwarrior 3.5.0, including `TestIntegrationProjectMigrationHonorsActiveContextWithoutChangingIt`, `TestIntegrationProjectMigrationIsPendingExactAndContextScoped`, and `TestIntegrationProjectMigrationReportsHookRejection`. Unit coverage includes guarded argv, no-active-context export, changed/skipped/failed/timeout/ambiguous reconciliation, and unsafe mapping rejection.  
**Commit:** `feat(taskwarrior): guard pending project reassignments`

### T07 — Rename preview/confirmation component

**Where:** proposed `internal/ui/projectrename.go` and tests.  
**Depends on:** T01. **Requirements:** SP-07–SP-12, SP-15, SP-16.  
**Tests:** pure UI state/input/render tests.

- [x] Show catalog-only default versus explicit pending migration; reset choices between edits; no migration controls for name-only changes.
- [x] Show old/new mappings, pending count, active context (or no active context), descendant choice with separate catalog/task counts, catalog-collision rejection, explicit effective-merge warning/confirmation for task-only destinations, historical-retention statement, and no-batch-undo explanation.
- [x] Require explicit confirmation; changing scope/options invalidates preview. Support zero matches, canceled intent, stale preview, and retry/error messaging.
- [x] Test all branches and supported widths; never insert display labels into task values.

**Gate:** `go test ./internal/ui -count=1` passed: 98 UI tests, including `TestProjectRenameCatalogOnlyIsDefaultAndShowsPreviewContext`, `TestProjectRenameIncludeSubprojectsShowsSeparateCatalogAndTaskCounts`, `TestProjectRenameModeChangeInvalidatesPreviewAndEmitsIntent`, `TestProjectRenameRequiresExplicitConfirmationAndEmitsOnlyTaskValues`, `TestProjectRenameTaskOnlyDestinationRequiresEffectiveMergeWarning`, `TestProjectRenameNameOnlyAndZeroMatchPreviewsRemainCatalogOnly`, `TestProjectRenameCancelStaleAndErrorKeepActionableState`, `TestProjectRenameWidthsStayBounded`, and `TestProjectRenameUpdatePreviewCopiesTaskMappings`. Full tests, vet, and formatting also passed.  
**Commit:** `feat(ui): preview pending project renames explicitly`

### T08 — Migration coordinator and partial outcome model

**Where:** proposed `internal/app/project_migration.go` / tests, typed command/result definitions.  
**Depends on:** T02, T06. **Requirements:** SP-08–SP-13, SP-15, SP-16.  
**Tests:** deterministic fake-store/fake-client orchestration tests with failures injected at each step.

- [x] Build a fresh preview and bind confirmation to its exact mappings/scope/revision; revalidate before writes and require a new confirmation when the plan changes.
- [x] After preflight, save the catalog first; only after success execute the confirmed UUID set through the guarded adapter. A catalog save failure must result in zero task mutations. Never append newly discovered tasks to the set mid-execution.
- [x] Return catalog-save result separately from task counts/UUID outcomes. Handle zero changes, preflight failure, first/middle/last task failure, cancellation, and ambiguous command results.
- [x] Keep successful task changes visible; do not falsely roll back in-memory state to imply task rollback. Retry is a fresh plan, never blindly replayed consent.
- [x] No implicit cross-store transaction, durable journal, automatic compensation, or automatic retry. Code comments/documentation state the chosen guarantees.

**Gate:** `go test ./internal/app ./internal/config ./internal/taskwarrior -count=1` passed; `go vet ./...`, formatting, and `go test ./... -count=1` also passed. The coordinator tests reported 114 top-level tests and 119 passing actions in `internal/app`, including catalog-first ordering, save failure with zero task calls, stale scope/set, first/middle/last failure, zero matches, preflight failure, ambiguity, and cancellation.  
**Commit:** `feat(app): coordinate explicit project migrations`

### T09 — Migration lifecycle in the mutation/sync state machine

**Where:** `internal/app/model.go`, `actions.go`, `sync.go`, `sync_state.go`, migration lifecycle tests.  
**Depends on:** T08. **Requirements:** SP-14–SP-16.  
**Tests:** state-machine tests with controlled clock, fake clients, and in-flight commands.

- [x] Acquire/release one migration operation gate; block competing task edits, repeated migration/save submission, and misleading quit/undo actions.
- [x] Do not begin migration during an in-flight sync/task write. Defer startup/manual/timer/retry/shutdown sync while migration owns the gate; handle stale timers/responses safely and resume scheduling afterward.
- [x] Task changes, including partial success, mark task data unsynced and schedule refresh/native sync according to existing configuration; zero-task/catalog-only operations do not.
- [x] Disable undo during migration and prevent native `u`/previous grace state from being advertised as reversing the batch or config save after task changes. Preserve later ordinary single-task undo. Do not lose pre-existing dirty task state on failure or no-op.
- [x] Test local-only operation, sync-disabled mode, pending previous undo grace, mid-migration sync ticks, refresh ticks, quit/cancel, success, partial failure, and preservation of later ordinary single-task undo behavior.

**Gate:** `go test ./internal/app -count=1` passed: 121 top-level tests and 128 passing actions including subtests; `go test -race ./internal/app`, full tests, vet, and formatting also passed. Migration lifecycle coverage includes `TestMigrationGateBlocksCompetingTaskWritesSyncAndUndo`, `TestCatalogOnlyMigrationRestoresSyncStateAndDoesNotRefreshTasks`, `TestMigrationTaskChangesDisableUndoMarkUnsyncedAndReleaseAfterRefresh`, `TestMigrationPartialTaskChangeKeepsDirtyStateAndLaterOrdinaryUndo`, cancellation/pre-existing-grace cases, timer deferral, and typed command results.  
**Commit:** `feat(sync): serialize project migrations safely`

### T10 — End-to-end settings rename wiring

**Where:** app settings/migration routing, typed messages and view composition; co-located workflow tests.  
**Depends on:** T05, T07, T09. **Requirements:** SP-06–SP-12, SP-14–SP-16.  
**Tests:** model workflow tests from draft through preview/confirmation/results using injected stores/adapters.

- [x] Wire value edits to catalog-only default or explicit preview, confirmation, execution, and final result. Labels/removal never dispatch migration.
- [x] Display catalog and task results distinctly, retain actionable drafts/errors, refresh live suggestions after the actual config save, and reconcile task lists after any changed task.
- [x] Preserve user focus/selection on failure and return to safe Settings state after completion; dirty drafts, stale previews, and repeated keys cannot bypass confirmation.
- [x] Verify source values may remain discoverable when historical/non-migrated or out-of-scope tasks retain them; do not label this as total rename failure.
- [x] Test restart, custom config path, stale discovery, zero tasks, task completion between preview/confirmation/execution, partial errors, and explicit retry.

**Gate:** `go test ./... -count=1` passed; `go test -race ./...`, `go vet ./...`, and formatting also passed. `internal/app` reported 127 top-level tests and 134 passing actions including subtests; workflow coverage includes Catalog-only confirmation, pending migration, stale preview with explicit retry, task-only merge warning, partial results, zero matches, and no hidden task mutations.  
**Commit:** `feat(settings): connect pending-only project renames`

### T11 — Acceptance evidence and final documentation

**Where:** `docs/UAT.md`, `README.md`, `DESIGN.md`, `IMPLEMENTATION_PLAN.md`, `CONTRIBUTING.md` if needed, `.notebook/`, this plan and ADR/spec status.  
**Depends on:** T10. **Requirements:** SP-01–SP-16.  
**Tests:** isolated manual keyboard UAT plus full automated regression gates. This supplements, never substitutes for, co-located tests in earlier tasks.

- [ ] Demonstrate Settings at wide/compact/narrow sizes; add/edit/reparent/remove, dirty cancel/quit, no-task catalog, and immediate suggestions.
- [ ] Demonstrate pending-only rename with completed/deleted/waiting/template records, descendants off/on, prefix lookalikes, agreed context scope/collision behavior, stale preview, custom config, and partial failure.
- [ ] Inspect before/after exports from an isolated database to prove historical values/statuses and unrelated fields are unchanged by Momentum; inspect temporary TOML to prove unrelated content/comments survive.
- [ ] Update current docs that say restart is always required or catalog edits can never explicitly initiate task operations; retain clarity that automatic side effects remain forbidden. Amend the narrow bulk-edit exception, not all bulk-operation non-goals.
- [ ] Record actual commands, test names/counts, Taskwarrior version, skips/blockers, manual outcomes, and requirement evidence below. Update ADR open-policy outcomes and spec status.
- [ ] Run formatting check, `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./cmd/momentum`, and `git diff --check`. All pass; required real integration tests must not merely skip.

**Commit:** `docs(settings): document and validate project management`

## Dependency / granularity validation

The tracker is the authoritative dependency graph; this table cross-checks every task body against it. No parallel execution is assumed and no separate diagram can drift from these definitions.

| Task | Body dependencies = tracker dependencies | Atomic deliverable | Result |
|---|---|---|---|
| T00 | none | One policy/feasibility record | Match |
| T01 | T00 | One pure edit/mapping planner | Match |
| T02 | T00 | One config store | Match |
| T03 | T02 | One active-config dependency wiring change | Match |
| T04 | T01 | One settings editor component | Match |
| T05 | T03, T04 | One settings route/save integration | Match |
| T06 | T01 | One guarded migration adapter | Match |
| T07 | T01 | One preview/confirmation component | Match |
| T08 | T02, T06 | One migration coordinator | Match |
| T09 | T08 | One migration lifecycle/state-machine integration | Match |
| T10 | T05, T07, T09 | One end-to-end rename routing integration | Match |
| T11 | T10 | One acceptance/documentation deliverable | Match |

## Test co-location / gate coverage

No `.specs/codebase/TESTING.md` exists at planning time. Test policy is derived from `CONTRIBUTING.md` and existing Go unit/isolated integration/UI tests; no new test framework is required.

| Layer / tasks | Required tests | Co-located in task? |
|---|---|---|
| Policy feasibility / T00 | Documented isolated behavior checks | Yes |
| Domain / T01 | Pure table-driven unit tests | Yes |
| Config / T02 | Temp-file round-trip/conflict/failure tests | Yes |
| Dependency wiring / T03 | CLI/model injection and path tests | Yes |
| UI / T04, T07 | Input, intent, and bounded rendering tests | Yes |
| App settings / T05 | Fake-store/client integration and no-side-effect tests | Yes |
| Taskwarrior / T06 | Runner argv + non-parallel isolated real integration | Yes |
| Coordinator / T08 | Per-step fault injection and stale-plan tests | Yes |
| Sync / T09 | State-machine/race/serialization tests | Yes |
| Workflow / T10 | Complete model interaction tests | Yes |
| Release evidence / T11 | All gates + isolated manual UAT | Yes (additional coverage) |

Record baseline and new test names/counts when executing; this document does not invent future test counts. Tests must not be deleted/skipped to meet a gate. Integration skips are explicitly reported as incomplete verification.

## Requirement traceability / sign-off

Populate evidence with a test name, UAT step, or commit plus result—not merely a checked implementation box.

| Requirement | Implementation tasks | Verified evidence |
|---|---|---|
| SP-01 | T05, T11 | T05: `TestSettingsRouteIsExplicitAndCannotMutateHiddenTask` and `TestSettingsMouseNavigationUsesActualCompactTabBoundary` passed; final manual width/UAT evidence remains in T11. |
| SP-02 | T04, T05, T11 | T04: list/empty/long-catalog rendering tests passed; T05: Settings opens the Projects section and updates configured suggestions through the save path; restart/UAT remains pending. |
| SP-03 | T01, T04, T11 | T01: `TestComposeProjectValueAndDraftResolution` and `TestPlanUpdateProjectClassifiesChangesAndMapsConfiguredDescendants` passed; T04 covers editable name/value/parent fields and full-value preview in `TestProjectSettingsAddComposesParentAndPublishesOnlyAfterSave`; end-to-end UI remains pending. |
| SP-04 | T01, T04, T11 | T01: draft/value validation and duplicate-result rejection passed in `TestComposeProjectValueAndDraftResolution`, `TestPlanAddProjectCopiesCatalogAndRejectsDuplicates`, and `TestPlanUpdateRejectsOwnDescendantAndResultingCollisions`; T04 invalid-hierarchy retention passed in `TestProjectSettingsRejectsInvalidHierarchyAndKeepsDraft`; remaining validation/UAT is pending. |
| SP-05 | T00, T01, T04, T05, T11 | T01: `TestPlanRemoveProjectHonorsSubtreeChoiceWithoutMutatingInput`; T04: `TestProjectSettingsRemoveAsksAboutConfiguredChildren` and `TestProjectSettingsRemoveWithoutChildrenEmitsSaveIntent` passed, including No/Yes/Cancel; app wiring remains pending. |
| SP-06 | T02–T05, T10, T11 | T02: explicit-path snapshot/save tests passed for missing, empty, array, and array-table catalogs; T03: `TestNewAppModelUsesTheResolvedConfigPathForEveryPathMode` passed for explicit/XDG/fallback resolution; T05: async save success/failure and immediate suggestion updates passed; T10 end-to-end catalog-only/pending workflow passed; final restart/UAT remains pending. |
| SP-07 | T01, T04, T05, T07, T10, T11 | T01: label-only/no-op classification and absence of task mappings passed in `TestPlanUpdateProjectClassifiesChangesAndMapsConfiguredDescendants` and `TestProjectCatalogPlanDoesNotOfferTaskMappingsForLabelOnlyOrNoOp`; T04 preserves that behavior in `TestProjectSettingsLabelOnlyEditDoesNotPlanTaskMappings`; workflow coverage remains pending. |
| SP-08 | T07, T08, T10, T11 | T01: stored-value edits are distinguished from label-only edits; T07: catalog-only default/mode-reset tests passed; T08 binds the explicit task choice to a fresh preflight; T10 `TestValueEditCatalogOnlySavesAfterExplicitPreviewConfirmation` and `TestValueEditPendingFlowRunsOnlyConfirmedMappingsAndRefreshes` passed. |
| SP-09 | T00, T01, T06–T08, T10, T11 | T01: exact pending/non-recurring UUID mapping passed; T06 active-context/export/guard integration passed; T10 `TestValueEditPendingFlowRunsOnlyConfirmedMappingsAndRefreshes` passed; final UAT remains pending. |
| SP-10 | T00, T01, T06–T08, T10, T11 | T01: configured and unconfigured dotted descendants plus `workshop` boundary passed; T06 applies explicit descendant mappings through guarded UUID commands; T07/T10 preview and workflow tests pass descendant option/counts and zero-match behavior; final UAT remains pending. |
| SP-11 | T01, T07, T08, T10, T11 | T01 explicit mappings/collision rejection, T07 mapping/context/count/zero-match/merge/history/undo preview, T08 catalog-first/failure outcomes, and T10 `TestTaskOnlyDestinationWarningIsNotBypassedByWorkflow`/Catalog-only confirmation tests pass. |
| SP-12 | T01, T06–T08, T10, T11 | T01 copied snapshots/exact mappings, T06 stale completion/reconciliation, T07 option invalidation, T08 exact-set preflight, and T10 `TestValueEditStalePreviewKeepsDraftAndPerformsNoWrites` plus explicit retry pass; final UAT remains pending. |
| SP-13 | T00, T02, T03, T05, T08, T11 | T02: source/comment/newline/permission/conflict/symlink tests passed; T03 preserves the injected active path; T05 retains the previous live catalog on async save failure; T08 separates catalog-save failure from task outcomes and never starts tasks after a failed save; final handling remains pending. |
| SP-14 | T05, T09, T10, T11 | T05 typed save commands/results and repeated-save blocking; T09 migration ownership, competing writes, sync/timer deferral, refresh release, and quit/undo suppression; T10 end-to-end typed preview/migration commands pass. |
| SP-15 | T00, T07–T11 | T07 task-only merge warning, stale/error, explicit confirmation, history, and no-batch-undo; T08 catalog-first/partial/ambiguous/no-retry; T09 undo/dirty-state lifecycle; T10 partial/zero-match final workflow tests pass. |
| SP-16 | T02, T04–T11 | T02: all store tests use `t.TempDir` and isolated config paths; invalid/read-only/conflict/symlink failures retain source bytes. T04 is pure and has no I/O dependency; T05 fake-store tests assert no task mutations; T06 all real commands use `newIsolatedEnvironment` with production-path guards, and hook/reconciliation tests pass; T08 fake-store/client fault injection has no production-data access. Remaining workflow/UAT isolation is pending. |

**Coverage at handoff:** 16/16 requirements mapped; T01–T10 implementation evidence recorded, with final manual UAT/release documentation pending; 0/16 requirements fully verified until T11; 10/12 tasks complete.

## Execution log

| Date | Task | State | Commit | Commands / evidence / blocker | Next action |
|---|---|---|---|---|---|
| 2026-09-16 | Planning | Complete | Not committed by this planning session | Source map, requirements, ADR, and task dependencies reviewed; no feature code changed | Resolve T00 before implementing policy-dependent paths |
| 2026-09-16 | T00 policy recording | Partial | Not committed | All product choices, including O2-R/O3-T, recorded consistently in ADR/spec/plan; no feature code or feasibility experiments executed | Complete technical checks; do not reopen resolved choices |
| 2026-09-16 | T00 technical feasibility | Complete; docs uncommitted | Pending explicit commit instruction | Isolated Taskwarrior 3.5.0 probes, official Taskwarrior hook/context/modify docs, BurntSushi encoder probe, and `go-toml-edit` v0.4.3 lossless-edit probe recorded in the T00 feasibility record below; baseline Go gates passed | Start T01 without reopening O1–O6 or O2-R/O3-T |
| 2026-09-16 | T01 pure catalog/migration planning | Complete | `feat(projects): plan catalog edits and pending renames` | Added source-independent `ProjectDraft`, catalog add/update/remove plans, collision and hierarchy validation, configured descendant mappings, and pending UUID old/new mappings. `go test ./internal/domain -count=1 -v` passed (35 top-level tests, 63 passing actions including subtests); `go test ./... -count=1` passed across 8 packages. | Start T02 safe config catalog persistence |
| 2026-09-16 | T02 safe config catalog persistence | Complete | `feat(config): safely persist project catalog edits` | Added `ProjectCatalogStore`/`FileProjectCatalogStore` with lossless array/array-table editing, strict pre-replacement validation, optimistic source/link/target checks, same-directory atomic replacement, mode/newline preservation, and isolated temp-file tests. `go test ./internal/config -count=1` passed (27 top-level tests, 35 passing actions including subtests); full tests/vet/formatting passed. | Start T03 active config path/store wiring |
| 2026-09-16 | T03 active config path/store wiring | Complete | `feat(app): inject the active project config store` | `newAppModel` now injects the exact `resolvedPath` into both `ModelOptions.ProjectStore` and `ProjectConfigPath`; rendering-only models leave the seam nil. Path-mode and construction-I/O tests passed; targeted packages, full tests, vet, and formatting passed. | Start T04 Projects settings editor |
| 2026-09-16 | T04 pure Projects settings editor | Complete | `feat(ui): add the project catalog settings editor` | Added the pure list/editor component with owned catalog snapshots, parent/value composition, domain-backed validation, explicit save/cancel/discard intents, child-removal No/Yes/Cancel prompt, and bounded responsive rendering. `go test ./internal/ui -count=1` passed (86 tests); full tests/vet/formatting passed. | Start T05 settings navigation and live catalog saves |
| 2026-09-16 | T05 settings navigation/catalog save integration | Complete | `feat(settings): connect project editing and live suggestions` | Added first-class Settings routing, no-count navigation tabs/sidebar hit testing, task-selection protection, async typed catalog saves, separate discovery snapshots, stale-result guards, immediate suggestion refresh, and dirty-quit handling. Targeted packages, 104 app tests, full tests, vet, and formatting passed. | Start T06 guarded pending-project adapter |
| 2026-09-16 | T06 guarded pending-project adapter | Complete | `feat(taskwarrior): guard pending project reassignments` | Added active-context capture, explicit pending/non-recurring export filters, one-UUID exact-old project guards, context-safe reconciliation with unscoped UUID fallback, typed outcome classification, timeout ambiguity handling, and isolated real Taskwarrior/hook tests. 45 adapter tests and 11 isolated Integration tests passed against 3.5.0. | Start T07 rename preview/confirmation component |
| 2026-09-16 | T07 rename preview/confirmation component | Complete | `feat(ui): preview pending project renames explicitly` | Added the pure preview with per-edit Catalog-only defaults, separate descendant counts/mappings, context/no-context copy, task-only effective-merge warning, explicit confirmation, stale/error handling, historical retention, and no-batch-undo messaging. `go test ./internal/ui -count=1` passed (98 tests); full tests/vet/formatting passed. | Start T08 migration coordinator |
| 2026-09-16 | T08 migration coordinator | Complete | `feat(app): coordinate explicit project migrations` | Added exact-scope/UUID-set preflight revalidation, catalog-first save ordering, separate catalog/task outcomes, stop-on-failure partial semantics, zero-match/catalog-only paths, cancellation, and no-retry behavior. Targeted app/config/Taskwarrior tests, 114 app tests, full tests, vet, and formatting passed. | Start T09 migration sync/undo lifecycle |
| 2026-09-16 | T09 migration sync/undo lifecycle | Complete | `feat(sync): serialize project migrations safely` | Added the migration gate, deferred competing sync/refresh/quit/undo paths, task-change unsynced scheduling without batch undo, refresh-time release, and preservation of prior dirty state on no-op/failure. `go test ./internal/app -count=1` passed (121 top-level tests, 128 actions); race/full tests/vet/formatting passed. | Start T10 end-to-end settings rename wiring |
| 2026-09-16 | T10 end-to-end settings rename wiring | Complete | `feat(settings): connect pending-only project renames` | Connected value drafts to fresh previews, explicit Catalog-only/pending confirmation, descendant options, Taskwarrior scope, coordinator/lifecycle, stale retry, partial/zero-match results, and post-save catalog/suggestion updates. Full tests/race/vet/formatting passed; final manual UAT remains for T11. | Start T11 acceptance evidence and documentation |
