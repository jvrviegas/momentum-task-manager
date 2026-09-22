# G1 re-review 2 — remaining R2 fixed

> **Repair verified (2026-09-21):** R2's long-value/cursor clipping is fixed. See [repair evidence](#r2-repair-evidence). The [final pass on 2026-09-22](g1-natural-language-capture-final-review.md) confirms R1–R4 remain resolved but identifies a separate R5 recurrence-preview mismatch. The pre-fix reproduction below is historical.

**Date:** 2026-09-21  
**Baseline:** current uncommitted working tree over `36b80bb`.  
**Current verdict:** **R1–R4 reported blockers resolved; R2 regression and automated gates pass.** Live UAT and the previously identified broader feasibility-evidence gaps are not certified by this repair.  
**Previous review:** [R1–R4](g1-natural-language-capture-rereview.md).

## Fresh gate results

- Full suite: **9 packages, 489 top-level test/fuzz entries, 588 passing test actions, no failures/skips** (`go test -json ./... -count=1`).
- Race (`-count=1`), vet, formatting, native build, Linux AMD64 build, macOS ARM64 build: PASS.
- Isolated Taskwarrior integration: PASS, 16 scenarios.
- Interpreter fuzz target: PASS, 30 seconds.
- Original R1–R4 independent parser/UI probes rerun: initial reproductions now behave correctly.
- Additional long-value narrow-review test: **FAIL**, Description and Project both hide the newly typed character.
- Live keyboard/visual UAT: not performed.

Raw results and the additional probe source are in `/tmp/momentum-g1-rereview2/`. Temporary probe files were removed from source directories. No implementation code was changed, no commits were created, and no production task data/sync server was used.

## Disposition

| Finding | Result | Observed behavior |
|---|---|---|
| R1 no-op recurrence conflict bypass | RESOLVED for reported reproduction | Typing x then deleting back to `weekly` returns `QuickAddErrorMsg`, not submit; explicit decisions and semantic changes are now distinguished |
| R2 narrow field visibility | PARTIAL — remaining blocker below | Short field values and full compact Due value are visible; long focused values and their editing cursor are still truncated |
| R3 plural recurrence intervals | RESOLVED | `every 2 weeks` → `2wks`, `every 2 days` → `2days`, both require review |
| R4 24-hour times before following tokens | RESOLVED | `tomorrow at 15:00 #work`, `tomorrow at 15:00 p1`, and `at 15:00 tomorrow` resolve to 15:00 correctly |

## R2 follow-up — input width exceeds actual row budget (high)

**Source:** `internal/ui/quickadd.go:SetSize` and `internal/ui/quickadd_review.go:reviewView`.  
**Requirements:** G1-12–G1-13, G1-17.

At narrow widths, `SetSize` gives every review text input `contentWidth - 6` columns. But `reviewView` prefixes each input with the full field name, provenance marker, and a space. Description's prefix is 13 columns, not 6; Project's is 9. The final whole-row `Truncate` cuts off the right side of the input, including the cursor. Bubbles cannot compensate: its internal viewport thinks it has more usable columns than the row actually renders.

### Reproduction

1. Capture `A sufficiently long description tomorrow #work.client.project`.
2. Open review and resize to **28×8**.
3. Focus Description, move the cursor to the end, and type `Z`.
4. Repeat for Project.

Actual probe output (ANSI styling omitted):

```text
Description value: A sufficiently long descriptionZ
Rendered row:      Descriptionr ong desc…

Project value:     work.client.projectZ
Rendered row:      Projecte .client.proj…
```

The typed `Z` is present in each stored field but absent from the rendered view. The editing cursor at the end is also clipped. Confirmation remains enabled. Short-value tests now pass, but they do not exercise text input scrolling or end-of-value editing.

### Required correction

Compute each field's input width from its actual rendered prefix width and cursor allowance, using the same compact/comfortable layout decision in sizing and rendering. Alternatively put the input on its own line. Avoid a second truncation that clips the focused input's own viewport. Account for the comfortable Due row's extra formatted-date prefix too.

### Regression acceptance

- At 28×8, focus long Description, Project, Scheduled, and Tags values; move Home/End and type/delete characters.
- Assert the newly typed character and cursor position are visible in the focused row, not merely the field label or a prefix of the value.
- Cover 49×18, 79×24, and wide layouts with long values and Unicode display widths.
- Preserve full Due inspection, diagnostic scrolling, and panel bounds.
- Where a safe editable review genuinely cannot fit, disable confirmation rather than permitting hidden edits.

## R2 repair evidence

The field prefix/suffix is now computed once by `reviewFieldAffixes` and shared by input sizing and rendering. Each input reserves the actual display width of its label, provenance, optional formatted Due value, closing bracket, and extra Bubbles cursor cell. The compact-layout decision is also shared. Width changes explicitly refresh Bubbles' horizontal viewport via `SetCursor(Position())`, because `SetWidth` alone does not recalculate scrolling.

Changes are confined to `internal/ui/quickadd.go`, `internal/ui/quickadd_review.go`, and the new `internal/ui/quickadd_review_layout_test.go`, plus evidence/tracking documentation. Existing implementation work was preserved.

Regression was written and observed failing before the fix. It now passes for long ASCII and wide Unicode values in Description, Project, Scheduled, Tags, and Due at 28×8, 49×18, 79×24, 120×30, and 120×8. Home/End insertion and deletion assert both the newly typed character and the complete styled input/cursor viewport survive rendering, with terminal width/height bounds. A separate case verifies the formatted Due prefix does not hide its editable timestamp.

Fresh results after the fix:

- `go test -json ./... -count=1`: **9 packages, 491 top-level test/fuzz entries, 640 passing actions, zero failures/skips**.
- Race (`-count=1`), vet, formatting, native/Linux AMD64/macOS ARM64 builds, and `git diff --check`: PASS.
- Isolated Taskwarrior integration: PASS, 16 scenarios.
- Prior R1/R3/R4 regressions remain passing in the full suite.

Logs: `/tmp/momentum-g1-r2-red.log` (pre-fix failure), `/tmp/momentum-g1-r2-green.log`, and `/tmp/momentum-g1-r2-fix/` (final gates). No commit, production sync, or live UAT was performed. R2 is closed; do not infer that this focused repair establishes the missing dateformat/timezone interoperability matrix or human UAT.
