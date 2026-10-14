# Screenshots

Release screenshots belong here. Capture the wide, compact, and narrow layouts in a terminal with deterministic fixture data; never use a personal Taskwarrior database. Add Nerd Font and ASCII examples only when they improve installation guidance.

The checked-in ANSI-stripped render fixtures under `internal/app/testdata/render/` and `internal/ui/testdata/render/` are the automated visual contract. They cover `120×30`, `79×24`, `49×18`, and `28×8`, including task, empty/loading/error/search, modal, and Settings states. The `readability-*-*.txt` files are sanitized Today-state terminal captures derived from those fixtures for line-by-line review; they contain no user data and are intentionally plain text so diffs remain reviewable.

To intentionally refresh fixtures after reviewing a presentation change:

```sh
MOMENTUM_UPDATE_GOLDENS=1 go test ./internal/ui ./internal/app -run 'RenderFixtures|ComponentRenderFixtures' -count=1
go test ./internal/ui ./internal/app -run 'RenderFixtures|ComponentRenderFixtures' -count=1
```

Normal test runs compare fixtures and never rewrite them. The `uat-*.png` images are maintainer-supplied real-terminal UAT captures from the isolated synthetic Taskwarrior profile (2026-09-25): wide dark ASCII/light Unicode, compact dark Unicode, and narrow dark Unicode. They contain no personal tasks, paths, usernames, sync identifiers, or secrets; terminal wallpaper remains visible through the transparent background. Terminal cell dimensions were not measured, so these are qualitative visual evidence, not exact-size golden fixtures. See `docs/UAT.md` for the maintainer's review and limitations.
