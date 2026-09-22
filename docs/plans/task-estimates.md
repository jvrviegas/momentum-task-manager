# Implementation plan — G0 optional task effort estimates

**Created:** 2026-09-20  
**Status:** Implemented; O1–O7 approved on 2026-09-20, automated gates pass; live UAT pending  
**Baseline:** `36b80bb` on `main`; recheck HEAD and working tree before starting  
**Goal:** [G0 — Optional task effort estimates](task-first-daily-planner-goals.md#g0--optional-task-effort-estimates)  
**Requirements:** EST-01–EST-05 from the goal plan

## Handoff objective

Deliver one safe, interoperable estimate concept across Taskwarrior export, quick capture, structured editing, task rows, and details. Leave a typed domain value that G1 natural-language capture and G3 daily planning can reuse without introducing another duration representation.

This tracker is the handoff entry point for the implementing agent. It is an implementation plan, not evidence that G0 is implemented.

## Before starting

1. Read `AGENTS.md`, `CONTRIBUTING.md`, `DESIGN.md`, the G0 section of the goal plan, and this file completely.
2. Recheck HEAD and `git status`. At planning time, `docs/plans/task-first-daily-planner-goals.md` was an intentional untracked documentation change; do not discard or overwrite it.
3. Inspect the current files listed in **Implementation map** rather than assuming the line-level map remains current.
4. Complete the remaining T00 feasibility checks before feature code. O1–O7 were approved by the maintainer on 2026-09-20. If Taskwarrior behavior contradicts the approved contract below, record the evidence and stop for a product decision rather than silently changing storage or syntax.
5. Keep Taskwarrior as the only task store. Never read or write `~/.task`, never run integration tests against real user data, and never pass user input through a shell.
6. Co-locate tests with each change. If commits are requested, follow `AGENTS.md`: stage only the intended file, verify its staged diff, and create a separate commit for each changed file.

## Approved product contract

The maintainer approved O1–O7 on 2026-09-20. T00 must reverify technical feasibility before T01 begins but must not reopen these product choices without contradictory evidence.

| ID | Approved decision | Rationale |
|---|---|---|
| O1 | Persist the value as a Taskwarrior UDA named `estimate` with `uda.estimate.type=duration` and label `Estimate`. | Taskwarrior remains the source of truth and exports duration UDAs canonically. |
| O2 | Represent estimates in Momentum as positive whole minutes, from 1 minute through 24 hours. Accept `15m`, `90m`, `1h`, `1.5h`, and combined `1h30m`; suggestions are `15m`, `30m`, `45m`, `1h`, `2h`, and `4h`. Do not accept `half-day` until G3 defines working-day capacity. | Arbitrary minute estimates are flexible while one canonical integer avoids floating-point and calendar-duration ambiguity. |
| O3 | Add explicit quick-capture trigger `~`, for example `Write report ~1h`; `\~1h` remains literal description text. | Keeps G0 usable before G1 natural-language parsing and follows the existing token-boundary grammar. |
| O4 | Add `E` as the direct Estimate editor shortcut while retaining `e` for Description. Estimate is also reachable by tabbing through the editor. | Preserves existing shortcuts and makes the new common field directly accessible. |
| O5 | Show estimates in comfortable/wide task-row metadata and task details. At compact/narrow widths, description and deadline/priority cues continue to take precedence. | Makes estimates visible without reducing narrow-screen task readability. |
| O6 | Momentum never silently installs or changes the UDA. Estimate mutations are blocked with setup guidance unless the active Taskwarrior configuration reports exactly `duration`; `momentum doctor` reports readiness. | Without the UDA, Taskwarrior treats `estimate:...` as description text and can corrupt the intended mutation. User Taskwarrior configuration must remain explicit. |
| O7 | Outgoing values use `<total-minutes>min`, never `<n>m`. Display values use compact Momentum formatting such as `15m`, `1h`, or `1h 30m`. | In Taskwarrior 3.5.0, `m` means months while `min` means minutes. |

If any decision changes, update this contract, affected acceptance cases, and the goal plan before implementation.

## Acceptance contract

### Domain and storage

| ID | Acceptance criterion |
|---|---|
| G0-01 | A task has either no estimate or one typed estimate measured in positive whole minutes. Absence is distinct from zero; zero and negative user input are rejected. |
| G0-02 | Taskwarrior's exported duration value is decoded into the typed estimate while the original `estimate` JSON remains in `RawFields`. Valid tasks without the field decode unchanged. |
| G0-03 | The domain owns parsing, validation, canonical Taskwarrior serialization, and human formatting. UI and adapters do not implement competing duration parsers. |
| G0-04 | Momentum-created estimates are between 1 minute and 24 hours. Decoding externally created duration values must follow the T00 compatibility decision and must never reinterpret calendar months as minutes. |
| G0-05 | `NewTask`, `EditSnapshot`, `TaskDiff`, and `Task` carry the same estimate type/semantics. Clearing an estimate is distinct from leaving it unchanged. |

### Capture and editing

| ID | Acceptance criterion |
|---|---|
| G0-06 | `~<duration>` is recognized only at a whitespace-delimited token boundary, allows one estimate, and is removed from the final description. Duplicate, empty, zero, negative, malformed, and over-limit estimates produce actionable local errors. |
| G0-07 | Existing triggers and escaping retain their behavior. `\~1h` becomes literal `~1h`; ordinary prose containing `~` is not reinterpreted unless it starts a token. |
| G0-08 | Estimate suggestions are deterministic and keyboard-selectable in quick capture. The compact syntax guide documents `~estimate` without exceeding modal width/height bounds. |
| G0-09 | Structured editing includes Estimate, supports the same parser and suggestions, marks changes, can clear the value, and produces a minimal diff. `E` opens that field directly. |
| G0-10 | Invalid editor input remains editable and causes no Taskwarrior command. Canceling quick capture or editing causes no mutation. |

### Adapter safety and interoperability

| ID | Acceptance criterion |
|---|---|
| G0-11 | Add and modify argv serialize estimates as `estimate:<total-minutes>min`; clear serializes as `estimate:`. They never emit the ambiguous Taskwarrior suffix `m`. |
| G0-12 | Immediately before any add/modify carrying an estimate change, the adapter verifies `rc.uda.estimate.type` is exactly `duration`. Missing or different configuration returns a typed actionable error and executes no mutation command. |
| G0-13 | Tasks without estimate changes preserve current add/modify command behavior and do not require the UDA readiness check. Unsupported/raw fields remain untouched. |
| G0-14 | Estimate mutations use the existing serialized async mutation path, refresh after success, and participate in native sync, undo grace, and unsynced-quit behavior exactly like existing task mutations. |
| G0-15 | The regular `task` CLI can export, display, modify, clear, and synchronize the UDA when both clients carry the documented UDA definition. Momentum does not create a second persistence mechanism. |

### Presentation and operations

| ID | Acceptance criterion |
|---|---|
| G0-16 | Task details show a friendly `Estimate` value and do not also show a duplicate raw `estimate` entry. |
| G0-17 | Comfortable/wide pending-task rows show the estimate in secondary metadata when space permits. Completed-task rows and compact layouts retain their current information priorities unless UAT approves otherwise. |
| G0-18 | Search semantics do not change; estimates are not silently added to fuzzy search in G0. |
| G0-19 | `momentum doctor` reports estimate UDA readiness without mutating Taskwarrior configuration. README/setup documentation explains that the UDA definition is required on every synchronized device. |
| G0-20 | Existing tasks without estimates, existing quick-add syntax, all views, Completed read-only behavior, project settings, synchronization, and responsive rendering retain regression coverage. |

### Deferred acceptance

EST-02's daily-planning clause and EST-03's capacity totals cannot be demonstrated until G3 exists. G0 is complete when it exposes one typed optional estimate that G3 can consume. G3 must count absent estimates separately rather than coercing them to zero.

## Non-goals

- Daily capacity calculations or summing selected work
- Natural-language phrases such as `about an hour`; G1 will consume the G0 parser
- `half-day` or other workday-relative aliases
- Automatic estimate prediction
- Start/end times, time tracking, or calendar time blocks
- Sorting, urgency changes, or search matching based on estimate
- Editing Taskwarrior configuration automatically
- A Momentum config fallback that competes with the Taskwarrior UDA
- Supporting arbitrary Taskwarrior calendar durations as user-entered effort

## Planning-time feasibility evidence

The following probes were run on 2026-09-20 with Taskwarrior 3.5.0 using temporary `TASKRC` and `TASKDATA` paths. The implementing agent must codify and reverify them in T00; do not run probes against real data.

- `uda.estimate.type=duration` accepts `estimate:1h` and exports `"estimate":"PT1H"`.
- `estimate:90min` and `estimate:1.5h` export `PT1H30M`.
- `estimate:1h30m` is rejected by Taskwarrior, so Momentum must normalize its accepted combined syntax to total minutes.
- Taskwarrior interprets `estimate:15m` as 15 months and exported `P450D` in the probe. Momentum must never pass its user-facing `m` suffix through unchanged.
- `estimate:` clears the UDA.
- `_get rc.uda.estimate.type` returned `duration` without exposing a secret.
- With the UDA absent, `task add "Probe" estimate:60min` appended the token to the description. A UUID modify with `estimate:2h` changed the task description rather than the UDA. This proves the readiness guard is a data-safety requirement, not optional diagnostics.
- Removing the UDA definition after a value existed did not remove the exported raw field, but subsequent CLI mutation was unsafe. Definitions therefore need to exist on every device that edits estimates.

T00 must additionally verify supported ISO duration export shapes, external values at/over the Momentum input limit, seconds/day/month/year behavior, and native sync documentation for UDAs.

## Existing implementation map

| Area | Current source | Planning implication |
|---|---|---|
| Export domain | `internal/domain/task.go`, `internal/domain/estimate.go` | `Task` carries an optional typed estimate; unsupported UDA values remain in `RawFields` with a details warning. |
| New task | `internal/domain/new_task.go` | Add one optional typed estimate used by all capture paths. |
| Edit diff | `internal/domain/edit.go` | `EditSnapshot`, `TaskDiff`, `Empty`, `Snapshot`, and `Diff` need estimate support with explicit clear semantics. |
| Quick parser | `internal/quickadd/parser.go` | Trigger enum, trigger recognition, escaping, duplicate tracking, field errors, and `NewTask` population are centralized here. |
| Suggestions | `internal/quickadd/suggest.go` | Add an estimate suggestion kind and fixed preset list; retain deterministic source ordering. |
| Quick UI | `internal/ui/quickadd.go` | Submission already carries `domain.NewTask`; update guidance and suggestion labels/bounds. |
| Structured editor | `internal/ui/edit.go` | The editor uses seven fields through one field-count constant; Estimate is last for stable existing traversal and is directly reachable with `E`. |
| Shortcut routing | `internal/app/edit.go` | Add uppercase `E`; retain lowercase `e` for Description. Help is generated from app key definitions elsewhere and must stay aligned. |
| Taskwarrior mutation | `internal/taskwarrior/mutations.go` | `AddArgs`/`ModifyArgs` build safe argv, but pure builders cannot inspect runtime UDA config. Keep serialization pure and guard execution in `CommandClient.Add/Modify`. |
| Adapter/runtime seam | `internal/taskwarrior/client.go` | Add a read-only UDA readiness method using `_get rc.uda.estimate.type`; avoid broadening the base `Client` unless all fakes are intentionally updated. |
| Mutation orchestration | `internal/app/commands.go`, `model.go` | Existing add/modify path already serializes writes, refreshes, syncs, and handles undo. Reuse it rather than adding estimate-specific orchestration. |
| Details | `internal/ui/details.go` | Show typed estimate and add `estimate` to known raw keys to prevent duplicate display. |
| Rows | `internal/ui/tasklist.go` | Add estimate to comfortable metadata with a deterministic priority; protect compact description/date affordances. |
| Doctor | `internal/diagnostics/doctor.go` | Uses capability interfaces for optional checks; add a read-only estimate configuration check with fake coverage. |
| Integration safety | `internal/taskwarrior/integration_test.go` | `newIsolatedEnvironment` is mandatory and currently writes the Taskwarrior config. Add the UDA only in estimate-specific fixtures or extend the helper explicitly without weakening production-path guards. Tests must not run in parallel. |
| Documentation | `README.md`, `config.example.toml`, `docs/UAT.md`, goal plan | Taskwarrior UDA belongs in `.taskrc`, not Momentum TOML. Update setup, controls, trigger grammar, field behavior, and tracking evidence. |

## Proposed domain boundary

Names may be refined to match code conventions, but responsibilities must remain centralized.

```go
// Estimate is focused work effort, never a wall-clock start/end range.
type Estimate struct {
    Minutes int
}

func ParseEstimate(text string) (Estimate, error)
func ParseTaskwarriorEstimate(text string) (Estimate, error)
func (e Estimate) TaskwarriorValue() string // e.g. "90min"
func (e Estimate) String() string           // e.g. "1h 30m"
```

Use optionality explicitly (`*Estimate` or an equivalent value-plus-presence type). Do not use a bare `time.Duration` with zero meaning both absent and invalid. Do not let UI packages parse durations independently.

For exported ISO 8601 values, either implement the deliberately narrow supported grammar in the domain package or choose a small dependency only after documenting why standard-library code is insufficient. Do not introduce a general date/calendar duration dependency for a minute-based effort field without evidence.

## Execution map

```text
T00 Policy confirmation and Taskwarrior feasibility
 ├──> T01 Domain estimate contract
 │     ├──> T03 Quick-capture slice
 │     ├──> T04 Structured-edit slice
 │     └──> T05 Presentation slice
 └──> T02 UDA readiness and safe adapter persistence
       ├──> T03 Quick-capture slice
       └──> T04 Structured-edit slice

T03 + T04 + T05 ──> T06 Doctor, documentation, and end-to-end validation
```

T01 and T02 can be developed sequentially. Do not parallelize files shared by quick capture/editor app tests. Real Taskwarrior integration tests remain non-parallel because they manipulate process environment.

## Tracker

Change `[ ]` to `[x]` only after the task's tests and completion criteria pass. Record concrete command output and commit identifiers if commits are created.

| Done | ID | Deliverable | Depends on | Evidence / commit |
|---|---|---|---|---|
| [x] | T00 | Confirm product decisions and reverify Taskwarrior duration-UDA semantics | — | Taskwarrior 3.5.0 isolated probes recorded above; domain/adapter integration tests pass |
| [x] | T01 | Add the typed estimate domain model, export decoding, formatting, and edit diffs | T00 | `go test ./internal/domain -count=1` PASS |
| [x] | T02 | Add UDA readiness checks and safe add/modify/clear adapter support | T00, T01 | `go test ./internal/taskwarrior -count=1` and isolated Integration run PASS |
| [x] | T03 | Deliver quick-capture estimate parsing, suggestions, UI, and app integration | T01, T02 | `go test ./internal/quickadd ./internal/ui ./internal/app -count=1` PASS |
| [x] | T04 | Deliver structured estimate editing and direct shortcut integration | T01, T02 | UI/app/domain/taskwarrior tests and full race gate PASS |
| [x] | T05 | Render estimates in details and responsive task rows | T01 | UI render fixtures and width assertions PASS |
| [x] | T06 | Add doctor/setup documentation, end-to-end UAT evidence, and final regression gates | T03, T04, T05 | All required format/test/race/vet/build gates PASS; live UAT pending |

## Task details

### T00 — Policy confirmation and isolated feasibility

**Where:** this plan, goal tracker, temporary Taskwarrior fixtures; no production task data.  
**Requirements:** EST-01–EST-05, G0-01–G0-20.  
**Deliverable:** confirmed O1–O7 and a feasibility record that fixes the supported external-duration behavior.

- [x] Maintainer confirmed O1–O7 on 2026-09-20.
- [x] Reproduce the planning-time probes against the available Taskwarrior 3.x version.
- [x] Verify `_get rc.uda.estimate.type` results for missing, `string`, `numeric`, and `duration` definitions and define typed readiness outcomes.
- [x] Verify export shapes for minutes, mixed hours/minutes normalized by Momentum, 24 hours, values over 24 hours created externally, days, seconds, months, years, null, malformed JSON types, and field absence.
- [x] Decide and record whether valid external values over Momentum's input limit remain displayable/editable, and whether unsupported calendar-duration values degrade to raw-only display or fail export. Preserve task-list availability with an explicit details warning.
- [x] Verify add, modify, clear, undo, and export using the UDA in the isolated integration harness.
- [x] Confirm from Taskwarrior documentation that UDA values sync with task data while UDA definitions must be configured on each client; no live sync server is required for the test suite.
- [x] Capture exact commands, Taskwarrior version, exit codes, and redacted outputs in this plan.

#### T00 feasibility record — 2026-09-21

Environment: macOS, `/opt/homebrew/bin/task`, Taskwarrior `3.5.0`; every probe used a temporary `TASKRC` and `TASKDATA` and was removed after the command. No real Taskwarrior path or sync server was used.

Commands and redacted results:

```text
TASKRC=<temporary> TASKDATA=<temporary> task --version
exit 0: 3.5.0

TASKRC=<temporary duration UDA> TASKDATA=<temporary> task _get rc.uda.estimate.type
exit 0: duration

TASKRC=<temporary missing UDA> TASKDATA=<temporary> task _get rc.uda.estimate.type
exit 0: empty stdout

TASKRC=<temporary string UDA> TASKDATA=<temporary> task _get rc.uda.estimate.type
exit 0: string

TASKRC=<temporary numeric UDA> TASKDATA=<temporary> task _get rc.uda.estimate.type
exit 0: numeric

TASKRC=<temporary duration UDA> TASKDATA=<temporary> task add "Probe" estimate:1h
exit 0: task created; export estimate PT1H

TASKRC=<temporary duration UDA> TASKDATA=<temporary> task add "Probe" estimate:90min
exit 0: task created; export estimate PT1H30M

TASKRC=<temporary duration UDA> TASKDATA=<temporary> task add "Probe" estimate:1.5h
exit 0: task created; export estimate PT1H30M

TASKRC=<temporary duration UDA> TASKDATA=<temporary> task add "Probe" estimate:1h30m
exit 2: The duration value '1h30m' is not supported.

TASKRC=<temporary duration UDA> TASKDATA=<temporary> task add "Probe" estimate:15m
exit 0: task created; export estimate P450D

TASKRC=<temporary duration UDA> TASKDATA=<temporary> task modify <uuid> estimate:
exit 0: estimate omitted from subsequent export
```

The duration probes also showed `24h`, `1440min`, and `86400s` export as `P1D`; values over the Momentum creation limit remain representable (`25h` → `P1DT1H`); `86401s` exports `P1DT1S`; and mixed `1d2h3min` is rejected by Taskwarrior. The official [UDA documentation](https://taskwarrior.org/docs/udas/) states that UDAs are preserved as orphan data when a replica lacks the definition and cannot be manipulated there; the [duration documentation](https://taskwarrior.org/docs/durations/) distinguishes date-component `M` (months) from time-component `M` (minutes); the [sync documentation](https://taskwarrior.org/docs/sync/) confirms that task data syncs between replicas while configuration must be set up on each client.

Decision recorded for T01: Momentum-created values accept positive whole minutes from 1 through 1440 and serialize as `<minutes>min`. Export decoding accepts precise positive ISO durations composed of days, hours, minutes, and seconds when they resolve to whole minutes, including external values above 1440; calendar months/years, negative/zero values, fractional minutes, and non-whole-minute seconds remain raw-only with a details warning rather than failing the whole task export. `P1D` is 1440 minutes; `P1M` is never treated as one minute. T02 integration coverage now codifies add/modify/clear/undo and the refusal path.

**Gate:** passed with temporary paths only; the readiness probe distinguishes configured, missing, wrong-type, and unavailable outcomes.

### T01 — Typed domain estimate

**Where:** proposed `internal/domain/estimate.go`, `estimate_test.go`; `task.go`, `task_test.go`, `new_task.go`, `edit.go`, `edit_test.go`.  
**Requirements:** G0-01–G0-05.

- [x] Implement one typed optional estimate with positive whole-minute validation and the confirmed upper input bound.
- [x] Parse all approved user forms into total minutes and reject empty/zero/negative/fractional-minute/overflow/over-limit input with actionable errors.
- [x] Format a compact human value and serialize a Taskwarrior-safe `<minutes>min` value.
- [x] Decode confirmed Taskwarrior ISO duration export shapes without confusing date-component `M` (months) with time-component `M` (minutes).
- [x] Add the optional estimate to `Task` while preserving raw `estimate` JSON.
- [x] Add the estimate to `NewTask`, `EditSnapshot`, `TaskDiff.Empty`, `Snapshot`, and `Diff`, including unchanged/set/clear cases.
- [x] Keep unsupported and unrelated raw fields untouched.
- [x] Table-test all approved aliases, boundaries, canonical formatting, ISO shapes, absent/null values, malformed values, and diff semantics.

**Independent demo:** decode a Taskwarrior fixture containing `PT1H30M`, display `1h 30m`, edit it to `45m`, and produce only one estimate field change with outgoing value `45min`.

**Gate:** `go test ./internal/domain -count=1`; formatting check for touched Go files.

### T02 — UDA readiness and safe Taskwarrior persistence

**Where:** `internal/taskwarrior/client.go`, `mutations.go`, corresponding tests, estimate-specific isolated integration tests; capability interfaces as needed.  
**Requirements:** G0-11–G0-15, G0-19.

- [x] Add a read-only typed readiness query using `_get rc.uda.estimate.type`; distinguish configured, missing, wrong type, command failure, and timeout without logging task content.
- [x] Extend pure add/modify argv builders with set/clear estimate serialization using only the domain's Taskwarrior-safe value.
- [x] In `CommandClient.Add` and `Modify`, run the readiness guard immediately before an estimate-bearing mutation. On non-ready/error outcomes, execute no mutation and return setup guidance.
- [x] Do not run the readiness query for mutations that do not add, set, or clear an estimate.
- [x] Retain UUID targeting, argument-vector execution, hooks, normal Taskwarrior validation, and command redaction.
- [x] Test exact runner call order and argv for add/set/clear, missing/wrong UDA, readiness command failure, mutation failure, and ordinary tasks without estimates.
- [x] Add isolated integration coverage for add/export, modify/export, clear/export, undo, and the proven missing-UDA corruption case as a guarded negative test. The negative test must assert Momentum refuses before mutation; it must not intentionally corrupt a fixture through the production adapter.
- [x] Keep all environment-based integration tests non-parallel and retain guards against `~/.taskrc` and `~/.task`.

**Independent demo:** with a temporary configured UDA, add `1h`, modify to `90m`, clear, and undo. With the UDA removed, show an actionable error and prove the task description and data remain unchanged.

**Gate:** `go test ./internal/taskwarrior -count=1`; `go test ./internal/taskwarrior -run Integration -v -count=1` against available Taskwarrior 3.x.

### T03 — Quick-capture vertical slice

**Where:** `internal/quickadd/parser.go`, `suggest.go`, tests; `internal/ui/quickadd.go`, tests/snapshots; `internal/app/quickadd_runtime_test.go` and relevant mutation tests.  
**Requirements:** G0-06–G0-08, G0-10–G0-14.

- [x] Add `~` to the token-boundary trigger grammar, field naming, escaping, scalar duplicate detection, and parsed `NewTask` output.
- [x] Delegate parsing to the domain estimate parser; map failures to a typed quick-add error without losing the entered command text.
- [x] Add fixed deterministic estimate suggestions and Tab acceptance through the existing suggestion context.
- [x] Update idle guidance and suggestion display while preserving all supported terminal bounds and existing trigger discoverability.
- [x] Ensure existing literal/escaped trigger, email, duplicate, empty-description, project, priority, date, scheduled, and tag cases remain unchanged.
- [x] Verify the root app routes the parsed estimate through the existing serialized add mutation, refresh, sync grace, and error display paths.
- [x] Test configured-UDA success and missing/wrong-UDA failure with fakes at the app seam; failed capture must remain actionable and must not claim task creation.

**Independent demo:** enter `Prepare proposal #work ~1h30m`, accept/submit, and observe a Taskwarrior add argv containing `estimate:90min`. Enter `Prepare proposal \~1h` and observe literal description text with no estimate.

**Gate:** `go test ./internal/quickadd ./internal/ui ./internal/app -count=1`; update only intentional render fixtures.

### T04 — Structured-edit vertical slice

**Where:** `internal/ui/edit.go` and tests; `internal/app/edit.go`, edit/action tests; domain/adapter tests only where coverage is not already present.  
**Requirements:** G0-05, G0-09–G0-14.

- [x] Add Estimate to the editor's field enum, input storage, names, open/current snapshot flow, field-window bounds, change markers, validation, suggestions, and rendering.
- [x] Remove hard-coded six-field assumptions safely. Prefer a single field-count constant or array length so later recurrence work does not repeat index bugs.
- [x] Use the domain parser for validation and canonical snapshot values; retain the user's invalid text and error in the modal.
- [x] Support clearing, unchanged values, direct preset acceptance, and arbitrary approved values.
- [x] Route uppercase `E` to Estimate and update generated help/keybinding definitions; lowercase `e` remains Description and Details→Edit behavior remains unchanged.
- [x] Verify completed tasks remain read-only and Settings cannot target a hidden task.
- [x] Verify successful saves use one existing modify mutation and failed readiness checks preserve task state with actionable status.
- [x] Cover first/last field traversal, short-terminal scrolling, suggestion navigation, changed markers, invalid values, clear/cancel/save, shortcut routing, and app mutation integration.

**Independent demo:** select an existing task, press `E`, set `45m`, save, refresh, reopen, clear it, and verify only the estimate changed each time.

**Gate:** `go test ./internal/ui ./internal/app ./internal/domain ./internal/taskwarrior -count=1`.

### T05 — Details and responsive row presentation

**Where:** `internal/ui/details.go`, `tasklist.go`, readability/render fixture tests.  
**Requirements:** G0-16–G0-18, O5.

- [x] Show a friendly Estimate in details using the domain formatter.
- [x] Mark raw `estimate` as known so it is not duplicated under technical properties. If T00 permits unsupported raw-only external values, present an explicit safe fallback exactly once.
- [x] Add estimate metadata to comfortable pending rows with a stable ordering relative to project, due, scheduled, and priority.
- [x] Preserve completed-row completion metadata and compact/narrow prioritization unless an explicit UAT change is recorded.
- [x] Ensure long estimates/metadata truncate by display width and selected full-row styling remains intact.
- [x] Add absent/present, selected, completed, comfortable, compact, narrow, malformed-external, and width-bound tests.
- [x] Update golden fixtures only after inspecting every changed line; do not regenerate unrelated snapshots blindly.

**Independent demo:** inspect the same estimated task at wide, compact, and narrow widths and in Details; the description remains primary and Estimate appears once where intended.

**Gate:** `go test ./internal/ui -count=1`; intentional render fixtures reviewed.

### T06 — Diagnostics, docs, UAT, and final gates

**Where:** `internal/diagnostics/doctor.go` and tests; `README.md`, `docs/UAT.md`, relevant design/goal docs, and configuration guidance.  
**Requirements:** EST-01–EST-05, G0-19–G0-20.

- [x] Add an optional read-only doctor capability check for the estimate UDA. Report ready, missing with exact setup lines, wrong type, or unavailable. Do not mutate `.taskrc` or print unrelated Taskwarrior configuration.
- [x] Document these lines for every device that edits/syncs estimates:

  ```text
  uda.estimate.type=duration
  uda.estimate.label=Estimate
  ```

- [x] Document `~estimate`, accepted input forms/range, `E`, display formatting, clearing, and the `m` versus `min` normalization.
- [x] Explain that UDA values are task data while the UDA definition is per-device configuration; Momentum blocks unsafe estimate writes when it is absent or wrong.
- [x] Update `DESIGN.md` where the supported task model, fields, quick-add grammar, details, controls, diagnostics, and acceptance criteria currently enumerate the v1 surface.
- [x] Update the G0 tracker row to Implemented only after automated gates pass; add links to this plan and UAT evidence. Mark Validated only after live maintainer UAT.
- [x] Record automated acceptance evidence and any live-UAT limitation in `docs/UAT.md` with traceability to G0-01–G0-20.
- [x] Run all final gates and record exact results below.

**Required final gates:**

```sh
test -z "$(gofmt -l .)"
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/momentum
GOOS=linux GOARCH=amd64 go build ./cmd/momentum
GOOS=darwin GOARCH=arm64 go build ./cmd/momentum
go test ./internal/taskwarrior -run Integration -v -count=1
```

Automated gate results on 2026-09-21: formatting, all eight package tests, race tests, vet, native build, Linux AMD64 cross-build, macOS ARM64 cross-build, and all 14 isolated Taskwarrior integration scenarios passed. No commit was created; the working tree retained the pre-existing untracked goal-plan documents.

**Live UAT:** capture `~1h30m`; inspect row/details; edit with `E`; clear; undo during sync grace; verify missing-UDA refusal in a disposable Taskwarrior profile; verify normal tasks remain usable without the UDA.

## Test matrix

| Layer | Required coverage |
|---|---|
| Domain | User parser, ISO export parser, boundaries, formatting, absence, raw retention, snapshot/diff set-clear-unchanged |
| Quick add | Trigger boundaries, escape, duplicate, invalid values, suggestions, insertion, unchanged existing grammar |
| Taskwarrior unit | Readiness states, exact argv, total-minute serialization, guard-before-mutation, no guard for unrelated mutations |
| Taskwarrior integration | Configured add/modify/clear/undo/export; missing-UDA refusal; temporary paths only |
| UI editor | Seventh field traversal, direct shortcut, presets, arbitrary value, invalid retention, clear/cancel/save, all widths |
| UI presentation | Details once, comfortable row, compact/narrow precedence, completed behavior, truncation/selection |
| App | Add/modify lifecycle, refresh, sync grace, undo, errors, Completed/Settings routing safety |
| Diagnostics | Ready/missing/wrong/unavailable without mutation or sensitive output |
| Regression | Existing views, search, quick triggers, project settings/migration, sync, render snapshots |

## Risks and controls

| Risk | Required control |
|---|---|
| Taskwarrior treats an unknown UDA token as description text | Runtime readiness guard immediately before every estimate-bearing add/modify; no mutation on non-ready state |
| `m` means months to Taskwarrior | Domain serializes total minutes with `min`; exact argv tests prohibit ambiguous `m` |
| Multiple duration parsers drift | Domain parser/formatter is the only semantic implementation used by quick add and editor |
| External UDA contains unsupported calendar units | Resolve in T00; preserve task visibility and raw data without silently reinterpreting units |
| Editor fixed-array index regression | Centralize field count/bounds and test full forward/backward traversal at short heights |
| Added metadata harms readability | Comfortable-only display, compact priority rules, Lip Gloss width assertions, inspected snapshots |
| UDA definition differs across devices | Doctor check and explicit per-device setup documentation; block writes on wrong/missing type |
| Future G1/G3 invent another duration model | Export the typed domain estimate and document that later goals must consume it directly |

## Completion definition

G0 reaches **Implemented** when T00–T06 are complete, G0-01–G0-20 have recorded automated evidence (except explicitly deferred G3 behavior), all required gates pass, and documentation accurately describes setup and limitations.

G0 reaches **Validated** only after the maintainer completes live keyboard/visual UAT on a real configured profile and confirms that the capture/edit/display workflow is useful. Do not mark G1 or G3 complete as part of this work.
