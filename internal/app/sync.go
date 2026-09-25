package app

import (
	"fmt"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

func refreshTickCommand(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(at time.Time) tea.Msg { return RefreshTickMsg(at) })
}

func syncTickCommandAt(at time.Time) tea.Cmd {
	if at.IsZero() {
		return nil
	}
	delay := time.Until(at)
	if delay <= 0 {
		return func() tea.Msg { return SyncTickMsg(at) }
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return SyncTickMsg(at) })
}

func (m *Model) startStartupSync() tea.Cmd {
	if m.MigrationRunning {
		m.SyncDeferred = true
		return nil
	}
	if !m.SyncReady || !m.SyncConfigured || m.Sync.Phase == SyncDisabled || m.Client == nil {
		return m.scheduleSync()
	}
	state, effect := m.Sync.Startup(m.now())
	m.Sync = state
	if effect.Action == SyncRun {
		m.Status = "Syncing…"
		return SyncCommand(m.ctx, m.Client)
	}
	return m.scheduleSync()
}

func (m *Model) scheduleSync() tea.Cmd {
	if m.MigrationRunning {
		return nil
	}
	if !m.SyncReady || !m.SyncConfigured || m.Sync.Phase == SyncDisabled || m.Sync.NextAt.IsZero() {
		return nil
	}
	return syncTickCommandAt(m.Sync.NextAt)
}

func (m *Model) applySyncConfig(message SyncConfigMsg) tea.Cmd {
	m.SyncReady = true
	if message.Err != nil || !message.Configured {
		m.SyncConfigured = false
		m.Sync.Enabled = false
		m.Sync.Phase = SyncDisabled
		if message.Err != nil {
			m.Status = "Local only: " + conciseError(message.Err)
		} else {
			m.Status = "Local only"
		}
		return nil
	}
	m.SyncConfigured = true
	if m.Mode == ModeReady && m.Sync.Enabled {
		return m.startStartupSync()
	}
	return m.scheduleSync()
}

func (m *Model) applySync(message SyncMsg) tea.Cmd {
	if m.MigrationRunning {
		m.SyncDeferred = true
		return nil
	}
	if !m.SyncConfigured || m.Sync.Phase == SyncDisabled {
		return nil
	}
	if message.Err != nil {
		state, effect := m.Sync.Failed(m.now())
		m.Sync = state
		m.Status = effect.Message + ": " + conciseError(message.Err)
		return m.scheduleSync()
	}
	state, effect := m.Sync.Succeeded(m.now())
	m.Sync = state
	m.Status = effect.Message
	switch effect.Action {
	case SyncRefresh:
		refresh := m.beginRefresh("sync")
		return tea.Batch(refresh, m.scheduleSync())
	case SyncQuit:
		return quitCommand()
	default:
		return m.scheduleSync()
	}
}

func (m *Model) handleSyncTick(at time.Time) tea.Cmd {
	if m.MigrationRunning {
		m.SyncDeferred = true
		return nil
	}
	if !m.SyncConfigured || m.Sync.Phase == SyncDisabled {
		return nil
	}
	state, effect := m.Sync.Timer(at)
	m.Sync = state
	if effect.Action == SyncRun {
		m.Status = "Syncing…"
		return SyncCommand(m.ctx, m.Client)
	}
	return m.scheduleSync()
}

func (m *Model) handleRefreshTick(_ time.Time) tea.Cmd {
	next := refreshTickCommand(m.Config.RefreshInterval)
	if m.MigrationRunning || m.Overlay != OverlayNone || m.Mode == ModeLoading || m.Mode == ModeMutating {
		return next
	}
	refresh := m.beginRefresh("timer")
	return tea.Batch(refresh, next)
}

// SyncCountdown returns the footer text for the mutation undo grace window.
func (m *Model) SyncCountdown(now time.Time) string {
	if m == nil || !m.Sync.UndoAvailable || m.Sync.UndoUntil.IsZero() {
		return ""
	}
	remaining := m.Sync.UndoUntil.Sub(now)
	if remaining <= 0 {
		return "Syncing now · undo unavailable"
	}
	seconds := int(math.Ceil(remaining.Seconds()))
	return fmt.Sprintf("Syncing in %ds · u to undo", seconds)
}

// SyncStatus returns a concise footer status for local-only, retry, or grace
// states without exposing Taskwarrior settings.
func (m *Model) SyncStatus(now time.Time) string {
	if m == nil {
		return "Local only"
	}
	if m.MigrationRunning {
		return "Project migration in progress · undo unavailable"
	}
	if !m.SyncConfigured || m.Sync.Phase == SyncDisabled {
		if m.Sync.UndoAvailable {
			return "Local only · u to undo"
		}
		return "Local only"
	}
	if countdown := m.SyncCountdown(now); countdown != "" {
		return countdown
	}
	if m.Sync.Phase == SyncRetrying {
		return "Sync unavailable · retrying"
	}
	if m.Sync.Phase == SyncInFlight {
		return "Syncing…"
	}
	return "Sync ready"
}
