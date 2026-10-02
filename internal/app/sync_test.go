package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/internal/config"
	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/taskwarrior"
	"github.com/jvrviegas/momentum-task-manager/internal/ui"
)

type configuredFakeClient struct {
	*fakeClient
	configured bool
	configErr  error
}

func (f configuredFakeClient) SyncConfigured(context.Context) (bool, error) {
	return f.configured, f.configErr
}

func TestLocalTasksAreVisibleBeforeStartupSyncCommandRuns(t *testing.T) {
	client := &fakeClient{}
	model := actionModel(client)
	model.Tasks = nil
	model.Views = domain.Views{}
	model.Mode = ModeLoading
	model.SyncReady = true
	model.SyncConfigured = true
	model.Update(TasksMsg{Tasks: []domain.Task{testTask("local", 1)}, Reason: "initial"})
	if len(model.Tasks) != 1 || model.Mode != ModeReady || model.Sync.Phase != SyncInFlight {
		t.Fatalf("model=%#v", model)
	}
}

func TestConfiguredCheckDisablesSyncForLocalOnlyMode(t *testing.T) {
	model := testModel(configuredFakeClient{fakeClient: &fakeClient{}, configured: false})
	model.Update(SyncConfigMsg{Configured: false})
	if model.SyncConfigured || model.Sync.Enabled || model.Sync.Phase != SyncDisabled || model.SyncStatus(time.Now()) != "Local only" {
		t.Fatalf("model=%#v", model)
	}
}

func TestLocalOnlyAddCanBeUndoneWithoutStartingSync(t *testing.T) {
	settings := config.Defaults()
	settings.Sync.Enabled = false
	client := &fakeClient{}
	model := NewModel(ModelOptions{Client: client, Config: settings, Now: modelNow})
	model.Update(TasksMsg{Reason: "initial"})

	add := model.beginMutation(MutationRequest{Kind: MutationAdd, Input: domain.NewTask{Description: "local-only add"}})
	if add == nil {
		t.Fatal("local-only add was not dispatched")
	}
	_, refresh := model.Update(add())
	if refresh == nil || model.Sync.Phase != SyncDisabled || !model.Sync.UndoAvailable || model.SyncConfigured || model.SyncStatus(model.now()) != "Local only · u to undo" {
		t.Fatalf("local-only add did not enable native undo: sync=%#v configured=%v status=%q", model.Sync, model.SyncConfigured, model.SyncStatus(model.now()))
	}
	model.Update(refresh())
	_, undo := model.Update(key("u"))
	if undo == nil {
		t.Fatal("u did not dispatch native undo after a local-only add")
	}
	message, ok := undo().(MutationMsg)
	if !ok || message.Kind != MutationUndo || len(client.mutations) != 2 || client.mutations[1].Kind != MutationUndo {
		t.Fatalf("undo=%#v mutations=%#v", message, client.mutations)
	}
	model.Update(message)
	if model.Sync.UndoAvailable || model.Sync.Unsynced || model.Sync.Phase != SyncDisabled || client.syncs != 0 || model.SyncStatus(model.now()) != "Local only" {
		t.Fatalf("undo left sync/undo state dirty: sync=%#v sync calls=%d", model.Sync, client.syncs)
	}
}

func TestConfiguredCheckStartsStartupSyncAfterLocalLoad(t *testing.T) {
	model := testModel(configuredFakeClient{fakeClient: &fakeClient{}, configured: true})
	model.Mode = ModeReady
	model.Update(SyncConfigMsg{Configured: true})
	if !model.SyncConfigured || model.Sync.Phase != SyncInFlight || model.Status != "Syncing…" {
		t.Fatalf("model=%#v", model)
	}
}

func TestConfiguredCheckErrorFallsBackWithoutSecrets(t *testing.T) {
	err := errors.New("sync settings unavailable")
	model := testModel(&fakeClient{})
	model.Update(SyncConfigMsg{Err: err})
	if model.SyncConfigured || model.Sync.Phase != SyncDisabled || !strings.Contains(model.Status, "Local only") {
		t.Fatalf("model=%#v", model)
	}
}

func TestSuccessfulSyncRefreshesAndSchedulesNextSync(t *testing.T) {
	client := &fakeClient{exports: [][]domain.Task{{testTask("after", 1)}}}
	model := actionModel(client)
	model.SyncConfigured = true
	model.Sync.Phase = SyncInFlight
	model.Sync.NextAt = time.Time{}
	_, cmd := model.Update(SyncMsg{Result: taskwarrior.SyncResult{Changed: true}})
	if cmd == nil || model.Status != "Changes synced" || model.Sync.Phase != SyncWaiting || model.Sync.Unsynced || model.Sync.NextAt.IsZero() {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
}

func TestFailedSyncKeepsApplicationOperationalAndSchedulesRetry(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.SyncConfigured = true
	model.Sync.Phase = SyncInFlight
	model.Update(SyncMsg{Err: errors.New("network unavailable")})
	if model.Mode == ModeError || model.Sync.Phase != SyncRetrying || model.Sync.NextAt.IsZero() || !strings.Contains(model.Status, "retry") {
		t.Fatalf("model=%#v", model)
	}
}

func TestManualSyncClosesUndoAndDispatchesAsyncCommand(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.SyncConfigured = true
	model.Sync, _ = model.Sync.Mutation(model.now())
	model.SyncConfigured = true
	cmd := model.beginSync()
	if cmd == nil || model.Sync.Phase != SyncInFlight || model.Sync.UndoAvailable || model.Status != "Syncing…" {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
}

func TestSyncTimerStartsOnlyWhenDue(t *testing.T) {
	now := modelNow()
	model := actionModel(&fakeClient{})
	model.SyncConfigured = true
	model.Sync.Phase = SyncWaiting
	model.Sync.NextAt = now.Add(time.Minute)
	model.now = func() time.Time { return now }
	model.Update(SyncTickMsg(now.Add(30 * time.Second)))
	if model.Sync.Phase != SyncWaiting {
		t.Fatalf("early timer changed state=%#v", model.Sync)
	}
	model.Sync.NextAt = now
	model.Update(SyncTickMsg(now))
	if model.Sync.Phase != SyncInFlight {
		t.Fatalf("due timer did not start sync=%#v", model.Sync)
	}
}

func TestRefreshTimerPausesDuringOverlay(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Overlay = OverlayQuickAdd
	model.QuickAdd.Open = true
	model.Update(RefreshTickMsg(time.Now()))
	if model.Mode != ModeReady {
		t.Fatalf("refresh ran during overlay: mode=%s", model.Mode)
	}
}

func TestRefreshTimerRunsWhenNoOverlay(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Config.RefreshInterval = time.Hour
	model.Update(RefreshTickMsg(time.Now()))
	if model.Mode != ModeRefreshing {
		t.Fatalf("mode=%s", model.Mode)
	}
}

func TestUndoCountdownRoundsUpAndExpires(t *testing.T) {
	now := modelNow()
	model := actionModel(&fakeClient{})
	model.now = func() time.Time { return now }
	model.SyncConfigured = true
	model.Sync, _ = model.Sync.Mutation(now)
	if got := model.SyncCountdown(now.Add(3 * time.Second)); got != "Syncing in 12s · u to undo" {
		t.Fatalf("countdown=%q", got)
	}
	if got := model.SyncCountdown(now.Add(16 * time.Second)); got != "Syncing now · undo unavailable" {
		t.Fatalf("expired=%q", got)
	}
}

func TestSyncStatusCoversConfiguredStates(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.SyncConfigured = false
	if got := model.SyncStatus(time.Now()); got != "Local only" {
		t.Fatalf("unconfigured=%q", got)
	}
	model.SyncConfigured = true
	model.Sync.Phase = SyncRetrying
	if got := model.SyncStatus(time.Now()); got != "Sync unavailable · retrying" {
		t.Fatalf("retry=%q", got)
	}
	model.Sync.Phase = SyncInFlight
	if got := model.SyncStatus(time.Now()); got != "Syncing…" {
		t.Fatalf("flight=%q", got)
	}
}

func TestUnsyncedQuitSyncChoiceRunsNativeSync(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.SyncConfigured = true
	model.Sync, _ = model.Sync.Mutation(model.now())
	model.requestQuit()
	if model.Overlay != OverlayQuit {
		t.Fatalf("quit overlay=%s", model.Overlay)
	}
	cmd := model.handleQuitChoice(ui.QuitSync)
	if cmd == nil || model.Sync.Phase != SyncInFlight || model.Sync.UndoAvailable {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
}

func TestUnsyncedQuitCancelRestoresOverlayState(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.SyncConfigured = true
	model.Sync, _ = model.Sync.Mutation(model.now())
	model.requestQuit()
	model.handleQuitChoice(ui.QuitCancel)
	if model.Overlay != OverlayNone || model.Sync.Phase != SyncGrace || !model.Sync.Unsynced {
		t.Fatalf("model=%#v", model)
	}
}

func TestDisabledRefreshIntervalInstallsNoRefreshCommand(t *testing.T) {
	settings := config.Defaults()
	settings.RefreshInterval = 0
	model := NewModel(ModelOptions{Config: settings, Client: &fakeClient{}})
	if cmd := model.Init(); cmd == nil {
		t.Fatal("initial load still required")
	}
}

func modelNow() time.Time { return time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC) }
