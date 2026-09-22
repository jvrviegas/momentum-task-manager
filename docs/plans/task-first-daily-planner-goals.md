# Goal plan — Task-first daily planner

**Created:** 2026-09-20  
**Status:** Proposed; product outcomes defined, individual feature specifications and implementation plans pending  
**Product direction:** Keep Momentum task-first and keyboard-first. Add the parts of Akiflow that improve capture and daily commitment without building a calendar application.

## Purpose

Momentum should help a user:

1. capture an actionable task in their own words;
2. maintain recurring responsibilities;
3. understand today's fixed commitments;
4. choose a realistic set of tasks for the day.

Taskwarrior remains the source of truth for tasks. Calendar data is contextual, read-only input to planning—not a second task store or a time-blocking surface.

## Tracking model

A goal advances through these states:

1. **Proposed** — outcome and boundaries are recorded here.
2. **Specified** — product behavior and policy decisions have acceptance criteria.
3. **Planned** — a trackable implementation plan exists with dependencies and gates.
4. **Implemented** — automated acceptance criteria pass.
5. **Validated** — live user-facing UAT is recorded.

| Goal | Outcome | Status | Specification | Implementation plan | UAT evidence |
|---|---|---|---|---|---|
| G0 | Tasks can carry optional effort estimates | Implemented | [EST-01–EST-05](#g0--optional-task-effort-estimates) | [Implementation tracker](task-estimates.md) | [Automated evidence](../UAT.md#g0-estimate-implementation-evidence-2026-09-21); live UAT pending |
| G1 | Command bar understands common natural-language task input | In progress — R1–R4 fixed; R5 recurrence-preview correction required | [G1-01–G1-23](natural-language-capture.md#acceptance-contract) | [Implementation tracker](natural-language-capture.md) | [Final review: R5 open](../reviews/g1-natural-language-capture-final-review.md); live UAT pending |
| G2 | Recurring tasks can be created and maintained safely | Implemented — automated evidence; live UAT pending | [REC-01–REC-08](recurring-tasks.md) | [Implementation tracker](recurring-tasks.md) | Isolated Taskwarrior recurrence lifecycle; live UAT pending |
| G3 | A daily ritual produces a deliberate, capacity-aware task plan | Implemented — automated evidence; live UAT pending | [RIT-01–RIT-10](daily-planning.md) | [Implementation tracker](daily-planning.md) | Domain/app/UI plan tests; live UAT pending |
| G4 | Daily planning reflects read-only calendar commitments | Implemented — local ICS provider; automated evidence; live UAT pending | [CAL-01–CAL-09](calendar-awareness.md) | [Implementation tracker](calendar-awareness.md) | ICS fixture tests; live UAT pending |
| G5 | The complete workflow is coherent, documented, and regression-tested | Implemented — automated gates pass; live UAT pending | [INT-01–INT-07](integrated-workflow.md) | [Implementation tracker](integrated-workflow.md) | Integrated code/docs/tests and full gates pass; live UAT pending |

Update this table as each feature receives its own specification, implementation tracker, and UAT record. A feature is not complete merely because its implementation plan was written.

## Product principles

- **Task-first:** no full calendar view, event editing, drag-and-drop time blocking, or calendar replacement.
- **Local and explicit:** preserve Momentum's offline-first behavior. Any calendar network access must be opt-in and clearly documented.
- **Taskwarrior-compatible:** use supported Taskwarrior concepts where they fit; do not create a competing task database or recurrence engine.
- **Progressive enhancement:** ordinary quick capture and task management must continue to work without estimates, recurrence, calendar access, or network access.
- **Explain before mutating:** ambiguous natural language, recurrence changes, and daily-plan commits must be reviewable before they alter tasks.
- **Keyboard-first:** every new workflow must be fully usable without a mouse and remain responsive at supported terminal widths.
- **Honest capacity:** estimates and calendar load are planning aids, not promises or automatic schedulers.
- **No telemetry:** success is established through explicit UAT and user feedback, not background product analytics.

## G0 — Optional task effort estimates

### Outcome

The user can express roughly how much focused work a task requires, allowing the daily ritual to compare selected work with available capacity.

### Goal-level acceptance criteria

- **EST-01:** A task may have no estimate or one human-readable estimate.
- **EST-02:** Capture, structured editing, details, and daily planning use the same estimate semantics.
- **EST-03:** Unestimated tasks remain fully valid and are counted separately during planning rather than treated as zero effort.
- **EST-04:** Estimates represent focused task effort, not calendar start/end times.
- **EST-05:** Storage remains interoperable with Taskwarrior and synchronization behavior is documented.

### Decision gates

- Choose storage: supported Taskwarrior field if available and portable, or a documented UDA.
- Choose accepted granularity and syntax, such as `15m`, `30m`, `1h`, `2h`, and `half-day`.
- Decide whether the first release accepts arbitrary durations or a constrained set.

### Not included

Automatic duration prediction, productivity scoring, or automatic time-block creation.

## G1 — Natural-language command-bar capture

### Outcome

The user can type common task phrases without remembering trigger syntax while retaining the speed and safety of the existing command bar.

Example:

```text
Prepare the proposal tomorrow at 3pm, p1, every Friday, about 1h #work
```

Before submission, Momentum should be able to present the interpreted task fields separately from the remaining description.

### Goal-level acceptance criteria

- **NLC-01:** Common dates and times can be extracted from ordinary phrases.
- **NLC-02:** Supported recurrence phrases can populate the recurrence definition from G2.
- **NLC-03:** Supported effort phrases can populate the estimate from G0.
- **NLC-04:** Existing explicit triggers (`#`, `!`, `@`, `>`, `+`) remain supported and have documented precedence over inferred values.
- **NLC-05:** The user can review and correct the parsed description and metadata before mutation when interpretation is ambiguous.
- **NLC-06:** Unrecognized prose remains task description text; the parser must not silently discard words.
- **NLC-07:** Parsing works locally and deterministically. No remote LLM or task-content transmission is required.
- **NLC-08:** Taskwarrior performs final mutation validation, and command execution continues to use argument vectors rather than a shell.

### Decision gates

- Define the supported language and locale for the first release.
- Define ambiguity rules for phrases such as `Friday`, `next Friday`, and bare times.
- Decide whether review is always shown or appears only for ambiguous/richer captures.
- Define precedence when natural language and explicit triggers disagree.

### Not included

Open-ended conversational task creation, generative decomposition, or claiming to understand arbitrary prose.

## G2 — Recurring-task management

### Outcome

The user can create, inspect, edit, and stop common recurring responsibilities without leaving Momentum.

### Goal-level acceptance criteria

- **REC-01:** Quick capture and structured editing support common patterns: daily, weekdays, weekly, selected weekday, monthly, and an interval such as every two weeks.
- **REC-02:** A recurrence definition has a clear anchor/first due date and a preview of its next expected occurrence.
- **REC-03:** Details distinguish a recurring template from a generated task instance where Taskwarrior exposes that distinction.
- **REC-04:** Editing or stopping recurrence explains which template or instance is affected and does not silently rewrite completed history.
- **REC-05:** Completing, skipping, deleting, or modifying an occurrence follows documented Taskwarrior behavior.
- **REC-06:** Momentum relies on Taskwarrior's recurrence semantics rather than running a background recurrence service.
- **REC-07:** Behavior across synchronized devices is documented, including Momentum's existing single recurrence-primary requirement.
- **REC-08:** Unsupported recurrence expressions remain safe: either pass through for Taskwarrior validation or fail with actionable guidance.

### Decision gates

- Verify recurrence behavior against the supported Taskwarrior 3.x version in an isolated database.
- **Decision:** initial presets are daily, weekdays, weekly, selected weekday, monthly, and positive day/week/month intervals such as every two weeks; unsupported expressions fail with guidance rather than bypassing validation.
- **Decision:** exports expose Taskwarrior parent/mask metadata; Details labels generated instances, recurrence-only edits target the template, and stopping expires the template with `until:today`.
- **Decision:** Momentum does not toggle recurrence generation; the existing recurrence-primary deployment policy remains documented and the normal Taskwarrior export is shown if generation is delayed.

### Not included

A Momentum-owned scheduler, background process while Momentum is closed, or retroactive changes to completed occurrences.

## G3 — Capacity-aware daily planning ritual

### Outcome

The user can intentionally choose today's work instead of treating every pending task as equally actionable.

### Proposed ritual

1. Review fixed obligations: overdue tasks, tasks due today, and—after G4—calendar meetings.
2. Review rollover work previously planned but unfinished.
3. Review candidate tasks from the Inbox.
4. Select or remove tasks from today's commitment.
5. Compare selected estimates with available focus capacity.
6. Confirm the plan and enter the normal Today view.

### Goal-level acceptance criteria

- **RIT-01:** The ritual is user-initiated and can be reopened; it does not block normal startup or force completion once per day.
- **RIT-02:** Overdue and due-today tasks are shown as obligations, distinct from tasks the user voluntarily commits to today.
- **RIT-03:** Candidate selection is keyboard-first and preserves project, priority, deadline, and estimate context.
- **RIT-04:** The user can add or remove planned tasks without changing their actual due dates.
- **RIT-05:** Confirming the ritual records today's selected task commitment using an explicit, Taskwarrior-compatible representation.
- **RIT-06:** Canceling leaves tasks unchanged. Partial mutation failures are reported honestly and can be retried safely.
- **RIT-07:** Capacity shows configured work time, fixed commitments, configurable buffer/break time, estimated selected work, and the count of unestimated tasks.
- **RIT-08:** Exceeding capacity is visibly warned about but does not prevent the user from confirming.
- **RIT-09:** The ritual remains useful without calendar configuration by using configured daily capacity and task obligations alone.
- **RIT-10:** Reopening the ritual shows the current plan rather than duplicating commitments.

### Decision gates

- **Decision:** daily commitments use the namespaced Taskwarrior tag `momentum-plan-YYYY-MM-DD`, not `scheduled`; this keeps commitments distinct from actual scheduling and needs no Momentum database or new UDA.
- Define rollover behavior for tasks planned on an earlier day but unfinished.
- Define candidate ordering and whether suggestions are automatic or merely sorted.
- Define working-day capacity, breaks, buffers, and per-weekday configuration.
- Define how changes made outside Momentum are reconciled while the ritual is open.

### Not included

Automatic plan confirmation, productivity grading, calendar time-block creation, or rescheduling task deadlines to make a plan fit.

## G4 — Read-only calendar awareness

### Outcome

During daily planning, the user can see how many meetings already occupy the day and how much focus capacity remains, without managing the calendar in Momentum.

### Goal-level acceptance criteria

- **CAL-01:** Calendar access is optional and read-only; Momentum never creates, edits, accepts, declines, or deletes events.
- **CAL-02:** The ritual shows today's relevant events with start/end times and a meeting-count summary.
- **CAL-03:** Capacity calculations merge overlapping busy intervals so overlapping events are not double-counted.
- **CAL-04:** Events outside configured working hours do not reduce working capacity by default.
- **CAL-05:** All-day events and free/transparent events have explicit, configurable treatment rather than silently consuming the whole day.
- **CAL-06:** Multiple calendars can be included or excluded, and duplicate instances are handled predictably.
- **CAL-07:** Calendar failure, stale data, missing credentials, or offline operation does not block task planning; the ritual clearly marks calendar capacity as unavailable or stale.
- **CAL-08:** Event details are not persisted in Taskwarrior, debug logs, or telemetry. Any local cache has a documented purpose, lifetime, and location.
- **CAL-09:** Calendar setup and doctor diagnostics reveal configuration state without exposing credentials or private event content.

### Blocking policy and feasibility gate

Momentum currently promises no application network traffic outside Taskwarrior sync. Before implementation planning, choose and document the first calendar source and amend that privacy contract if necessary. Options to evaluate include:

- a local or subscribed ICS source;
- CalDAV;
- direct provider APIs such as Google Calendar;
- a local operating-system calendar bridge.

The choice must be assessed for Linux/macOS support, authentication, recurring-event expansion, timezone handling, offline behavior, secret storage, and maintenance burden. Provider selection is decided for the first implementation: explicitly configured local ICS files. Momentum does not fetch URLs or store credentials; an external calendar client may refresh files. CalDAV/provider APIs remain deferred until a separate privacy/maintenance decision.

### Not included

Calendar browsing beyond planning context, event search, event mutation, invitations, attendee management, or drag-and-drop time blocking.

## G5 — Integrated workflow quality

### Outcome

The features behave as one workflow rather than four unrelated additions.

### Goal-level acceptance criteria

- **INT-01:** A recurring task captured in natural language can carry an estimate, generate occurrences through Taskwarrior, and appear as a daily-planning candidate or obligation as appropriate.
- **INT-02:** The daily plan distinguishes deadlines, recurrence, user commitment, and calendar commitments without conflating them.
- **INT-03:** Existing Inbox, Today, Completed, search, sync, undo, project settings, and quick-trigger behavior retain regression coverage.
- **INT-04:** Every new mutation participates correctly in refresh, synchronization, undo limitations, and unsynced-quit behavior.
- **INT-05:** Wide, compact, narrow, empty, offline, stale-calendar, and error states have automated rendering coverage and live keyboard UAT.
- **INT-06:** README, help, configuration examples, doctor output, privacy documentation, and UAT evidence describe the shipped behavior accurately.
- **INT-07:** The full project safety gates in `CONTRIBUTING.md` pass, and integration tests never access real Taskwarrior or calendar data.

## Dependency map

```text
G0 Estimates ───────────────┐
                            ├──> G3 Daily ritual ──┐
G2 Recurrence ──────────────┘                     ├──> G5 Integrated workflow
                                                  │
G4 Calendar awareness ──> G3 calendar enhancement ┘

G1 Natural-language capture
  ├── can ship basic date/time parsing independently
  ├── depends on G0 semantics for estimate phrases
  └── depends on G2 semantics for recurrence phrases
```

G3 should first ship as a useful task-only ritual, then incorporate G4. This keeps calendar-provider complexity from blocking the core planning workflow.

## Suggested delivery sequence

This is a product sequence, not an implementation plan:

| Milestone | User-visible increment | Goals |
|---|---|---|
| M1 | Tasks can carry and display optional effort estimates | G0 |
| M2 | A task-only daily ritual creates a realistic commitment using configured capacity | G3 without G4 |
| M3 | Common recurring tasks can be managed safely | G2 |
| M4 | The ritual accounts for read-only meetings and remaining focus time | G4 + G3 enhancement |
| M5 | The command bar understands agreed natural-language dates, recurrence, and estimates | G1 |
| M6 | Cross-feature polish, documentation, and live UAT | G5 |

G1's basic date/time slice may be delivered earlier if desired, but estimate and recurrence phrases must use the final G0/G2 semantics rather than inventing parallel representations.

## Overall success scenarios

The program is complete when live UAT demonstrates all of the following:

1. **Capture:** Enter a natural-language task with date, priority, project, estimate, and recurrence; review the interpretation; create the intended Taskwarrior task.
2. **Recurrence:** Generate and complete occurrences without rewriting history or creating duplicates across synchronized devices.
3. **Plan without calendar:** Open the ritual offline, review obligations and rollover, select estimated and unestimated tasks, confirm, and see the same commitment in Today.
4. **Plan with calendar:** Load a day containing overlapping and all-day events, show the correct meeting load according to configuration, and warn when selected work exceeds remaining capacity.
5. **Degrade safely:** Repeat planning with calendar access unavailable; task planning remains usable and no stale calendar result is represented as current.
6. **Interoperate:** Inspect and modify the resulting tasks through the regular `task` CLI without a Momentum-owned task database.

## Global non-goals

- Full calendar UI or event management
- Automatic time blocking
- Automated task scheduling or AI-generated daily plans
- Remote LLM processing
- Momentum-owned task persistence or recurrence service
- Background execution while Momentum is closed
- Team planning, assignment, or shared capacity
- Productivity scores, streaks, or telemetry

## Next planning steps

Create one specification and one trackable implementation plan for each goal, rather than one cross-cutting implementation tracker. Recommended order:

1. Execute the approved [G0 estimate implementation tracker](task-estimates.md);
2. G3 — task-only daily ritual;
3. G2 — recurring tasks;
4. G4 — calendar awareness after its provider/privacy feasibility decision;
5. Execute the remaining feasibility gate in the [G1 natural-language capture implementation tracker](natural-language-capture.md); O1–O9 are approved and its basic capture slice may ship before G2, but recurrence integration remains blocked on G2;
6. G5 — integration and release validation.

Each feature plan must link back to the requirement IDs in this document, record unresolved product decisions before implementation, identify independent tracer-bullet demos, and include isolated Taskwarrior/calendar test safeguards.
