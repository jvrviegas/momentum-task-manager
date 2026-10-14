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

Delivered follow-up: `docs/plans/settings-projects.md` tracks T01–T11; `docs/spec/settings-projects.md` defines SP-01–SP-16; `docs/adr/0001-project-settings-and-pending-task-renames.md` records the accepted policies (active context, child-removal prompt, duplicate-catalog rejection, catalog-first partial outcomes, no batch undo, symlink preservation). `Settings → Projects` now owns the catalog editor, source-preserving config store, preview/confirmation, and guarded pending-only migration. O2-R/O3-T remain implemented: Include subprojects covers catalog/task descendants as applicable, and task-only destinations require an effective-merge warning/confirmation. Final automated gates and evidence are in `docs/UAT.md`; human visual terminal UAT is explicitly distinguished there.

Updated: 2026-09-16
