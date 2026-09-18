# Implementation plan — UI readability and comfortable layout

**Created:** 2026-09-17  
**Status:** T07 in progress; 7 / 8 tasks complete (automated gates pass; live visual UAT remains)  
**Baseline:** `1605293` on `main`; recheck HEAD and the working tree before starting.  
**Design input:** [Momentum Design §6](../../DESIGN.md#6-visual-design) and [UI readability audit](../../.notebook/ui-readability-audit.md)  
**Primary scope:** `internal/ui`, `internal/app/view.go`, `internal/app/update.go`, rendering tests, screenshots, and UAT evidence

## Next-agent instructions

1. Read this plan, `DESIGN.md` §6, `.notebook/INDEX.md`, `.notebook/ui-flow.md`, `.notebook/ui-readability-audit.md`, `CONTRIBUTING.md`, and the relevant source/tests before editing.
2. Treat this file as the tracker. Change `[ ]` to `[~]` when starting and `[x]` only after the task gate passes. Update **Status**, the progress count, and the task's Evidence cell after each task.
3. Keep every task independently reviewable and tested. Prefer one focused commit per task; record its hash/message in the tracker. Do not push, publish, or rewrite unrelated work.
4. Preserve all behavior: Taskwarrior semantics, view contents/order, keyboard shortcuts, mouse support, search, sync state, settings workflows, and overlay precedence. This project changes presentation, not task behavior.
5. Keep rendering pure. UI components must not perform filesystem, network, Taskwarrior, or other subprocess work.
6. Verify current Charm v2 APIs in official documentation before introducing a viewport, layer/compositor, or unfamiliar Lip Gloss primitive. Do not add a dependency unless the standard library/current Charm stack cannot meet a recorded requirement.
7. Never use the user's Taskwarrior data for screenshots or UAT. Use deterministic fixture tasks and isolated temporary `TASKRC`/`TASKDATA` if a live TUI is needed.
8. Preserve ASCII, Unicode, dark, and light modes. Color may reinforce state but must never be its only signal.

**Suggested skills:** `codenavi` for repository navigation, `diagnose` for rendering/width regressions, and `handoff` if work stops before T07. This Markdown checklist is the requested tracking artifact; no external issue tracker is required.

## Objective

Make Momentum easier to scan and less visually cramped by introducing a deliberate spacing rhythm, clearer shell hierarchy, comfortable task rows, explicit metadata labels, and readable secondary screens without reducing terminal compatibility or changing interaction behavior.

## Approved design direction

```text
 MOMENTUM

 VIEWS
 ▌ Today          3       TODAY                               3 tasks
   Inbox         12
                         OVERDUE · 1
 MANAGE                  ▌ ● Renew domain registration
   Projects                  Due Sep 16, 10:00 · Personal · High

                         DUE TODAY · 1
                         ○ Review API proposal
                           Due 17:00 · Work

                         SCHEDULED · 1
                         ○ Prepare weekly planning
                           Scheduled 09:00 · Work
 ─────────────────
 Context: work
 Synced                  [c] Create  [/] Search  [?] Help  [q] Quit
```

The mockup communicates hierarchy and spacing, not exact glyphs or colors. Final output must use the configured icon mode and remain width-safe.

## Requirements

| ID | Requirement |
|---|---|
| UI-01 | Normal-width task rows separate the primary description from secondary metadata. |
| UI-02 | Metadata uses explicit labels such as `Due`, `Scheduled`, `High`, `Medium`, and `Low`; raw `H/M/L` is not the default presentation. |
| UI-03 | Inbox rows do not repeat project metadata already communicated by their project group heading. Today may retain project metadata because its grouping is temporal. |
| UI-04 | Page title, section headings, body, and footer use a consistent 1-cell local / 2-cell section spacing rhythm where terminal height permits. |
| UI-05 | Selection and focus combine color with a non-color cue such as a gutter marker, border, or explicit active label. Multi-line selected rows read as one selected unit. |
| UI-06 | Wide layout appears only when the remaining main content is readable. Initial target: sidebar at `>=104` columns, tabs below it; validate during UAT rather than silently reverting to 80. |
| UI-07 | Responsive modes are deterministic: comfortable rows at `>=50` columns, reduced single-line rows at `28–49`, and the existing minimum-size warning below `28×8`. |
| UI-08 | The footer separates contextual actions from context/sync/status information when space permits and degrades by priority without hiding active errors. |
| UI-09 | Details and help wrap or scroll rather than silently truncating inaccessible content. Existing close/edit controls remain available. |
| UI-10 | Quick capture, edit, confirmation, quit, and Settings surfaces share a consistent title/body/error/action hierarchy and safe viewport margins. |
| UI-11 | Wide, compact, narrow, minimum, dark, light, ASCII, empty, loading, error, selected, search, and modal states remain bounded by terminal dimensions. |
| UI-12 | Existing keyboard/mouse actions and task selection identity remain correct after multi-line rendering and scrolling changes. |
| UI-13 | Deterministic render fixtures and release screenshots make visual regressions reviewable. |

## Non-goals

- No permanent task-preview pane.
- No new task fields, Taskwarrior commands, views, navigation routes, or configurable keybindings.
- No density preference or runtime layout toggle in this iteration; comfortable is the default and responsive fallback handles constrained terminals.
- No animation, image protocol, Nerd Font requirement, or terminal-specific rendering path.
- No broad rewrite of the Bubble Tea model or migration/settings business logic.
- No palette redesign beyond contrast corrections required by readability testing.

## Verified current implementation map

| Area | Source | Planning implication |
|---|---|---|
| Shell composition | `internal/app/view.go:Model.renderBase()` | Product title, view title, body, and footer are vertically joined with no spacing abstraction. Sidebar has a one-column gutter. |
| Overlay composition | `internal/app/view.go:Model.render()` | Most overlays replace the base with a centered component; search appends an extra line and then clips through `fitLines`. |
| Task rows | `internal/ui/tasklist.go:RenderTaskRow()` | Every task is one line; metadata is suffix-packed and priority is raw. |
| Grouping/windowing | `internal/app/view.go:Model.renderTaskBody()` | Scrolling counts logical entries as one terminal line. Multi-line rows require height-aware blocks, not embedded newlines passed to the old range calculation. |
| Mouse mapping | `internal/app/update.go:Model.taskUUIDAt()` | Click-to-task mapping assumes fixed header offsets and one terminal line per task. It must consume the same render geometry as the view. |
| Responsive policy | `internal/ui/layout.go:ChooseLayout()` | Current breakpoints are 80/50 with a fixed 24-column sidebar; at 80 columns main content drops to 55 columns. |
| Styles | `internal/ui/theme.go:Styles` | Semantic colors exist, but spacing, section, keycap, focus, status, and modal-shell vocabulary does not. |
| Details/help | `internal/ui/details.go`, `internal/ui/help.go` | Content is cut to modal height; there is no scrolling and long values are truncated. |
| Forms | `internal/ui/quickadd.go`, `internal/ui/edit.go`, `internal/ui/projectsettings.go`, `internal/ui/projectrename.go` | Each surface assembles its own title, guidance, errors, and actions; spacing is inconsistent. |
| Existing tests | `internal/ui/*_test.go`, `internal/app/composition_test.go` | Tests cover content and width at 120/79/49/minimum but are not reviewable golden layouts. |
| Screenshots | `docs/screenshots/README.md` | No production screenshot is checked in. |

## Research basis

These references inform the plan; they are design guidance, not framework contracts:

- [Bijou design-system foundations](https://github.com/flyingrobots/bijou/blob/main/docs/design-system/foundations.md): spacing rhythm, hierarchy, restrained color, non-color focus cues, responsive degradation.
- [Textual style guide](https://textual.textualize.io/guide/styles/): padding aids readability; margins/gutters separate widgets; borders consume terminal cells and must be budgeted.
- [Command Line Interface Guidelines](https://clig.dev/): human-readable output, restrained color, scanable hierarchy, responsiveness, and actionable state.
- [TUI application patterns](https://github.com/hyperb1iss/hyperskills/blob/main/skills/tui-design/references/app-patterns.md): stable context, responsive collapse, selector/detail tradeoffs, and composing only what the workflow needs.

Official Charm documentation remains authoritative for Bubble Tea, Bubbles, and Lip Gloss APIs.

## Gate commands

| Gate | Command |
|---|---|
| Format | `test -z "$(gofmt -l .)"` |
| Unit | `go test ./... -count=1` |
| UI | `go test ./internal/ui ./internal/app -count=1` |
| Race | `go test -race ./...` |
| Vet | `go vet ./...` |
| Build | `go build ./cmd/momentum` |
| Cross-platform | `GOOS=linux GOARCH=amd64 go build ./cmd/momentum` and `GOOS=darwin GOARCH=arm64 go build ./cmd/momentum` |
| Diff hygiene | `git diff --check` |

## Tracker

| Done | ID | Deliverable | Depends on | Evidence / commit |
|---|---|---|---|---|
| [x] | T00 | Baseline render fixtures and visual contract | — | `internal/app/render_fixtures_test.go`, `internal/ui/render_fixtures_test.go`, checked-in `testdata/render/*`; opt-in update flag; UI tests pass twice |
| [x] | T01 | Shared style, spacing, and layout vocabulary | T00 | `internal/ui/layout.go`, `theme.go`; exact breakpoint/geometry tests; `go test ./internal/ui -run 'Theme|Layout'` |
| [x] | T02 | Readable shell, navigation, and footer hierarchy | T01 | `internal/app/view.go`, `internal/ui/sidebar.go`; shell/composition fixtures and app tests |
| [x] | T03 | Comfortable task rows and height-aware scrolling | T01, T02 | `internal/ui/tasklist.go`; block/selection tests, Today/Inbox fixture renders |
| [x] | T04 | Responsive reflow and geometry-correct mouse input | T03 | shared `ShellGeometry`, `internal/app/update.go`; boundary and second-line mouse tests |
| [x] | T05 | Readable, scrollable details and help surfaces | T01, T04 | wrapped/scrollable `details.go` and `help.go`; routing and trailing-field tests |
| [x] | T06 | Consistent capture, edit, confirmation, and Settings surfaces | T01, T04 | shared bounded panel plus updated form/confirmation components; existing workflow tests and component fixtures |
| [~] | T07 | Accessibility, screenshots, UAT, and final regression gate | T02–T06 | Automated fixture/unit gates pass; maintainer real-terminal visual UAT and sanitized release screenshots remain |


**Suggested sequence:** T00 → T01 → T02 → T03 → T04 → T05 → T06 → T07. T05 and T06 may be developed in parallel only after T04 if they do not edit shared style/composition files concurrently.

---

## T00 — Baseline render fixtures and visual contract

**Where:** proposed `internal/app/testdata/render/`, `internal/ui/testdata/render/`, rendering test helpers, `docs/screenshots/README.md`, this plan.  
**Depends on:** none.  
**Requirements:** UI-11, UI-13.

- [ ] Recheck HEAD and record any unrelated working-tree changes; do not overwrite them.
- [ ] Add a deterministic fixture model with representative overdue, due, scheduled, active, priority, project, long-description, and unassigned tasks. Use fixed time and no user data.
- [ ] Add ANSI-stripped golden text renders for at least `120×30`, `79×24`, `49×18`, and `28×8`.
- [ ] Cover Today, Inbox, selected row, empty, loading/error, search, details, help, quick capture, edit, Settings list, and Settings editor. Avoid combinatorial duplication: component goldens may cover modal variants while app goldens cover composition.
- [ ] Provide an explicit opt-in update mechanism such as a test flag or environment variable; normal test runs must compare, never rewrite fixtures.
- [ ] Keep semantic assertions for important content and input behavior; goldens supplement rather than replace focused tests.
- [ ] Record baseline limitations in fixture comments or a short README so expected intentional changes are reviewable in later tasks.

**Gate:** UI tests pass twice without modifying fixtures; `git diff --check` passes.  
**Commit:** suggested `test(ui): add deterministic render fixtures`

## T01 — Shared style, spacing, and layout vocabulary

**Where:** `internal/ui/theme.go`, `internal/ui/layout.go`, proposed small pure helpers and tests.  
**Depends on:** T00.  
**Requirements:** UI-04–UI-07, UI-10–UI-11.

- [ ] Add named style roles for page title, section title, supporting metadata, focus/selection gutter, key hints, status/error, and modal structure. Do not scatter new palette literals across components.
- [ ] Add centralized spacing and geometry constants/helpers: local gap `1`, section gap `2`, content gutter `2`, minimum modal edge margin `1` (prefer `2`), sidebar target width, and modal max widths.
- [ ] Replace terminal-width-only decisions with a layout contract that exposes actual regions and row density. Initial policy: wide/sidebar at `>=104`, tabs below; comfortable rows at `>=50`; compact rows at `28–49`; minimum below `28×8`.
- [ ] Ensure border and padding costs are included in content widths; every helper must return non-negative usable dimensions.
- [ ] Verify official current Lip Gloss/Bubbles APIs before selecting an ANSI-aware overlay, wrapping, or viewport mechanism. Record the selected approach in Evidence; avoid a new dependency if current Charm APIs suffice.
- [ ] Test exact boundary values (`27/28`, `49/50`, `103/104`) plus short heights and very wide terminals.

**Gate:** `go test ./internal/ui -run 'Theme|Layout' -count=1`; format and vet pass.  
**Commit:** suggested `refactor(ui): define readable layout and style vocabulary`

## T02 — Readable shell, navigation, and footer hierarchy

**Where:** `internal/app/view.go`, `internal/ui/sidebar.go`, related app/UI tests and goldens.  
**Depends on:** T01.  
**Requirements:** UI-04–UI-08, UI-11.

- [ ] Render one clear page header; remove the consecutive duplicate-feeling `Momentum`/view-title stack while retaining application identity.
- [ ] Add deliberate space between header, content sections, and footer when height permits. Degrade spacing before removing meaningful content on short terminals.
- [ ] Give sidebar navigation visible `VIEWS` and `MANAGE` grouping without adding a misleading Settings count. Preserve actual click targets and configured icons.
- [ ] Use the centralized 2-column content gutter in wide mode.
- [ ] Render section titles with counts (`OVERDUE · 1`, etc.) while preserving Today section order and Inbox project grouping.
- [ ] Split footer responsibilities: contextual actions and application status/context/sync. Define truncation priority so active errors and mutation/sync state survive before generic `? help · q quit` guidance.
- [ ] Keep search query/match state visible without appending a line that causes the top of the base view to be clipped.
- [ ] Preserve Settings routing and its own screen title without duplicate shell/component titles.

**Gate:** shell/component goldens reviewed at all target sizes; existing view/search/sync/settings composition tests pass.  
**Commit:** suggested `feat(ui): clarify shell navigation and status hierarchy`

## T03 — Comfortable task rows and height-aware scrolling

**Where:** `internal/ui/tasklist.go`, `internal/app/view.go`, related tests and fixtures.  
**Depends on:** T01, T02.  
**Requirements:** UI-01–UI-05, UI-07, UI-11–UI-12.

- [ ] Introduce a render result that can represent a task as a logical block with known terminal height and UUID. Do not rely on `strings.Split` or assume every item consumes one line.
- [ ] At comfortable widths, render description/state on line one and metadata on line two. Do not insert a blank line between adjacent tasks; section spacing supplies breathing room.
- [ ] Label dates as `Due …` or `Scheduled …` and priorities as `High`, `Medium`, or `Low`. Preserve overdue emphasis and active/completed/pending icons.
- [ ] Omit project metadata in Inbox task blocks because project headings provide it. Retain project metadata in Today blocks when present.
- [ ] Apply selection treatment to the full task block and add a non-color left-gutter cue. The cue must work in ASCII and Unicode modes.
- [ ] At compact row density, retain state + description and only the highest-priority metadata that fits; details remain available through Enter.
- [ ] Replace line-count scrolling with a height-budget algorithm that keeps the entire selected block visible where possible. Define behavior when a single block exceeds the viewport.
- [ ] Preserve UUID-based selection across filtering, refresh, completion, deletion, and view switches.
- [ ] Test long Unicode descriptions, ANSI display width, missing fields, due vs scheduled, all priorities, active/completed states, small heights, section transitions, and selected blocks at the first/last viewport positions.

**Gate:** `go test ./internal/ui ./internal/app -run 'Task|Today|Inbox|Composition|Selection|Search' -count=1`; updated goldens are reviewed.  
**Commit:** suggested `feat(ui): render comfortable task blocks`

## T04 — Responsive reflow and geometry-correct mouse input

**Where:** `internal/ui/layout.go`, `internal/ui/sidebar.go`, `internal/app/view.go`, `internal/app/update.go`, interaction tests.  
**Depends on:** T03.  
**Requirements:** UI-06–UI-08, UI-11–UI-12.

- [ ] Apply the approved layout modes at exact boundaries and verify the sidebar never leaves an unreadably narrow main region.
- [ ] Ensure wide, tabs/comfortable, tabs/compact, and minimum layouts all fit both width and height without negative sizes or clipped essential state.
- [ ] Derive mouse hit regions from the same shell/task geometry used for rendering. Remove hard-coded assumptions that tabs are always at row 1 or each task occupies one line.
- [ ] Clicking either line of a comfortable task selects the same UUID; clicking section gaps/headings does not select a different task.
- [ ] Preserve sidebar/tab navigation clicks and wheel behavior across all modes.
- [ ] Verify resize transitions preserve active view, selected UUID, search filter, Settings draft, and overlay state.
- [ ] Exercise heights where optional spacing must collapse and where only one task block fits.

**Gate:** responsive and mouse tests pass at boundary widths and at least three heights per mode; full UI gate passes.  
**Commit:** suggested `fix(ui): align responsive rendering and mouse geometry`

## T05 — Readable, scrollable details and help surfaces

**Where:** `internal/ui/details.go`, `internal/ui/help.go`, `internal/app/view.go`, `internal/app/model.go` only as needed for scroll routing, modal tests/goldens.  
**Depends on:** T01, T04.  
**Requirements:** UI-05, UI-09–UI-12.

- [ ] Build or reuse a bounded modal shell with stable title, body, and action footer regions plus safe edge margins.
- [ ] Group task details into understandable sections (core state, schedule, organization, technical/raw) without exposing less information than today.
- [ ] Wrap description, annotations, dependencies, and raw values to display width. Add vertical scrolling when content exceeds the body; do not silently discard trailing fields.
- [ ] Keep `e`, Enter, and Esc behavior; add discoverable `j/k` and arrow scrolling without allowing background actions through the overlay.
- [ ] Group help bindings into Navigation, Tasks, Search/Create/Edit, and Application. Use two columns only when both columns remain readable; otherwise stack and scroll.
- [ ] Keep generated binding data as the behavioral source of truth; category metadata must not duplicate key definitions in an unmaintainable second list.
- [ ] If the verified Charm stack supports safe base-plus-overlay composition, retain visible base context behind the modal. Otherwise keep centered bounded rendering and record the technical reason in Evidence rather than adding a fragile ANSI compositor.
- [ ] Test first/last scroll positions, resize while scrolled, long Unicode content, small terminal fallback, and all close/edit controls.

**Gate:** details/help component and routing tests pass; no supported field becomes inaccessible; modal goldens reviewed.  
**Commit:** suggested `feat(ui): make details and help readable and scrollable`

## T06 — Consistent capture, edit, confirmation, and Settings surfaces

**Where:** `internal/ui/quickadd.go`, `edit.go`, `confirm.go`, `quit.go`, `projectsettings.go`, `projectrename.go`, shared modal/form helpers, tests/goldens.  
**Depends on:** T01, T04.  
**Requirements:** UI-04–UI-05, UI-10–UI-12.

- [ ] Apply the shared title/body/error/action hierarchy to quick capture, task edit, delete/quit confirmation, Settings project editor, and rename preview.
- [ ] Visually separate field labels from values and focused state; focused fields require a non-color cue. Preserve changed-field markers and all input/suggestion semantics.
- [ ] Keep primary instructions readable rather than dimming every action hint. Reserve muted style for genuinely secondary explanation.
- [ ] Give validation/errors stable space when possible so an error does not unpredictably replace essential controls.
- [ ] Keep suggestion selection visible and bounded; preserve Tab/arrow/Ctrl+P/Ctrl+N behavior and project labels/values.
- [ ] Maintain quick-capture syntax guidance, but group it below the input rather than presenting an undifferentiated text stack.
- [ ] Preserve every Settings save/discard/remove/migration warning and outcome; presentation changes must not alter typed intents or asynchronous orchestration.
- [ ] Test narrow/short fallbacks, long labels and errors, suggestion overflow, dirty drafts, destructive confirmations, and all existing key routes.

**Gate:** all affected component tests and app workflow tests pass; goldens reviewed; no domain/config/taskwarrior production code changed.  
**Commit:** suggested `feat(ui): unify form and confirmation hierarchy`

## T07 — Accessibility, screenshots, UAT, and final regression gate

**Where:** UI/app tests, `docs/screenshots/`, `docs/screenshots/README.md`, `docs/UAT.md`, `README.md` if screenshots are embedded, `.notebook/ui-flow.md`, this plan.  
**Depends on:** T02–T06.  
**Requirements:** UI-01–UI-13.

- [ ] Verify critical state remains understandable with colors stripped: active view, selected/focused row, overdue, priority, error, active task, and sync state.
- [ ] Verify dark and light themes in a real terminal; correct only demonstrated contrast/readability problems and retain semantic token roles.
- [ ] Verify Unicode and ASCII modes live. Nerd mode may be automated unless the required font is available; record the limitation honestly.
- [ ] Capture deterministic wide (`>=120`), compact (`79–90`), and narrow (`49`) screenshots using fixture data. Do not include personal tasks, paths, usernames, sync identifiers, or secrets.
- [ ] Run keyboard/mouse smoke flows: switch views, scroll/select, search, details scroll/edit, quick capture suggestions, edit/save/cancel, complete/start/delete confirmation, Settings list/editor, help, sync/local-only status, resize, and quit.
- [ ] Run the full gate, race gate, cross-platform builds, and `git diff --check`.
- [ ] Update `docs/UAT.md` with exact commands, environment, automated evidence, manual results, and any limitations. Do not mark visual checks passed if they were not performed by a human in a real terminal.
- [ ] Update `.notebook/ui-flow.md` and the readability audit with durable implementation decisions and file pointers.
- [ ] Mark this plan complete only when all requirements have evidence and no unchecked task remains.

**Gate:** format, unit, race, vet, build, cross-platform, diff hygiene, and documented manual visual UAT pass.  
**Commit:** suggested `docs(ui): record readability redesign evidence`

## Acceptance matrix

| Requirement | Primary task | Required evidence |
|---|---|---|
| UI-01–UI-03 | T03 | Task-block unit tests and Today/Inbox goldens |
| UI-04 | T01, T02, T06 | Shared tokens plus shell/form goldens |
| UI-05 | T03, T05, T06 | Color-stripped focus/selection assertions |
| UI-06–UI-08 | T02, T04 | Exact breakpoint, footer-priority, and resize tests |
| UI-09 | T05 | Scroll/wrap tests proving final fields remain reachable |
| UI-10 | T05, T06 | Modal/form component goldens and preserved routing tests |
| UI-11 | T00, T04, T07 | Fixture matrix, display-width assertions, live UAT |
| UI-12 | T03–T06 | App selection, mouse, overlay, and workflow tests |
| UI-13 | T00, T07 | Checked-in goldens, sanitized screenshots, documented update process |

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Two-line rows reduce visible task count. | Use two lines without per-task blank rows; collapse to compact rows below 50 columns; test short heights. |
| Existing scrolling and mouse logic select the wrong task. | Represent rendered blocks with explicit height/UUID geometry and reuse that geometry for viewport and hit testing. |
| Borders/padding overflow by one or two cells. | Centralize box measurements and assert `lipgloss.Width` plus terminal line count at every boundary. |
| Goldens become noisy or brittle. | Strip ANSI, fix time/data, keep a small representative matrix, and retain semantic tests. |
| Modal scrolling conflicts with global navigation keys. | Preserve overlay-first routing and test that background mutations/navigation do not fire. |
| Accessibility is inferred from colors alone. | Add color-stripped assertions and non-color markers before visual UAT. |
| Scope expands into behavior or architecture changes. | Enforce non-goals and check diffs per task; business-logic packages should remain untouched. |
| Existing uncommitted work is overwritten. | Recheck status before every task, make surgical edits, and never reset/stash unrelated changes without instruction. |

## Completion record

Fill this section as work progresses.

| Date | Task | Result | Tests / evidence | Commit |
|---|---|---|---|---|
| 2026-09-17 | T00–T06 | Implemented | `go test ./... -count=1`, fixture comparison twice, `git diff --check` | — |
| 2026-09-17 | T07 | In progress | Automated visual contract is checked in; live terminal UAT is still required | — |

## Deviations and decisions

Record any accepted departure before implementing it. Include the reason, affected requirements/tasks, and approval source.

| Date | Decision | Reason | Affected tasks | Approved by |
|---|---|---|---|---|
| 2026-09-17 | Use pure wrapped line blocks and shared shell geometry instead of a viewport/compositor dependency | Current Charm stack already provides the required Lip Gloss width/style primitives; task identity and mouse hit testing need explicit block geometry, and an ANSI compositor would add fragility | T01–T05 | This plan's rendering-purity and no-unnecessary-dependency constraints |
