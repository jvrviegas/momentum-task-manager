package app

import (
	"testing"
	"time"
)

func TestMutationGraceExpiryClosesUndoBeforeStartingSync(t *testing.T) {
	now := modelNow()
	state, _ := NewSyncState(syncSettings(), now).Mutation(now)
	state, effect := state.Timer(now.Add(15 * time.Second))
	if effect.Action != SyncRun || state.Phase != SyncInFlight || state.UndoAvailable || !state.UndoUntil.IsZero() {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
}
