# Momentum

Momentum is a keyboard-first task manager for [Taskwarrior 3.x](https://taskwarrior.org/), with a terminal app and a native macOS app.

| Folder | What it holds |
|---|---|
| `core/` | Go code shared by the apps and the backend: task models, views, edit rules, and the backend's API contract |
| [`tui/`](./tui/README.md) | The terminal app in Go, plus the Go code the macOS app shares through `momentum-bridge` |
| [`macos/`](./macos/README.md) | The native SwiftUI app (preview) |
| [`backend/`](./backend/README.md) | A Go backend to replace Taskwarrior, in progress |

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for the project shape, checks and release rules. Momentum is released under the [MIT License](./LICENSE).
