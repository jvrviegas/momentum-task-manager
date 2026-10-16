package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/backend"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// runAll runs command and every command it batches, and sends each message
// to model, as Bubble Tea would, except timers: it returns the instants of
// the sync ticks instead. Commands must not wait, as tea.Tick does for a
// future time.
func runAll(model *Model, command tea.Cmd) (ticks []time.Time) {
	if command == nil {
		return nil
	}
	switch message := command().(type) {
	case tea.BatchMsg:
		for _, each := range message {
			ticks = append(ticks, runAll(model, each)...)
		}
		return ticks
	case SyncTickMsg:
		return []time.Time{time.Time(message)}
	default:
		_, next := model.Update(message)
		return runAll(model, next)
	}
}

// TestBackendModeOffersUndoForTheMutationDelay runs the model on the backend
// client: Momentum's sync schedule and unsynced-changes prompt stay off
// (P1), and undo reverts the latest change until mutation_delay passes (D7).
func TestBackendModeOffersUndoForTheMutationDelay(t *testing.T) {
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
	now := modelNow()
	model := NewModel(ModelOptions{Client: client, Config: config.Defaults(), Now: func() time.Time { return now }, Backend: true})
	complete := func() {
		t.Helper()
		command := model.beginMutation(MutationRequest{Kind: MutationComplete, UUID: id})
		if command == nil {
			t.Fatal("complete was not dispatched")
		}
		if ticks := runAll(model, command); len(ticks) != 1 || !ticks[0].Equal(now.Add(15*time.Second)) {
			t.Fatalf("ticks %v, want one when undo closes", ticks)
		}
	}

	model.Update(LoadTasksCommand(context.Background(), client, "initial", now)())
	if len(model.Tasks) != 1 || model.SyncConfigured || model.Sync.Phase != SyncDisabled || model.Server != ServerOnline || model.SyncStatus(now) != "Online" {
		t.Fatalf("after load: tasks %v, sync configured %v, phase %s, server %q, status %q", model.Tasks, model.SyncConfigured, model.Sync.Phase, model.Server, model.SyncStatus(now))
	}

	complete()
	if !model.Sync.UndoAvailable || !model.Sync.UndoUntil.Equal(now.Add(15*time.Second)) || model.SyncStatus(now) != "Saved · u to undo" {
		t.Fatalf("after complete: sync %#v, status %q", model.Sync, model.SyncStatus(now))
	}
	if _, tick := model.Update(SyncTickMsg(now.Add(10 * time.Second))); tick != nil || !model.Sync.UndoAvailable {
		t.Fatalf("an earlier change's timer closed undo: %#v", model.Sync)
	}
	now = now.Add(14 * time.Second)
	_, undo := model.Update(key("u"))
	if undo == nil {
		t.Fatal("u did not dispatch undo within the window")
	}
	runAll(model, undo)
	if model.Sync.UndoAvailable || model.SyncStatus(now) != "Online" {
		t.Fatalf("after undo: sync %#v", model.Sync)
	}

	complete()
	model.Update(SyncTickMsg(model.Sync.UndoUntil))
	if _, undo := model.Update(key("u")); undo != nil || model.Sync.UndoAvailable {
		t.Fatalf("undo after the timer closed the window: %#v", model.Sync)
	}

	complete()
	now = now.Add(15 * time.Second) // the window passed before its timer fired
	if _, undo := model.Update(key("u")); undo != nil || model.Sync.UndoAvailable || model.SyncStatus(now) != "Online" {
		t.Fatalf("undo after the window passed: %#v", model.Sync)
	}

	if quit := model.requestQuit(); quit == nil || model.Overlay == OverlayQuit {
		t.Fatalf("quit opened %s", model.Overlay)
	} else if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("quit did not quit")
	}
	want := []string{api.RouteListTasks, api.RouteListTasks, // initial load: pending, completed
		api.RouteCompleteTask, api.RouteListTasks, api.RouteListTasks, api.RouteUndo, api.RouteListTasks, api.RouteListTasks,
		api.RouteCompleteTask, api.RouteListTasks, api.RouteListTasks, api.RouteCompleteTask, api.RouteListTasks, api.RouteListTasks}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("routes %v, want %v", routes, want)
	}
}

func TestBackendModeShowsTheServersState(t *testing.T) {
	unreachable := fmt.Errorf("%w: dial tcp 127.0.0.1:1: connection refused", api.ErrUnreachable)
	for _, width := range []int{120, 79, 50} {
		model := NewModel(ModelOptions{Client: &fakeClient{}, Config: config.Defaults(), Now: modelNow, Width: width, Height: 24, Backend: true})
		footer := func() string {
			t.Helper()
			lines := strings.Split(ansi.Strip(model.View().Content), "\n")
			for _, line := range lines {
				if lipgloss.Width(line) > width {
					t.Errorf("width %d: line %q is %d wide", width, line, lipgloss.Width(line))
				}
			}
			return strings.Join(lines[len(lines)-2:], "\n")
		}
		tasks := TasksMsg{Reason: "refresh", Tasks: []domain.Task{{UUID: "u", Description: "Send invoice", Status: "pending"}}}

		if got := footer(); !strings.Contains(got, "Waiting for the server") || width > 50 && !strings.Contains(got, "Loading tasks from the server") {
			t.Errorf("width %d loading: %q", width, got)
		}
		model.Update(tasks)
		if got := footer(); !strings.Contains(got, "✓ Online") {
			t.Errorf("width %d online: %q", width, got)
		}
		model.Update(TasksMsg{Reason: "refresh", Err: unreachable})
		if got := footer(); !strings.Contains(got, "! Server unreachable") || width == 120 && !strings.Contains(got, "Server unreachable · r to retry") {
			t.Errorf("width %d unreachable: %q", width, got)
		}
		if _, retry := model.Update(key("r")); retry == nil {
			t.Errorf("width %d: r did not retry", width)
		}
		model.Update(tasks)
		model.Update(MutationMsg{Kind: MutationComplete, UUID: "u", Err: &api.Error{Code: api.CodeUnauthorized, Message: "sign in again"}})
		if got := footer(); !strings.Contains(got, "! Signed out") || width == 120 && !strings.Contains(got, "Signed out · run momentum login") {
			t.Errorf("width %d signed out: %q", width, got)
		}
		model.Update(MutationMsg{Kind: MutationAdd, Err: &api.Error{Code: api.CodeInvalid, Message: "due: unknown date", Field: "due"}})
		if model.Server != ServerSignedOut {
			t.Errorf("width %d: a refusal the adapter makes itself changed the state to %q", width, model.Server)
		}
		model.Update(tasks)
		if got := footer(); !strings.Contains(got, "✓ Online") || strings.Contains(got, "Local only") {
			t.Errorf("width %d back online: %q", width, got)
		}
	}
}

// TestTaskwarriorModeIgnoresServerErrors keeps the server state to backend
// mode.
func TestTaskwarriorModeIgnoresServerErrors(t *testing.T) {
	model := testModel(&fakeClient{})
	model.Update(TasksMsg{Reason: "refresh", Err: api.ErrUnreachable})
	if model.Server != ServerUnknown || strings.Contains(model.SyncStatus(modelNow()), "Server") {
		t.Errorf("server %q, status %q", model.Server, model.SyncStatus(modelNow()))
	}
}
