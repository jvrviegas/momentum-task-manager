# Implementation plan — integrated task-first workflow

**Goal:** [G5 — Integrated workflow quality](task-first-daily-planner-goals.md#g5--integrated-workflow-quality)
**Status:** Implemented in code and automated tests; live UAT pending

## Delivered integration

- G0 estimates are shared by domain parsing, capture, editor, details, rows, recurring capture, and planning capacity.
- G1 interpretation is local and deterministic. Natural dates/times, `about`/`for` effort, `p1`/`p2`/`p3`, recurrence phrases, explicit precedence, source retention, review, correction, and explicit-only fallback are covered in `internal/quickadd/interpret.go` and the review overlay.
- G2 recurrence uses Taskwarrior templates/instances and native generation. Details expose parent/template context; stopping expires the template.
- G3 plan commitments use a namespaced Taskwarrior tag and the normal add/modify refresh, sync grace, undo, and unsynced-quit lifecycle.
- G4 is a local-only, read-only ICS enhancement. Calendar errors never block task planning.

## Safety gates

- All subprocesses remain `exec.CommandContext` argv calls.
- Plan and recurrence mutations use UUIDs and isolated Taskwarrior integration tests.
- Calendar tests use fixture files and never access a real calendar account.
- Existing views, search, settings/project migration, Completed read-only behavior, responsive fixtures, and sync state tests remain in the full package gate.

## Remaining human validation

Live UAT must exercise wide/compact/narrow planner layouts, offline planning, partial calendar failure, recurring capture/stop, natural-language review around midnight/timezone boundaries, estimate-UDA refusal, and regular `task` CLI interoperability. Automated evidence does not replace that visual/user check.
