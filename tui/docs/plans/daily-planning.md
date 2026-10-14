# Implementation plan — task-first daily planning ritual

**Goal:** [G3 — Capacity-aware daily planning ritual](task-first-daily-planner-goals.md#g3--capacity-aware-daily-planning-ritual)
**Status:** Implemented; automated acceptance coverage present; live keyboard UAT pending

## Decisions

- A commitment is represented by the inspectable Taskwarrior tag `momentum-plan-YYYY-MM-DD`.
- The tag is removed/replaced on confirmation; due dates and scheduled dates are never changed by the ritual.
- Overdue/due/scheduled tasks are fixed obligations. Previously tagged unfinished work is rollover; other pending work is a candidate.
- Capacity defaults to 8 hours with a 1-hour buffer. `[planning].weekday_capacity` accepts per-weekday duration overrides.
- Missing estimates are counted separately and never coerced to zero.
- Calendar minutes are an optional enhancement supplied by the local ICS source in the G4 plan.
- Confirming a plan runs UUID-targeted Taskwarrior `modify` commands through the existing serialized mutation and sync-grace lifecycle. Partial failures refresh the export and leave the ritual open for an idempotent retry.

## Acceptance evidence

| Requirement | Implementation | Automated evidence |
|---|---|---|
| RIT-01/10 | `internal/app/planner.go`, `internal/ui/planner.go` | planner open/cancel/reopen and current-tag projection tests |
| RIT-02/03/04 | `internal/domain/planner.go`, planner UI | domain separation and tag-only diff tests |
| RIT-05/06 | `internal/app/commands.go`, `model.go` | serialized plan mutation and partial retry path |
| RIT-07/08/09 | `config/planning.go`, domain capacity summary, UI warning | config and planner rendering tests |

The durable representation remains visible through the regular `task export` and can be edited with the regular CLI. No Momentum plan database or background service exists.

## Independent demo

1. Load pending tasks containing an overdue/due task, an unfinished previous-day plan tag, an estimated candidate, and an unestimated candidate.
2. Press `P`, inspect the separate sections, toggle candidates with Space, and observe capacity/over-capacity warning.
3. Cancel and verify no mutation call.
4. Confirm and inspect `task export`: selected work has today's `momentum-plan-YYYY-MM-DD` tag, deselected plan tags are gone, and dates are unchanged.
5. Reopen `P`; the current plan is shown rather than duplicated.
