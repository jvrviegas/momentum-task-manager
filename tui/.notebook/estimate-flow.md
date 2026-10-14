# Estimate Flow
> Optional focused-effort estimate across domain, Taskwarrior, capture, edit, and presentation

Entry: `internal/domain/estimate.go:ParseEstimate()` and `internal/quickadd/parser.go:Parse()`

Domain: `domain.Estimate` stores positive whole minutes. User-created values are 1–1440 minutes; `ParseTaskwarriorEstimate()` accepts precise ISO days/hours/minutes/whole-minute seconds without treating calendar `M` as minutes. Unsupported external values remain in `Task.RawFields` with `EstimateWarning`.

Storage: Taskwarrior UDA configuration must be present on every editing replica:
`uda.estimate.type=duration`, `uda.estimate.label=Estimate`.
`internal/taskwarrior/estimate.go:CommandClient.EstimateUDAReadiness()` runs `_get rc.uda.estimate.type`; estimate-bearing add/modify refuses before mutation unless exact `duration`. Outgoing values use `estimate:<minutes>min`.

Capture/edit: `~` is a token-boundary quick-add trigger; `internal/ui/edit.go` owns eight fields and uses the domain parser. Editable inputs (editor and capture review) open with `Estimate.InputValue()` (`1h30m`), never the display `String()` (`1h 30m`), which the parser rejects. Uppercase `E` focuses Estimate; lowercase `e` remains Description.

Presentation: `internal/ui/details.go` renders typed Estimate once and warns once for unsupported raw values. `internal/ui/tasklist.go:taskMetadata()` adds Estimate to pending rows as the lowest-priority line-2 part, so it drops first as width shrinks; completed rows never show it.

Integration: `internal/taskwarrior/integration_test.go` uses appended UDA lines only in estimate fixtures; all real Taskwarrior tests use temporary `TASKRC`/`TASKDATA` and no sync server.

Malformed external durations: `parseISOSection()` must check for a missing unit after digits; truncated values such as `P1`, `PT1`, `P1D2`, and `P1T1H` stay raw-only with `EstimateWarning` and never break task export.

Updated: 2026-10-02
