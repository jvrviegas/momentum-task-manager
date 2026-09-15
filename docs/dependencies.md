# Dependencies

Momentum uses the current stable releases available when the implementation was started (2026-09-08):

- `charm.land/bubbletea/v2 v2.0.9` — Elm-style terminal event loop and asynchronous commands. The v2 `Model` returns `tea.View`, and messages use `tea.KeyPressMsg`.
- `charm.land/bubbles/v2 v2.2.1` — maintained terminal widgets, including `textinput`; Momentum keeps view composition in its own small components.
- `charm.land/lipgloss/v2 v2.0.6` — terminal-safe styling, width measurement, layout joining, and background-aware palette support.
- `github.com/BurntSushi/toml v1.6.0` — small TOML decoder. `MetaData.Undecoded()` is checked so configuration typos fail instead of being ignored.

The standard library handles CLI parsing, process execution, JSON decoding, time, and logging. No CLI framework or external fuzzy-search package is needed.

Versions were verified against the modules' published version lists and current v2 API documentation. The Charm v2 import paths are intentional; v1 examples use different packages and return types.
