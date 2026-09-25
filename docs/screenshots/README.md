# Screenshots

Release screenshots belong here. Capture the wide, compact, and narrow layouts in a terminal with deterministic fixture data; never use a personal Taskwarrior database. Add Nerd Font and ASCII examples only when they improve installation guidance.

The checked-in ANSI-stripped render fixtures under `internal/app/testdata/render/` and `internal/ui/testdata/render/` are the automated visual contract. They cover `120×30`, `79×24`, `49×18`, and `28×8`, including task, empty/loading/error/search, modal, and Settings states. The `readability-*-*.txt` files are sanitized Today-state terminal captures derived from those fixtures for line-by-line review; they contain no user data and are intentionally plain text so diffs remain reviewable.

To intentionally refresh fixtures after reviewing a presentation change:

```sh
MOMENTUM_UPDATE_GOLDENS=1 go test ./internal/ui ./internal/app -run 'RenderFixtures|ComponentRenderFixtures' -count=1
go test ./internal/ui ./internal/app -run 'RenderFixtures|ComponentRenderFixtures' -count=1
```

Normal test runs compare fixtures and never rewrite them. A maintainer must still capture and review real-terminal screenshots for dark/light themes and Unicode/ASCII icon modes before release; the checked-in text captures are not a substitute for that human check.
