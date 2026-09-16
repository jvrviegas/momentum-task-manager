package app

import (
	"testing"
	"time"
)

func TestManualAndStartupSyncDoNotDuplicateInFlightWork(t *testing.T) {
	now := modelNow()
	state, _ := NewSyncState(syncSettings(), now).Startup(now)
	if _, effect := state.Manual(now); effect.Action != SyncNoAction {
		t.Fatal("manual sync duplicated in-flight work")
	}
	if _, effect := state.Startup(now); effect.Action != SyncNoAction {
		t.Fatal("startup sync duplicated in-flight work")
	}
}

func TestRetryIndexIsClampedAfterMalformedState(t *testing.T) {
	state := NewSyncState(syncSettings(), modelNow())
	state.RetryIndex = 999
	state, effect := state.Failed(modelNow())
	if effect.RetryDelay != 5*time.Minute || state.RetryIndex != len(RetryDelays())-1 {
		t.Fatalf("state=%#v effect=%#v", state, effect)
	}
}
