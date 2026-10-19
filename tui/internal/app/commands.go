package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/ui"
)

// Client is the Momentum server as the TUI reaches it; *backend.Client
// implements it. Completed tasks, tags, and stopping a series are optional
// readers, checked with type assertions so tests can use small fakes.
type Client interface {
	ExportPending(context.Context) ([]domain.Task, error)
	Add(context.Context, domain.NewTask) error
	Modify(context.Context, string, domain.TaskDiff) error
	Complete(context.Context, string) error
	Delete(context.Context, string) error
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Undo(context.Context) error
	Projects(context.Context) ([]string, error)
}

var (
	errNilClient               = errors.New("no Momentum server is configured")
	errProjectStoreUnavailable = errors.New("project config store is not available")
)

// MutationRequest contains one operation for the serialized mutation queue.
type MutationRequest struct {
	Kind          MutationKind
	UUID          string
	Input         domain.NewTask
	Diff          domain.TaskDiff
	PlanMutations []domain.PlanMutation

	// Capture fields are populated only for quick-add mutations. They let a
	// failed add restore the exact reviewed draft without coupling the generic
	// mutation queue to UI state.
	CaptureRevision uint64
	CaptureSource   string
	CaptureReview   bool
}

// LoadTasksCommand returns a non-blocking Bubble Tea command for the pending
// export and, when supported by the client, the recent completed export.
func LoadTasksCommand(ctx context.Context, client Client, reason string, at ...time.Time) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return TasksMsg{Err: errNilClient, Reason: reason}
		}
		commandCtx := commandContext(ctx)
		tasks, err := client.ExportPending(commandCtx)
		message := TasksMsg{Tasks: tasks, Err: err, Reason: reason}
		reader, ok := client.(interface {
			ExportCompleted(context.Context, time.Time) ([]domain.Task, error)
		})
		if !ok {
			return message
		}
		message.CompletedLoaded = true
		now := time.Now()
		if len(at) > 0 && !at[0].IsZero() {
			now = at[0]
		}
		loc := now.Location()
		today := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)
		message.Completed, message.CompletedErr = reader.ExportCompleted(commandCtx, today.AddDate(0, 0, -30))
		return message
	}
}

// RefreshTasksCommand is the named refresh variant used by the root model.
func RefreshTasksCommand(ctx context.Context, client Client) tea.Cmd {
	return LoadTasksCommand(ctx, client, "refresh")
}

// MutationCommand runs exactly one client mutation and returns a typed result.
func MutationCommand(ctx context.Context, client Client, request MutationRequest) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return MutationMsg{Kind: request.Kind, UUID: request.UUID, Err: errNilClient, Revision: request.CaptureRevision}
		}
		ctx := commandContext(ctx)
		var err error
		switch request.Kind {
		case MutationAdd:
			err = client.Add(ctx, request.Input)
		case MutationModify:
			err = client.Modify(ctx, request.UUID, request.Diff)
		case MutationComplete:
			err = client.Complete(ctx, request.UUID)
		case MutationDelete:
			err = client.Delete(ctx, request.UUID)
		case MutationStart:
			err = client.Start(ctx, request.UUID)
		case MutationStop:
			err = client.Stop(ctx, request.UUID)
		case MutationUndo:
			err = client.Undo(ctx)
		case MutationStopRecurrence:
			if stopper, ok := client.(interface {
				StopRecurrence(context.Context, string) error
			}); ok {
				err = stopper.StopRecurrence(ctx, request.UUID)
			} else {
				// Lightweight fakes clear the rule instead.
				err = client.Modify(ctx, request.UUID, domain.TaskDiff{Recurrence: domain.FieldChange{Kind: domain.Clear}})
			}
		case MutationPlan:
			applied := 0
			for _, mutation := range request.PlanMutations {
				err = client.Modify(ctx, mutation.UUID, mutation.Diff)
				if err != nil {
					return MutationMsg{Kind: request.Kind, UUID: mutation.UUID, Err: fmt.Errorf("daily plan applied %d of %d changes; retry is safe: %w", applied, len(request.PlanMutations), err), Revision: request.CaptureRevision, PlanApplied: applied, PlanTotal: len(request.PlanMutations)}
				}
				applied++
			}
			return MutationMsg{Kind: request.Kind, UUID: request.UUID, Revision: request.CaptureRevision, PlanApplied: applied, PlanTotal: len(request.PlanMutations)}
		default:
			err = errors.New("unknown mutation")
		}
		return MutationMsg{Kind: request.Kind, UUID: request.UUID, Err: err, Revision: request.CaptureRevision}
	}
}

// ProjectsCommand and TagsCommand load autocomplete data asynchronously.
func ProjectsCommand(ctx context.Context, client interface {
	Projects(context.Context) ([]string, error)
}) tea.Cmd {
	return ProjectsCommandWithID(ctx, client, 0)
}

func ProjectsCommandWithID(ctx context.Context, client interface {
	Projects(context.Context) ([]string, error)
}, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ProjectsMsg{Err: errNilClient, RequestID: requestID}
		}
		values, err := client.Projects(commandContext(ctx))
		return ProjectsMsg{Values: values, Err: err, RequestID: requestID}
	}
}

func TagsCommand(ctx context.Context, client interface {
	Tags(context.Context) ([]string, error)
}) tea.Cmd {
	return TagsCommandWithID(ctx, client, 0)
}

func TagsCommandWithID(ctx context.Context, client interface {
	Tags(context.Context) ([]string, error)
}, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return TagsMsg{Err: errNilClient, RequestID: requestID}
		}
		values, err := client.Tags(commandContext(ctx))
		return TagsMsg{Values: values, Err: err, RequestID: requestID}
	}
}

// ProjectRenamePreviewCommand exports pending tasks and builds exact and
// Include-subprojects task mappings asynchronously.
func ProjectRenamePreviewCommand(ctx context.Context, client ProjectMigrationClient, id uint64, exactPlan, subprojectsPlan domain.ProjectCatalogPlan) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ProjectRenamePreviewMsg{ID: id, Err: ErrProjectMigrationNoClient}
		}
		tasks, err := client.ExportPending(commandContext(ctx))
		if err != nil {
			return ProjectRenamePreviewMsg{ID: id, Err: err}
		}
		before := make(map[string]struct{}, len(exactPlan.Before))
		for _, project := range exactPlan.Before {
			before[project.Value] = struct{}{}
		}
		destination := exactPlan.DestinationValue
		destinationTaskOnly := false
		if _, exists := before[destination]; !exists {
			for _, task := range tasks {
				if task.Project == destination || strings.HasPrefix(task.Project, destination+".") {
					destinationTaskOnly = true
					break
				}
			}
		}
		return ProjectRenamePreviewMsg{
			ID: id,
			Preview: ui.ProjectRenamePreview{
				ExactPlan:           exactPlan,
				SubprojectsPlan:     subprojectsPlan,
				ExactTasks:          exactPlan.PendingTaskMappings(tasks),
				SubprojectTasks:     subprojectsPlan.PendingTaskMappings(tasks),
				DestinationTaskOnly: destinationTaskOnly,
			},
		}
	}
}

// ProjectMigrationCommand runs one confirmed migration asynchronously through
// the injected coordinator.
func ProjectMigrationCommand(ctx context.Context, coordinator *ProjectMigrationCoordinator, id uint64, request ProjectMigrationRequest) tea.Cmd {
	return func() tea.Msg {
		if coordinator == nil {
			return ProjectMigrationMsg{ID: id, Result: ProjectMigrationResult{PreflightErr: ErrProjectMigrationNoClient}}
		}
		return ProjectMigrationMsg{ID: id, Result: coordinator.Execute(commandContext(ctx), request)}
	}
}

// ProjectCatalogSnapshotCommand reads the active config source revision
// asynchronously without mutating it.
func ProjectCatalogSnapshotCommand(ctx context.Context, store config.ProjectCatalogStore) tea.Cmd {
	return func() tea.Msg {
		if store == nil {
			return ProjectCatalogSnapshotMsg{Err: errProjectStoreUnavailable}
		}
		snapshot, err := store.Read(commandContext(ctx))
		return ProjectCatalogSnapshotMsg{Snapshot: snapshot, Err: err}
	}
}

// ProjectCatalogSaveCommand persists one already validated catalog plan
// asynchronously through the injected store.
func ProjectCatalogSaveCommand(ctx context.Context, store config.ProjectCatalogStore, snapshot config.ProjectCatalogSnapshot, plan domain.ProjectCatalogPlan, id uint64) tea.Cmd {
	return func() tea.Msg {
		if store == nil {
			return ProjectCatalogSaveMsg{ID: id, Plan: plan, Err: errProjectStoreUnavailable}
		}
		saved, err := store.Save(commandContext(ctx), snapshot, plan.After)
		return ProjectCatalogSaveMsg{ID: id, Plan: plan, Snapshot: saved, Err: err}
	}
}

// ThemeSaveCommand persists the theme asynchronously through the store when
// it supports theme writes.
func ThemeSaveCommand(ctx context.Context, store config.ProjectCatalogStore, snapshot config.ProjectCatalogSnapshot, theme string) tea.Cmd {
	return func() tea.Msg {
		themes, ok := store.(config.ThemeStore)
		if !ok {
			return ThemeSaveMsg{Theme: theme, Err: errProjectStoreUnavailable}
		}
		saved, err := themes.SaveTheme(commandContext(ctx), snapshot, theme)
		return ThemeSaveMsg{Theme: theme, Snapshot: saved, Err: err}
	}
}

func commandContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
