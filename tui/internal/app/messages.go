package app

import (
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/ui"
)

// TasksMsg is returned by both initial load and refresh commands.
type TasksMsg struct {
	Tasks           []domain.Task
	Err             error
	Completed       []domain.Task
	CompletedLoaded bool
	CompletedErr    error
	Reason          string
}

// MutationKind identifies a serialized task mutation.
type MutationKind string

const (
	MutationAdd            MutationKind = "add"
	MutationModify         MutationKind = "modify"
	MutationComplete       MutationKind = "complete"
	MutationDelete         MutationKind = "delete"
	MutationStart          MutationKind = "start"
	MutationStop           MutationKind = "stop"
	MutationUndo           MutationKind = "undo"
	MutationPlan           MutationKind = "daily_plan"
	MutationStopRecurrence MutationKind = "stop_recurrence"
)

// MutationMsg is returned by a mutation command.
type MutationMsg struct {
	Kind        MutationKind
	UUID        string
	Err         error
	Revision    uint64
	PlanApplied int
	PlanTotal   int
}

// UndoTickMsg and RefreshTickMsg keep the two independent clocks explicit.
type UndoTickMsg time.Time
type RefreshTickMsg time.Time

// TickMsg drives refresh and countdown timers.
type TickMsg time.Time

// RefreshRequestedMsg asks the root model to keep the current content while it
// starts an asynchronous export.
type RefreshRequestedMsg struct {
	Reason string
}

// MutationRequestedMsg is the only entry point for starting a mutation.
type MutationRequestedMsg struct {
	Request MutationRequest
	Done    chan struct{}
}

// ProjectsMsg and TagsMsg populate contextual autocomplete data.
type ProjectsMsg struct {
	Values    []string
	Err       error
	RequestID uint64
}

type TagsMsg struct {
	Values    []string
	Err       error
	RequestID uint64
}

// CalendarMsg contains the ephemeral read-only calendar projection used by
// the open planner. Event details never reach the server or disk.
type CalendarMsg struct {
	ID     uint64
	Result CalendarState
}

// ProjectCatalogSnapshotMsg is the typed result of the async config revision read.
type ProjectCatalogSnapshotMsg struct {
	Snapshot config.ProjectCatalogSnapshot
	Err      error
}

// ProjectCatalogSaveMsg is the typed result of an asynchronous catalog save.
type ProjectCatalogSaveMsg struct {
	ID       uint64
	Plan     domain.ProjectCatalogPlan
	Snapshot config.ProjectCatalogSnapshot
	Err      error
}

// ThemeSaveMsg is the typed result of an asynchronous theme save.
type ThemeSaveMsg struct {
	Theme    string
	Snapshot config.ProjectCatalogSnapshot
	Err      error
}

// ProjectRenamePreviewMsg is the typed result of one async preview export.
type ProjectRenamePreviewMsg struct {
	ID      uint64
	Preview ui.ProjectRenamePreview
	Err     error
}

// ProjectMigrationMsg is the typed result of one asynchronous migration.
type ProjectMigrationMsg struct {
	ID     uint64
	Result ProjectMigrationResult
}

// ToastMsg displays a short in-application status message.
type ToastMsg struct {
	Text string
	Err  bool
}
