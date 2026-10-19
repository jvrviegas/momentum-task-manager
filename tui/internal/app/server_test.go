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
// the undo ticks instead. Commands must not wait, as tea.Tick does for a
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
	case UndoTickMsg:
		return []time.Time{time.Time(message)}
	default:
		_, next := model.Update(message)
		return runAll(model, next)
	}
}

// TestUndoIsOfferedForTheMutationDelay runs the model on the backend client:
// undo reverts the latest change until mutation_delay passes (D7), and
// quitting never asks first (P1).
func TestUndoIsOfferedForTheMutationDelay(t *testing.T) {
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
	model := NewModel(ModelOptions{Client: client, Config: config.Defaults(), Now: func() time.Time { return now }})
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
	if len(model.Tasks) != 1 || model.Server != ServerOnline || model.ServerStatus(now) != "Online" {
		t.Fatalf("after load: tasks %v, server %q, status %q", model.Tasks, model.Server, model.ServerStatus(now))
	}

	complete()
	if !model.Undo.Available(now) || !model.Undo.Until.Equal(now.Add(15*time.Second)) || model.ServerStatus(now) != "Saved · u to undo" {
		t.Fatalf("after complete: undo %#v, status %q", model.Undo, model.ServerStatus(now))
	}
	if _, tick := model.Update(UndoTickMsg(now.Add(10 * time.Second))); tick != nil || !model.Undo.Available(now) {
		t.Fatalf("an earlier change's timer closed undo: %#v", model.Undo)
	}
	now = now.Add(14 * time.Second)
	_, undo := model.Update(key("u"))
	if undo == nil {
		t.Fatal("u did not dispatch undo within the window")
	}
	runAll(model, undo)
	if model.Undo.Available(now) || model.ServerStatus(now) != "Online" {
		t.Fatalf("after undo: undo %#v", model.Undo)
	}

	complete()
	now = model.Undo.Until // the timer fires as the window closes
	model.Update(UndoTickMsg(now))
	if _, undo := model.Update(key("u")); undo != nil || model.ServerStatus(now) != "Online" {
		t.Fatalf("undo after the window closed: %#v", model.Undo)
	}

	if quit := model.requestQuit(); quit == nil {
		t.Fatal("quit asked first")
	} else if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("quit did not quit")
	}
	want := []string{api.RouteListTasks, api.RouteListTasks, // initial load: pending, completed
		api.RouteCompleteTask, api.RouteListTasks, api.RouteListTasks, api.RouteUndo, api.RouteListTasks, api.RouteListTasks,
		api.RouteCompleteTask, api.RouteListTasks, api.RouteListTasks}
	if !reflect.DeepEqual(routes, want) {
		t.Errorf("routes %v, want %v", routes, want)
	}
}

func TestTheFooterShowsTheServersState(t *testing.T) {
	unreachable := fmt.Errorf("%w: dial tcp 127.0.0.1:1: connection refused", api.ErrUnreachable)
	for _, width := range []int{120, 79, 50} {
		model := NewModel(ModelOptions{Client: &fakeClient{}, Config: config.Defaults(), Now: modelNow, Width: width, Height: 24})
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

func TestSuccessfulUndoClosesTheWindow(t *testing.T) {
	model := actionModel(&fakeClient{exports: [][]domain.Task{{}}})
	model.Undo = model.Undo.Open(model.now())
	model.PendingMutation = &MutationRequest{Kind: MutationUndo}
	model.MutationRunning = true
	model.Update(MutationMsg{Kind: MutationUndo})
	if model.Undo.Available(model.now()) {
		t.Fatalf("undo left the window open: %#v", model.Undo)
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

func TestDisabledRefreshIntervalInstallsNoRefreshCommand(t *testing.T) {
	settings := config.Defaults()
	settings.RefreshInterval = 0
	model := NewModel(ModelOptions{Config: settings, Client: &fakeClient{}})
	if cmd := model.Init(); cmd == nil {
		t.Fatal("initial load still required")
	}
}

func modelNow() time.Time { return time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC) }
