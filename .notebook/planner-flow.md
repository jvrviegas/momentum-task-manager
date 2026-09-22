# Daily planning flow
> Taskwarrior-tagged daily commitment ritual

Entry: `internal/app/planner.go:OpenPlanner()`
Flow: pending export → `domain.BuildDailyPlan()` → planner overlay → UUID/tag diffs → serialized `MutationPlan` → refresh.

Durable marker: `domain.DailyPlanTagFor()` → `momentum-plan-YYYY-MM-DD`; no plan database and no due-date mutation.
- Obligations: `domain.ClassifyToday()` result; fixed in UI.
- Rollover: prior plan marker on unfinished task; selected by default.
- Candidates: remaining pending tasks; Space toggles selection.
- Partial plan writes refresh the export and leave the planner open; rerunning `BuildPlanMutations()` is idempotent.

Capacity: `config.PlanningConfig` → configured focus minutes − estimated fixed obligations − merged calendar busy minutes − buffer. Missing estimates are counted separately.

Updated: 2026-09-21
