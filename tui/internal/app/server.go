package app

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// ServerState is what the app last learned about its server.
type ServerState string

const (
	ServerUnknown     ServerState = ""
	ServerOnline      ServerState = "online"
	ServerUnreachable ServerState = "unreachable"
	ServerSignedOut   ServerState = "signed_out"
)

// noteServer records what a request's outcome says about the server. Other
// failures leave the state alone: the client refuses some requests itself,
// without reaching the server.
func (m *Model) noteServer(err error) {
	var failure *api.Error
	switch {
	case err == nil:
		m.Server = ServerOnline
	case errors.Is(err, api.ErrUnreachable):
		m.Server = ServerUnreachable
	case errors.As(err, &failure) && failure.Code == api.CodeUnauthorized:
		m.Server = ServerSignedOut
	}
}

func refreshTickCommand(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(at time.Time) tea.Msg { return RefreshTickMsg(at) })
}

// scheduleUndoTick asks for a tick when the undo window closes, so the footer
// stops offering undo on time.
func (m *Model) scheduleUndoTick() tea.Cmd {
	if m.MigrationRunning || !m.Undo.Available(m.now()) {
		return nil
	}
	at := m.Undo.Until
	return tea.Tick(time.Until(at), func(time.Time) tea.Msg { return UndoTickMsg(at) })
}

func (m *Model) handleRefreshTick(_ time.Time) tea.Cmd {
	next := refreshTickCommand(m.Config.RefreshInterval)
	if m.MigrationRunning || m.Overlay != OverlayNone || m.Mode == ModeLoading || m.Mode == ModeMutating {
		return next
	}
	refresh := m.beginRefresh("timer")
	return tea.Batch(refresh, next)
}

// ServerStatus returns a concise footer status: what the app last learned
// about its server, and whether the latest change can still be undone.
func (m *Model) ServerStatus(now time.Time) string {
	if m == nil {
		return ""
	}
	switch {
	case m.MigrationRunning:
		return "Project migration in progress · undo unavailable"
	case m.Server == ServerSignedOut:
		return "Signed out · run momentum login"
	case m.Server == ServerUnreachable:
		return "Server unreachable · r to retry"
	case m.Undo.Available(now):
		return "Saved · u to undo"
	}
	return "Online"
}
