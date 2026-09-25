package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/ui"
)

// SelectedTask returns the task currently targeted by an action.
func (m *Model) SelectedTask() (domain.Task, bool) {
	if m == nil {
		return domain.Task{}, false
	}
	tasks := m.tasksFor(m.ActiveView)
	index := m.Selections[normalizeView(m.ActiveView)]
	if index < 0 || index >= len(tasks) {
		return domain.Task{}, false
	}
	return tasks[index], true
}

// CompleteSelected dispatches completion through the serialized mutation gate.
func (m *Model) CompleteSelected() tea.Cmd {
	if m.completedTasksReadOnly() {
		return nil
	}
	task, ok := m.SelectedTask()
	if !ok {
		return nil
	}
	return m.beginMutation(MutationRequest{Kind: MutationComplete, UUID: task.UUID})
}

// ToggleStartSelected starts an idle task or stops the active task.
func (m *Model) ToggleStartSelected() tea.Cmd {
	if m.completedTasksReadOnly() {
		return nil
	}
	task, ok := m.SelectedTask()
	if !ok {
		return nil
	}
	kind := MutationStart
	if task.Start != nil {
		kind = MutationStop
	}
	return m.beginMutation(MutationRequest{Kind: kind, UUID: task.UUID})
}

// DeleteSelected opens the app-owned confirmation overlay; it never invokes a
// destructive process by itself.
func (m *Model) DeleteSelected() {
	if m.completedTasksReadOnly() {
		return
	}
	task, ok := m.SelectedTask()
	if !ok || m.MutationRunning {
		return
	}
	m.DeleteTarget = task.UUID
	m.Confirm.OpenFor("Delete task", "Delete "+task.Description+" permanently?")
	m.Overlay = OverlayConfirm
}

// ConfirmDelete routes a confirmed choice to the mutation adapter.
func (m *Model) ConfirmDelete(action ui.ConfirmAction) tea.Cmd {
	if action != ui.ConfirmYes {
		if action == ui.ConfirmNo || action == ui.ConfirmCancel {
			m.DeleteTarget = ""
			m.RecurrenceTarget = ""
			m.Overlay = OverlayNone
		}
		return nil
	}
	if m.RecurrenceTarget != "" {
		uuid := m.RecurrenceTarget
		m.RecurrenceTarget = ""
		m.Overlay = OverlayNone
		return m.beginMutation(MutationRequest{Kind: MutationStopRecurrence, UUID: uuid})
	}
	uuid := m.DeleteTarget
	m.DeleteTarget = ""
	m.Overlay = OverlayNone
	return m.beginMutation(MutationRequest{Kind: MutationDelete, UUID: uuid})
}

// UndoLast invokes native Taskwarrior undo while the last app mutation is
// undoable. In local-only mode no sync can close the undo window.
// StopRecurrenceSelected explains and confirms a native recurrence-template
// change. Generated instances target their parent template; history is never
// rewritten.
func (m *Model) StopRecurrenceSelected() tea.Cmd {
	if m.completedTasksReadOnly() {
		return nil
	}
	task, ok := m.SelectedTask()
	if !ok || task.Recurrence == "" || m.MutationRunning {
		return nil
	}
	m.RecurrenceTarget = task.RecurrenceTargetUUID()
	m.Confirm.OpenFor("Stop recurrence", "Stop the recurrence template for this task? Existing completed occurrences stay unchanged.")
	m.Overlay = OverlayConfirm
	return nil
}

func (m *Model) UndoLast() tea.Cmd {
	if !m.Sync.UndoAvailable || m.MutationRunning || m.MigrationRunning {
		return nil
	}
	return m.beginMutation(MutationRequest{Kind: MutationUndo})
}

// OpenQuickAdd opens the command bar and asks the client for suggestions in
// separate asynchronous commands when the adapter supports them.
func (m *Model) OpenQuickAdd() tea.Cmd {
	if m.MutationRunning {
		return nil
	}
	m.Overlay = OverlayQuickAdd
	m.FailedQuickAdd = nil
	m.QuickAdd.SetSize(m.Width, m.Height)
	commands := []tea.Cmd{m.QuickAdd.OpenQuickAdd("")}
	m.ProjectDiscoveryID++
	requestID := m.ProjectDiscoveryID
	if reader, ok := m.Client.(interface {
		Projects(context.Context) ([]string, error)
	}); ok {
		commands = append(commands, ProjectsCommandWithID(m.ctx, reader, requestID))
	}
	if reader, ok := m.Client.(interface {
		Tags(context.Context) ([]string, error)
	}); ok {
		commands = append(commands, TagsCommandWithID(m.ctx, reader, requestID))
	}
	return tea.Batch(commands...)
}

// OpenSearch opens the local in-memory filter.
func (m *Model) OpenSearch() tea.Cmd {
	if m.MutationRunning {
		return nil
	}
	m.Overlay = OverlaySearch
	m.Search.SetSize(m.Width, m.Height)
	return m.Search.OpenSearch(m.Search.Query)
}

// OpenEditor opens the structured editor for the selected task.
func (m *Model) OpenEditor(field ui.EditField) tea.Cmd {
	if m.completedTasksReadOnly() {
		return nil
	}
	task, ok := m.SelectedTask()
	if !ok || m.MutationRunning {
		return nil
	}
	m.Overlay = OverlayEdit
	m.Editor.SetSize(m.Width, m.Height)
	commands := []tea.Cmd{m.Editor.OpenTask(task, field)}
	m.ProjectDiscoveryID++
	requestID := m.ProjectDiscoveryID
	if reader, ok := m.Client.(interface {
		Projects(context.Context) ([]string, error)
	}); ok {
		commands = append(commands, ProjectsCommandWithID(m.ctx, reader, requestID))
	}
	if reader, ok := m.Client.(interface {
		Tags(context.Context) ([]string, error)
	}); ok {
		commands = append(commands, TagsCommandWithID(m.ctx, reader, requestID))
	}
	return tea.Batch(commands...)
}

func (m *Model) completedTasksReadOnly() bool {
	if m != nil && m.ActiveView == ViewCompleted {
		m.Status = "Completed tasks are read-only"
		return true
	}
	return false
}

// OpenDetails opens the read-only task details modal.
func (m *Model) OpenDetails() {
	task, ok := m.SelectedTask()
	if !ok {
		return
	}
	m.Details.OpenTask(task)
	m.Details.SetSize(m.Width, m.Height)
	m.Overlay = OverlayDetails
}

// OpenHelp opens generated key help.
func (m *Model) OpenHelp() {
	m.Help.OpenHelp()
	m.Help.SetSize(m.Width, m.Height)
	m.Overlay = OverlayHelp
}

// TickCommand schedules one app timer. A zero duration is intentionally a
// no-op, which is how refresh-disabled configurations avoid background work.
func TickCommand(duration time.Duration) tea.Cmd {
	if duration <= 0 {
		return nil
	}
	return tea.Tick(duration, func(at time.Time) tea.Msg { return TickMsg(at) })
}

// Context returns the app command context for tests and integrations.
func (m *Model) Context() context.Context {
	if m == nil || m.ctx == nil {
		return context.Background()
	}
	return m.ctx
}

func (m *Model) beginSync() tea.Cmd {
	if m.MigrationRunning {
		m.SyncDeferred = true
		m.Status = "Sync deferred until project migration finishes"
		return nil
	}
	if m.Sync.Phase == SyncDisabled || !m.SyncConfigured || m.Client == nil {
		return nil
	}
	state, effect := m.Sync.Manual(m.now())
	m.Sync = state
	if effect.Action != SyncRun {
		return nil
	}
	m.Status = "Syncing…"
	return SyncCommand(m.ctx, m.Client)
}

func (m *Model) requestQuit() tea.Cmd {
	if m.MigrationRunning {
		m.Status = "Quit deferred until project migration finishes"
		return nil
	}
	if !m.Sync.Enabled || !m.SyncConfigured {
		return quitCommand()
	}
	state, effect := m.Sync.Apply(SyncEvent{Kind: SyncQuitRequested, At: m.now()})
	m.Sync = state
	switch effect.Action {
	case SyncPromptQuit:
		m.Quit.OpenQuit()
		m.Quit.SetSize(m.Width, m.Height)
		m.Overlay = OverlayQuit
		return nil
	case SyncQuit:
		return quitCommand()
	default:
		return nil
	}
}

func (m *Model) handleQuitChoice(choice ui.QuitChoice) tea.Cmd {
	var event SyncEvent
	switch choice {
	case ui.QuitSync:
		event.Kind = SyncSyncAndQuit
	case ui.QuitLocal:
		event.Kind = SyncQuitLocally
	case ui.QuitCancel:
		event.Kind = SyncQuitCancelled
	default:
		return nil
	}
	state, effect := m.Sync.Apply(SyncEvent{Kind: event.Kind, At: m.now()})
	m.Sync = state
	switch effect.Action {
	case SyncRun:
		m.Status = effect.Message
		return SyncCommand(m.ctx, m.Client)
	case SyncQuit:
		return quitCommand()
	case SyncCancelQuit:
		m.Overlay = OverlayNone
	}
	return nil
}
