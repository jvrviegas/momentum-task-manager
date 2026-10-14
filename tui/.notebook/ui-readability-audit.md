# UI Readability Audit
> Current density causes hierarchy and scanning problems despite responsive width safety

Entry: `internal/app/view.go:Model.renderBase()`

## Baseline findings

The original audit found:

- The shell stacked product title, view title, body, and footer without vertical breathing room; the sidebar had only a one-column separation.
- Task rows compressed description, project, date, and abbreviated priority onto one line. Inbox repeated project metadata beneath project group headings.
- Dates did not label whether they were due or scheduled, and priorities rendered as raw `H/M/L`.
- Wide mode began at 80 columns and immediately removed 25 columns from main content.
- Details and help truncated fixed label/value lists by height.
- Modal routes replaced the underlying shell and each form assembled its own hierarchy.
- Styling had semantic colors but no shared spacing, section-heading, status, action, or focus vocabulary.
- No deterministic production-like render fixtures were checked in.

## Implemented decisions (2026-09-17)

- `internal/ui/layout.go` centralizes a 1-cell local rhythm, 2-cell section gaps, modal width budgets, the 104-column sidebar boundary, the 50-column comfortable-row boundary, and the 28×8 minimum warning. `Layout.Geometry()` is shared by rendering and mouse hit testing.
- `internal/ui/tasklist.go` renders `TaskBlock` values. Comfortable rows put description/state on line one and labeled metadata on line two; Inbox task blocks omit project metadata because `internal/app/view.go:taskBlocks()` renders project headings. Selected blocks use a left gutter marker on every line in both Unicode and ASCII modes.
- `internal/app/view.go` composes one page header, grouped shell navigation, explicit Today counts, section gaps, and a two-line footer when height permits. Search occupies a real shell row rather than being appended after a clipped base view.
- `internal/ui/details.go` and `help.go` wrap content and retain body scroll state. `theme.go` supplies shared bounded modal rendering for details, help, capture, edit, confirmations, and Settings surfaces.
- Deterministic ANSI-stripped fixtures live in `internal/app/testdata/render/` and `internal/ui/testdata/render/`. They are compared by default and updated only with `MOMENTUM_UPDATE_GOLDENS=1`; sanitized Today captures for wide/compact/narrow review live in `docs/screenshots/`.
- `internal/ui/sidebar.go:RenderTabs()` underlines the active tab with `Icons.RuleActive` on a separate row, so active navigation remains identifiable after ANSI/color stripping; `TabIndexAt()` uses the same `tabLayout()` geometry.

## Validation outcome

Automated width, height, color-stripped content, block-selection, mouse, workflow, race, vet, cross-build, and fixture checks pass. The maintainer accepted live dark/light and Unicode/ASCII presentation, task mouse selection after retest, and keyboard/Settings/quit flows in the isolated synthetic profile. `docs/screenshots/uat-*.png` retains wide/compact/narrow real-terminal captures. Exact cell sizes were not measured for those images; the transparent light-theme background shows terminal wallpaper and is a recorded release-image limitation. Nerd Font was not checked live. See `docs/UAT.md` for the T07 sign-off; no personal Taskwarrior data was used.

Updated: 2026-10-02
