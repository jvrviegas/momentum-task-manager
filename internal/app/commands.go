package app

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/config"
	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
	"github.com/jvrviegas/momentum/internal/ui"
)

var (
	errNilClient               = errors.New("Taskwarrior client is not configured")
	errProjectStoreUnavailable = errors.New("project config store is not available")
)

// MutationRequest contains one operation for the serialized mutation queue.
type MutationRequest struct {
	Kind  MutationKind
	UUID  string
	Input domain.NewTask
	Diff  domain.TaskDiff
}

// LoadTasksCommand returns a non-blocking Bubble Tea command for the pending
// export and, when supported by the client, the recent completed export.
func LoadTasksCommand(ctx context.Context, client taskwarrior.Client, reason string, at ...time.Time) tea.Cmd {
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
func RefreshTasksCommand(ctx context.Context, client taskwarrior.Client) tea.Cmd {
	return LoadTasksCommand(ctx, client, "refresh")
}

// MutationCommand runs exactly one client mutation and returns a typed result.
func MutationCommand(ctx context.Context, client taskwarrior.Client, request MutationRequest) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return MutationMsg{Kind: request.Kind, UUID: request.UUID, Err: errNilClient}
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
		default:
			err = errors.New("unknown mutation")
		}
		return MutationMsg{Kind: request.Kind, UUID: request.UUID, Err: err}
	}
}

// SyncCommand invokes Taskwarrior's native sync asynchronously.
func SyncCommand(ctx context.Context, client taskwarrior.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return SyncMsg{Err: errNilClient}
		}
		result, err := client.Sync(commandContext(ctx))
		return SyncMsg{Result: result, Err: err}
	}
}

// SyncConfigCommand checks readiness without returning any configured value.
func SyncConfigCommand(ctx context.Context, client interface {
	SyncConfigured(context.Context) (bool, error)
}) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return SyncConfigMsg{Err: errNilClient}
		}
		configured, err := client.SyncConfigured(commandContext(ctx))
		return SyncConfigMsg{Configured: configured, Err: err}
	}
}

// ContextCommand reads the active Taskwarrior context without changing it.
func ContextCommand(ctx context.Context, client interface {
	Context(context.Context) (string, error)
}) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ContextMsg{Err: errNilClient}
		}
		name, err := client.Context(commandContext(ctx))
		return ContextMsg{Name: name, Err: err}
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

// ProjectRenamePreviewCommand exports the active context and builds exact and
// Include-subprojects task mappings asynchronously.
func ProjectRenamePreviewCommand(ctx context.Context, client ProjectMigrationClient, id uint64, exactPlan, subprojectsPlan domain.ProjectCatalogPlan) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ProjectRenamePreviewMsg{ID: id, Err: ErrProjectMigrationNoClient}
		}
		export, err := client.ExportPendingInContext(commandContext(ctx))
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
			for _, task := range export.Tasks {
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
				ExactTasks:          exactPlan.PendingTaskMappings(export.Tasks),
				SubprojectTasks:     subprojectsPlan.PendingTaskMappings(export.Tasks),
				ContextName:         export.Scope.ContextName,
				ReadFilter:          export.Scope.ReadFilter,
				HasActiveContext:    export.Scope.ContextName != "",
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

func commandContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
