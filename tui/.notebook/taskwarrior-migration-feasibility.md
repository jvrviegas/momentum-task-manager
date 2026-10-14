# Taskwarrior migration feasibility
> T00 evidence for Settings → Projects pending-only renames

Entry: `docs/plans/settings-projects.md#t00-feasibility-record--2026-09-16`

- Taskwarrior 3.5.0 `export` deliberately ignores active context. Read `_get rc.context` and `_get rc.context.<name>.read`; pass `(<read-filter>)` explicitly as one argv element. No active context means no extra filter.
- Guard every one-UUID write with `status:pending recur.none: project.is:<old>` and reconcile the UUID afterward. `status:pending` excludes current waiting tasks; `recur.none:` excludes recurring templates/instances; `project.is:` avoids descendant/prefix matches.
- Normal `on-modify` hooks remain enabled and may reject or mutate additional fields. Treat exit code as a command result, not a changed-task count.
- Native undo is per latest Taskwarrior action; it cannot undo config or a multi-command migration. Catalog-first execution is sequential and may leave honest partial outcomes.
- BurntSushi encoding is lossy. T00 verified `github.com/smm-h/go-toml-edit v0.4.3` for source-preserving indexed catalog edits; its generic writer must be wrapped to resolve symlinks and atomically replace the target, with late content/link/mode conflict checks.

No feature code or user data was touched during T00 probes.
