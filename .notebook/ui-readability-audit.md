# UI Readability Audit
> Current density causes hierarchy and scanning problems despite responsive width safety

Entry: `internal/app/view.go:Model.renderBase()`

Findings:
- The shell stacks product title, view title, body, and footer without vertical breathing room; the sidebar is separated from content by only one column (`internal/app/view.go:Model.renderBase()`).
- Task rows compress description, project, date, and abbreviated priority into one line. Inbox also repeats project metadata beneath project group headings (`internal/ui/tasklist.go:RenderTaskRow()`, `internal/app/view.go:Model.renderTaskBody()`).
- Date metadata does not label whether it is due or scheduled, and priority renders as raw `H/M/L`, increasing interpretation cost (`internal/ui/tasklist.go:relevantDate()`, `internal/ui/tasklist.go:RenderTaskRow()`).
- Wide mode begins at 80 columns and immediately removes 25 columns from main content; the transition to 79-column tabs is abrupt (`internal/ui/layout.go:ChooseLayout()`).
- Details and help are fixed label/value lists that truncate by height rather than wrapping or scrolling (`internal/ui/details.go:DetailsModel.View()`, `internal/ui/help.go:RenderHelp()`).
- Modal routes replace the underlying shell rather than retaining visual context (`internal/app/view.go:Model.render()`).
- Styling has semantic colors but no shared spacing, section-heading, keycap, status, or focus tokens (`internal/ui/theme.go:Styles`).
- No production screenshots are checked in, so visual regressions are covered indirectly by width/content assertions only (`docs/screenshots/README.md`, `internal/app/composition_test.go`).

Recommended direction:
- Make a comfortable two-line task row the default at usable sizes, with title on line one and explicitly labeled metadata on line two.
- Introduce a small shared spacing rhythm (1-cell local, 2-cell section separation), stronger section headings with counts, and a 2-column content gutter.
- Remove redundant project badges from Inbox rows because project headers already carry that information.
- Rework shell hierarchy into one page header, content section, and stable status/action footer; avoid duplicate titles.
- Give details/help/edit surfaces consistent modal shells, wrapped or scrollable content, and contextual action footers.
- Add ANSI-stripped golden render fixtures for wide, compact, narrow, dark, light, empty, selected, and modal states before broad visual refactoring.

Updated: 2026-09-17
