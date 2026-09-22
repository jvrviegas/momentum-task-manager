# Natural-language capture flow
> Bounded local interpretation with review before mutation

Entry: `internal/quickadd/interpret.go:Interpret()`
Flow: explicit `Parse()`/token spans → protected explicit/escaped tokens → whitelist date/time/effort/priority/recurrence candidates → precedence/source accounting → `domain.NewTask` draft → `ui.QuickAddModel` review or immediate explicit-only submit.

Review: `internal/ui/quickadd_review.go` keeps original input, frozen absolute inferred dates, timezone, editable fields, blocking diagnostics, and an explicit-syntax-only escape on Ctrl+X. Project, scheduled, tag, estimate, priority, due, and recurrence values remain editable; Ctrl+R explicitly keeps a blocking candidate literal. Compact layouts put the value before verbose provenance and scroll wrapped details while keeping Ctrl+S visible.

Lifecycle: `internal/ui/quickadd.go:QuickAddModel` increments a capture revision on open/close/input edits and stamps parse/review/submit/error messages. `internal/app/model.go:Model.Update` checks revision/source ownership before starting `MutationAdd`; failed adds reopen the confirmed draft with the adapter error for deliberate retry and never auto-submit.

Date policy: local injected reference time; weekdays include today; `next <weekday>` means following calendar week; bare times require a date; malformed/incomplete times and DST nonexistent/ambiguous wall times are blocking. DST folds are enumerated across local offsets rather than assuming a one-hour transition. G0 estimate semantics are reused; recurrence phrases use the native G2 model.

Updated: 2026-09-21
