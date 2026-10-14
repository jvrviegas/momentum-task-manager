# Momentum

Momentum is a keyboard-first task manager for [Taskwarrior 3.x](https://taskwarrior.org/), with a terminal app and a native macOS app.

| Folder | What it holds |
|---|---|
| `core/` | Go code shared by the apps and the planned backend: task models, views, and edit rules |
| [`tui/`](./tui/README.md) | The terminal app in Go, plus the Go code the macOS app shares through `momentum-bridge` |
| [`macos/`](./macos/README.md) | The native SwiftUI app (preview) |
| [`backend/`](./backend/README.md) | A planned backend to replace Taskwarrior; not started |

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for the project shape, checks and release rules. Momentum is released under the [MIT License](./LICENSE).
