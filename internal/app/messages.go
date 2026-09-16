package app

import (
	"time"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
)

// TasksMsg is returned by both initial load and refresh commands.
type TasksMsg struct {
	Tasks  []domain.Task
	Err    error
	Reason string
}

// MutationKind identifies a serialized task mutation.
type MutationKind string

const (
	MutationAdd      MutationKind = "add"
	MutationModify   MutationKind = "modify"
	MutationComplete MutationKind = "complete"
	MutationDelete   MutationKind = "delete"
	MutationStart    MutationKind = "start"
	MutationStop     MutationKind = "stop"
	MutationUndo     MutationKind = "undo"
)

// MutationMsg is returned by a mutation command.
type MutationMsg struct {
	Kind MutationKind
	UUID string
	Err  error
}

// SyncMsg is returned by a native Taskwarrior sync command.
type SyncMsg struct {
	Result taskwarrior.SyncResult
	Err    error
}

// SyncConfigMsg reports sync readiness without exposing credentials.
type SyncConfigMsg struct {
	Configured bool
	Err        error
}

// SyncTickMsg and RefreshTickMsg keep the two independent clocks explicit.
type SyncTickMsg time.Time
type RefreshTickMsg time.Time

// TickMsg drives refresh, sync, and countdown timers.
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

// ContextMsg reports the active Taskwarrior context without changing it.
type ContextMsg struct {
	Name string
	Err  error
}

// ProjectsMsg and TagsMsg populate contextual autocomplete data.
type ProjectsMsg struct {
	Values []string
	Err    error
}

type TagsMsg struct {
	Values []string
	Err    error
}

// ToastMsg displays a short in-application status message.
type ToastMsg struct {
	Text string
	Err  bool
}
