# Momentum UAT

Date: 2026-09-21
Host: macOS arm64 (`Darwin Mac.lan`), Go 1.27.1, Taskwarrior 3.5.0  
Linux runtime: Debian sid arm64 container, Go 1.27.1, Taskwarrior 3.5.0 (baseline v1 UAT)  
Scope: local-only, isolated Taskwarrior workflows, task-first planner/recurrence/capture workflows, local ICS fixtures, and Settings → Projects; no sync server was configured.

## Automated gates

| Gate | Result | Evidence |
|---|---|---|
| Format | PASS | `test -z "$(gofmt -l .)"` |
| Unit | PASS | `go test ./... -count=1`: 9 packages, 544 passing test actions, 0 skips |
| Vet | PASS | `go vet ./...` |
| Build | PASS | `go build ./cmd/momentum` |
| Race | PASS | `go test -race ./...` |
| Linux amd64 build | PASS | `GOOS=linux GOARCH=amd64 go build ./cmd/momentum` |
| macOS arm64 build | PASS | `GOOS=darwin GOARCH=arm64 go build ./cmd/momentum` |
| Linux full/race gates | PASS | Prior Debian sid arm64 container UAT: format, unit, vet, build, race |
| Real Taskwarrior integration (macOS) | PASS | `go test ./internal/taskwarrior -run Integration -v -count=1`: 16 isolated scenarios, including estimates, interpreted dates, native recurrence template/stop, and project migration |
| Real Taskwarrior integration (Linux) | PASS | Prior Debian sid arm64 container run: 8 isolated scenarios |
| Final working-tree diff check | PASS | `git diff --check` in the working tree |

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

The pre-G0 baseline at `36b80bb` was 8 packages, 409 top-level tests / 467 passing actions, and 0 skips. The current repository has 9 packages, 544 passing test actions, and 0 skips; the new package is the isolated local-ICS calendar parser. The final command output used `-count=1`; the race gate also passed, with estimate, capture, recurrence, planner, calendar, adapter, UI, app, and diagnostics coverage listed below.

Key added suites:

- Domain: `TestComposeProjectValueAndDraftResolution`, `TestPlanUpdateProjectClassifiesChangesAndMapsConfiguredDescendants`, `TestProjectCatalogPlanMapsOnlyEligiblePendingTasks`, and collision/removal/no-match cases.
- Config: `TestProjectCatalogStorePreservesUnrelatedTOMLAndArrayTableComments`, `TestProjectCatalogStorePreservesCRLFForNewArrayTables`, external-edit/read-only/symlink cases, and missing/array/array-table round trips.
- UI: Project Settings and rename preview interaction/rendering tests at supported widths.
- App: Settings routing/save/discovery tests, coordinator fault injection, migration gate/sync/undo lifecycle, and end-to-end Catalog-only/pending/stale/partial/zero-match workflows.
- Taskwarrior: adapter readiness/argv tests and 16 isolated `Integration` tests passed against Taskwarrior 3.5.0, including interpreted-date round trip, native recurrence template/stop, active-context, pending exact/descendant, hook rejection, stale-completion, and estimate-UDA lifecycle/refusal cases.

## Acceptance evidence for the original v1 surface

| # | Criterion | Result |
|---:|---|---|
| 1 | Taskwarrior 3.x launch | PASS on macOS and prior Linux runtime |
| 2 | Inbox/Today disjoint semantics | PASS — domain table tests |
| 3 | Today grouping and no duplicates | PASS — domain and composition tests |
| 4 | Safe quick add and six triggers, including optional estimates | PASS — parser, adapter, and runtime smoke tests |
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

## Readability implementation status (2026-09-25)

The UI readability plan added deterministic ANSI-stripped render fixtures at `120×30`, `79×24`, `49×18`, and `28×8` under `internal/app/testdata/render/` and `internal/ui/testdata/render/`. The fixtures cover Today/Inbox composition, empty/loading/error/search states, task selection, details/help, capture/edit, confirmations, and Settings surfaces. They compare by default and require `MOMENTUM_UPDATE_GOLDENS=1` to rewrite. Sanitized Today-state text captures for wide/compact/narrow review are checked in under `docs/screenshots/`; they are fixture-derived, not a substitute for real-terminal image screenshots.

Automated evidence run in this workspace:

```sh
test -z "$(gofmt -l .)"
go test ./... -count=1
go test -race ./...
go vet ./...
go build ./cmd/momentum
GOOS=linux GOARCH=amd64 go build ./cmd/momentum
GOOS=darwin GOARCH=arm64 go build ./cmd/momentum
git diff --check
```

All commands passed with Go 1.27.1 on macOS arm64, including the Linux amd64 and macOS arm64 cross-builds. Color-stripped tests now assert active navigation, selected/active/overdue/priority task cues, errors, and sync status; compact tab hit testing uses the same marker-slot geometry as rendering. A human real-terminal pass for the redesigned spacing, dark/light contrast, Unicode/ASCII glyphs, and release screenshots remains pending; this record intentionally does not claim that visual check.

## G0 estimate implementation evidence (2026-09-21)

The optional estimate slice uses the Taskwarrior `estimate` duration UDA. Automated tests use temporary `TASKRC`/`TASKDATA` paths; no sync server or production task database is used.

| Area | Evidence | Result |
|---|---|---|
| Typed domain and export compatibility | `internal/domain/estimate_test.go`; valid `PT1H30M`, `P1D`, whole-minute seconds, over-limit external values, raw retention, calendar-month rejection, null/malformed handling | PASS |
| Quick capture | `~` token-boundary/escape/duplicate/error tests, deterministic presets, Tab acceptance, root app routing | PASS |
| Structured editing | Seventh-field traversal, `E` routing, presets, canonical values, clear/cancel, invalid-input retention, over-limit external preservation | PASS |
| Adapter safety | Exact `estimate:<minutes>min` argv, readiness states, guard-before-mutation, no guard for ordinary mutations, timeout/failure coverage | PASS |
| Isolated Taskwarrior integration | `TestIntegrationEstimateAddModifyClearAndUndo`, `TestIntegrationEstimateMutationRefusesMissingUDAWithoutChangingTaskData` against Taskwarrior 3.5.0 | PASS |
| Presentation | Details shows typed Estimate once; unsupported raw values show one warning; comfortable rows show Estimate; compact/narrow rows retain description/date priority | PASS |
| Diagnostics and docs | Doctor readiness check plus setup guidance; README, DESIGN, config example, and estimate plan updated | PASS |

Required setup on each client that edits or synchronizes estimate-bearing tasks:

```text
uda.estimate.type=duration
uda.estimate.label=Estimate
```

The live keyboard/visual pass remains maintainer work: capture `~1h30m`, inspect comfortable/compact/narrow rows and Details, edit with `E`, clear, exercise undo during sync grace, and verify missing-UDA refusal with a disposable profile. G0 is recorded as **Implemented**, not **Validated**, until that pass is observed.

## G1–G4 task-first planner automated evidence (2026-09-21)

| Goal | Automated evidence | Result |
|---|---|---|
| G1 natural-language capture | R1–R5 resolved; date matrix and 12-case native recurrence differential matrix pass; see [final review and resolution](reviews/g1-natural-language-capture-final-review.md) | PASS automated; subsequent [T10 maintainer UAT](#g1-t10-live-maintainer-uat-2026-09-25) validated |
| G2 recurring tasks | Domain recurrence presets, quick-add review, recurrence editor/details, `integration_recurrence_test.go` template/instance/`until` lifecycle | PASS; live keyboard UAT pending |
| G3 daily ritual | `domain/planner_test.go`, planner UI/app tests, idempotent namespaced tag diffs, capacity warning, config weekday overrides | PASS; live keyboard/visual UAT pending |
| G4 calendar awareness | `internal/calendar/calendar_test.go` overlap merge, recurrence expansion, all-day/transparency policy, dedup/missing source; local-only config validation | PASS; live fixture/terminal UAT pending |
| G5 integration | Full package, race, vet, native/cross build, isolated Taskwarrior gates; README/DESIGN/help/config/doctor/docs updated | PASS automated; no live UAT claim |

Final automated commands on this working tree:

```sh
test -z "$(gofmt -l .)"
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./cmd/momentum
GOOS=linux GOARCH=amd64 go build ./cmd/momentum
GOOS=darwin GOARCH=arm64 go build ./cmd/momentum
go test ./internal/taskwarrior -run Integration -v -count=1
git diff --check
```

All listed commands passed on macOS arm64 with Go 1.27.1 and Taskwarrior 3.5.0. Integration tests use temporary `TASKRC`/`TASKDATA`; no calendar account, sync server, or production task database was accessed.

## G1 independent acceptance validation (2026-09-21)

**CHANGES REQUESTED:** [Detailed report and repair tasks](reviews/g1-natural-language-capture-validation.md). Fresh runs passed 9 packages, 473 top-level tests, 544 passing test actions, no skips, race/vet/native and cross-build gates, and 16 isolated Taskwarrior scenarios. Targeted probes nevertheless reproduced broken explicit-only submission, stale capture acceptance, invalid-draft confirmation, silent scalar overwrites, prose corruption, missed DST ambiguity, and incomplete review/correction behavior. The report also identifies failed-add recovery and evidence gaps. Temporary probes were removed; implementation code was not changed. No live UAT was performed.

## G1 follow-up acceptance validation (2026-09-21)

**CHANGES REQUESTED:** [Re-review and R1–R4 repair requirements](reviews/g1-natural-language-capture-rereview.md). Fresh validation passed 9 packages, 485 top-level test/fuzz entries, 573 test actions, no skips, race/vet/build checks, 16 isolated integration scenarios, and a 30-second interpreter fuzz run. Original fallback, stale-message, scalar-duplicate, protected-prose, DST, estimate-correction, and recovery reproductions are corrected. Remaining failures: no-op editing bypasses an unresolved recurrence conflict, narrow review hides actual field values, supported plural recurrence intervals are ignored, and valid 24-hour times fail before another token. The prior section is historical; use the re-review for current dispositions. No live UAT was performed.

## G1 second follow-up acceptance validation (2026-09-21)

**CHANGES REQUESTED — one remaining R2 issue:** [Latest report](reviews/g1-natural-language-capture-rereview-2.md). Fresh full suite passed 9 packages, 489 top-level test/fuzz entries, 588 actions, no skips; race/vet/format/build checks, 16 isolated integration scenarios, and 30-second fuzzing passed. Independent reproductions confirm R1/R3/R4 corrections. Short narrow-review values are visible, but a new end-of-value editing probe fails for long Description and Project fields: typed characters and cursor are clipped while confirmation remains enabled. Previous review sections are historical. No live UAT was performed.

## G1 R2 repair validation (2026-09-21)

**R2 fixed:** [Repair and regression evidence](reviews/g1-natural-language-capture-rereview-2.md#r2-repair-evidence). Per-field width budgeting now shares actual prefixes/suffixes with rendering and refreshes horizontal scrolling. Long ASCII/Unicode Home/End editing tests assert the full input/cursor viewport remains visible at narrow, compact, wide, and short sizes; the test failed before the fix and passes afterward. Final suite: 9 packages, 491 top-level test/fuzz entries, 640 passing actions, no skips; race/vet/format/native and cross-build gates plus 16 isolated integration scenarios pass. R1–R4 are resolved. Prior failure sections are historical. Broader dateformat/timezone feasibility evidence and human UAT remain pending; neither is claimed by this focused repair.

## G1 final review (2026-09-22)

**CHANGES REQUESTED — R5:** [Final review and exact reproductions](reviews/g1-natural-language-capture-final-review.md). Fresh existing suite/gates remain green (9 packages, 491 top-level test/fuzz entries, 640 passing actions, no skips; race/vet/build/format; 16 isolated integration scenarios; 30-second fuzz run). An additional 16-profile dateformat/timezone matrix passed interpreter-to-export instant comparisons. Separate real Taskwarrior preview comparisons failed: the helper predicts midnight instead of preserving 15:00, and March 3 instead of Taskwarrior's February 28 for a January 31 monthly anchor. Prior R1–R4 remain resolved. Temporary probes were removed; no implementation changes or live UAT were performed.

## G1 R5 recurrence-preview repair (2026-09-22)

**R5 fixed:** `domain.RecurrenceNext` no longer truncates anchors to midnight and now matches observed Taskwarrior 3.5 calendar/duration behavior. Monthly and annual previews preserve local wall-clock time and clamp month-end/leap-day dates; fixed daily/weekly and numeric day/week/month intervals preserve instants, including across DST (`mo` is Taskwarrior's 30-day duration). Domain regressions, quick-capture review rendering, details goldens, and a permanent 12-case isolated preview-vs-generated-occurrence matrix pass. README explicit-recurrence guidance and the G1 tracker/checklists are reconciled.

Formatting, full tests, race tests, vet, native/Linux AMD64/macOS ARM64 builds, all isolated Integration tests, and `git diff --check` pass. The full suite has 9 packages, 494 top-level test/fuzz entries, 664 passing actions, and no skips; the isolated run has 17 top-level scenarios and 29 passing actions. Tests used disposable `TASKRC`/`TASKDATA`; no production task data or sync server was accessed. Live keyboard/visual UAT was not performed.

## Taskwarrior sync-readiness automated follow-up (2026-09-23)

**PASS automated; not live UAT.** On macOS 27.0.0 arm64 with Go 1.27.1 and Taskwarrior 3.5.0, an isolated profile exposed that `task _get sync.server.url` is not a Taskwarrior DOM reference, while `task _get rc.sync.server.url` is. `SyncConfigured` now checks the `rc.sync.*` references. `TestIntegrationSyncConfiguredReadsTaskwarriorDOMReferences` verifies missing and present settings through temporary `TASKRC`/`TASKDATA`; `momentum doctor` reports the isolated profile configured afterward.

The regression profile uses only `http://127.0.0.1:1`, disables startup sync, and the test performs readiness queries only; it never invokes `task sync` or contacts a real sync server. Format, full tests (9 packages, 495 top-level test/fuzz entries, 665 passing actions, 0 skips), race, vet, native/Linux AMD64/macOS ARM64 builds, the isolated Integration run (18 top-level scenarios, 30 passing actions, 0 skips), and `git diff --check` all pass.

The coding harness has no TTY, so terminal dimensions and human keyboard/visual observations are unavailable. T10 remains unchecked; no live UAT or maintainer verdict is claimed.

## G1 local-only undo follow-up (2026-09-23)

The maintainer reported the earlier live checklist checks as passing but reported that `u` did not undo a newly added task in the disposable local-only profile. The app-level regression `TestLocalOnlyAddCanBeUndoneWithoutStartingSync` reproduced the failure: disabled sync prevented the mutation state from enabling native undo. The local-only state now offers undo for the latest in-app mutation without scheduling sync; the focused regression passes, and isolated native Taskwarrior undo tests pass. **The maintainer reports that `u` now works in the live local-only retest.** The reported observations are not yet a complete T10 record of dimensions, platform, per-criterion outcomes (including the separate missing-UDA profile), and maintainer verdict; T10 remains unchecked.

## G1 T10 live maintainer UAT (2026-09-25)

**Maintainer verdict: PASS; T10 complete.** The maintainer ran the development build in real terminals against disposable local-only Taskwarrior profiles under `/private/tmp/momentum-uat.iizqSt`, explicitly asked to mark T10 done, and reported the capture/review checks passing. This is a maintainer report, not an agent-observed interactive session. The host used for the development build is macOS arm64; the isolated Taskwarrior binary reports 3.5.0. The maintainer reported exercising all requested wide (~120×30), compact (~79×24), narrow (~49×18), and short (~28×8) layouts; exact measured terminal dimensions were not supplied.

| T10 check | Live outcome / evidence |
|---|---|
| Natural due date/time review and correction; explicit priority/estimate precedence | PASS, maintainer-reported; ready-profile export includes captured proposal and a 30-minute estimate overriding an hour phrase. |
| Literal-Friday fallback, invalid/duplicate/bare-time correction, back/cancel and repeated confirmation | PASS, maintainer-reported. Ready-profile export includes the literal-Friday capture; no claim that every negative path is reconstructable from export alone. |
| Wide/compact/narrow/short keyboard review and scrolling | PASS, maintainer-reported at the four requested layout classes; no automated render is substituted for this observation. |
| Recurrence anchor and mixed-field review against generated Taskwarrior instances | PASS, maintainer-reported; ready-profile export includes native recurring templates and instances. |
| Local-only undo | Initially FAIL. App regression reproduced the disabled-sync gate, `SyncState` was corrected, full format/test/race/vet/native/cross-build gates passed, and the maintainer reported `u` working on live retest. No production sync was invoked. |
| Missing Estimate UDA refuses estimate-bearing add | PASS. In the no-UDA profile (`task _get rc.uda.estimate.type` empty), the maintainer supplied a live review screenshot displaying the actionable UDA error and “no estimate mutation was attempted”; the subsequent pending export contains no `Write report` task. The identically named estimate-bearing task found earlier belonged to the separate UDA-ready profile. |
| Ordinary capture without Estimate UDA | PASS. The maintainer supplied no-UDA `status:pending export` showing `Just a test` (UUID `8bbbc650-c320-402e-b842-a7daaf913abf`) in addition to the original seed. |

The automated evidence above remains historical and distinct from these live reports. The temporary-profile wrappers checked `rc.data.location` before launch; neither profile had sync credentials, and Momentum's `[sync].enabled` was false. The remaining readability, Settings, G0, G2, G3, G4, and integrated-flow live UAT work is not marked complete by this G1 verdict.

## UI readability T07 live follow-up (2026-09-25)

**CHANGES REQUESTED; T07 remains in progress.** The maintainer exercised the isolated synthetic UI profile and reported wide/compact/narrow layout, Unicode/ASCII, search/overlays, capture/edit, and task actions as acceptable. Four temporary real-terminal screenshots were reviewed in this session; they show synthetic tasks and a responsive wide/compact/narrow presentation, but are not checked-in release image artifacts. Exact terminal-cell dimensions were not recorded. The Settings/quit item was not explicitly signed off.

Two issues prevent T07 approval: (1) clicking a task reportedly does not change selection, despite existing automated `MouseClickMsg` tests; confirm by clicking a *different* visible task and observing the selection marker, then investigate terminal mouse forwarding versus app hit testing. (2) the light-theme screenshot has dark text/metadata over a visibly dark, transparent-looking terminal background, making contrast uncertain; retest with an opaque light terminal background or fix the app's light-theme surface before signing off. The maintainer described it as “maybe too opaque.” No observed contrast correction or mouse retest is claimed. The required sanitized real-terminal image screenshots also remain to be captured/reviewed for release; fixture-derived text captures do not satisfy that gate.

## UI readability T07 maintainer retest and validation (2026-09-25)

**PASS, maintainer-reported; T07 complete.** The maintainer clarified that clicking a different task changes selection, accepted the light-theme presentation, and confirmed the Settings/quit flow. The earlier concerns above are historical reports, not open mouse defects. The maintainer had already reported the wide/compact/narrow responsive layouts, Unicode/ASCII modes, search/overlays, capture/edit, and task actions as acceptable. This is a real-terminal maintainer verdict, not agent-observed interaction; Nerd Font was not checked live.

Four reviewed real-terminal PNGs from the isolated `/private/tmp/momentum-ui-uat.TxPvxw` synthetic Taskwarrior profile are preserved as `docs/screenshots/uat-wide-dark-ascii.png`, `uat-wide-light-unicode.png`, `uat-compact-dark-unicode.png`, and `uat-narrow-dark-unicode.png`. The images display only fixture tasks and no user task data, paths, usernames, sync identifiers, or secrets. The translucent terminal wallpaper is visible through the light theme and may reduce apparent contrast on dark backgrounds; the maintainer accepted the presentation, but an opaque light terminal background would yield a clearer release image. Exact terminal cell dimensions were not independently measured; screenshot labels are layout classes rather than measured test coordinates. Fixture-derived sanitized text captures remain the deterministic size contract.

Automated formatting, full/unit and race tests, vet, native build, Linux AMD64/macOS ARM64 cross-builds, and `git diff --check` passed on the working tree. T07 is marked validated with these limitations recorded. This verdict does not validate unrelated Settings, G0, G2, G3, G4, or integrated workflow UAT.

## Overall status

G1 natural-language capture and UI readability T07 have automated evidence and maintainer-reported live validation. Release readiness still requires separate live visual/keyboard UAT for Settings, G0 estimate workflow, broader recurrence/template stop, daily planning, and local-ICS degradation. These sign-offs do not imply those other goals passed live UAT.
