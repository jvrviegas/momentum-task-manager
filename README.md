# Momentum

Momentum is a keyboard-first task manager with a terminal app and a native macOS app. Both keep tasks on a Momentum server; `momentum import` brings tasks over from [Taskwarrior](https://taskwarrior.org/).

| Folder | What it holds |
|---|---|
| `core/` | Go code shared by the apps and the backend: task models, views, edit rules, and the backend's API contract |
| [`tui/`](./tui/README.md) | The terminal app in Go, plus the Go code the macOS app shares through `momentum-bridge` |
| [`macos/`](./macos/README.md) | The native SwiftUI app (preview) |
| [`backend/`](./backend/README.md) | The Momentum server: Go and PostgreSQL |

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for the project shape, checks and release rules. Momentum is released under the [MIT License](./LICENSE).
