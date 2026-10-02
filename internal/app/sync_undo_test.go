package app

import (
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func TestSuccessfulUndoClearsSyncGraceInsteadOfCreatingAnotherMutation(t *testing.T) {
	model := actionModel(&fakeClient{exports: [][]domain.Task{{}}})
	model.SyncConfigured = true
	model.Sync, _ = model.Sync.Mutation(model.now())
	model.PendingMutation = &MutationRequest{Kind: MutationUndo}
	model.MutationRunning = true
	model.Update(MutationMsg{Kind: MutationUndo})
	if model.Sync.Unsynced || model.Sync.UndoAvailable || model.Sync.Phase == SyncGrace {
		t.Fatalf("undo left grace state=%#v", model.Sync)
	}
}
