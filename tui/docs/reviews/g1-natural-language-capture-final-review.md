# G1 final review — recurrence preview requires correction

**Date:** 2026-09-22  
**Baseline:** current uncommitted working tree over `36b80bb`.  
**Verdict:** **CHANGES REQUESTED for full G1 — R5 below.** Prior R1–R4 remain resolved. This is a new interoperability finding from comparing the review preview with actual Taskwarrior-generated occurrences, not a reopening of the narrow-layout repair.

**Resolution (2026-09-22):** R5 is fixed in `internal/domain/recurrence.go`. Permanent domain, review-rendering, and isolated differential integration tests now cover preserved time-of-day, month-end/leap-year clamping, interval semantics, and DST transitions against Taskwarrior 3.5 generated occurrences. README guidance now correctly distinguishes natural-language review from explicit `^` fast capture. Automated gates pass; T10 live keyboard/visual UAT remains pending.

## Fresh validation

| Check | Result |
|---|---|
| `go test -json ./... -count=1` | PASS: 9 packages, 491 top-level test/fuzz entries, 640 passing test actions, no failures/skips |
| `go test -race ./... -count=1` | PASS |
| `go vet ./...`, formatting, `git diff --check` | PASS |
| Native, Linux AMD64, macOS ARM64 builds | PASS; temporary build output paths |
| Existing isolated Taskwarrior integration suite | PASS: 16 scenarios |
| Interpreter fuzz target | PASS: 30 seconds |
| Additional isolated dateformat/timezone matrix | PASS: 16 cases, detailed below |
| Additional recurrence-preview comparisons | **FAIL: daily time-of-day and monthly end-of-month mismatch** |
| Live keyboard/visual UAT | NOT PERFORMED |

The complete current suite includes the prior review fixes and the new long-value/cursor regression. No implementation code was changed in this review. Temporary probe tests were removed after execution; logs and probe sources are saved locally under `/tmp/momentum-g1-final-review/`. No production task profile, sync server, or production sync was used.

## R5 — Review's next occurrence disagrees with Taskwarrior (high)

**Sources:**

- `internal/domain/recurrence.go:110–128` — `RecurrenceNext` resets the anchor to midnight and uses Go `AddDate` for monthly recurrence.
- `internal/ui/quickadd_review.go:617` — review displays the helper's result as `Next occurrence`.

**Requirements:** G1-12, G1-20 / NLC-02, and G2 REC-02/REC-06 (correct preview using Taskwarrior semantics).

### Reproduction A: time-of-day is lost

A disposable Taskwarrior profile was configured with `recurrence.limit=2`, and the regular adapter created a daily task due **2026-09-23 15:00 Europe/Lisbon**.

| Value | Observed |
|---|---|
| First actual occurrence | 2026-09-23 15:00 local (14:00 UTC) |
| Second actual occurrence | **2026-09-24 15:00 local** (14:00 UTC) |
| Momentum next-occurrence preview | **2026-09-24 00:00 local** |

`RecurrenceNext` calls `midnight(value)` before applying the recurrence. Consequently a natural capture with an explicit time, such as `Review every Friday at 3pm`, shows the wrong next time even though Taskwarrior persists the intended time.

### Reproduction B: month-end overflow differs from native recurrence

A second isolated task used **2027-01-31 00:00 local**, recurrence `monthly`, again with `recurrence.limit=2`.

| Value | Observed |
|---|---|
| First actual occurrence | 2027-01-31 00:00 |
| Second actual occurrence | **2027-02-28 00:00** |
| Momentum next-occurrence preview | **2027-03-03 00:00** |

Go's `AddDate(0, 1, 0)` normalizes an invalid February 31 into March. The observed Taskwarrior monthly occurrence uses February's final valid day instead. This is not merely presentation formatting: the predicted date itself is wrong.

### Required fix

1. Preserve the anchor's wall-clock time in supported recurrence previews.
2. Match Taskwarrior's observed month-end behavior rather than relying on Go's overflow normalization.
3. Codify preview-vs-export comparisons in the isolated integration suite using the existing temporary-profile guards.
4. Cover daily, weekly, weekdays, monthly/interval presets, month-end/leap-year boundaries, and relevant timezone transitions. For behavior not yet verified, display preview unavailable instead of a confidently incorrect timestamp.
5. Add review rendering assertions for correct next occurrence, not only helper-unit expectations that duplicate the implementation.

**Acceptance:** preview instant/date equals the actual next generated occurrence for supported patterns, including both failing cases above. The UI must never silently promise a different next occurrence from Taskwarrior.

This affects review accuracy, not the observed stored mutation values. The native adapter created correct task occurrences in both probes.

## Dateformat/timezone feasibility evidence now observed

The additional matrix exercised interpreter → `domain.NewTask` → actual adapter Add → export → Undo in **16 independent temporary profiles**:

- Zones: UTC, Europe/Lisbon, America/New_York, Australia/Lord_Howe.
- Taskwarrior `dateformat`: `Y-M-D`, `D/M/Y`.
- Input: `Probe tomorrow`, `Probe tomorrow at 15:30`.
- Reference: 2026-09-21 10:00 in each zone.

For each case the exported due instant equaled the interpreted local absolute timestamp. This closes the earlier missing runtime evidence for those specific format/zone combinations. The existing DST unit tests and original fold probes remain separate evidence; this matrix is not a proof of all possible zones/calendar dates. Probe source: `date_matrix_probe.go`; output: `date-matrix.log` in the directory above. Consider retaining the matrix as permanent regression coverage.

## Documentation consistency follow-up

`README.md:78` says explicit `^` recurrence forms require review, while `docs/plans/recurring-tasks.md` describes explicit `^` as fast capture. The current interpreter retains the explicit-only fast path when no natural-language candidate exists. Align the README with the intended G2 policy or deliberately change/test the interaction; do not leave contradictory user guidance. Also reconcile completed tracker rows with their still-unchecked task-body checklists before final sign-off.

This is documentation/acceptance cleanup, not evidence that the G1 O3 explicit-trigger fast path should be removed.

## Prior findings

| Findings | Current status |
|---|---|
| Original capture fallback, stale/busy lifecycle, scalar duplicates, protected prose, invalid-time detection, DST handling, correction/recovery | Prior regression tests remain passing |
| R1 no-op recurrence conflict bypass | Resolved; regression passes |
| R2 long-value/cursor clipping | Resolved; 50 layout/text/field combinations plus formatted Due regression pass |
| R3 plural recurrence intervals | Resolved; regression passes |
| R4 24-hour time followed by tokens | Resolved; regression passes |
| R5 native recurrence preview equivalence | **Open — reproduced against real isolated Taskwarrior** |

## Completion boundary

Do not mark full G1 Validated. Address R5 and documentation consistency, rerun the preview comparisons and final gates, then perform the planned live keyboard/visual UAT. Basic capture fixes remain accepted within their reviewed scope; this finding is in the G1b/G2 recurrence-preview integration.
