# Implementation plan — G1 natural-language command-bar capture

**Created:** 2026-09-21  
**Status:** Validated — T00–T10 complete; maintainer-reported live keyboard/visual UAT recorded in `docs/UAT.md`
**Baseline:** `36b80bb` on `feat/task-estimates`, with the uncommitted G0 implementation present; recheck HEAD and working tree before starting  
**Goal:** [G1 — Natural-language command-bar capture](task-first-daily-planner-goals.md#g1--natural-language-command-bar-capture)  
**Requirements:** NLC-01–NLC-08 from the goal plan

## Independent validation — 2026-09-21

**Final review R5 resolved.** The [2026-09-22 final pass](../reviews/g1-natural-language-capture-final-review.md) confirmed R1–R4 and found that recurrence previews dropped time-of-day and disagreed with Taskwarrior at month end. `domain.RecurrenceNext` now mirrors observed Taskwarrior 3.5 calendar/duration semantics. Permanent domain, review-rendering, and 12-case isolated differential integration coverage verifies time-of-day, month-end/leap-year clamping, supported intervals, and DST transitions. The README now correctly documents explicit `^` fast capture. Full, race, vet, native/cross-build, integration, formatting, and diff gates pass.

T10 live maintainer acceptance was recorded on 2026-09-25 in `docs/UAT.md`. Earlier automated evidence remains distinct from the maintainer's real-terminal observations.

## Handoff objective

Extend quick capture with a small, local, deterministic English grammar. Present inferred fields separately from the remaining description and require review before inferred metadata reaches Taskwarrior. Preserve the explicit-trigger fast path, reuse G0 estimates, and consume—not invent—the G2 recurrence contract.

This file combines the feature specification and trackable implementation plan, following the G0 tracker. O1–O9 approvals and the completed T00–T10 evidence are recorded below; the historical prerequisites are retained for traceability.

## Before starting

1. Read `AGENTS.md`, `CONTRIBUTING.md`, `DESIGN.md`, the G1 goal, this file, and the G0 estimate contract completely.
2. Recheck HEAD and `git status --short`. At planning time, 42 modified/untracked paths contained the G0 implementation and documentation. Preserve that work; do not reset, overwrite, stage, or commit it as part of G1 planning.
3. Reinspect the implementation map; it describes the current working tree, not just HEAD.
4. Follow the approved O1–O9 contract and complete T00 feasibility. Stop for a decision if feasibility contradicts the approved behavior.
5. Treat G2 recurrence as an external blocker. Do not add an independent recurrence field model, parser, scheduler, or unsafe raw `recur:` pass-through to bypass it.
6. Use temporary `TASKRC` and `TASKDATA` for every Taskwarrior probe/integration test. Retain the production-path guards, never run these tests in parallel, and never create a sync server or run production sync.
7. Co-locate tests with each implementation task. If commits are requested, stage one intended file, verify its staged diff, and create one Conventional Commit per changed file, as required by `AGENTS.md`.

## Delivery boundary and external gates

| Slice | Deliverable | Gate |
|---|---|---|
| G1a | Dates/times, priority shorthand, G0 effort phrases, explicit precedence, lossless interpretation, review/correction, safe submission | O1–O9 approved; G0 domain and guarded adapter available; T00–T07 and T09 pass |
| G1b | Supported recurrence phrases populate G2's recurrence definition and review its anchor/next occurrence | G2 contract and tested creation path available; T08 plus rerun T09 pass |
| Full G1 | NLC-01–NLC-08 implemented and subsequently live-validated | Both slices complete; T10 live UAT passes for Validated status |

G1a may ship independently under approved O9. It must be labeled a partial delivery, not full G1 Implemented. NLC-02 remains blocked, not waived. G0 is currently Implemented with live UAT pending; G1 must preserve that distinction and cannot claim to validate G0 by implication.

## Approved product contract

O1 was approved by the maintainer on 2026-09-21: case-insensitive English, machine-local timezone, unambiguous ISO dates, ambiguous numeric dates retained as prose, and local parsing without network/LLM processing. O2 was approved by the maintainer on 2026-09-21: inferred dates set Due, date-only values use local midnight, bare weekdays include today, `next <weekday>` means the following Monday–Sunday calendar week, and bare times require a date. O3 was approved by the maintainer on 2026-09-21: plain/explicit-trigger capture retains immediate Enter submission; natural-language interpretation opens review showing description, fields, exact dates/timezone, provenance, and conflicts; Ctrl+S confirms only a valid draft. O4 was approved by the maintainer on 2026-09-21: explicit scalar triggers override inference regardless of order, including `!none` and `~`; conflicting inferred phrases remain description text with a warning; duplicate explicit fields remain errors; multiple inferred values require correction or selection. O5 was approved by the maintainer on 2026-09-21: recognize approved `about`/`for` effort phrases and hour/minute aliases through G0's parser, preserving its positive whole-minute 1–1440 limit and UDA safety checks; bare durations, `half-day`, and calendar units remain prose; recognized invalid values require correction or explicit-only capture. O6 was approved with revisions by the maintainer on 2026-09-21: standalone case-insensitive `p1`/`p2`/`p3` mean High/Medium/Low; no priority is represented by omitting a priority indication, with no `p4` shorthand. Priority phrases remain prose. Existing explicit priority triggers, including `!none`, remain supported for compatibility and precedence. O7 was approved by the maintainer on 2026-09-21: review supports keyboard editing/clearing, return to original input, explicit-syntax-only capture preserving natural-language prose while processing existing triggers, and cancellation without mutation. O8 was approved by the maintainer on 2026-09-21: each interpretation uses a fresh local reference time, reviewed inferred dates remain frozen through confirmation, reinterpreting uses a fresh reference time, and nonexistent/ambiguous DST wall times require correction. O9 was approved by the maintainer on 2026-09-21: deliver G1a before G2, preserve reserved recurrence phrases as prose with an unsupported notice, and complete G1b using G2's shared semantics before marking full G1 complete. All O1–O9 are approved; the isolated date/DST feasibility record and fixtures are implemented in the interpreter test suite.

| ID | Approved decision | Rationale / implementation constraint |
|---|---|---|
| O1 | First release recognizes case-insensitive English phrases only, with machine-local timezone and an injected reference time. No locale guessing or ambiguous numeric dates. | Deterministic offline behavior; translated grammars are out of scope. |
| O2 | Infer Due, not Scheduled, from supported unqualified date/time phrases. Date-only inference resolves to local midnight. Bare weekday means the next occurrence including today; `next <weekday>` means that weekday in the following Monday–Sunday calendar week. A bare time requires a date rather than silently choosing today/tomorrow. | Distinguishes deadlines from G3 commitments. Alternative weekday/midnight policies must be settled before coding. |
| O3 | Any recognized natural-language candidate opens review on Enter; plain text and explicit-trigger-only input keep one-Enter capture. Review shows exact date/time and timezone, provenance, conflicts, and remaining description. Ctrl+S confirms only a valid reviewed draft. | Avoids claiming confidence from a heuristic score; one extra confirmation for inference. |
| O4 | Explicit scalar triggers win over inference, including `!none` and G0 `~`. Duplicate explicit scalars remain errors. Conflicting inferred phrases remain description text with a visible warning; they are not silently discarded. Multiple inferred candidates for one field require correction or explicit selection. | Deterministic precedence independent of token order; explicit absence must not look like no explicit value. |
| O5 | Support `about <duration>` and `for <duration>` using the G0 parser, with a small adapter for `a/an hour`, numeric minutes/hours, and their singular/plural spellings. Preserve G0's 1–1440 whole-minute limit and guarded UDA persistence. | No competing duration semantics; `half-day`, bare `1h`, and calendar units remain prose. |
| O6 | Recognize standalone `p1` → High, `p2` → Medium, and `p3` → Low instead of priority phrases. No indication means no priority; there is no `p4` shorthand. Existing `!high`/`!medium`/`!low`/`!none` remain supported; projects and tags still require explicit triggers. | Approved with revisions. Match whole tokens case-insensitively, not substrings; unsupported tokens such as `p4` remain prose. |
| O7 | Review allows keyboard editing/clearing of the draft fields, returning to the original input, and choosing “Use explicit syntax only.” The latter parses existing triggers but leaves all natural-language prose literal. Cancellation makes no mutation. | A guaranteed escape hatch for titles such as `Discuss Friday` without introducing quote/backslash grammar that breaks current capture. |
| O8 | Capture a fresh local reference time at each interpretation attempt; freeze it for that review. Confirmation uses the reviewed absolute inferred values without reparsing relative dates. Going back and interpreting again obtains a new reference time. Nonexistent/ambiguous DST wall times require correction, not silent normalization. | Prevents stale startup dates and preview/commit drift across midnight or DST. |
| O9 | Deliver G1a before G2 if desired; until G2 is ready, recurrence phrases remain prose and receive an unsupported-recurrence notice for the reserved patterns. Never claim a recurring task was created. After G2, recognized recurrence always requires review using G2's anchor and preview rules. | Keeps basic capture independently deliverable while making NLC-02's dependency explicit. |

### Proposed grammar boundary

This is a whitelist, not a promise to understand arbitrary prose. T00 must turn it into approved example fixtures. Match complete phrases at word boundaries; never inside URLs, emails, explicit trigger values, or escaped trigger tokens. Preserve original capitalization and punctuation in unconsumed text. Whitespace normalization may retain today's `Parse` behavior, but words may not disappear.

| Field | Supported candidates | Excluded / correction behavior |
|---|---|---|
| Due date | `today`, `tomorrow`, full weekday names, `next <weekday>`, ISO `YYYY-MM-DD`, `in <positive integer> day/days` | No `03/04`, month-name dates, abbreviations, `next week`, `this Friday`, or business-day arithmetic in this release; unsupported phrases remain intact |
| Due time | `at 3pm`, `at 3:30pm`, `at 15:00`, combined with one supported date | `at 3`, standalone time, invalid hour/minute, or multiple time candidates require correction; do not pick a date or AM/PM silently |
| Estimate | `about 1h`, `about 1h30m`, `about an hour`, `for 30 minutes`, `for 1.5 hours` | Recognized numeric duration forms failing G0 validation remain visible with an error; unsupported units/phrases remain prose |
| Priority | Standalone `p1` (High), `p2` (Medium), `p3` (Low); omit indication for no priority | `p4`, priority phrases such as `high priority`, bare `high`, `urgent`, and `important` remain prose; existing explicit priority triggers remain supported |
| Recurrence, after G2 | Candidate vocabulary: `every day`, `every weekday`, `every week`, `every <weekday>`, `every month`, `every <positive integer> weeks` | Final accepted forms and mappings are blocked on G2; unsupported combinations remain prose, never guessed |
| Explicit metadata | Existing `#`, `!`, `@`, `>`, `+`, `~`, including escaping | Existing duplicate/error rules and Taskwarrior date-expression pass-through remain unchanged |

Do not implement the exclusions by accidentally extracting a shorter supported phrase from them: e.g. `this Friday` must not become description `this` plus an inferred date. Prefer longest complete supported/reserved phrase recognition. A comma or separator can be consumed only when its ownership by an accepted metadata clause is explicit and tested; otherwise preserve it in the description.

### Example acceptance fixtures (proposed)

Reference time: Monday 2026-09-21 10:00 in a fixed test timezone. Exact instants must be asserted using that timezone, not the test machine's clock.

| Input | Expected interpretation / submission behavior |
|---|---|
| `Prepare proposal tomorrow at 3pm #work` | Review description `Prepare proposal`, project `work`, Due 2026-09-22 15:00 local |
| `Write report about 1h30m` | Review description `Write report`, typed Estimate 90 minutes; adapter emits `estimate:90min` only after UDA guard |
| `Call Friday` | Review Due 2026-09-25 00:00; “Use explicit syntax only” instead creates literal description `Call Friday` |
| `Call next Friday` | Review Due 2026-10-02 00:00 under O2 |
| `Call at 3pm` | Unresolved date; no submission until corrected or explicit-only capture selected |
| `Call tomorrow @2026-09-30` | Explicit Due wins; `tomorrow` remains description text with a conflict notice |
| `Write report about 1h ~30m` | Explicit Estimate 30 minutes wins; `about 1h` remains description text with notice |
| `Write report p1 !none` | Explicit no-priority wins; inferred phrase remains description text with notice |
| `Call tomorrow Friday` | Two inferred Due candidates; no silent first/last-wins selection |
| `Discuss this Friday` | Unsupported phrase remains intact; do not extract its `Friday` substring |
| `Email bob@example.com about the Friday release` | Email survives; any weekday candidate is reviewable and reversible, never an immediate mutation |
| `Write report \\~1h` | Existing escape yields literal `~1h`; no inferred estimate |
| `Write report for 0 minutes` | Recognized invalid estimate retains its source and reports G0 validation; no mutation until corrected/bypassed |
| `Pay rent every month` before G2 | Recurrence phrase remains intact with a notice; review must not advertise a recurrence field value |
| `Prepare proposal tomorrow at 3pm, p1, every Friday, about 1h #work` after G2 | Review description and all metadata separately; G2 must resolve/validate recurrence anchor interaction before confirmation |

## Acceptance contract

### Interpretation and semantics

| ID | Acceptance criterion |
|---|---|
| G1-01 | Given the same input, approved policy, reference instant, and location, interpretation returns the same draft, consumed spans, provenance, and diagnostics without I/O, network, subprocesses, or a remote LLM. |
| G1-02 | Supported date phrases resolve according to O1/O2/O8 with calendar arithmetic, including month/year rollover and leap days; unsupported dates remain prose. |
| G1-03 | Time inference combines with an unambiguous date, validates ranges and DST transitions, and never silently chooses a date, meridiem, or alternate wall time. |
| G1-04 | Inferred dates are serialized to a Taskwarrior-verified unambiguous absolute format; reviewed dates cannot change between confirmation and mutation. Explicit date expressions retain existing Taskwarrior validation and are labeled expressions rather than falsely resolved previews. |
| G1-05 | Effort phrases use the existing domain Estimate parser/type/range/formatter/serialization and retain missing/wrong-UDA refusal. |
| G1-06 | Only standalone `p1`/`p2`/`p3` infer priority; omission means no priority and `p4` remains prose; projects/tags and Scheduled remain explicit-only in G1. |
| G1-07 | All six existing triggers, escaping, duplicate scalar rejection, unique tags, suggestions, and unknown project/tag acceptance retain their behavior. Presence is tracked separately from empty priority so `!none` overrides inference. |
| G1-08 | Explicit metadata wins independently of token order; overridden/conflicting inferred source text remains visible and accounted for. Multiple inferred scalar candidates never silently choose one. |
| G1-09 | Every removed source span corresponds to accepted metadata or an explicitly owned separator. Unrecognized prose, unsupported clauses, Unicode, and escaped tokens remain description text; no overlapping/double-consumed spans. |
| G1-10 | Empty descriptions and recognized invalid/incomplete metadata produce actionable diagnostics and prevent confirmation until corrected or intentionally captured through explicit-only mode. |

### Review and correction

| ID | Acceptance criterion |
|---|---|
| G1-11 | Plain/explicit-only input retains the existing immediate submission path. Any inferred/reserved candidate, conflict, or inference error enters review without executing a mutation. |
| G1-12 | Review displays remaining description, inferred and explicit fields, field provenance, exact inferred dates/timezone, and all blocking diagnostics. No metadata that will be submitted is silently hidden. |
| G1-13 | All supported draft fields can be corrected/cleared by keyboard; unresolved candidates can be selected, returned to prose, or removed only by explicit user action. Original capture input remains recoverable. |
| G1-14 | Explicit-only capture preserves natural-language prose, still validates explicit triggers, and requires a deliberate choice; it does not execute arbitrary Taskwarrior arguments. |
| G1-15 | Enter opens review, Ctrl+S confirms a valid draft, returning to input invalidates the old draft, and cancellation produces zero mutation calls. Suggestion keys and Esc dismissal remain coherent and documented. |
| G1-16 | Stale parse/review messages, changed inputs, closed/reopened overlays, repeated confirmation, and mutation-busy state cannot submit an obsolete draft or create duplicate tasks. |
| G1-17 | Review is navigable and bounded at wide, compact, narrow, and short terminal sizes; scrolling exposes all fields/diagnostics, with no hidden confirmation. Too-small layouts refuse unsafe confirmation rather than suppress required review. |

### Mutation, recurrence, and operation

| ID | Acceptance criterion |
|---|---|
| G1-18 | Confirmed drafts use the existing serialized add/refresh/sync/undo/unsynced-quit lifecycle, argv execution, hooks, finite timeouts, and Taskwarrior final validation; no shell or direct database access. |
| G1-19 | Invalid drafts and canceled/stale submissions call no mutation. Adapter/hook failures show honest errors and retain a recoverable draft without automatic retries that could duplicate an add. |
| G1-20 | After G2 is available, approved recurrence phrases populate exactly G2's creation model and review anchor/next occurrence using G2 semantics. Conflicts between due and recurrence anchors require correction under that contract. |
| G1-21 | Before G2, reserved recurrence phrases remain prose with an explicit unsupported notice and cannot generate recurrence argv. Unsupported post-G2 phrases also remain intact. |
| G1-22 | Capture remains local and usable offline; parsing does not log task content. Existing views, search, editing, project settings, Completed capture, synchronization, and ordinary estimate-free tasks retain regression coverage. |
| G1-23 | README, help, DESIGN, UAT evidence, and the goal tracker describe the precise shipped grammar, precedence, correction/literal path, timezone policy, and recurrence limitations rather than advertising arbitrary language understanding. |

## Non-goals

- Open-ended conversation, remote/local LLM integration, generative decomposition, or arbitrary-language understanding
- NLP for task search, existing-task editing, or arbitrary Taskwarrior commands
- Calendar events, time blocking, G3 daily commitments, or inferred Scheduled dates
- New estimate units/limits, automatic UDA installation, or duration prediction
- A recurrence engine, recurrence editing/stopping, or guessing G2's anchor/template behavior
- Natural-language project/tag inference or locale-dependent numeric date guessing
- Reinterpreting existing explicit Taskwarrior date expressions with the new grammar
- Adding persistent drafts, telemetry, network access, or a second task store

## Planning-time source evidence and feasibility gaps

This planning pass inspected the working-tree source; no new runtime probes or test runs were performed. G0 validation belongs to its existing evidence, not this plan.

- `internal/quickadd/parser.go:Parse` returns `domain.NewTask` directly, consumes whitespace-delimited trigger tokens, unescapes triggers before description assembly, and keeps scalar presence only in a local map. G1 needs richer provenance without changing legacy semantics.
- `splitTokens` already records rune offsets. `suggest.go:ContextAt` and `ApplySuggestion` also use rune offsets; span tracking must use the same convention.
- `internal/domain/new_task.go` has description, project, priority, Due/Scheduled expression strings, Estimate, and tags; it has no recurrence creation field. `Task.Recurrence` is an export/read value, not a safe recurrence-creation contract.
- `internal/ui/quickadd.go:submit` asynchronously parses and emits `QuickAddSubmitMsg`; `ApplyMessage` closes on successful parsing. There is currently no review state.
- `internal/app/model.go:Update` routes an open quick-add submit directly into `beginMutation`. Review must happen before that message, not after task creation. Existing overlay-open checks are not a full per-draft generation identity.
- `QuickAddModel.Now` is initialized on construction, with a fallback only when zero. G1 must obtain a fresh interpretation-time clock rather than assuming this field stays current.
- `internal/taskwarrior/mutations.go:AddArgs` accepts date-expression strings and only normalizes `next-week`. `CommandClient.Add` guards estimate writes before running add. Preserve both paths.

**Unverified:** absolute date format behavior across Taskwarrior date formats/timezones, exact date-only exports, DST handling in the proposed resolver, and G2 recurrence mappings. T00 must establish date feasibility; T08 must establish recurrence compatibility. Do not present proposed serialization or recurrence previews as observed behavior.

## Existing implementation map

| Area | Source | Planned responsibility |
|---|---|---|
| Explicit grammar | `internal/quickadd/parser.go`, `parser_test.go` | Preserve Parse API/behavior; expose internal span and explicit-presence information without an independent second trigger grammar |
| Interpretation | Proposed `internal/quickadd/interpret.go`, `interpret_test.go` | Pure draft/provenance/diagnostics contract, protected spans, explicit precedence, description reconstruction |
| Date inference | Proposed `internal/quickadd/dates.go`, `dates_test.go` | Narrow grammar and reference-time/location resolver; calendar and DST fixtures |
| Estimate domain | `internal/domain/estimate.go`, `estimate_test.go` | Reuse unchanged semantics; phrase adapter delegates to ParseEstimate |
| Other phrases | Proposed `internal/quickadd/phrases.go`, `phrases_test.go` | Estimate aliases, priority shorthand, reserved recurrence clauses |
| Suggestions | `internal/quickadd/suggest.go`, `suggest_test.go` | Preserve explicit completion and cursor offsets; no speculative NLP completion engine |
| Capture/review UI | `internal/ui/quickadd.go`, proposed `quickadd_review.go`, matching tests | Input/review state, editable draft, original source, correction/literal path, bounded rendering |
| Structured editor patterns | `internal/ui/edit.go` | Reuse appropriate field validation/render patterns, not existing-task modify messages or diff lifecycle |
| App orchestration | `internal/app/model.go`, `commands.go`, quick-add/runtime/mutation tests | Inject fresh clock, reject stale/busy submissions, hand off one confirmed draft to existing add lifecycle |
| Adapter | `internal/taskwarrior/mutations.go`, matching tests | Verify exact date argv and preserved estimate guard; extend only if verified date encoding needs it |
| Real integration | `internal/taskwarrior/integration_test.go:newIsolatedEnvironment` | Isolated interpreted draft → adapter → export/undo coverage; no production configuration |
| G2 seam | Future G2 plan/domain/adapter/tests | Block recurrence creation until the shared model and safety contract exist |
| Docs | `README.md`, `DESIGN.md`, `internal/ui/help.go`, `docs/UAT.md`, goal plan | Shipped behavior and evidence, with partial/full completion honestly distinguished |

Names for new files/types are proposed, not existing APIs. Prefer standard-library implementation for the bounded grammar; introducing a general NLP/date dependency requires evidence and explicit rationale.

## Proposed interpretation boundary

Keep `Parse(input)` as the existing explicit-only API/escape hatch. Add a pure interpretation entry point receiving input plus reference time/location. Its result should carry:

- original source and a draft `domain.NewTask`;
- explicit-field presence (including empty values such as `!none`);
- candidate values with original rune spans, target field, and explicit/inferred provenance;
- accepted, conflicting, invalid, and unsupported classifications;
- diagnostics and whether review/confirmation is permitted.

The UI owns draft edits and selection/rejection of candidates. Reconstruct description from unconsumed spans; do not repeatedly regex-replace an already-mutated description. Never feed reconstructed escaped text through natural-language recognition a second time.

An inferred date may carry a resolved instant internally, but serialization must use the T00-verified format at the existing NewTask/adapter boundary. Explicit expressions stay expressions. User correction of a date to a Taskwarrior expression must label it as such rather than retaining an obsolete resolved preview.

The app owns a capture/draft revision identity and asynchronous mutation lifecycle. Only a valid current confirmed draft may emit the final submit message. Review state is ephemeral; no persistence is introduced.

## Execution map and tracker

The table is the dependency map; each dependency is repeated in the task body below. Execute sequentially by default: parser/UI/app tasks share files, and real Taskwarrior tests use process environment. No tasks are marked parallel-safe.

Change `[ ]` to `[x]` only after the task's checks pass. Record command results, scenario/test counts, evidence paths, and commit IDs only if commits are requested. Blocked is not completed.

| Done | ID | Deliverable | Depends on | Evidence / commit |
|---|---|---|---|---|
| [x] | T00 | Approved policy and isolated date feasibility record | — | O1–O9, absolute-date round trip, timezone/DST tests |
| [x] | T01 | Span-aware interpretation contract preserving explicit parsing | T00 | `internal/quickadd/interpret.go` and parser regressions |
| [x] | T02 | Deterministic date/time resolver | T01 | Local calendar arithmetic, bare-time and DST tests |
| [x] | T03 | Estimate/priority phrase recognizer and recurrence reservations | T01 | G0 parser reuse, p1/p2/p3, native recurrence phrases |
| [x] | T04 | Complete interpretation/precedence pipeline | T02, T03 | Source-accounting and precedence fixtures |
| [x] | T05 | Keyboard review/correction component | T04 | Review/edit/explicit-only/cancel and bounded render tests |
| [x] | T06 | Capture-to-mutation app integration | T05 | Existing serialized add lifecycle plus review tests |
| [x] | T07 | Verified interpreted-date adapter interoperability | T04 | Isolated absolute-date and recurrence Taskwarrior tests |
| [x] | T08 | G2-backed recurrence phrase integration | T06, T07, external G2 contract and creation path | G2 contract in `recurring-tasks.md`; native template/stop integration passes |
| [x] | T09 | Shipped documentation and automated release evidence | T06, T07; T08 for full G1 | README/DESIGN/help/UAT updates and automated gates |
| [x] | T10 | Live keyboard/visual UAT | T09 | Maintainer-reported checks and isolated profile evidence in `docs/UAT.md`; local-only undo fixed and retested |

## Task details

### T00 — Policy approval and isolated date feasibility

**Depends on:** —  
**Where:** this plan, goal tracker, temporary Taskwarrior fixtures.  
**Requirements:** NLC-01–NLC-08, O1–O9.  
**Reuses:** G0 feasibility-record style and existing isolation safeguards.

- [x] O1 approved by the maintainer on 2026-09-21 (language, timezone, ISO dates, literal ambiguous numeric dates, local processing).
- [x] O2 approved by the maintainer on 2026-09-21 (Due semantics, local midnight, weekday rules, and no assumed date for bare times).
- [x] O3 approved by the maintainer on 2026-09-21 (inference review, explicit-only fast path, and Ctrl+S confirmation of valid drafts).
- [x] O4 approved by the maintainer on 2026-09-21 (explicit precedence, retained conflicting prose, duplicate errors, and inferred-value conflict resolution).
- [x] O5 approved by the maintainer on 2026-09-21 (effort phrases/aliases, shared G0 validation and UDA guards, literal unsupported forms, and invalid-value correction).
- [x] O6 approved with revisions by the maintainer on 2026-09-21 (`p1`/`p2`/`p3` shorthand; omission means no priority; no `p4`; existing explicit triggers preserved).
- [x] O7 approved by the maintainer on 2026-09-21 (keyboard correction/clear, original-input recovery, explicit-only fallback, and mutation-free cancellation).
- [x] O8 approved by the maintainer on 2026-09-21 (fresh interpretation clock, frozen reviewed dates, fresh re-interpretation, and DST correction).
- [x] O9 approved by the maintainer on 2026-09-21 (G1a delivery before G2, safe recurrence reservations, and full completion only after G1b).
- [x] Freeze positive/negative grammar fixtures against the approved O1–O9 contract.
- [x] Record Taskwarrior version and isolated commands/exit codes proving an unambiguous absolute timestamp encoding, local date-only midnight, and times with minutes.
- [x] Probe a non-default Taskwarrior date format and multiple timezones; prove preview instant equals exported instant. Record supported encoding, not a guessed date string.
- [x] Specify/test a reliable ambiguity/nonexistence check for local DST wall times; do not rely on silent `time.Date` normalization.
- [x] Confirm exact review keys, short-terminal behavior, literal fallback, and field correction/clear semantics.
- [x] Record G2 dependency status and explicit G1a authorization if shipping before G2.

**Tests:** isolated feasibility probes plus approved fixture review; no production task paths.  
**Gate:** documented O1–O9 decisions and reproducible date evidence with expected/actual results; unresolved blockers prohibit dependent feature code.  
**Independent demo:** show one proposed local date/time, its actual argv encoding, and the matching exported instant in a disposable Taskwarrior profile.

### T01 — Span-aware interpretation contract

**Depends on:** T00  
**Where:** `internal/quickadd/parser.go`, proposed `interpret.go`, corresponding tests.  
**Requirements:** G1-01, G1-07–G1-10.  
**Reuses:** `splitTokens`, ParseError, trigger validation, rune-based suggestion offsets, `domain.NewTask`.

- [x] Define candidate spans, field presence, provenance, diagnostics, and draft/result types.
- [x] Reuse one explicit lexer/validator; keep public Parse behavior and error cases compatible.
- [x] Protect explicit values and escaped tokens from later inference; retain `!none` presence.
- [x] Establish non-overlapping source accounting and description reconstruction with Unicode/punctuation fixtures.
- [x] Co-locate tests for duplicate fields, escapes, all triggers, empty descriptions, tags, and unchanged explicit suggestions.

**Tests:** unit and legacy regression fixtures.  
**Gate:** `go test ./internal/quickadd -count=1`; all existing cases retained and new span/presence assertions pass.  
**Independent demo:** inspect explicit field presence and source spans for `Write report !none \\~1h #work` without altering existing Parse output.

### T02 — Deterministic date/time resolver

**Depends on:** T01  
**Where:** proposed `internal/quickadd/dates.go`, `dates_test.go`.  
**Requirements:** G1-01–G1-04, G1-09–G1-10.  
**Reuses:** T01 spans and T00 approved calendar/encoding decisions.

- [x] Implement only the approved date/time whitelist using injected reference time/location.
- [x] Resolve calendar days/weekdays across month/year/leap boundaries without adding fixed 24-hour durations for calendar days.
- [x] Validate date/time ranges, duplicate candidates, bare times, overflow, DST gaps and folds.
- [x] Suppress substring extraction from unsupported/reserved clauses; preserve rejected source spans.
- [x] Test Monday/Sunday boundaries, same-day weekday, next weekday, midnight rollover, leap day, UTC and DST-observing zones; tests never depend on the host clock.

**Tests:** table-driven resolver and negative/protected-span unit tests.  
**Gate:** `go test ./internal/quickadd -count=1`; exact expected instants/diagnostics asserted.  
**Independent demo:** the same phrase/reference/location always returns the same due instant, while `at 3pm` remains unresolved.

### T03 — Effort, priority, and reserved recurrence recognizer

**Depends on:** T01  
**Where:** proposed `internal/quickadd/phrases.go`, corresponding tests.  
**Requirements:** G1-05–G1-06, G1-09–G1-10, G1-21.  
**Reuses:** `domain.ParseEstimate`, formatting, typed estimates, T01 candidates.

- [x] Normalize only approved effort aliases into G0 parser input; no independent numeric/range semantics.
- [x] Recognize standalone `p1`/`p2`/`p3` priority shorthand; test token boundaries, case-insensitivity, explicit-trigger precedence, omission as no priority, and `p4`/priority phrases retained as prose.
- [x] Protect longest recurrence clauses from accidental weekday/date extraction even before G2.
- [x] Return reserved recurrence notices with intact source; emit no recurrence creation field.
- [x] Test G0 minimum/maximum/fractional-minute/overflow failures, ordinary prose, phrase boundaries, punctuation, and unsupported duration units.

**Tests:** phrase unit tests plus G0 domain regression.  
**Gate:** `go test ./internal/quickadd ./internal/domain -count=1`.  
**Independent demo:** `about an hour` produces the existing 60-minute Estimate; `every Friday` remains a complete reserved clause without a one-off Due inference.

### T04 — Interpretation and precedence pipeline

**Depends on:** T02, T03  
**Where:** proposed `internal/quickadd/interpret.go`, corresponding tests/fuzz targets.  
**Requirements:** G1-01–G1-11, G1-14, G1-21.  
**Reuses:** T01 explicit lexer/provenance, T02 date resolver, T03 phrases.

- [x] Compose protected-span recognition, candidate extraction, conflict resolution, and final description construction.
- [x] Apply explicit precedence independently of token order; retain overridden source text and diagnostics.
- [x] Block unresolved inferred duplicates and invalid/empty drafts; do not silently first/last-win.
- [x] Preserve unsupported phrases and separators according to the approved contract; expose review requirement.
- [x] Cover the complete example table and mixed-field conflicts; add fuzz/property tests for panic freedom, valid/non-overlapping rune spans, determinism, and source accounting.

**Tests:** unit, regression, and bounded fuzz/property tests.  
**Gate:** `go test ./internal/quickadd -count=1`; run each added fuzz target with `-fuzztime=30s` and record its name/results.  
**Independent demo:** print a mixed explicit/inferred draft whose final metadata and every consumed span are explainable, with no mutation.

### T05 — Keyboard review and correction component

**Depends on:** T04  
**Where:** `internal/ui/quickadd.go`, proposed `quickadd_review.go`, matching UI tests/render fixtures.  
**Requirements:** G1-11–G1-17, G1-21.  
**Reuses:** text input, bounded modal rendering, editor field validation patterns, existing project/tag suggestions.

- [x] Add explicit input/review states and retain original source separately from edited draft.
- [x] Render field provenance, exact inferred date/time/timezone, expressions, conflicts, and unsupported recurrence notice.
- [x] Support keyboard correction/clear, candidate selection/rejection, return to input, explicit-only fallback, confirm, and cancel.
- [x] Preserve trigger-only fast capture and existing suggestion navigation/Esc semantics in input state.
- [x] Validate every draft edit and suppress submit for unresolved/invalid values.
- [x] Cover 120×30, 79×24, 49×18, and 28×8 layouts plus too-small dimensions, long Unicode input, all fields, scrolling, and errors; inspect intentional golden diffs.

**Tests:** UI state-transition, keyboard, Lip Gloss width/height, and render-fixture tests in this task.  
**Gate:** `go test ./internal/ui ./internal/quickadd -count=1`; no hidden diagnostics or out-of-bounds frames.  
**Independent demo:** review `Call Friday`, restore Friday to prose through explicit-only capture, and cancel without producing a submit message.

### T06 — Capture-to-mutation app integration

**Depends on:** T05  
**Where:** `internal/app/model.go`, relevant commands/overlay routing, `internal/ui/quickadd.go`, quick-add/runtime/mutation tests.  
**Requirements:** G1-04, G1-08, G1-11–G1-19, G1-22.  
**Reuses:** existing clock seam, typed messages, `beginMutation`, add/refresh/sync/undo lifecycle.

- [x] Supply a fresh clock/location per interpretation attempt and freeze reviewed inferred values.
- [x] Attach/reject capture revision identities so delayed messages cannot submit after edits, cancel, or reopen.
- [x] Allow only one current confirmed draft into MutationAdd; repeated keys and busy state cannot lose/duplicate capture.
- [x] Keep confirmed draft recoverable on add failure; do not retry automatically or claim success.
- [x] Preserve refresh pausing, Completed/global capture, project catalog suggestions, sync grace, undo, and unsynced quit.
- [x] Assert exact fake-client calls: zero before confirmation/on cancel/errors, one on valid confirmation, correct draft values on mutation, and no stale submission.

**Tests:** app update-loop/runtime tests with fake clock/client and UI lifecycle regression.  
**Gate:** `go test ./internal/app ./internal/ui -count=1`; `go test -race ./internal/app ./internal/ui`.  
**Independent demo:** review tomorrow before midnight, confirm after midnight, and prove the single add uses the originally reviewed absolute date.

### T07 — Interpreted-date adapter interoperability

**Depends on:** T04  
**Where:** `internal/taskwarrior/mutations.go` only if needed, unit tests and isolated `integration_test.go`.  
**Requirements:** G1-04–G1-05, G1-07, G1-18–G1-19, G1-22.  
**Reuses:** `AddArgs`, `CommandClient.Add`, estimate readiness guard, isolated environment helper.

- [x] Codify T00's absolute inferred date encoding in exact argv and export assertions; preserve explicit expression behavior and `next-week` alias.
- [x] Exercise interpreter → NewTask → real adapter add/export/undo in disposable profiles.
- [x] Cover timezone/dateformat variation, date-only/time-bearing values, explicit precedence, and mixed estimate/date capture.
- [x] Verify missing/wrong Estimate UDA refuses without add while ordinary capture remains usable; hook/Taskwarrior validation errors stay honest.
- [x] Verify shell metacharacters remain data in separate arguments; no raw inferred text becomes command syntax.
- [x] Retain non-parallel execution and real-path guards; no sync server.

**Tests:** exact argv/runner unit tests and isolated real-Taskwarrior integration in this task.  
**Gate:** `go test ./internal/taskwarrior -count=1`; `go test ./internal/taskwarrior -run Integration -v -count=1` with Taskwarrior available; skips do not establish interoperability.  
**Independent demo:** export one interpreted date/estimate capture through the regular CLI and compare it to the reviewed draft, then undo.

### T08 — G2-backed recurrence phrases

**Depends on:** T06, T07, external G2 approved contract and tested creation path  
**Where:** phrase/interpreter/review/app tests and the shared G2 domain/adapter creation seam once available.  
**Requirements:** NLC-02, G1-09–G1-10, G1-12–G1-13, G1-18–G1-21.  
**Reuses:** G2 recurrence model, validation, anchor, next-occurrence preview, persistence, and generation policy.

- [x] Link the actual G2 plan, APIs, and test evidence here before changing code; refine this task against that contract.
- [x] Approve the exact phrase-to-G2 mapping and due/anchor conflict rules; no guessed preset semantics.
- [x] Replace supported reservations with typed G2 candidates; keep unsupported recurrence prose intact.
- [x] Show anchor, recurrence, expected next occurrence, and recurrence-primary/delayed-generation implications as G2 requires.
- [x] Verify correction/cancel/conflict cases and one confirmed creation through the shared adapter.
- [x] Co-locate parser/UI/app and isolated recurrence integration coverage, including combined due/priority/estimate/project capture and devices with recurrence generation disabled.

**Tests:** unit/UI/app plus G2-backed isolated integration; exact commands/scenarios must be finalized when G2 exists.  
**Gate:** `go test ./internal/quickadd ./internal/ui ./internal/app ./internal/taskwarrior -count=1` and isolated Integration run; G2 creation acceptance must pass. The linked G2 contract and scenario list are now in `recurring-tasks.md`; the native template/instance scenarios pass in the isolated Taskwarrior suite.  
**Independent demo:** capture a supported recurring responsibility with an estimate, review its anchor/next occurrence, and inspect the resulting Taskwarrior data under G2's documented semantics.

### T09 — Documentation and automated release evidence

**Depends on:** T06, T07; T08 for full G1  
**Where:** `README.md`, `DESIGN.md`, help guidance, `docs/UAT.md`, this plan, goal tracker.  
**Requirements:** NLC-01–NLC-08, G1-22–G1-23.  
**Reuses:** G0 evidence layout and repository gates.

- [x] Document the shipped grammar with positive/negative examples, reference-time/timezone rules, review keys, correction/literal escape hatch, and precedence including `~`/`!none`.
- [x] Explain Taskwarrior expression versus resolved date previews, G0 UDA requirements, and G2 availability/limitations.
- [x] Keep DESIGN's existing behavior accurate; amend the quick-add contract only when the implementation ships.
- [x] Add a G1 evidence section in `docs/UAT.md` mapping acceptance IDs to test names/files and exact commands/results.
- [x] Record package/test/action/scenario counts and skips without assuming G0's counts remain current; explain any removed tests.
- [x] Run final gates below; record a G1a result separately if T08 is blocked, then rerun for full G1 after T08.
- [x] Update goal status honestly: a partial slice does not satisfy NLC-02; automated evidence is not live validation.

**Tests:** documentation/link checks, help rendering tests if changed, and full regression.  
**Gate:** all required final gates pass with evidence; no fabricated live observations.  
**Independent demo:** another maintainer can reproduce a documented capture and distinguish supported phrases from safe literal text.

### T10 — Live keyboard and visual UAT

**Depends on:** T09; full-G1 execution requires completed T08  
**Where:** `docs/UAT.md`, this plan, goal tracker.  
**Requirements:** G1-11–G1-23 and goal-level NLC-01–NLC-08.  
**Reuses:** documented disposable Taskwarrior profiles and supported terminal layouts.

- [x] Capture `Prepare proposal tomorrow at 3pm #work`; inspect exact date/time and correct it before confirmation.
- [x] Capture effort phrases and priority shorthand; prove `~30m` and `!none` override conflicting inference visibly.
- [x] Capture a literal title containing Friday using explicit-only mode; verify no word disappears.
- [x] Exercise duplicate/invalid/bare-time cases, back/edit/reparse, cancel, and repeated confirmation.
- [x] Verify all review content at wide/compact/narrow/short dimensions and mouse-free correction/confirmation.
- [x] Verify missing-UDA refusal in a disposable profile, ordinary capture without UDA, offline add, and undo during the grace period. Do not run production sync.
- [x] After G2, capture the full roadmap example and verify the actual recurrence/anchor outcome against G2 rather than assuming the mixed dates are compatible.
- [x] Record platform, terminal size, Taskwarrior version, observed outcomes, failures, and maintainer verdict; rerun failures after fixes.

**Tests:** human-observed keyboard/visual UAT; automated simulation does not check this box.  
**Gate:** maintainer-approved recorded observations for every shipped slice; mark full G1 Validated only after G1b passes too.

## Requirement traceability

| Goal requirement | Acceptance coverage | Tasks | Current state |
|---|---|---|---|
| NLC-01 — dates/times | G1-01–G1-04, G1-09–G1-10 | T00–T02, T04–T07, T09–T10 | Validated in maintainer UAT |
| NLC-02 — recurrence | G1-20–G1-21 | T03–T06, T08–T10 | Validated with G2 capture in maintainer UAT |
| NLC-03 — estimates | G1-05 | T03–T07, T09–T10 | Validated in maintainer UAT |
| NLC-04 — explicit precedence | G1-07–G1-08 | T01, T04–T07, T09–T10 | Validated in maintainer UAT |
| NLC-05 — review/correction | G1-10–G1-17, G1-19 | T04–T06, T08–T10 | Validated in maintainer UAT |
| NLC-06 — retain prose | G1-09–G1-10, G1-13–G1-14, G1-21 | T01–T05, T08–T10 | Validated in maintainer UAT |
| NLC-07 — local/deterministic | G1-01–G1-04, G1-22 | T00–T04, T06, T09–T10 | Validated in maintainer UAT |
| NLC-08 — safe final validation | G1-04–G1-05, G1-18–G1-19 | T06–T10 | Validated in maintainer UAT |

## Plan consistency and test co-location checks

The dependency table is the execution map (no separate diagram to drift). These checks validate plan structure, not implementation correctness. Repository test requirements come from `CONTRIBUTING.md` and DESIGN §15.

| Task | Body dependencies match map | Cohesive deliverable | Required / co-located validation |
|---|---|---|---|
| T00 | Yes: none | Policy/feasibility record | Approval + isolated probes |
| T01 | Yes: T00 | Interpretation contract | Parser unit/regression |
| T02 | Yes: T01 | Date resolver | Date/calendar/DST unit |
| T03 | Yes: T01 | Non-date recognizer | Phrase/domain unit |
| T04 | Yes: T02, T03 | Interpretation pipeline | Unit/property/fuzz |
| T05 | Yes: T04 | Review component | Keyboard/state/render/width |
| T06 | Yes: T05 | Submission lifecycle | App fake-client/runtime/race |
| T07 | Yes: T04 | Date interoperability contract | Adapter unit + isolated integration |
| T08 | Yes: T06, T07, G2 | G2 phrase bridge | Parser/UI/app + isolated integration; refine after G2 |
| T09 | Yes: T06, T07; T08 for full | Release evidence | Docs/help + full regression |
| T10 | Yes: T09 | Live acceptance record | Human keyboard/visual UAT |

## Risks and controls

| Risk | Required control |
|---|---|
| NLP strips meaningful prose | Whitelist, provenance/spans, mandatory inference review, reversible candidate decisions, explicit-only path |
| Unsupported phrase partially matches | Longest-clause reservations and negative fixtures, especially recurrence/weekdays |
| Explicit empty priority loses precedence | Track explicit field presence independently of serialized value |
| Clock or timezone changes preview at commit | Fresh interpretation snapshot; freeze resolved inferred date; fake-clock and DST tests |
| Date serialization depends on Taskwarrior config | T00 isolated dateformat/timezone probes and T07 export equivalence tests |
| Stale async submit creates wrong/duplicate task | Capture revision identity, overlay lifecycle checks, single mutation gate |
| Review hides fields on short terminals | Scrollable bounded content; no confirm without accessible review |
| Estimate parsing diverges from G0 | Phrase adapter delegates to shared domain parser and guarded adapter |
| G1 silently invents recurrence semantics | External G2 blocker, reserved-prose behavior, no premature full-G1 status |
| Automatic retry duplicates a partly successful add | Preserve draft/error for deliberate recovery; no auto-resubmission |

## Required final gates

Run from the repository root. Use a temporary build output directory to avoid overwriting an existing root binary. Real Taskwarrior integration tests must retain temporary-path guards and run non-parallel.

```sh
test -z "$(gofmt -l .)"
go test ./...
go test -race ./...
go vet ./...
build_dir=$(mktemp -d)
go build -o "$build_dir/momentum" ./cmd/momentum
GOOS=linux GOARCH=amd64 go build -o "$build_dir/momentum-linux-amd64" ./cmd/momentum
GOOS=darwin GOARCH=arm64 go build -o "$build_dir/momentum-darwin-arm64" ./cmd/momentum
go test ./internal/taskwarrior -run Integration -v -count=1
git diff --check
```

**Automated execution evidence (2026-09-22):** formatting, `go test ./... -count=1`, race, vet, native build, Linux AMD64 build, macOS ARM64 build, the complete isolated Integration run, and `git diff --check` pass. The full suite had 9 packages, 494 top-level test/fuzz entries, 664 passing test actions, and no skips. The isolated run had 17 top-level scenarios and 29 passing actions; the recurrence differential test contributed 12 Taskwarrior 3.5 cases and retained temporary-path guards. Subsequent local-only undo regression and full gates passed; maintainer-reported T10 UAT and the corrected undo retest are recorded separately in `docs/UAT.md`.

## Completion definition

- **G1a/G1b Implemented:** T00–T09 complete for both slices, NLC-01–NLC-08 and G1-01–G1-23 have automated evidence, and the full project gates pass.
- **G1 Validated:** T10 live maintainer UAT passes with recorded keyboard/visual observations.

The recurrence bridge consumes the independently documented G2 contract; it does not create a second recurrence model. Automated completion does not claim live validation.
