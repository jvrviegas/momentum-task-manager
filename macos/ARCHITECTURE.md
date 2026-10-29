# Momentum for macOS: architecture

How the Mac app is put together, as of 2026-10-29. What it does is in [`README.md`](./README.md); the open questions about its shape are in [`../docs/architecture-review-2026-10-29.md`](../docs/architecture-review-2026-10-29.md).

## Processes

The app keeps no tasks of its own. It starts the Go `momentum-bridge` (`../tui/cmd/momentum-bridge`) as a child process and exchanges one JSON object per line with it. The bridge reads `config.toml` and talks to the Momentum server ([ADR 0002](../docs/adr/0002-go-postgres-backend.md)). The `Build momentum-bridge` phase in `project.yml` compiles it into the app bundle.

## Layers

One app target, `Momentum`, and one test target, `MomentumTests`, which runs inside the app. Dependencies point down:

```text
Views/            SwiftUI; take the AppStore from the environment
Model/AppStore    @MainActor @Observable: tasks, selection, changes and undo,
                  Plan Today, toasts, the account and sign-out
Model/*Model      owned by the store, one per Settings tab or sheet:
                  settings, catalog (Projects), diagnostics,
                  macCalendar, googleCalendar. Each holds a weak back-reference
                  to the store for the bridge, the sign-in and a reload.
                  signInModel (the sign-in sheet) has none: it is handed the
                  bridge, the SignInStore and sleep, and tells the store
                  through onSignedIn.
Bridge/           TaskBridge (the protocol), ProcessBridge (an actor), Wire.swift
```

## Seams

Each has a production adapter, a test adapter, and a Debug adapter for the snapshot pages (`Snapshot/`, compiled only in Debug).

| Seam | Production | Tests | Snapshots |
| --- | --- | --- | --- |
| `TaskBridge` | `ProcessBridge` | `FakeBridge` | `SnapshotBridge` |
| `SignInStore` | `KeychainSignInStore` | `FakeSignInStore` | `SnapshotSignInStore` |
| `CalendarEventSource` | `EventKitSource` (`NoCalendarSource` until chosen) | `FakeCalendarSource` | `SnapshotCalendarSource` |

## The wire

`Bridge/Wire.swift` mirrors `../tui/internal/bridge/wire.go` by hand. Two tests keep them in step:

- `TestWire…` in `../tui/internal/bridge` sends each op through the bridge's real request path and keeps the answer as a file in `../tui/internal/bridge/testdata/wire`. It fails when an answer changes.
- `MomentumTests/WireContractTests.swift` decodes every file with `ProcessBridge.decoder` and checks that every key reached a property, which also catches a renamed optional key that decodes as `nil`.

After changing the wire on purpose, run `go test ./internal/bridge -run TestWire -update` in `../tui` and add any new file to `fixtures` in the Swift test. This covers the bridge's answers; the parameters the app sends are not covered.
