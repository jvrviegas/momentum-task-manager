package app

import (
	"time"

	"github.com/jvrviegas/momentum/internal/config"
)

// SyncPhase is the pure synchronization lifecycle state.
type SyncPhase string

const (
	SyncDisabled SyncPhase = "disabled"
	SyncWaiting  SyncPhase = "waiting"
	SyncInFlight SyncPhase = "in_flight"
	SyncGrace    SyncPhase = "undo_grace"
	SyncRetrying SyncPhase = "retrying"
	SyncQuitWait SyncPhase = "quit_wait"
)

// SyncAction tells the Bubble Tea shell what asynchronous work or modal action
// should happen after a transition.
type SyncAction string

const (
	SyncNoAction   SyncAction = ""
	SyncRun        SyncAction = "run_sync"
	SyncRefresh    SyncAction = "refresh"
	SyncPromptQuit SyncAction = "prompt_quit"
	SyncQuit       SyncAction = "quit"
	SyncCancelQuit SyncAction = "cancel_quit"
)

// SyncEventKind identifies an input to the synchronization state machine.
type SyncEventKind string

const (
	SyncStartup       SyncEventKind = "startup"
	SyncTimer         SyncEventKind = "timer"
	SyncMutation      SyncEventKind = "mutation"
	SyncManual        SyncEventKind = "manual"
	SyncSucceeded     SyncEventKind = "succeeded"
	SyncFailed        SyncEventKind = "failed"
	SyncQuitRequested SyncEventKind = "quit_requested"
	SyncSyncAndQuit   SyncEventKind = "sync_and_quit"
	SyncQuitLocally   SyncEventKind = "quit_locally"
	SyncQuitCancelled SyncEventKind = "quit_cancelled"
	SyncUndo          SyncEventKind = "undo"
)

// SyncEvent is deliberately small so transitions remain deterministic.
type SyncEvent struct {
	Kind SyncEventKind
	At   time.Time
}

// SyncEffect is the side-effect request emitted by a pure transition.
type SyncEffect struct {
	Action     SyncAction
	NextAt     time.Time
	RetryDelay time.Duration
	Message    string
}

// SyncState owns timing and UI state, not Taskwarrior's sync protocol.
type SyncState struct {
	Enabled        bool
	StartupEnabled bool
	Shutdown       bool
	Interval       time.Duration
	MutationDelay  time.Duration
	Phase          SyncPhase
	NextAt         time.Time
	RetryIndex     int
	Unsynced       bool
	UndoAvailable  bool
	UndoUntil      time.Time
	QuitPending    bool
}

var retryDelays = []time.Duration{
	15 * time.Second,
	30 * time.Second,
	1 * time.Minute,
	2 * time.Minute,
	5 * time.Minute,
}

// RetryDelays returns a copy of the approved retry progression.
func RetryDelays() []time.Duration {
	return append([]time.Duration(nil), retryDelays...)
}

// NewSyncState creates the initial state. Disabled sync installs no timer.
func NewSyncState(settings config.SyncConfig, now time.Time) SyncState {
	state := SyncState{
		Enabled:        settings.Enabled,
		StartupEnabled: settings.Startup,
		Shutdown:       settings.Shutdown,
		Interval:       settings.Interval,
		MutationDelay:  settings.MutationDelay,
		Phase:          SyncWaiting,
	}
	if !state.Enabled {
		state.Phase = SyncDisabled
		return state
	}
	if state.Interval <= 0 {
		state.Interval = 5 * time.Minute
	}
	if state.MutationDelay < 0 {
		state.MutationDelay = 0
	}
	if state.StartupEnabled {
		state.NextAt = now
	} else {
		state.NextAt = now.Add(state.Interval)
	}
	return state
}

// Apply performs one pure state transition.
func (s SyncState) Apply(event SyncEvent) (SyncState, SyncEffect) {
	at := event.At
	if at.IsZero() {
		at = time.Now()
	}
	if !s.Enabled {
		s.Phase = SyncDisabled
		return s, SyncEffect{}
	}

	switch event.Kind {
	case SyncStartup:
		if s.StartupEnabled && !s.Unsynced && !s.QuitPending {
			return s.beginSync()
		}
		return s, SyncEffect{}
	case SyncTimer:
		if (s.Phase == SyncWaiting || s.Phase == SyncRetrying || s.Phase == SyncGrace) && !s.NextAt.After(at) {
			return s.beginSync()
		}
		return s, SyncEffect{}
	case SyncMutation:
		s.Unsynced = true
		s.UndoAvailable = true
		s.UndoUntil = at.Add(s.MutationDelay)
		s.NextAt = s.UndoUntil
		s.Phase = SyncGrace
		return s, SyncEffect{NextAt: s.NextAt, Message: "Changes pending sync"}
	case SyncManual:
		s.UndoAvailable = false
		s.UndoUntil = time.Time{}
		return s.beginSync()
	case SyncSucceeded:
		s.Unsynced = false
		s.UndoAvailable = false
		s.UndoUntil = time.Time{}
		s.RetryIndex = 0
		s.Phase = SyncWaiting
		s.NextAt = at.Add(s.Interval)
		effect := SyncEffect{Action: SyncRefresh, NextAt: s.NextAt, Message: "Changes synced"}
		if s.QuitPending {
			s.QuitPending = false
			effect.Action = SyncQuit
			effect.Message = "Changes synced; quitting"
		}
		return s, effect
	case SyncFailed:
		s.Phase = SyncRetrying
		delay := retryDelays[s.RetryIndex]
		if s.RetryIndex < len(retryDelays)-1 {
			s.RetryIndex++
		}
		s.NextAt = at.Add(delay)
		return s, SyncEffect{NextAt: s.NextAt, RetryDelay: delay, Message: "Sync unavailable; retry scheduled"}
	case SyncQuitRequested:
		if !s.Unsynced || !s.Shutdown {
			return s, SyncEffect{Action: SyncQuit}
		}
		s.QuitPending = true
		s.Phase = SyncQuitWait
		return s, SyncEffect{Action: SyncPromptQuit, Message: "Unsynced changes"}
	case SyncSyncAndQuit:
		if !s.QuitPending {
			return s, SyncEffect{}
		}
		s.UndoAvailable = false
		s.UndoUntil = time.Time{}
		s.Phase = SyncInFlight
		return s, SyncEffect{Action: SyncRun, Message: "Syncing before quit"}
	case SyncQuitLocally:
		s.QuitPending = false
		return s, SyncEffect{Action: SyncQuit}
	case SyncQuitCancelled:
		s.QuitPending = false
		if s.Unsynced {
			s.Phase = SyncGrace
		} else {
			s.Phase = SyncWaiting
		}
		return s, SyncEffect{Action: SyncCancelQuit}
	case SyncUndo:
		if s.UndoAvailable && s.Unsynced {
			s.Unsynced = false
			s.UndoAvailable = false
			s.UndoUntil = time.Time{}
			s.Phase = SyncWaiting
			s.NextAt = at.Add(s.Interval)
			return s, SyncEffect{Action: SyncRefresh, NextAt: s.NextAt, Message: "Last change undone"}
		}
	}
	return s, SyncEffect{}
}

func (s SyncState) beginSync() (SyncState, SyncEffect) {
	s.Phase = SyncInFlight
	s.NextAt = time.Time{}
	return s, SyncEffect{Action: SyncRun}
}

// Startup, Timer, Mutation, Manual, Succeeded, and Failed are convenience
// wrappers around Apply for update-loop code and tests.
func (s SyncState) Startup(now time.Time) (SyncState, SyncEffect) {
	return s.Apply(SyncEvent{Kind: SyncStartup, At: now})
}
func (s SyncState) Timer(now time.Time) (SyncState, SyncEffect) {
	return s.Apply(SyncEvent{Kind: SyncTimer, At: now})
}
func (s SyncState) Mutation(now time.Time) (SyncState, SyncEffect) {
	return s.Apply(SyncEvent{Kind: SyncMutation, At: now})
}
func (s SyncState) Manual(now time.Time) (SyncState, SyncEffect) {
	return s.Apply(SyncEvent{Kind: SyncManual, At: now})
}
func (s SyncState) Succeeded(now time.Time) (SyncState, SyncEffect) {
	return s.Apply(SyncEvent{Kind: SyncSucceeded, At: now})
}
func (s SyncState) Failed(now time.Time) (SyncState, SyncEffect) {
	return s.Apply(SyncEvent{Kind: SyncFailed, At: now})
}
