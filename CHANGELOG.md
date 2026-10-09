# Changelog

Notable changes to Momentum are documented here. Releases follow Semantic Versioning.

## [0.1.0] - 2026-10-09

### Added

- Keyboard-first Taskwarrior 3.x terminal frontend for macOS and Linux, with Inbox, Today, and a read-only recent Completed view.
- Quick capture with project, priority, date, tag, estimate, and recurrence triggers, contextual suggestions, and local natural-language interpretation. Valid captures submit immediately; invalid or conflicting interpretations open correction review.
- Structured task editing, task details, completion, start/stop, deletion confirmation, search, and undo.
- Daily planning with effort estimates, capacity, local ICS calendar context, and Taskwarrior-backed daily commitments.
- Native Taskwarrior recurrence creation and editing, plus an explicit stop-series action.
- Config-backed project catalog with readable hierarchical labels and optional guarded pending-task project migrations.
- Responsive terminal layouts, dark/light/terminal themes, live Appearance settings, and Unicode, Nerd Font, or ASCII icons.
- Native Taskwarrior synchronization with retry handling, undo grace, local-only operation, and diagnostics through `momentum doctor`.

### Changed

- Standardized capture and task editing: Enter submits current input, Tab accepts visible suggestions or advances edit fields, and Shift+Tab moves backward without accepting suggestions. Ctrl+S is an alternative to Enter, including capture correction review.
