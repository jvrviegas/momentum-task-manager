# UI Flow
> Charm v2 model and responsive pure components

Entry: `internal/app/model.go:Model.Update()`
Flow: typed Bubble Tea message → overlay precedence → pure app transition or async command → `internal/ui` component render.

Current modules: `charm.land/bubbletea/v2 v2.0.9`, `charm.land/bubbles/v2 v2.2.1`, `charm.land/lipgloss/v2 v2.0.6`.
- v2 models return `tea.View`; keyboard events are `tea.KeyPressMsg`.
- `ui.ChooseLayout()` uses 80/50/minimum breakpoints; `ui.Truncate()` delegates ANSI-aware display-width truncation.
- Search filters in-memory view data; Today section rendering re-filters each section while preserving domain order. `internal/ui/search.go:parseSearchQuery()` recognizes an exact, case-insensitive `project:<name>` qualifier (`project:none` means unassigned) and applies remaining terms through the existing fuzzy field match.
- Inbox order comes from `internal/domain/views.go:SortInboxTasks()` (project alphabetically, creation oldest-first, unassigned/missing dates last). `internal/app/view.go:renderTaskBody()` inserts project headers after filtering and accounts for headers when scrolling to the selected UUID; navigation still indexes the flat task list. Today retains urgency ordering.
- Quick capture renders as a centered responsive modal with persistent metadata-trigger guidance; contextual completions remain owned by `internal/ui/quickadd.go:QuickAddModel`.

Updated: 2026-09-08
