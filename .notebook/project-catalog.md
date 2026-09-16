# Project Catalog
> Momentum config-backed suggestions; never task entities

Entry: `internal/config/config.go:LoadWithOptions()` → `domain.ProjectCatalog.Validate()`.
Flow: `app.NewModel()` seeds both overlays before discovery → `Model.Update(ProjectsMsg)` merges configured catalog with discovered values → `ui.SetProjectCatalog()` snapshots values/labels.

- `internal/domain/project.go`: dotted full values encode hierarchy; `Labels()` resolves configured ancestor names, `Merge()` copies sources and retains configured order.
- `internal/quickadd/suggest.go:SuggestionsWithProjectLabels()`: label/value fuzzy matching; `Suggestion.Text` remains the parser-safe token. Display labels must never enter `ApplySuggestion()` output.
- `internal/ui/quickadd.go:View()`: suggestion window follows selection for catalogs longer than five entries.
- `TagsMsg` uses existing `SetCatalog()` deliberately: preserves project labels. Failed `ProjectsMsg` leaves the current catalog untouched; successful empty discovery retains configured entries only.
- `config.Config` contains a slice and is no longer comparable; use `Config.IsZero()` for constructor default detection.
- User-facing configuration semantics: `README.md` → Project suggestion catalog. Personal catalog stays in local configuration, not compiled defaults.

Updated: 2026-09-16
