package app

import (
	"reflect"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/internal/config"
)

func syncSettings() config.SyncConfig {
	return config.SyncConfig{Enabled: true, Interval: 5 * time.Minute, MutationDelay: 15 * time.Second, Startup: true, Shutdown: true}
}

func TestNewSyncStateSchedulesStartupOrPeriodicWork(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	startup := NewSyncState(syncSettings(), now)
	if startup.Phase != SyncWaiting || !startup.NextAt.Equal(now) {
		t.Fatalf("startup=%#v", startup)
	}
	settings := syncSettings()
	settings.Startup = false
	periodic := NewSyncState(settings, now)
	if !periodic.NextAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("periodic=%#v", periodic)
	}
}

func TestDisabledSyncSchedulesNoWork(t *testing.T) {
	settings := syncSettings()
	settings.Enabled = false
	state := NewSyncState(settings, time.Now())
	if state.Phase != SyncDisabled || !state.NextAt.IsZero() {
		t.Fatalf("state=%#v", state)
	}
	_, effect := state.Timer(time.Now())
	if effect.Action != SyncNoAction {
		t.Fatalf("effect=%#v", effect)
	}
}

func TestStartupTransitionRunsSync(t *testing.T) {
	now := time.Now()
	state := NewSyncState(syncSettings(), now)
	state, effect := state.Startup(now)
	if state.Phase != SyncInFlight || effect.Action != SyncRun || !state.NextAt.IsZero() {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
}

func TestPeriodicTimerDoesNotRunEarly(t *testing.T) {
	now := time.Now()
	settings := syncSettings()
	settings.Startup = false
	state := NewSyncState(settings, now)
	state, effect := state.Timer(now.Add(4 * time.Minute))
	if effect.Action != SyncNoAction || state.Phase != SyncWaiting {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
}

func TestMutationResetsUndoDelay(t *testing.T) {
	now := time.Now()
	state := NewSyncState(syncSettings(), now)
	state, effect := state.Mutation(now)
	if state.Phase != SyncGrace || !state.Unsynced || !state.UndoAvailable || !state.UndoUntil.Equal(now.Add(15*time.Second)) || effect.NextAt != state.UndoUntil {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
	later := now.Add(3 * time.Second)
	state, _ = state.Mutation(later)
	if !state.UndoUntil.Equal(later.Add(15 * time.Second)) {
		t.Fatalf("mutation did not reset delay: %#v", state)
	}
}

func TestTimerStartsSyncAfterMutationDelay(t *testing.T) {
	now := time.Now()
	state, _ := NewSyncState(syncSettings(), now).Mutation(now)
	state, effect := state.Timer(now.Add(15 * time.Second))
	if state.Phase != SyncInFlight || effect.Action != SyncRun || !state.NextAt.IsZero() {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
}

func TestRetryProgressionAndCap(t *testing.T) {
	now := time.Now()
	state := NewSyncState(syncSettings(), now)
	want := []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	for index, delay := range want {
		var effect SyncEffect
		state, effect = state.Failed(now)
		if effect.RetryDelay != delay || !state.NextAt.Equal(now.Add(delay)) {
			t.Fatalf("failure %d state=%#v effect=%#v", index, state, effect)
		}
	}
	if state.RetryIndex != 4 {
		t.Fatalf("retry index should cap at last slot: %d", state.RetryIndex)
	}
}

func TestSuccessResetsBackoffAndRequestsRefresh(t *testing.T) {
	now := time.Now()
	state := NewSyncState(syncSettings(), now)
	state.RetryIndex = 4
	state.Phase = SyncRetrying
	state, effect := state.Succeeded(now)
	if state.RetryIndex != 0 || state.Unsynced || state.UndoAvailable || state.Phase != SyncWaiting || effect.Action != SyncRefresh || !state.NextAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
	if effect.Message != "Changes synced" {
		t.Fatalf("message=%q", effect.Message)
	}
}

func TestManualSyncClosesUndoWindow(t *testing.T) {
	now := time.Now()
	state, _ := NewSyncState(syncSettings(), now).Mutation(now)
	state, effect := state.Manual(now.Add(time.Second))
	if state.Phase != SyncInFlight || state.UndoAvailable || !state.UndoUntil.IsZero() || effect.Action != SyncRun || !state.Unsynced {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
}

func TestQuitWithoutChangesIsImmediate(t *testing.T) {
	state := NewSyncState(syncSettings(), time.Now())
	state, effect := state.Apply(SyncEvent{Kind: SyncQuitRequested, At: time.Now()})
	if effect.Action != SyncQuit || state.QuitPending {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
}

func TestUnsyncedQuitPromptsAndSupportsChoices(t *testing.T) {
	now := time.Now()
	state, _ := NewSyncState(syncSettings(), now).Mutation(now)
	state, effect := state.Apply(SyncEvent{Kind: SyncQuitRequested, At: now})
	if effect.Action != SyncPromptQuit || !state.QuitPending || state.Phase != SyncQuitWait {
		t.Fatalf("prompt state=%#v effect=%#v", state, effect)
	}
	state, effect = state.Apply(SyncEvent{Kind: SyncQuitLocally, At: now})
	if effect.Action != SyncQuit || state.QuitPending {
		t.Fatalf("local state=%#v effect=%#v", state, effect)
	}
}

func TestSyncAndQuitWaitsForSuccess(t *testing.T) {
	now := time.Now()
	state, _ := NewSyncState(syncSettings(), now).Mutation(now)
	state, _ = state.Apply(SyncEvent{Kind: SyncQuitRequested, At: now})
	state, effect := state.Apply(SyncEvent{Kind: SyncSyncAndQuit, At: now})
	if state.Phase != SyncInFlight || effect.Action != SyncRun || state.UndoAvailable {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
	state, effect = state.Succeeded(now)
	if effect.Action != SyncQuit || state.QuitPending || state.Unsynced {
		t.Fatalf("success state=%#v effect=%#v", state, effect)
	}
}

func TestCancelledQuitReturnsToGrace(t *testing.T) {
	now := time.Now()
	state, _ := NewSyncState(syncSettings(), now).Mutation(now)
	state, _ = state.Apply(SyncEvent{Kind: SyncQuitRequested, At: now})
	state, effect := state.Apply(SyncEvent{Kind: SyncQuitCancelled, At: now})
	if effect.Action != SyncCancelQuit || state.Phase != SyncGrace || !state.Unsynced || state.QuitPending {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
}

func TestUndoOnlyAvailableDuringGrace(t *testing.T) {
	now := time.Now()
	state := NewSyncState(syncSettings(), now)
	original := state
	state, effect := state.Apply(SyncEvent{Kind: SyncUndo, At: now})
	if effect.Action != SyncNoAction || !reflect.DeepEqual(state, original) {
		t.Fatal("undo should be ignored without a dirty grace window")
	}
	state, _ = state.Mutation(now)
	state, effect = state.Apply(SyncEvent{Kind: SyncUndo, At: now})
	if effect.Action != SyncRefresh || state.UndoAvailable || state.Unsynced || !state.UndoUntil.IsZero() || state.Phase != SyncWaiting {
		t.Fatalf("undo state=%#v effect=%#v", state, effect)
	}
}

func TestConvenienceTransitionsMatchApply(t *testing.T) {
	now := time.Now()
	state := NewSyncState(syncSettings(), now)
	got, effect := state.Failed(now)
	want, wantEffect := state.Apply(SyncEvent{Kind: SyncFailed, At: now})
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(effect, wantEffect) {
		t.Fatalf("got=%#v/%#v want=%#v/%#v", got, effect, want, wantEffect)
	}
}
