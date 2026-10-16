package backend

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/app"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/bridge"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/taskwarrior"
)

// The interfaces the TUI and the bridge reach storage through.
var (
	_ taskwarrior.Client          = (*Client)(nil)
	_ bridge.Client               = (*Client)(nil)
	_ app.ProjectMigrationClient  = (*Client)(nil)
	_ app.ProjectMigrationApplier = (*Client)(nil)
	_ interface {
		ExportCompleted(context.Context, time.Time) ([]domain.Task, error)
		StopRecurrence(context.Context, string) error
		Tags(context.Context) ([]string, error)
		SyncConfigured(context.Context) (bool, error)
	} = (*Client)(nil)
)

type request struct{ pattern, query, body string }

// fakeServer answers each core/api route with the answer listed for its
// pattern, or an empty task, and records the requests it receives.
type fakeServer struct {
	mu       sync.Mutex
	requests []request
	answers  map[string]string
	statuses map[string]int
}

func newFakeServer(t *testing.T, zone string) (*Client, *fakeServer) {
	t.Helper()
	fake := &fakeServer{answers: map[string]string{}, statuses: map[string]int{}}
	mux := http.NewServeMux()
	for _, route := range []string{
		api.RouteVersion, api.RouteListTasks, api.RouteCreateTask, api.RouteModifyTask, api.RouteDeleteTask,
		api.RouteCompleteTask, api.RouteStartTask, api.RouteStopTask, api.RouteStopRecurrence, api.RouteUndo,
		api.RouteListProjects, api.RouteListTags, api.RouteSetTimeZone, api.RouteApplyProjectMigration,
	} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			fake.mu.Lock()
			defer fake.mu.Unlock()
			fake.requests = append(fake.requests, request{r.Pattern, r.URL.RawQuery, string(body)})
			answer, ok := fake.answers[r.Pattern]
			if !ok {
				answer = `{"uuid":"u","description":"d","status":"pending"}`
			}
			status := fake.statuses[r.Pattern]
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			io.WriteString(w, answer)
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return New(server.URL, "mtk_device", zone), fake
}

func (f *fakeServer) answer(pattern string, status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers[pattern], f.statuses[pattern] = body, status
}

func (f *fakeServer) seen() []request {
	f.mu.Lock()
	defer f.mu.Unlock()
	requests := f.requests
	f.requests = nil
	return requests
}

func (f *fakeServer) patterns() []string {
	var patterns []string
	for _, r := range f.seen() {
		patterns = append(patterns, r.pattern)
	}
	return patterns
}

func ptr[T any](value T) *T { return &value }

func TestExportsConvertServerTasks(t *testing.T) {
	c, fake := newFakeServer(t, "")
	at := func(hour int) time.Time { return time.Date(2026, 10, 14, hour, 30, 0, 0, time.UTC) }
	full := api.Task{
		UUID: "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10", Description: "Write the plan", Status: api.StatusPending,
		Project: "work.client", Priority: "H", EstimateMinutes: 90, Entry: at(8), Modified: at(9),
		Due: ptr(at(17)), Scheduled: ptr(at(10)), Start: ptr(at(11)), Wait: ptr(at(7)), Until: ptr(at(23)), End: ptr(at(12)),
		Tags: []string{"next"}, Annotations: []api.Annotation{{Entry: at(9), Description: "Outline agreed"}},
		Depends: []string{"0d1c6c58-8d4b-4a8e-9f4e-1b2b3c4d5e6f"}, Recurrence: "weekly", Parent: "a3f1c2d4-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
		Urgency: 50.2215, Extra: map[string]json.RawMessage{"imask": json.RawMessage(`3`), "jira": json.RawMessage(`"MOM-1"`)},
	}
	data, _ := json.Marshal(api.TaskList{Tasks: []api.Task{full, {UUID: "bare", Description: "Bare", Status: api.StatusPending}}})
	fake.answer(api.RouteListTasks, http.StatusOK, string(data))

	tasks, err := c.ExportPending(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Task{{
		UUID: full.UUID, Description: "Write the plan", Status: "pending", Project: "work.client", Priority: "H",
		Estimate: &domain.Estimate{Minutes: 90}, Entry: ptr(at(8)), Modified: ptr(at(9)),
		Due: ptr(at(17)), Scheduled: ptr(at(10)), Start: ptr(at(11)), Wait: ptr(at(7)), Until: ptr(at(23)), End: ptr(at(12)),
		Tags: []string{"next"}, Annotations: []domain.Annotation{{Description: "Outline agreed", Entry: ptr(at(9))}},
		Dependencies: full.Depends, Recurrence: "weekly", Parent: full.Parent, Urgency: 50.2215, RawFields: full.Extra,
	}, {UUID: "bare", Description: "Bare", Status: "pending"}}
	if !reflect.DeepEqual(tasks, want) {
		t.Errorf("tasks:\n got %#v\nwant %#v", tasks, want)
	}

	lisbon, _ := time.LoadLocation("Europe/Lisbon")
	if _, err := c.ExportCompleted(t.Context(), time.Date(2026, 9, 14, 0, 0, 0, 0, lisbon)); err != nil {
		t.Fatal(err)
	}
	if got := fake.seen(); len(got) != 2 || got[0].query != "status=pending" || got[1].query != "ended_after=2026-09-13T23%3A00%3A00Z&status=completed" {
		t.Errorf("requests %+v", got)
	}
}

func TestAddResolvesDatesInTheDeviceZone(t *testing.T) {
	c, fake := newFakeServer(t, "")
	lisbon, _ := time.LoadLocation("Europe/Lisbon")
	c.now = func() time.Time { return time.Date(2026, 10, 24, 15, 0, 0, 0, lisbon) } // the day before DST ends
	err := c.Add(t.Context(), domain.NewTask{
		Description: "Water plants", Project: "home", Priority: "L", Due: "tomorrow", Scheduled: "2026-10-26 09:30",
		Recurrence: "weekly", Estimate: &domain.Estimate{Minutes: 15}, Tags: []string{"chores", "garden"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"description":"Water plants","project":"home","priority":"L","due":"2026-10-24T23:00:00Z",` +
		`"scheduled":"2026-10-26T09:30:00Z","recurrence":"weekly","estimateMinutes":15,"tags":["chores","garden"]}`
	if got := fake.seen(); len(got) != 1 || got[0].pattern != api.RouteCreateTask || got[0].body != want {
		t.Errorf("requests %+v\nwant body %s", got, want)
	}

	if err := c.Add(t.Context(), domain.NewTask{Description: "Call Ana"}); err != nil {
		t.Fatal(err)
	}
	if got := fake.seen(); len(got) != 1 || got[0].body != `{"description":"Call Ana"}` {
		t.Errorf("requests %+v", got)
	}
}

func TestModifySendsTheDiff(t *testing.T) {
	c, fake := newFakeServer(t, "")
	lisbon, _ := time.LoadLocation("Europe/Lisbon")
	c.now = func() time.Time { return time.Date(2026, 10, 16, 15, 0, 0, 0, lisbon) } // a Friday
	err := c.Modify(t.Context(), "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10", domain.TaskDiff{
		Description: domain.FieldChange{Kind: domain.Set, Value: "Renamed"},
		Project:     domain.FieldChange{Kind: domain.Clear},
		Priority:    domain.FieldChange{Kind: domain.Set, Value: "M"},
		Due:         domain.FieldChange{Kind: domain.Set, Value: "next-week"},
		Scheduled:   domain.FieldChange{Kind: domain.Clear},
		Recurrence:  domain.FieldChange{Kind: domain.Set, Value: "2w"},
		Estimate:    domain.EstimateChange{Kind: domain.Set, Value: &domain.Estimate{Minutes: 45}},
		Tags:        domain.TagChange{Changed: true, Add: []string{"next"}, Remove: []string{"later"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"description":"Renamed","project":null,"priority":"M","due":"2026-10-22T23:00:00Z","scheduled":null,` +
		`"recurrence":"2w","estimateMinutes":45,"tags":{"add":["next"],"remove":["later"]}}`
	if got := fake.seen(); len(got) != 1 || got[0].pattern != api.RouteModifyTask || got[0].body != want {
		t.Errorf("requests %+v\nwant body %s", got, want)
	}

	err = c.Modify(t.Context(), "u", domain.TaskDiff{Estimate: domain.EstimateChange{Kind: domain.Clear}, Tags: domain.TagChange{Add: []string{"ignored"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.seen(); len(got) != 1 || got[0].body != `{"estimateMinutes":null}` {
		t.Errorf("requests %+v", got)
	}
}

func TestUnsupportedDatesAreRefusedBeforeSending(t *testing.T) {
	c, fake := newFakeServer(t, "Europe/Lisbon")
	for _, test := range []struct {
		send  func() error
		field string
	}{
		{func() error { return c.Add(t.Context(), domain.NewTask{Description: "Hunt eggs", Due: "easter"}) }, "due"},
		{func() error { return c.Add(t.Context(), domain.NewTask{Description: "Plan", Scheduled: "eom-1d"}) }, "scheduled"},
		{func() error {
			return c.Modify(t.Context(), "u", domain.TaskDiff{Due: domain.FieldChange{Kind: domain.Set, Value: "3d"}})
		}, "due"},
		{func() error {
			return c.Modify(t.Context(), "u", domain.TaskDiff{Scheduled: domain.FieldChange{Kind: domain.Set, Value: "tom"}})
		}, "scheduled"},
	} {
		var failure *api.Error
		if err := test.send(); !errors.As(err, &failure) || failure.Code != api.CodeInvalid || failure.Field != test.field {
			t.Errorf("error = %#v, want invalid on %s", err, test.field)
		}
	}
	if got := fake.seen(); len(got) != 0 {
		t.Errorf("sent %+v", got)
	}
}

func TestEachChangeReachesItsRoute(t *testing.T) {
	c, fake := newFakeServer(t, "")
	ctx := t.Context()
	for _, send := range []func() error{
		func() error { return c.Complete(ctx, "u") }, func() error { return c.Delete(ctx, "u") },
		func() error { return c.Start(ctx, "u") }, func() error { return c.Stop(ctx, "u") },
		func() error { return c.StopRecurrence(ctx, "u") }, func() error { return c.Undo(ctx) },
	} {
		if err := send(); err != nil {
			t.Fatal(err)
		}
	}
	fake.answer(api.RouteListProjects, http.StatusOK, `{"projects":["home"]}`)
	fake.answer(api.RouteListTags, http.StatusOK, `{"tags":["next"]}`)
	fake.answer(api.RouteVersion, http.StatusOK, `{"server":"v1.2.3"}`)
	projects, _ := c.Projects(ctx)
	tags, _ := c.Tags(ctx)
	version, _ := c.Version(ctx)
	if !reflect.DeepEqual(projects, []string{"home"}) || !reflect.DeepEqual(tags, []string{"next"}) || version != "v1.2.3" {
		t.Errorf("projects %v, tags %v, version %q", projects, tags, version)
	}
	want := []string{api.RouteCompleteTask, api.RouteDeleteTask, api.RouteStartTask, api.RouteStopTask, api.RouteStopRecurrence,
		api.RouteUndo, api.RouteListProjects, api.RouteListTags, api.RouteVersion}
	if got := fake.patterns(); !reflect.DeepEqual(got, want) {
		t.Errorf("routes %v, want %v", got, want)
	}

	fake.answer(api.RouteUndo, http.StatusConflict, `{"code":"conflict","message":"the task changed since; undo is not available"}`)
	var failure *api.Error
	if err := c.Undo(ctx); !errors.As(err, &failure) || failure.Code != api.CodeConflict {
		t.Errorf("undo error = %#v", err)
	}
}

func TestProjectMigrationAppliesInOneRequest(t *testing.T) {
	c, fake := newFakeServer(t, "")
	ctx := t.Context()
	fake.answer(api.RouteListTasks, http.StatusOK, `{"tasks":[{"uuid":"a","description":"Plan","status":"pending","project":"work"}]}`)
	export, err := c.ExportPendingInContext(ctx)
	if err != nil || export.Scope != (taskwarrior.ProjectMigrationScope{}) || len(export.Tasks) != 1 || export.Tasks[0].Project != "work" {
		t.Fatalf("export %+v, %v", export, err)
	}

	plan := domain.ProjectCatalogPlan{SourceValue: "work", DestinationValue: "job", IncludeSubprojects: true}
	tasks := []domain.ProjectTaskMapping{{UUID: "a", OldValue: "work", NewValue: "job"}, {UUID: "b", OldValue: "work.client", NewValue: "job.client"}}
	fake.answer(api.RouteApplyProjectMigration, http.StatusOK, `{"changed":2}`)
	if err := c.ApplyProjectMigration(ctx, plan, tasks); err != nil {
		t.Fatal(err)
	}
	want := `{"source":"work","destination":"job","includeSubprojects":true,"tasks":[{"uuid":"a","oldValue":"work","newValue":"job"},{"uuid":"b","oldValue":"work.client","newValue":"job.client"}]}`
	if got := fake.seen(); len(got) != 2 || got[1].pattern != api.RouteApplyProjectMigration || got[1].body != want {
		t.Errorf("requests %+v", got)
	}

	fake.answer(api.RouteApplyProjectMigration, http.StatusConflict, `{"code":"conflict","message":"the tasks changed since the preview"}`)
	var failure *api.Error
	if err := c.ApplyProjectMigration(ctx, plan, tasks); !errors.As(err, &failure) || failure.Code != api.CodeConflict {
		t.Errorf("stale apply error = %#v", err)
	}
}

func TestTheTimeZoneIsSetBeforeTheFirstChange(t *testing.T) {
	c, fake := newFakeServer(t, "Europe/Lisbon")
	ctx := t.Context()
	c.ExportPending(ctx)
	c.Projects(ctx)
	if got := fake.patterns(); !reflect.DeepEqual(got, []string{api.RouteListTasks, api.RouteListProjects}) {
		t.Errorf("reads sent %v", got)
	}

	fake.answer(api.RouteSetTimeZone, http.StatusInternalServerError, `{"code":"internal","message":"the server failed; try again"}`)
	if err := c.Complete(ctx, "u"); err == nil {
		t.Error("complete succeeded without the time zone")
	}
	if got := fake.patterns(); !reflect.DeepEqual(got, []string{api.RouteSetTimeZone}) {
		t.Errorf("failed zone sent %v", got)
	}

	fake.answer(api.RouteSetTimeZone, http.StatusOK, `{"timeZone":"Europe/Lisbon"}`)
	c.Complete(ctx, "u")
	c.Start(ctx, "u")
	got := fake.seen()
	if len(got) != 3 || got[0].pattern != api.RouteSetTimeZone || got[0].body != `{"timeZone":"Europe/Lisbon"}` ||
		got[1].pattern != api.RouteCompleteTask || got[2].pattern != api.RouteStartTask {
		t.Errorf("requests %+v", got)
	}

	unknown, fake := newFakeServer(t, "")
	unknown.Complete(ctx, "u")
	if got := fake.patterns(); !reflect.DeepEqual(got, []string{api.RouteCompleteTask}) {
		t.Errorf("without a zone sent %v", got)
	}
}

func TestTaskwarriorOnlyCallsAnswerLocally(t *testing.T) {
	c, fake := newFakeServer(t, "Europe/Lisbon")
	ctx := t.Context()
	synced, syncErr := c.Sync(ctx)
	configured, configuredErr := c.SyncConfigured(ctx)
	name, contextErr := c.Context(ctx)
	estimates, estimatesErr := c.EstimateUDAReadiness(ctx)
	if synced != (taskwarrior.SyncResult{}) || configured || name != "" || estimates != taskwarrior.EstimateUDAConfigured ||
		syncErr != nil || configuredErr != nil || contextErr != nil || estimatesErr != nil {
		t.Errorf("sync %+v %v, configured %v %v, context %q %v, estimates %q %v",
			synced, syncErr, configured, configuredErr, name, contextErr, estimates, estimatesErr)
	}
	if got := fake.seen(); len(got) != 0 {
		t.Errorf("sent %+v", got)
	}
}
