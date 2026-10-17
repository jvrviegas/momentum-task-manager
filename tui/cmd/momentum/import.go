package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/backend"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// exporter reads every Taskwarrior task without changing Taskwarrior's data.
type exporter interface {
	ExportAll(context.Context) ([]domain.Task, error)
}

// importTasks runs momentum import: it reads every task from Taskwarrior and
// creates, on the [backend] server, the ones the signed-in user does not
// have. With dryRun it only reports what it would do. Taskwarrior's data is
// never changed.
func importTasks(ctx context.Context, settings config.Config, configPath string, source exporter, dryRun bool, stdout, stderr io.Writer) int {
	if !settings.Backend.Enabled() {
		fmt.Fprintf(stderr, "momentum: import needs a [backend] url in %s\n", configPath)
		return 1
	}
	url := settings.Backend.URL
	credentials, err := backend.SavedCredentials(url)
	if errors.Is(err, backend.ErrNotSignedIn) {
		fmt.Fprintf(stderr, "momentum: %v; run momentum login first\n", err)
		return 1
	} else if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	exported, err := source.ExportAll(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "momentum: read Taskwarrior:", err)
		return 1
	}
	if len(exported) == 0 {
		fmt.Fprintln(stdout, "Taskwarrior has no tasks; nothing to import.")
		return 0
	}
	plan, err := backend.NewImport(exported)
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	fmt.Fprintf(stdout, "Read %s from Taskwarrior: %s.\n", tasks(len(plan.Tasks)), counts(plan.Statuses, false))
	if len(plan.Extra) > 0 {
		fmt.Fprintf(stdout, "Kept as fields Momentum does not use, with how many tasks have each: %s.\n", counts(plan.Extra, true))
	}
	preview, err := plan.Preview(ctx, url, credentials.Token)
	if err != nil {
		fmt.Fprintln(stderr, "momentum: preview the import:", err)
		return 1
	}
	fmt.Fprintf(stdout, "On %s: %d to create, %d already there, %d different.\n", url, len(preview.Created), len(preview.Unchanged), len(preview.Different))
	for _, difference := range preview.Different {
		fmt.Fprintf(stdout, "  %s differs in %s; it is left as it is.\n", difference.UUID, strings.Join(difference.Fields, ", "))
	}
	switch {
	case dryRun:
		fmt.Fprintln(stdout, "Dry run: nothing was imported.")
		return 0
	case len(preview.Created) == 0:
		fmt.Fprintln(stdout, "Nothing to import.")
		return 0
	}
	result, err := plan.Apply(ctx, url, credentials.Token)
	if err != nil {
		fmt.Fprintf(stderr, "momentum: import: %v\nTasks imported before the failure stay; run momentum import again to finish.\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Imported %s. Taskwarrior was not changed.\n", tasks(len(result.Created)))
	return 0
}

// counts lists values by key, the largest first: as "3 pending, 1 completed",
// or by name, as "imask (2), mask (1)".
func counts(values map[string]int, byName bool) string {
	keys := slices.Collect(maps.Keys(values))
	slices.SortFunc(keys, func(a, b string) int {
		if values[a] != values[b] {
			return values[b] - values[a]
		}
		return strings.Compare(a, b)
	})
	parts := make([]string, len(keys))
	for i, key := range keys {
		if byName {
			parts[i] = fmt.Sprintf("%s (%d)", key, values[key])
		} else {
			parts[i] = fmt.Sprintf("%d %s", values[key], key)
		}
	}
	return strings.Join(parts, ", ")
}

func tasks(count int) string {
	if count == 1 {
		return "1 task"
	}
	return fmt.Sprintf("%d tasks", count)
}
