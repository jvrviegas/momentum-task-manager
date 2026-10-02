# Natural-language capture flow
> Bounded local interpretation with immediate valid capture and correction review

Entry: `internal/quickadd/interpret.go:Interpret()`
Flow: explicit `Parse()`/token spans → protected explicit/escaped tokens → whitelist date/time/effort/priority/recurrence candidates → precedence/source accounting → `domain.NewTask` draft → `ui.QuickAddModel` immediate submit for valid interpretations or correction review for invalid/conflicting drafts.

Review: `internal/ui/quickadd_review.go` keeps original input, frozen absolute inferred dates, timezone, editable fields, blocking diagnostics, and an explicit-syntax-only escape on Ctrl+X. Project, scheduled, tag, estimate, priority, due, and recurrence values remain editable; Ctrl+R explicitly keeps a blocking candidate literal. Compact layouts put the value before verbose provenance and scroll wrapped details while keeping Ctrl+S visible. The review renders in the shared modal `Frame`; field values use `inputSpans()` budgeted by `reviewFieldAffixes()`, so long values scroll to the cursor instead of clipping behind labels.

Live highlight: `internal/ui/quickadd.go:captureTokenCell()` reruns `Interpret()` on every render and underlines accepted inferred candidate spans in their field tone (blocking ones in red), so recognition is visible before Enter; Enter submits valid drafts immediately and opens review only when `Interpretation.Valid` is false. `Esc` on a highlighted phrase (`phraseAtCursor()`) appends it to `QuickAddModel.Literals`, which `InterpretWithLiterals()` treats as protected; `shiftLiterals()` moves kept spans with edits and drops any the edit touches.

Lifecycle: `internal/ui/quickadd.go:QuickAddModel` increments a capture revision on open/close/input edits and stamps parse/review/submit/error messages. `internal/app/model.go:Model.Update` checks revision/source ownership before starting `MutationAdd`; failed adds reopen the confirmed draft with the adapter error for deliberate retry and never auto-submit.

Date policy: local injected reference time; weekdays include today; `next <weekday>` means following calendar week; bare times require a date; malformed/incomplete times and DST nonexistent/ambiguous wall times are blocking. DST folds are enumerated across local offsets rather than assuming a one-hour transition. G0 estimate semantics are reused; recurrence phrases use the native G2 model.

Updated: 2026-10-02
