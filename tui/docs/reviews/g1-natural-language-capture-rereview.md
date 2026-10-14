# G1 re-review — changes requested

> **Historical second pass.** All R1–R4 reported blockers are now resolved; see the [latest repair evidence](g1-natural-language-capture-rereview-2.md#r2-repair-evidence).

**Date:** 2026-09-21  
**Scope:** Updated working tree over `36b80bb`; follow-up to [F1–F10](g1-natural-language-capture-validation.md).  
**Verdict:** **CHANGES REQUESTED.** Most original reproductions are fixed, but four blocking findings remain. No implementation changes were made by this review; live UAT remains unperformed.

## Fresh validation

| Check | Result |
|---|---|
| `go test -json ./... -count=1` | PASS: 9 packages, 485 top-level test/fuzz entries, 573 passing test actions, no failures/skips |
| `go test -race ./... -count=1` | PASS |
| `go vet ./...` | PASS |
| Formatting and `git diff --check` | PASS |
| Native, Linux AMD64, macOS ARM64 builds | PASS; temporary output paths |
| Isolated Taskwarrior integration | PASS: 16 scenarios, no skips |
| `FuzzInterpretIsDeterministicAndSpanSafe`, 30 seconds | PASS |
| Original independent parser/DST reproductions | Outcomes corrected for duplicate scalars, protected email/URL/Unicode prose, invalid times, `every other Friday`, and Lisbon/Berlin/Lord Howe folds |
| Focused root-app/review regression tests | PASS for explicit-only mutation, stale reopen messages, busy-state retention, failed-add recovery, ordinary x editing, unchanged conflict blocking, and estimate correction |
| Additional independent parser/review probes | Four failures below reproduced |

Counts increased from 473 top-level entries / 544 passing actions at the first review. The new source-accounting fuzz target and root-level lifecycle tests improve coverage materially, but passing those gates does not establish the remaining behavior.

Logs and temporary probe sources: `/tmp/momentum-g1-rereview/`. Temporary source-tree probe files were removed after execution. Existing integration isolation guards were preserved; no production Taskwarrior paths or sync server were used.

## F1–F10 disposition

“Resolved” here applies to the original finding and inspected regression paths, not a blanket approval of all capture behavior.

| Original finding | Result | Evidence |
|---|---|---|
| F1 explicit-only submission / x interception | RESOLVED | Ctrl+X keeps draft open until root accepts Add; ordinary x edits; root mutation regression passes |
| F2 invalid review confirmation | PARTIAL — R1 remains | Unchanged conflicts now block, but touch tracking can falsely declare them resolved |
| F3 stale/busy submission | RESOLVED for inspected paths | Revision/source identities, immutable submitted values, guard-before-close; stale reopen and busy tests pass |
| F4 duplicate scalar overwrite | RESOLVED | Independent original probes now reject duplicates and retain all source text |
| F5 protected prose extraction | RESOLVED for original reproductions | Email, URL, `étape-p1`, and `every other Friday` preserved; fuzz target added |
| F6 invalid time detection | Original defect RESOLVED; R4 is a related regression | `at 25:00` / `at 3` now block; valid 24-hour phrases with following tokens fail |
| F7 DST fold handling | RESOLVED for reported zones | Offset enumeration rejects all three original ambiguous wall times |
| F8 inaccessible review | PARTIAL — R2 remains | All eight fields exist and focus scrolls, but narrow field values remain invisible |
| F9 estimate correction/error display | RESOLVED for reported paths | `0 minutes` → `30m` now submits; review renders errors |
| F10 failed-add recovery | RESOLVED for inspected paths | Failed confirmed draft reopens; retry is deliberate, not automatic; regression tests pass |

## Remaining required changes

### R1 — No-op editing bypasses unresolved recurrence conflicts (high)

**Source:** `internal/ui/quickadd_review.go:validateReviewValues`, particularly `changed` and the recurrence-weekday special case.  
**Contract:** G1-10, G1-13, G1-15, G1-19–G1-20; original F2.

Reproduce at Monday 2026-09-21:

1. Interpret `Report tomorrow every Friday`.
2. Review correctly reports the Tuesday/Friday conflict.
3. Focus Recurrence, initially `weekly`.
4. Type `x`, then Backspace; the value is exactly `weekly` again.
5. Press Ctrl+S.

**Actual:** emits `QuickAddSubmitMsg` even though no conflicting value or candidate decision changed. The initial conflict was not resolved; typing then deleting only set `ReviewTouched=true`.

**Cause:** `changed(index)` treats any historical touch as a change. For a recurrence-weekday conflict, `changed(Recurrence)` immediately skips the anchor check. This is not an explicit select/keep/literal/clear action and permits an unresolved draft to be submitted.

**Required fix:** distinguish current semantic changes and explicit resolution decisions from typing history. Revalidate the current recurrence/anchor combination; do not resolve a blocking candidate through diagnostic-string matching and a dirty bit alone. Keep intentional clear/selection actions explicit.

**Regression gate:** unchanged, type-and-delete, and edit-then-restore must all remain blocked; changing to a compatible anchor or explicitly resolving/rejecting the candidate must pass. Assert no root mutation until resolution.

### R2 — Narrow review shows labels but hides every field value (high)

**Source:** `internal/ui/quickadd_review.go:reviewView`, `reviewConfirmationAccessible`; `internal/ui/quickadd.go:SetSize`.  
**Contract:** G1-12–G1-13, G1-17; original F8.

Reproduce `Call tomorrow #work +client >today` at **28×8**, then Tab through every field.

**Actual:** the field row is consumed by padded label/provenance, e.g. `Due         [inferred…` and `Project     [explicit…`. `Call`, the exact Due value, `work`, `today`, and `client` are not visible in their focused input rows. Ctrl+S still returns a submit message.

The updated narrow test asserts only that field **labels** and `Ctrl+S` exist. It passes while the field content is inaccessible. Scrolling to a label is not reviewability. The fixed provenance prefix exceeds the content width, so moving the input cursor cannot reveal the value either.

**Required fix:** use a stacked/compact field layout that budgets actual value/cursor space before provenance, or refuse confirmation until a usable review layout is available. Preserve exact date/expression visibility and provide access to complete diagnostics, not only truncated prefixes.

**Regression gate:** assert focused field **values** and editing cursor/content are observable at 28×8 and other supported widths, including full due inspection; otherwise confirmation must be disabled with actionable guidance. Verify long diagnostics can be read, not merely selected.

### R3 — Supported plural recurrence intervals are silently ignored (high)

**Source:** `internal/quickadd/interpret.go:recurrenceRE` and `naturalMatches`.  
**Contract:** NLC-02, G1-20, approved grammar `every <positive integer> weeks`.

Independent probe:

```text
Input:  Report every 2 weeks
Actual: Valid=true, RequiresReview=false, Recurrence="", Due=""
        Description="Report every 2 weeks"
```

`every 2 days` behaves the same way. The normal fast path therefore creates an ordinary nonrecurring task without review or a notice, despite this being a supported interval form.

**Cause:** the regexp alternation puts `week` before `weeks` (likewise `day` before `days`). The regexp returns the shorter prefix; the subsequent standalone-boundary check rejects it because the following `s` is not a boundary. It does not retry the longer alternative.

**Required fix:** use longest valid complete units or optional plural suffixes, validating boundaries as part of the complete recognizer rather than discarding a shorter match afterward.

**Regression gate:** supported singular/plural interval pairs must create the same typed recurrence and require review, including `every 2 weeks` → G2 `2wks`, with end-of-input, punctuation, and following metadata. Preserve invalid-interval safety and source accounting.

### R4 — Valid 24-hour times fail when another token follows (medium)

**Source:** `internal/quickadd/interpret.go:naturalTimeRE`, `naturalMatches`, `incompleteTimeMatches`.  
**Contract:** NLC-01, G1-02–G1-03; approved `at 15:00` syntax.

Independent probes:

| Input | Actual |
|---|---|
| `Call tomorrow at 15:00` | Valid; Due at 15:00 |
| `Call tomorrow at 15:00 #work` | Invalid; diagnostic says incomplete `at`; Due remains midnight |
| `Call tomorrow at 15:00 p1` | Same failure |
| `Call at 15:00 tomorrow` | Same failure |

**Cause:** `\s*` before optional AM/PM greedily consumes the separator following the 24-hour time. The matched span now ends at the next token, so the separate boundary check rejects it; fallback classifies only `at` as incomplete.

**Required fix:** consume whitespace only when a meridiem actually follows, or trim candidate spans before boundary checking without losing source accounting. Recognition must work regardless of metadata order.

**Regression gate:** assert equivalent resolved times for these permutations, multiple spaces, punctuation, and AM/PM forms, while retaining rejection of invalid hours, missing meridiem, duplicate times, and DST folds.

## Requirement / evidence status

| Area | Re-review status |
|---|---|
| NLC-01 date/time inference | FAIL — R4; original invalid-time/DST failures corrected |
| NLC-02 G2 recurrence integration | FAIL — R1/R3 |
| NLC-03 effort / NLC-04 precedence | Original reported failures corrected; existing plus targeted tests pass |
| NLC-05 review/correction | FAIL — R1/R2 |
| NLC-06 prose preservation | Original reported examples corrected; broader grammar still needs coverage |
| NLC-07 local/deterministic parsing | Supplied-clock and 30-second fuzz tests pass; zero-time fallback remains a documented limitation from the first review |
| NLC-08 safe mutation boundary | argv/Taskwarrior path retained; unresolved confirmation R1 prevents full acceptance |
| T00/T07 dateformat/timezone interoperability | Still incomplete evidence: integration capture test covers one local absolute date, not the planned configuration/timezone matrix |
| Live UAT | NOT PERFORMED |

The original report remains historical. This document supersedes its F1–F10 disposition for the current working tree. Do not interpret the old list as ten still-unfixed defects, or the new gate pass as feature approval.

## Next action

Address **R1–R4**, add the stated regressions, reconcile the tracker/evidence, then rerun acceptance validation. No new product decision is needed: these are failures of already approved behavior. Live keyboard/visual UAT follows automated acceptance, not substitutes for these fixes.
