# ADR 0001 — Project settings and opt-in pending-task renames

- **Date:** 2026-09-16
- **Status:** Accepted and implemented; product policies, T00 feasibility, and T01–T11 delivery recorded 2026-09-16. Acceptance evidence: [UAT](../UAT.md).
- **Source:** User conversation following commits `d8c1728`, `84bb513`, and `b5c567a`
- **Requirements:** [Settings → Projects specification](../spec/settings-projects.md)
- **Delivery:** [Trackable implementation plan](../plans/settings-projects.md)

## Context

Momentum already has a config-backed project suggestion catalog. A catalog entry has a human-readable `name` and a full dotted Taskwarrior `value`. Taskwarrior itself has no independent project record: each task carries its project value. The current catalog is edited manually in TOML and loaded at startup.

The user wants to edit the catalog inside Momentum and optionally apply a project rename to current pending tasks while retaining historical project values on completed tasks.

## Decision

1. Add a **Settings** destination in app navigation, initially containing **Projects**. This is configuration management, not another task dashboard.
2. Continue storing the catalog in Momentum's active TOML config file. Do not introduce a separate project database, stable project IDs, or Taskwarrior project entities.
3. Support creating, editing, and removing catalog entries in the app, preserving dotted hierarchy and showing the resulting Taskwarrior value. Successful explicit saves update suggestions without restarting.
4. Distinguish **display-name edits** from **project-value changes**:
   - Editing `name` changes catalog labels only. It does not rename task project values.
   - Editing `value` defaults to **Catalog only**.
   - The user can explicitly choose **Catalog + pending tasks** for a value change. Completed and deleted tasks keep their existing project values.
5. Preview the affected task count and require confirmation before task changes. Offer an explicit **Include subprojects** choice; descendant renames preserve suffixes (`old.child` → `new.child`). Never silently broaden scope.
6. Removal is a catalog operation, not task deletion, completion, or clearing task project fields. Explain that a removed value may still be discovered from Taskwarrior.
7. Preserve the active config path (including `--config`), unrelated settings, and comments. Report failures honestly; a config file and Taskwarrior are not one transaction.

## Alternatives not selected

- **TOML-only management:** remains supported, but is not convenient enough for routine edits.
- **Always rename all tasks:** destroys the user's intended historical grouping and lacks consent.
- **Automatically rename tasks after a label edit:** conflates presentation with the actual stored project value.
- **Independent project database/status/dashboard:** unnecessary for a suggestion catalog and outside the requested scope.

## Consequences

- Taskwarrior remains authoritative for all task data. The new migration is an explicit task operation, not an automatic catalog side effect.
- This is a narrow future exception to the original bulk-operations non-goal, not authorization to build general bulk task editing.
- The existing single-task mutation, sync grace, and native undo paths cannot be assumed to provide atomic batch migration or cross-store rollback.
- The existing project-catalog implementation and its manual config workflow remain valid until this feature ships.
- Existing historical and remaining pending values can still appear through Taskwarrior discovery. This is not a failed catalog rename.

## Policy decisions — user confirmation, 2026-09-16

The user explicitly chose active context, asking about children on removal, rejection of duplicate catalog values, and the recommended partial-failure, undo, and symlink behavior. These choices supersede the earlier recommendations; do not ask the user to approve them again.

| ID | Status | Recorded decision |
|---|---|---|
| O1 | Resolved | Migrate pending tasks in the **active Taskwarrior context** only. Show its name in preview; with no active context, explicitly show that all otherwise eligible pending tasks are in scope. Never use Today/search filters as migration scope. A changed context invalidates the preview. No global override option. |
| O2 | Resolved | When removing a parent with configured descendants, **ask whether to remove the children too**. Show the affected entries/count. No removes only the selected entry and retains descendant values; Yes removes its whole configured subtree, including deeper descendants. Cancel changes nothing. For a parent value rename, one explicit **Include subprojects** option renames configured descendant values and, only when pending-task migration is selected, eligible pending-task descendants. Preview catalog and task counts separately. Never silently move descendants. |
| O3 | Resolved | **Reject duplicate catalog values**, including collisions created by any descendant mapping. Exclude the edited entry itself when checking an unchanged value. Do not merge catalog entries or use confirmation to bypass this rejection. If the destination is used by Taskwarrior tasks but has no catalog entry, allow the rename only after an explicit warning/confirmation describing the effective task-project merge. |
| O4 | Resolved | Validate/preflight both stores, **save the catalog first**, then migrate the confirmed task set. A failed catalog save means no task migration. Keep a successfully saved catalog and successful task updates if later updates fail; report outcomes separately and require a fresh preview and explicit confirmation to retry. No automatic rollback or retry. |
| O5 | Resolved | **No batch undo initially.** Explain before confirmation that `u` cannot reverse the migration or config save; disable it during migration and prevent prior undo state from being misrepresented after task changes. Preserve ordinary single-task undo afterward. |
| O6 | Resolved | **Support symlinked configs:** preserve the link and safely update its resolved target with conflict checks. Do not replace the link or choose blanket refusal of symlink saves as the normal implementation. Unsafe/unresolvable target or changed-link cases should fail without clobbering data. |

### Technical feasibility record — 2026-09-16

T00 used isolated temporary Taskwarrior data and a temporary TOML source; no user task data or local catalog was touched.

- Taskwarrior 3.5.0 accepts a guarded single-task command of the form `task <uuid> (<active-read-filter>) status:pending recur.none: project.is:<old> modify project:<new>`. `status:pending` excludes currently waiting tasks, `recur.none:` excludes recurring templates and instances, and `project.is:` is required for exact matching. A stale task that became completed after preview was rejected in-command; no-match returned exit 1, while a successful modify could return exit 0 with no output, so UUID re-export reconciliation is mandatory.
- Taskwarrior's machine-readable `export` deliberately ignores the active context. The active name is available from `_get rc.context`, and its read filter from `_get rc.context.<name>.read`; wrapping that filter in parentheses as one argv element made explicit export/modify filtering obey O1. No active context means no extra filter. The pre-existing generic export adapter must not be treated as context-safe for migration.
- Normal hooks remain enabled. An `on-modify` hook receives original and modified JSON, can reject the operation, or can change additional fields. Momentum must preserve hooks and report/reconcile their effects; it must not disable them to claim isolation.
- BurntSushi/toml v1.6.0 cannot preserve source comments/layout through its encoder. T00 verified `github.com/smm-h/go-toml-edit v0.4.3` as a lossless AST candidate for indexed `[[projects]]` edits, interleaved tables, and comments. Its generic writer must not be used on a symlink path: the store must resolve and snapshot the link/target, validate the edited bytes, and atomically replace the resolved target in its own directory while retaining the link and target mode.
- Catalog-first migration is intentionally a sequence rather than a transaction. Config failure means zero task commands; later timeout/hook/task failure or quit can leave the saved catalog and earlier task successes. Report those outcomes separately, never expand the confirmed UUID set, and require fresh preview/confirmation for retry. Native undo is per last Taskwarrior action and cannot undo the catalog or a batch.

Official references used: [hooks](https://taskwarrior.org/docs/hooks/), [modify](https://taskwarrior.org/docs/commands/modify/), [context](https://taskwarrior.org/docs/context/), [filter](https://taskwarrior.org/docs/filter/), and [export](https://taskwarrior.org/docs/commands/export/). Full command output, filesystem probes, dependency details, and baseline gate results are in [T00](../plans/settings-projects.md#t00-feasibility-record--2026-09-16).

### Product-policy completion

On 2026-09-16 the user also accepted the O2-R and O3-T recommendations: the single explicit Include subprojects option covers configured descendants and eligible pending-task descendants as applicable, with separate preview counts; a destination used only by Taskwarrior is allowed after an explicit effective-merge warning and confirmation.

No product decisions remain open. T00 technical feasibility is now recorded above and does not authorize feature-code implementation by itself. If implementation constraints conflict with an accepted policy, report the conflict rather than silently changing behavior.
