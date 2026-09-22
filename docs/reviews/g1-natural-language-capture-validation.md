# G1 implementation validation — changes requested

> **Historical first pass.** The subsequent R1–R4 code blockers are now resolved; see the [latest repair evidence](g1-natural-language-capture-rereview-2.md#r2-repair-evidence).

**Date:** 2026-09-21  
**Baseline:** working tree over `36b80bb`; G0–G4 changes coexist and are uncommitted.  
**Contract:** [G1 implementation plan](../plans/natural-language-capture.md), approved O1–O9.  
**Verdict:** **CHANGES REQUESTED — neither full G1 nor G1a meets its acceptance contract.**

Existing automated gates pass, but targeted runtime probes and source review expose material failures in capture, review, source preservation, and timezone handling. G2 code now exists and is used by G1; recurrence is no longer merely an absent dependency, but its G1 integration fails conflict handling. This review is not a full G2–G4 audit.

## Validation performed

| Gate | Observed result |
|---|---|
| `test -z "$(gofmt -l .)"` | PASS on implementation tree, before temporary probes |
| `go test -json ./... -count=1` | PASS: 9 packages, 473 top-level tests, 544 passing test actions, 0 failures/skips |
| `go test -race ./... -count=1` | PASS |
| `go vet ./...` | PASS |
| Native build, Linux AMD64 build, macOS ARM64 build | PASS; outputs written to temporary directory |
| `go test ./internal/taskwarrior -run Integration -v -count=1` | PASS: 16 isolated scenarios, no skips |
| `git diff --check` | PASS |
| Additional parser, root-app, and review-render probes | Reproduced failures below; two root-app assertions fail |
| Live keyboard/visual UAT | NOT PERFORMED; no human-observed acceptance claimed |

Environment: macOS ARM64, Go 1.27.1. Existing integration evidence identifies Taskwarrior 3.5.0. Integration tests retain temporary TASKRC/TASKDATA and production-path guards; no sync server or production task database was used.

The previous G0 handoff reported 440 top-level tests / 511 passing actions in 8 packages; the current tree adds 33 of each and one package, across multiple goals. This is not a G1-only test delta. Reviewed quick-add test diffs retain existing assertions and extend them; the main problem is absent coverage, not weakened assertions in those inspected files.

Probe sources and raw output are retained locally at `/tmp/momentum-g1-validation/` (`parser_probe.go`, `app_probe.go`, `ui_probe.go`, `probes.log`, `ui-probes.log`, `tests.json`, `race.log`, `integration.log`). Temporary probe files were removed from source directories after execution. No implementation code was changed. The reproductions below remain usable if temporary logs expire.

## Required fixes

### F1 — Explicit-only capture closes before the app can submit (high)

**Location:** `internal/ui/quickadd_review.go:54–63`; `internal/app/model.go:270–276` and overlay routing around 659.  
**Requirements:** G1-13–G1-15, G1-18.

Reproduce through the root model: open capture → enter `Discuss Friday #work` → Enter → press `x`. The UI calls `q.Close()` before emitting `QuickAddSubmitMsg`. Root key routing clears the overlay, and the later submit handler rejects the message because quick add is no longer open. Observed: `overlay=""`, `open=false`, no mutation command. The existing component test checks only the emitted message, missing its rejection by the app.

The unmodified `x` key also intercepts ordinary editing in every review field: a description containing a typed `x` invokes the fallback instead of inserting the character.

**Fix task:** keep the overlay/draft alive until the app accepts the submission; expose explicit-only through a non-conflicting action/key. Add a root-model fallback test proving exactly one Add, and field-edit tests proving ordinary `x` remains typeable.

### F2 — Blocking interpretation errors can be confirmed unchanged (high)

**Location:** `internal/ui/quickadd_review.go:134–161`.  
**Requirements:** G1-10, G1-13, G1-15, G1-19–G1-20.

`Call tomorrow at 3pm at 4pm` yields `Interpretation.Valid=false` and a blocking multiple-times diagnostic. Nevertheless, Ctrl+S immediately emits `QuickAddSubmitMsg` with a midnight due date. The validator only handles diagnostic-bearing individual candidates and ignores global blocking diagnostics when any candidate exists.

Likewise, `Report tomorrow every Friday` with tomorrow not Friday is invalid in the interpreter but confirms without correction: `reviewValidForValues` explicitly skips conflict candidates. This defeats G2 anchor review rather than resolving it.

**Fix task:** validate the entire edited draft and outstanding decisions, including global diagnostics and anchor conflicts; never equate missing per-candidate errors with resolution. Test zero submit/mutation calls until each conflict is explicitly resolved.

### F3 — Stale and busy submissions are not tied to a capture revision (high)

**Location:** `internal/ui/quickadd.go:submit`, message types; `internal/app/model.go:258–276,514–521`.  
**Requirements:** G1-16, G1-18–G1-19.

Reproduce: schedule Enter for `Old capture`, close/reopen quick add with `New capture`, then deliver the old command result. The app accepts the old task and closes the new capture because it checks only whether an overlay is open. No source/revision identity exists. The root-model assertion reproduced this failure.

Code review also shows submit closes the overlay before `beginMutation` checks the busy guard; a busy mutation can therefore silently discard a confirmed draft. `submitReview` captures some values but reads mutable `q.Review` when its command executes, further weakening snapshot isolation.

**Fix task:** attach capture/draft revisions to parse/review/submit/error messages, capture immutable command inputs, and check busy/current ownership before closing. Test delayed messages after edits/cancel/reopen, repeated confirmation, and busy-state draft preservation.

### F4 — Multiple inferred scalars silently overwrite one another (high)

**Location:** `internal/quickadd/interpret.go:103–136,262–322`.  
**Requirements:** O4, G1-08–G1-10, G1-20.

Observed at fixed Monday 2026-09-21 10:00 UTC:

| Input | Actual result |
|---|---|
| `Report p1 p2` | Valid, priority M, description `Report`, no diagnostic |
| `Report about 1h for 30 minutes` | Valid, estimate 30m, description `Report`, no diagnostic |
| `Report every day every Friday` | Valid, weekly recurrence but Monday anchor, description `Report`, no diagnostic |

All candidate spans are consumed and marked accepted. Only dates/times have duplicate handling; priority, estimate, and recurrence use loop overwrite semantics.

**Fix task:** group all scalar candidates before accepting/removing any, require selection/correction for duplicates, and apply explicit precedence consistently. Add order-permutation and source-accounting tests for every scalar field.

### F5 — Parser extracts metadata from protected prose and unsupported clauses (high)

**Location:** `internal/quickadd/interpret.go:66–71,368–386` and recognizer loops.  
**Requirements:** O1/O6, G1-07, G1-09, G1-21.

Observed:

| Input | Actual description / metadata |
|---|---|
| `Email friday@example.com` | `Email @example.com`; inferred Friday Due |
| `Read https://example.com/tomorrow` | `Read https://example.com/`; inferred tomorrow Due |
| `Review étape-p1` | `Review étape-`; inferred High priority |
| `Discuss every other Friday` | `Discuss every other`; one-off Friday Due |

Regex `\b` is not the approved standalone-token/Unicode boundary, and protected spans only cover explicit/escaped trigger tokens. Unsupported recurrence is not reserved as a complete clause, allowing its weekday to become a one-off deadline.

**Fix task:** protect URLs/emails, enforce real standalone priority boundaries, reserve complete unsupported recurrence clauses, and preserve all unrecognized spans. Add Unicode, punctuation, URL/email, and unsupported-clause fixtures plus the planned source-accounting fuzz/property tests (no quick-add fuzz target currently exists).

### F6 — Invalid time phrases silently become midnight deadlines (high)

**Location:** `internal/quickadd/interpret.go:67,189–260`.  
**Requirements:** O2, G1-03, G1-10.

`Call tomorrow at 25:00` and `Call tomorrow at 3` both return valid drafts with tomorrow at midnight, retain the malformed time in prose, and report no diagnostic. The regex matches valid times only, so recognized-but-invalid time clauses never reach validation. This contradicts the explicit grammar requirement to correct invalid or ambiguous hour/meridiem input.

**Fix task:** recognize candidate time clauses before validating their ranges/completeness. Invalid `at` time forms must block until corrected or deliberately captured as prose. Test invalid hours/minutes, missing meridiem, partial inputs, and explicit precedence.

### F7 — DST ambiguity detection misses supported local zones (high)

**Location:** `internal/quickadd/interpret.go:537–554`.  
**Requirements:** O8, G1-03–G1-04.

All of these ambiguous wall times were accepted as valid without diagnostics:

- Europe/Lisbon: `Call 2026-10-25 at 1:30am`.
- Europe/Berlin: `Call 2026-10-25 at 2:30am`.
- Australia/Lord_Howe: `Call 2026-04-05 at 1:45am`.

The resolver checks only `candidate + 1 hour`. Go can choose the later occurrence, and transitions need not be one hour. Existing DST tests cover only America/New_York. Inferred serialization also lacks a timezone offset, making correct ambiguity refusal particularly important.

**Fix task:** detect all matching instants around the relevant zone transition, regardless of which occurrence Go chooses or transition duration. Add both directions, 30-minute folds, gaps, and boundary fixtures; reverify preview/export equality under varied timezone/dateformat settings.

### F8 — Review cannot expose/correct all submitted fields, especially on short terminals (high)

**Location:** `internal/ui/quickadd_review.go:13,197–247`; `internal/ui/quickadd.go:ReviewInputs`.  
**Requirements:** G1-12–G1-13, G1-17.

Review has only Description, Due, Recurrence, Estimate, and Priority. For `Call tomorrow #work +client >today`, Project, Tags, and Scheduled are neither shown nor editable even at 120×30, although they are submitted. Explicit provenance is absent except for recurrence.

At 28×8, the rendered output shows only title, interpretation, a truncated candidate, and truncated key help. The focused Priority field is not visible; there is no scroll state to expose it, yet confirmation remains enabled. Review also prints the raw compact timestamp rather than a clearly labeled human date/expression.

**Fix task:** expose every submitted field with provenance, implement field/diagnostic scrolling and candidate decisions, and gate confirmation when review cannot be accessed. Add content-visibility assertions, not just frame-width checks, at all planned sizes.

### F9 — Valid correction is rejected and review errors are invisible (high)

**Location:** `internal/ui/quickadd_review.go:134–161,197–247`.  
**Requirements:** G1-10, G1-13, G1-19.

Enter `Report for 0 minutes`, replace the review Estimate with `30m`, and confirm. It still returns `resolve the highlighted interpretation...` because validation requires the original `q.Review.Task.Estimate` to be non-nil. Correcting the field cannot satisfy this check. Clearing invalid metadata is likewise not represented as an explicit resolution.

After applying `QuickAddErrorMsg`, `ParseErr` is set but `reviewView` never renders it. The probe verified the actual error text is absent even at normal size.

**Fix task:** validate current field values and explicit correction/clear decisions rather than original task presence; render save errors in review. Test invalid→valid, invalid→clear, repeated correction, and adapter error display.

### F10 — Failed adds lose recoverable draft state (medium)

**Location:** `internal/app/model.go:270–276,524–547`; `internal/ui/quickadd.go:OpenQuickAdd/Close`.  
**Requirements:** G1-19.

Source review shows quick add closes before Add executes. On error the app clears `PendingMutation` and shows a status message but does not reopen or retain an accessible draft. The next normal capture opens with an empty string. The existing missing-UDA app test checks the status, not recovery. Users must reconstruct a reviewed task after setup/hook failures.

**Fix task:** retain the immutable confirmed draft/source through Add failure and offer deliberate correction/retry without automatic resubmission. Test missing-UDA, hook/runner errors, and reopening recovery; ensure no duplicate Add is triggered.

## Acceptance traceability

PASS below is bounded to inspected implementation/existing tests; it does not override failures in adjacent criteria.

| Criteria | Result | Evidence / limitation |
|---|---|---|
| G1-01 | PARTIAL | Pure path with supplied clock is deterministic; zero clock silently calls `time.Now`; fuzz/source invariants not covered |
| G1-02 | PARTIAL | Common dates work; broad calendar/overflow/rollover matrix remains incomplete |
| G1-03 | FAIL | F6/F7 invalid times and DST ambiguity |
| G1-04 | PARTIAL | Single local absolute-date roundtrip passes; multi-timezone/dateformat coverage incomplete; F7 |
| G1-05 | PARTIAL | G0 type/parser/guard reused; effort duplicate and correction failures F4/F9 |
| G1-06 | FAIL | p1/p2/p3 map correctly but standalone boundary fails, F5 |
| G1-07 | PARTIAL | Legacy explicit parser regressions pass; inference corrupts protected prose, F5 |
| G1-08–G1-10 | FAIL | F2/F4/F5/F6/F9 |
| G1-11 | PASS, bounded | Tested ordinary explicit fast path and inferred review entry; malformed time detection excluded by F6 |
| G1-12–G1-17 | FAIL | F1/F2/F3/F8/F9 |
| G1-18 | PARTIAL | Existing argv/add/refresh/sync machinery reused; fallback and lifecycle defects F1/F3 |
| G1-19 | FAIL | Invalid submissions and missing recovery, F2/F9/F10 |
| G1-20–G1-21 | FAIL | G2 model reused, but unresolved conflicts/unsupported recurrence fail, F2/F4/F5 |
| G1-22 | PARTIAL | Existing regression gates pass and parser has no remote calls; not a full multi-goal regression audit |
| G1-23 | FAIL | Current docs overstate passing acceptance; tracker task details and evidence remain inconsistent |

| Goal-level requirement | Verdict |
|---|---|
| NLC-01 dates/times | FAIL — invalid times and DST |
| NLC-02 recurrence | FAIL — conflicting/unsupported recurrence handling |
| NLC-03 effort | PARTIAL — shared representation works; conflicts/correction fail |
| NLC-04 precedence | PARTIAL — single explicit overrides work; inferred duplicates fail |
| NLC-05 review/correction | FAIL — fallback, validation, recovery, visibility |
| NLC-06 retain prose | FAIL — URL/email and unsupported-clause extraction |
| NLC-07 local/deterministic | PARTIAL — local parser and injected-clock tests pass; full invariants incomplete |
| NLC-08 final safe mutation | PARTIAL — argv/final Taskwarrior validation retained; invalid drafts still reach submission |

## Tracking and completion corrections

The tracker marks T00–T09 complete, but task-body checklists remain largely unchecked and its execution evidence section says “none yet.” Only seven interpreter tests, three review component tests, and one local absolute-date integration test were found for the principal new capture files. These do not establish the planned duplicate, rune-boundary, stale-message, busy-state, full-field, short-terminal, multi-zone, fuzz, or end-to-end review acceptance.

Treat T00–T09 checkmarks as prior implementation claims, not independently verified completion. T10 remains unperformed. The plan/roadmap/UAT summary are annotated with this review rather than certifying the feature as Implemented/Validated.

## Recommended repair sequence

1. F1–F3: repair fallback, blocking validation, and capture identity before allowing further submission testing.
2. F4–F7: repair scalar conflict resolution, protected prose, invalid-time recognition, and timezone resolution, with co-located regression/property tests.
3. F8–F10: complete review visibility/editability, correction feedback, and deliberate error recovery.
4. Rerun all gates and every reproduction above; reconcile task checklists and requirement evidence.
5. Only then run the planned human keyboard/visual UAT and consider Validated status.

No fixes, commits, production sync, or changes to task data were performed by this review.
