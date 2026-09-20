# UI Flow
> Charm v2 model and responsive pure components

Entry: `internal/app/model.go:Model.Update()`
Flow: typed Bubble Tea message → overlay precedence → pure app transition or async command → `internal/ui` component render.

Current modules: `charm.land/bubbletea/v2 v2.0.9`, `charm.land/bubbles/v2 v2.2.1`, `charm.land/lipgloss/v2 v2.0.6`.
- v2 models return `tea.View`; keyboard events are `tea.KeyPressMsg`.
- `ui.ChooseLayout()` uses 104/50/minimum breakpoints and exposes `RowDensity`, actual `MainWidth`, and a two-cell wide-mode gutter; `ui.Truncate()` remains ANSI-aware.
- `ui.Layout.Geometry()` is the shared vertical shell contract. Header, tabs, search, body, footer gaps, and footer rows are budgeted before rendering; `internal/app/update.go:taskUUIDAt()` consumes the same body geometry as `internal/app/view.go`.
- Task rows are `ui.TaskBlock` values. Comfortable blocks use a primary line plus labeled metadata; compact rows remain single-line. `ui.VisibleTaskBlockRange()` keeps blocks and their UUIDs together, including both-line mouse hits.
- Search filters in-memory view data; Today section rendering re-filters each section while preserving domain order. `internal/ui/search.go:parseSearchQuery()` recognizes an exact, case-insensitive `project:<name>` qualifier (`project:none` means unassigned) and applies remaining terms through the existing fuzzy field match.
- Inbox order comes from `internal/domain/views.go:SortInboxTasks()` (project alphabetically, creation oldest-first, unassigned/missing dates last). `internal/app/view.go:taskBlocks()` inserts project headers and section gaps after filtering, omits repeated Inbox project metadata, and accounts for block heights when scrolling; navigation still indexes the flat task list. Today retains urgency ordering.
- Completed is loaded through a separate context-filtered export, retains its last successful snapshot on archive-only refresh failures, and derives Today/Yesterday/Earlier sections over 30 local calendar days. Rows show project/completion time and task-targeted mutations are blocked.
- Details and help use wrapped body lines with local scroll state; forms and confirmations use the shared bounded panel vocabulary in `internal/ui/theme.go`.
- Quick capture renders as a compact centered modal. Its idle metadata guide is replaced by field-specific completions while a trigger is active; completion state remains owned by `internal/ui/quickadd.go:QuickAddModel`.

Updated: 2026-09-08
