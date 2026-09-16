# Settings → Projects: feature requirements

**Date:** 2026-09-16  
**Status:** Implemented; acceptance evidence is recorded in [docs/UAT.md](../UAT.md) and the execution tracker. All product policies were confirmed on 2026-09-16.  
**Decision:** [ADR 0001](../adr/0001-project-settings-and-pending-task-renames.md)  
**Tracking / handoff entry point:** [Implementation plan](../plans/settings-projects.md)

## Problem and scope

Users previously edited `[[projects]]` manually and restarted Momentum. Settings → Projects now manages the existing suggestion catalog in-app and can optionally migrate a changed project value on eligible pending tasks without rewriting history.

### Terminology

- **Display name:** `Project.Name`, e.g. `Carbon Products`; labels the final path segment.
- **Project value:** `Project.Value`, e.g. `carbon-products.dash2zero`; stored on a Taskwarrior task.
- **Descendant:** value starting with `old + "."`, not merely starting with `old`.
- **Pending:** exactly Taskwarrior `status:pending`. Do not silently include waiting, recurring-template, completed, or deleted statuses. Verify version-specific export behavior in isolated integration tests.
- **Catalog-only save:** edits Momentum configuration without any Taskwarrior mutation.
- **Migration:** explicitly confirmed project-value reassignment on eligible pending tasks.

## Agreed functional requirements

### Story A — Manage common projects without leaving Momentum (P1)

| ID | Acceptance criterion |
|---|---|
| SP-01 | WHEN navigating the app, Settings SHALL be reachable from the wide sidebar and via keyboard at all supported terminal widths; its first section SHALL be Projects. It SHALL not fall through to Inbox or expose task actions against a hidden task selection. |
| SP-02 | WHEN opening Projects, Momentum SHALL list configured entries with readable hierarchy and stored values, including projects with no tasks. An empty catalog SHALL offer creation. Discovered-only task values SHALL not silently become configured entries. |
| SP-03 | WHEN adding/editing an entry, the user SHALL be able to change its display name, value, and hierarchy/parent and see the full resulting dotted value before saving. Parent selection must compose a value rather than create a second persisted hierarchy model. |
| SP-04 | WHEN validating a draft, existing catalog rules SHALL apply: non-empty display name without control characters; lowercase alphanumeric/hyphen/dotted values; unique full values, rejecting duplicate catalog destinations rather than merging (including descendant mapping collisions). Invalid drafts SHALL remain editable and cause no writes. The UI SHALL prevent invalid hierarchy moves. |
| SP-05 | WHEN removing an entry with configured descendants, Momentum SHALL ask whether to remove the children too and show the affected count. No removes only the selected entry, retaining descendants and their values; Yes removes the configured subtree including deeper descendants; Cancel changes nothing. It SHALL never delete, complete, or unassign tasks. |
| SP-06 | WHEN the user explicitly saves, Momentum SHALL persist to the active config file (including `--config`), update the in-memory catalog and both suggestion surfaces without restart, and retain normal merging with discovered values. Canceled/unsaved drafts SHALL not affect disk, live suggestions, or tasks. Navigation/quit with a dirty draft SHALL require an explicit discard or save choice. |
| SP-07 | WHEN only the display name changes, Momentum SHALL update labels/breadcrumbs and SHALL NOT offer or execute a task-value migration. Existing task project values SHALL remain unchanged. |

**Independent demo:** Create an empty project, use it immediately in quick capture and the project editor, restart and verify persistence; then remove the entry without changing any task.

### Story B — Rename current work without rewriting history (P1, dependent on Story A)

| ID | Acceptance criterion |
|---|---|
| SP-08 | WHEN the project value changes, the save flow SHALL offer **Catalog only** (default) and **Catalog + pending tasks** (explicit opt-in). Migration options SHALL reset for each edit and never inherit consent from a previous edit. |
| SP-09 | WHEN migration is chosen, only eligible tasks with `status:pending` in the **active Taskwarrior context** SHALL receive the new project value. Today/search filters SHALL NOT restrict or broaden migration scope. Completed/deleted tasks and unrelated task fields SHALL remain unchanged by Momentum. No other status is implicitly eligible. |
| SP-10 | WHEN **Include subprojects** is off, a parent value rename SHALL affect only the selected catalog entry and exact-match pending tasks if migration is selected. WHEN on, the same explicit option SHALL rename configured descendant values and, only when task migration is selected, eligible dot-delimited pending-task descendants, preserving suffixes. `workshop` SHALL NOT match a rename of `work`. The preview SHALL show catalog-descendant and task-descendant counts separately; descendants SHALL never move silently. |
| SP-11 | BEFORE a migration, Momentum SHALL show old→new values, eligible task count, active context (or explicitly no active context), descendant choice, and any collision/merge implications; duplicate catalog values SHALL block saving rather than offer a merge; a destination used by Taskwarrior tasks but absent from the catalog SHALL require an explicit effective-merge warning and confirmation; it SHALL state that historical tasks retain their values. No task mutations SHALL occur before explicit confirmation. Zero matches SHALL be a valid catalog save with no task mutations. |
| SP-12 | WHEN a preview becomes stale, Momentum SHALL NOT broaden the confirmed task set or alter newly ineligible tasks. Revalidate on confirmation; changed eligibility/mapping/scope, including a changed active context, SHALL require a fresh preview and confirmation. Execution SHALL remain pending-only even if a task is completed after preview. |

**Independent demo:** Seed pending, completed, deleted, waiting, and subproject tasks in an isolated Taskwarrior database. Confirm a pending-only rename; verify only the explicitly previewed eligible set changes. Repeat with the subproject toggle off/on.

## Required engineering safeguards

These are derived safety/quality constraints, not claims that the user selected a particular implementation library or transaction protocol.

| ID | Acceptance criterion |
|---|---|
| SP-13 | Saving SHALL preserve unrelated TOML settings and comments; SHALL NOT serialize environment-derived overrides as file settings; and SHALL detect external file changes rather than blindly overwrite them. Use safe same-directory replacement and deliberate permissions handling. For symlinked configs, preserve the link and safely update its resolved target with conflict checks on both link resolution and target content; do not replace the symlink. On parse, permission, conflict, or write error, retain the user's draft and the previous live catalog. Missing config can be created at the resolved path. |
| SP-14 | Config and Taskwarrior operations SHALL execute asynchronously through typed app results. Migration SHALL be serialized with Momentum task writes and sync; the UI SHALL remain responsive. Block/defer competing actions and quit during execution with clear status; no hidden task commands from Settings keybindings. |
| SP-15 | After validation/preflight, save the catalog **before** migrating the confirmed tasks. If catalog saving fails, no task migration SHALL run. If task updates fail afterward, retain the saved catalog and successful task updates, report their outcomes separately, refresh task state after any task changes, and require fresh preview/confirmation to retry. No automatic rollback/retry or batch undo. Explain before confirmation that native `u` cannot reverse the migration/config save; disable it during migration and prevent misleading prior undo state after task changes, while preserving ordinary single-task undo afterward. Task changes alone participate in native sync; catalog-only edits SHALL not trigger task undo/sync bookkeeping. |
| SP-16 | Tests SHALL isolate filesystem/config writes and all real Taskwarrior commands from the user's data. Include read-only config, discovery errors, stale previews, failure mid-migration, history retention, hierarchy boundary matching, and width/input-routing regressions. Test hooks/concurrent task changes enough to establish the chosen adapter's guarantees; do not disable real-user hooks as an implementation shortcut. |

## Behavior examples

| Edit | Choice | Expected task effect |
|---|---|---|
| Name `Carbon Products` → `Carbon`; value unchanged | Label only | No Taskwarrior mutations; descendant breadcrumbs reflect the new ancestor label. |
| Value `carbon-products` → `carbon` | Catalog only | No Taskwarrior mutations. Old task values may still appear through discovery. |
| Same value change | Pending tasks; descendants off | Eligible tasks at exact `carbon-products` become `carbon`; child and historical tasks retain values. |
| Same value change | Pending tasks; descendants on | Eligible `carbon-products.dash2zero` becomes `carbon.dash2zero`; suffix and all historical values remain intact. |
| Remove catalog `carbon-products` | No to removing children | Remove only the parent catalog entry; retain descendant entries/values. No task mutation. |
| Remove catalog `carbon-products` | Yes to removing children | Remove the parent and configured descendants. No task mutation. |
| Rename to a value held by another catalog entry | Any mode | Reject before saving or migrating; no catalog merge. |

## Policy decisions / remaining gates

**ADR 0001** records the user's choices: active context; ask about children on removal; reject duplicate catalog values; catalog-first save with honest partial results and explicit retry; no batch undo; preserve symlinks and safely update their targets. These are agreed, not recommendations awaiting approval. With no active Taskwarrior context, all otherwise eligible pending tasks are naturally in scope and the preview must say so; there is no global override option.

The user subsequently resolved **O2-R** and **O3-T**: one explicit Include subprojects option covers configured descendants and eligible pending-task descendants as applicable, with separate counts; destinations used only by Taskwarrior are allowed after an explicit effective-merge warning and confirmation. No product-policy questions remain. T00 technical feasibility is recorded below; these notes constrain implementation without changing the acceptance criteria or reopening O1–O6.

### T00 technical feasibility notes — 2026-09-16

Isolated probes used temporary Taskwarrior/config paths only. Taskwarrior 3.5.0's `export` command deliberately ignores the active context, so migration must read `_get rc.context` and `_get rc.context.<name>.read`, then pass the read filter explicitly (parenthesized as one argv element) together with `status:pending recur.none:`. Use `project.is:` for exact values. Reconcile each UUID after successful and ambiguous commands because a successful modify can be silent and a no-match guard returns exit 1.

The existing BurntSushi encoder is not source-preserving. T00 verified a lossless AST editor candidate for indexed `[[projects]]` edits, comments, and interleaved tables; the eventual store must still validate through the existing strict loader and atomically replace the resolved regular target, never the symlink path. Hooks remain enabled and may reject or further modify a Taskwarrior change; outcomes must be reconciled and reported. The catalog-first sequence is intentionally not transactional: config failure means zero task changes, while later task failure/quit may leave saved config and earlier task successes. Full evidence is in the [T00 feasibility record](../plans/settings-projects.md#t00-feasibility-record--2026-09-16).

## Out of scope

- Theme/icons/refresh/sync settings editors (Settings should allow future sections, but do not implement them now).
- Project dashboards, progress metrics, archived project states, separate database or stable IDs.
- Migrating completed/deleted/waiting/recurring-template tasks, changing recurrence definitions, or retroactively rewriting history.
- General bulk task editing, automatic merges, automatic catalog growth from tasks.
- Cloud sync of the catalog, a new sync protocol, or direct access to Taskwarrior data files.
- Cross-store transactional rollback, native batch undo guarantees, or a persistent migration journal unless separately approved after a demonstrated need.

## Completion

All SP-01–SP-16 have implementation evidence in the plan's traceability table and automated acceptance evidence in [docs/UAT.md](../UAT.md); all applicable O1–O6 decisions are recorded. Catalog management and the explicit pending-only migration are shipped. Actual keyboard affordances are documented in the README and help screen, with existing task controls kept safe and discoverable. Live visual/keyboard sign-off remains the maintainer's final release check.
