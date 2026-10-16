package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/backend"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// TestBackendModeChangesAndUndoesWithoutSync runs the model on the backend
// client: Momentum's sync schedule and unsynced-changes prompt stay off
// (P1), while undo still reverts the latest change.
func TestBackendModeChangesAndUndoesWithoutSync(t *testing.T) {
	const id = "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10"
	task := `{"uuid":"` + id + `","description":"Send invoice","status":"pending","entry":"2026-09-07T09:00:00Z","modified":"2026-09-07T09:00:00Z","urgency":2}`
	var mu sync.Mutex
	var routes []string
	mux := http.NewServeMux()
	for route, answer := range map[string]string{
		api.RouteListTasks: `{"tasks":[` + task + `]}`, api.RouteCompleteTask: task, api.RouteUndo: task,
	} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			mu.Lock()
			routes = append(routes, r.Pattern)
			mu.Unlock()
			io.WriteString(w, answer)
		})
	}
	server := httptest.NewServer(mux)
	defer server.Close()
	client := backend.New(server.URL, "mtk_device", "")
	ctx := context.Background()

	model := NewModel(ModelOptions{Client: client, Config: config.Defaults(), Now: modelNow})
	model.Update(LoadTasksCommand(ctx, client, "initial", modelNow())())
	model.Update(SyncConfigCommand(ctx, client)())
	if len(model.Tasks) != 1 || model.Tasks[0].UUID != id || model.SyncConfigured || model.Sync.Phase != SyncDisabled {
		t.Fatalf("after load: tasks %v, sync configured %v, phase %s", model.Tasks, model.SyncConfigured, model.Sync.Phase)
	}

	complete := model.beginMutation(MutationRequest{Kind: MutationComplete, UUID: id})
	if complete == nil {
		t.Fatal("complete was not dispatched")
	}
	_, refresh := model.Update(complete())
	if refresh == nil || model.Sync.Phase != SyncDisabled || !model.Sync.UndoAvailable || !model.Sync.NextAt.IsZero() {
		t.Fatalf("after complete: sync %#v", model.Sync)
	}
	model.Update(refresh())
	_, undo := model.Update(key("u"))
	if undo == nil {
		t.Fatal("u did not dispatch undo")
	}
	model.Update(undo())
	if quit := model.requestQuit(); quit == nil || model.Overlay == OverlayQuit {
		t.Fatalf("quit opened %s", model.Overlay)
	} else if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("quit did not quit")
	}
	want := []string{api.RouteListTasks, api.RouteListTasks, api.RouteCompleteTask, api.RouteListTasks, api.RouteListTasks, api.RouteUndo}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("routes %v, want %v", routes, want)
	}
}
