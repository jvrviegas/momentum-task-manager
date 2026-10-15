package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/taskevent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/storetest"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// clock is a time the test moves by hand.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) Advance(d time.Duration) { c.now = c.now.Add(d) }

type fixture struct {
	service      *Service
	client       *ent.Client
	clock        *clock
	alice, bob   context.Context
	aliceSession int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	client := storetest.Client(t)
	f := &fixture{service: New(client), client: client, clock: &clock{now: time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)}}
	f.service.now = f.clock.Now
	f.alice, f.aliceSession = signedIn(t, client, "alice@example.com")
	f.bob, _ = signedIn(t, client, "bob@example.com")
	return f
}

// signedIn returns a context acting for a new user, through a session of
// theirs, and the session's id.
func signedIn(t *testing.T, client *ent.Client, email string) (context.Context, int) {
	t.Helper()
	user := client.User.Create().SetEmail(email).SaveX(t.Context())
	session := client.Session.Create().SetUserID(user.ID).SetTokenHash([]byte(email)).SetExpiresAt(time.Now().Add(time.Hour)).SaveX(t.Context())
	ctx := viewer.NewContext(t.Context(), user.ID)
	return auth.NewContext(ctx, auth.Session{ID: session.ID, UserID: user.ID, Email: email}), session.ID
}

// insert stores a task for the user ctx acts for, entered at the fixture's
// time unless change says otherwise.
func (f *fixture) insert(t *testing.T, ctx context.Context, description string, change func(*ent.TaskCreate)) *ent.Task {
	t.Helper()
	userID, _ := viewer.UserID(ctx)
	create := f.client.Task.Create().SetUserID(userID).SetDescription(description).SetStatus(task.StatusPending).
		SetEntry(f.clock.now).SetModified(f.clock.now)
	if change != nil {
		change(create)
	}
	return create.SaveX(ctx)
}

func (f *fixture) create(t *testing.T, input api.CreateTaskRequest) api.Task {
	t.Helper()
	created, err := f.service.Create(f.alice, uuid.NewString(), input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return created
}

// events returns the task events of id, oldest first.
func (f *fixture) events(t *testing.T, ctx context.Context, id string) []*ent.TaskEvent {
	t.Helper()
	return f.client.TaskEvent.Query().Where(taskevent.TaskID(uuid.MustParse(id))).Order(taskevent.ByID()).AllX(ctx)
}

func wantError(t *testing.T, err error, code, field string) {
	t.Helper()
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Code != code || apiErr.Field != field {
		t.Errorf("error = %#v, want %s on %q", err, code, field)
	}
}

func set[T any](value T) api.Change[T] { return api.Change[T]{Kind: domain.Set, Value: value} }

func cleared[T any]() api.Change[T] { return api.Change[T]{Kind: domain.Clear} }

func at(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
	return &parsed
}

// snapshot decodes a task event's before or after state.
func snapshot(t *testing.T, data json.RawMessage) api.Task {
	t.Helper()
	var decoded api.Task
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func withoutUrgency(task api.Task) api.Task {
	task.Urgency = 0
	return task
}

func TestCreateStoresTheTaskAndItsEvent(t *testing.T) {
	f := newFixture(t)
	created := f.create(t, api.CreateTaskRequest{
		Description: "Write the plan", Project: "work.client", Priority: "H",
		Due: at("2026-10-16T17:00:00.75+01:00"), Scheduled: at("2026-10-15T08:00:00Z"),
		EstimateMinutes: 90, Tags: []string{"next", "writing", "next"},
	})

	want := api.Task{
		UUID: created.UUID, Description: "Write the plan", Status: api.StatusPending, Project: "work.client", Priority: "H",
		EstimateMinutes: 90, Entry: f.clock.now, Modified: f.clock.now,
		Due: at("2026-10-16T16:00:00Z"), Scheduled: at("2026-10-15T08:00:00Z"), Tags: []string{"next", "writing"},
		Urgency: created.Urgency,
	}
	if !reflect.DeepEqual(created, want) {
		t.Errorf("created\n%+v, want\n%+v", created, want)
	}
	// A project 1, scheduled before now 5, two tags 0.9, due in 30.5 hours
	// 12 × 0.684921, priority H 6, and the next tag 15.
	if math.Abs(created.Urgency-36.119) > 0.001 {
		t.Errorf("urgency = %v, want 36.119", created.Urgency)
	}
	listed, err := f.service.List(f.alice, api.StatusPending, time.Time{})
	if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0], created) {
		t.Errorf("listed %+v, %v; want the created task", listed, err)
	}

	events := f.events(t, f.alice, created.UUID)
	if len(events) != 1 || events[0].Action != "create" || events[0].Before != nil ||
		events[0].SessionID == nil || *events[0].SessionID != f.aliceSession || !events[0].CreatedAt.Equal(f.clock.now) {
		t.Fatalf("events = %+v", events)
	}
	if after := snapshot(t, events[0].After); !reflect.DeepEqual(after, withoutUrgency(created)) {
		t.Errorf("event after = %+v", after)
	}
}

func TestCreateRejectsInvalidFields(t *testing.T) {
	f := newFixture(t)
	valid := api.CreateTaskRequest{Description: "Call the bank"}
	for _, test := range []struct {
		name  string
		edit  func(*api.CreateTaskRequest)
		field string
	}{
		{"blank description", func(r *api.CreateTaskRequest) { r.Description = "  " }, "description"},
		{"unknown priority", func(r *api.CreateTaskRequest) { r.Priority = "high" }, "priority"},
		{"negative estimate", func(r *api.CreateTaskRequest) { r.EstimateMinutes = -5 }, "estimateMinutes"},
		{"estimate over a day", func(r *api.CreateTaskRequest) { r.EstimateMinutes = 24*60 + 1 }, "estimateMinutes"},
		{"tag with a space", func(r *api.CreateTaskRequest) { r.Tags = []string{"ok", "two words"} }, "tags"},
		{"empty tag", func(r *api.CreateTaskRequest) { r.Tags = []string{""} }, "tags"},
		{"recurrence before T11", func(r *api.CreateTaskRequest) { r.Recurrence, r.Due = "weekly", at("2026-10-16T09:00:00Z") }, "recurrence"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.edit(&request)
			_, err := f.service.Create(f.alice, "key-"+test.name, request)
			wantError(t, err, api.CodeInvalid, test.field)
		})
	}
	if count := f.client.Task.Query().CountX(f.alice); count != 0 {
		t.Errorf("%d tasks were created", count)
	}
	if _, err := f.service.Create(f.alice, "key-blank description", valid); err != nil {
		t.Errorf("a refused request's key: %v", err)
	}
	if created := f.create(t, api.CreateTaskRequest{Description: "Plan", EstimateMinutes: 24 * 60}); created.EstimateMinutes != 24*60 {
		t.Errorf("a day's estimate is refused: %+v", created)
	}
}

func TestModifyChangesOnlyTheFieldsItNames(t *testing.T) {
	f := newFixture(t)
	created := f.create(t, api.CreateTaskRequest{
		Description: "Draft", Project: "work", Due: at("2026-10-20T12:00:00Z"), EstimateMinutes: 30, Tags: []string{"a", "b"},
	})
	f.clock.Advance(time.Hour)

	modified, err := f.service.Modify(f.alice, uuid.NewString(), uuid.MustParse(created.UUID), api.ModifyTaskRequest{
		Description: set("Final"), Project: cleared[string](), Priority: set("M"), Due: cleared[time.Time](),
		Scheduled: set(time.Date(2026, 10, 18, 9, 0, 0, 999, time.FixedZone("Lisbon", 3600))), EstimateMinutes: set(45),
		Tags: &api.TagChange{Add: []string{"c", "a"}, Remove: []string{"b", "absent"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := api.Task{
		UUID: created.UUID, Description: "Final", Status: api.StatusPending, Priority: "M", EstimateMinutes: 45,
		Entry: created.Entry, Modified: f.clock.now, Scheduled: at("2026-10-18T08:00:00Z"), Tags: []string{"a", "c"},
		Urgency: modified.Urgency,
	}
	if !reflect.DeepEqual(modified, want) {
		t.Errorf("modified\n%+v, want\n%+v", modified, want)
	}

	// Fields left out stay as they are, including an estimate over the
	// creation limit, which an import can bring.
	imported := f.insert(t, f.alice, "Imported", func(c *ent.TaskCreate) { c.SetEstimateMinutes(2000).SetProject("home") })
	kept, err := f.service.Modify(f.alice, uuid.NewString(), imported.ID, api.ModifyTaskRequest{Tags: &api.TagChange{Add: []string{"x"}}})
	if err != nil || kept.EstimateMinutes != 2000 || kept.Project != "home" || kept.Description != "Imported" {
		t.Errorf("kept = %+v, %v", kept, err)
	}

	events := f.events(t, f.alice, created.UUID)
	if len(events) != 2 || events[1].Action != "modify" {
		t.Fatalf("events = %+v", events)
	}
	if before := snapshot(t, events[1].Before); !reflect.DeepEqual(before, withoutUrgency(created)) {
		t.Errorf("event before = %+v", before)
	}
	if after := snapshot(t, events[1].After); !reflect.DeepEqual(after, withoutUrgency(modified)) {
		t.Errorf("event after = %+v", after)
	}
}

func TestModifyRejectsInvalidChanges(t *testing.T) {
	f := newFixture(t)
	created := f.create(t, api.CreateTaskRequest{Description: "Draft", Tags: []string{"a"}})
	id := uuid.MustParse(created.UUID)
	for _, test := range []struct {
		name    string
		request api.ModifyTaskRequest
		field   string
	}{
		{"nothing", api.ModifyTaskRequest{}, ""},
		{"empty tag change", api.ModifyTaskRequest{Tags: &api.TagChange{}}, ""},
		{"cleared description", api.ModifyTaskRequest{Description: cleared[string]()}, "description"},
		{"blank description", api.ModifyTaskRequest{Description: set(" ")}, "description"},
		{"unknown priority", api.ModifyTaskRequest{Priority: set("low")}, "priority"},
		{"zero estimate", api.ModifyTaskRequest{EstimateMinutes: set(0)}, "estimateMinutes"},
		{"tag with a space", api.ModifyTaskRequest{Tags: &api.TagChange{Add: []string{"two words"}}}, "tags"},
		{"recurrence before T11", api.ModifyTaskRequest{Recurrence: set("weekly")}, "recurrence"},
		{"cleared recurrence before T11", api.ModifyTaskRequest{Recurrence: cleared[string]()}, "recurrence"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.service.Modify(f.alice, uuid.NewString(), id, test.request)
			wantError(t, err, api.CodeInvalid, test.field)
		})
	}
	if events := f.events(t, f.alice, created.UUID); len(events) != 1 {
		t.Errorf("%d events, want only the create", len(events))
	}
}

func TestStateChangesFollowTaskwarrior(t *testing.T) {
	f := newFixture(t)
	first := uuid.MustParse(f.create(t, api.CreateTaskRequest{Description: "First"}).UUID)
	second := uuid.MustParse(f.create(t, api.CreateTaskRequest{Description: "Second"}).UUID)
	key := uuid.NewString

	f.clock.Advance(time.Minute)
	started, err := f.service.Start(f.alice, key(), first)
	if err != nil || started.Start == nil || !started.Start.Equal(f.clock.now) || !started.Modified.Equal(f.clock.now) {
		t.Fatalf("start = %+v, %v", started, err)
	}
	_, err = f.service.Start(f.alice, key(), first)
	wantError(t, err, api.CodeConflict, "")
	stopped, err := f.service.Stop(f.alice, key(), first)
	if err != nil || stopped.Start != nil {
		t.Fatalf("stop = %+v, %v", stopped, err)
	}
	_, err = f.service.Stop(f.alice, key(), first)
	wantError(t, err, api.CodeConflict, "")

	// Completing stops the task, as Taskwarrior's done does.
	if _, err := f.service.Start(f.alice, key(), first); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	completed, err := f.service.Complete(f.alice, key(), first)
	if err != nil || completed.Status != api.StatusCompleted || completed.Start != nil || completed.End == nil || !completed.End.Equal(f.clock.now) {
		t.Fatalf("complete = %+v, %v", completed, err)
	}
	// Deleting keeps the start, as Taskwarrior's delete does.
	if _, err := f.service.Start(f.alice, key(), second); err != nil {
		t.Fatal(err)
	}
	deleted, err := f.service.Delete(f.alice, key(), second)
	if err != nil || deleted.Status != api.StatusDeleted || deleted.Start == nil || deleted.End == nil || !deleted.End.Equal(f.clock.now) {
		t.Fatalf("delete = %+v, %v", deleted, err)
	}

	// Only pending tasks change.
	for name, change := range map[string]func() error{
		"complete": func() error { _, err := f.service.Complete(f.alice, key(), first); return err },
		"delete":   func() error { _, err := f.service.Delete(f.alice, key(), first); return err },
		"start":    func() error { _, err := f.service.Start(f.alice, key(), second); return err },
		"modify": func() error {
			_, err := f.service.Modify(f.alice, key(), second, api.ModifyTaskRequest{Description: set("Again")})
			return err
		},
	} {
		if err := change(); err == nil || !strings.Contains(err.Error(), "only pending tasks can change") {
			t.Errorf("%s of a finished task: %v", name, err)
		}
	}

	var actions []string
	for _, event := range f.events(t, f.alice, first.String()) {
		actions = append(actions, event.Action)
	}
	if want := []string{"create", "start", "stop", "start", "complete"}; !reflect.DeepEqual(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}

func TestOtherUsersTasksAreNotFound(t *testing.T) {
	f := newFixture(t)
	alices := f.create(t, api.CreateTaskRequest{Description: "Alice's task"})
	id := uuid.MustParse(alices.UUID)
	for _, test := range []struct {
		ctx context.Context
		id  uuid.UUID
	}{{f.bob, id}, {f.alice, uuid.New()}} {
		for name, change := range map[string]func(context.Context, string, uuid.UUID) (api.Task, error){
			"complete": f.service.Complete, "delete": f.service.Delete, "start": f.service.Start, "stop": f.service.Stop,
			"modify": func(ctx context.Context, key string, id uuid.UUID) (api.Task, error) {
				return f.service.Modify(ctx, key, id, api.ModifyTaskRequest{Description: set("Changed by Bob")})
			},
		} {
			var apiErr *api.Error
			if _, err := change(test.ctx, uuid.NewString(), test.id); !errors.As(err, &apiErr) || apiErr.Code != api.CodeNotFound {
				t.Errorf("%s: %v, want not_found", name, err)
			}
		}
	}
	if listed, _ := f.service.List(f.bob, api.StatusPending, time.Time{}); len(listed) != 0 {
		t.Errorf("Bob lists %+v", listed)
	}
	read := f.client.Task.GetX(f.alice, id)
	if read.Description != "Alice's task" || read.Status != task.StatusPending || read.Start != nil {
		t.Errorf("Alice's task is now %+v", read)
	}
	if count := f.client.TaskEvent.Query().CountX(f.alice); count != 1 {
		t.Errorf("Alice has %d task events, want 1", count)
	}
}

func TestRetriesWithTheSameKeyApplyOnce(t *testing.T) {
	f := newFixture(t)
	input := api.CreateTaskRequest{Description: "Pay rent", Tags: []string{"home"}}
	first, err := f.service.Create(f.alice, "retry", input)
	if err != nil {
		t.Fatal(err)
	}
	// A retry answers as the first attempt did, even after the task changed.
	f.clock.Advance(time.Minute)
	if _, err := f.service.Start(f.alice, "start", uuid.MustParse(first.UUID)); err != nil {
		t.Fatal(err)
	}
	again, err := f.service.Create(f.alice, "retry", input)
	if err != nil || !reflect.DeepEqual(again, first) {
		t.Errorf("retry = %+v, %v; want %+v", again, err, first)
	}
	if again, err := f.service.Start(f.alice, "start", uuid.MustParse(first.UUID)); err != nil || again.Start == nil {
		t.Errorf("retried start = %+v, %v", again, err)
	}
	if count := f.client.Task.Query().CountX(f.alice); count != 1 {
		t.Errorf("%d tasks, want 1", count)
	}
	if count := f.client.TaskEvent.Query().CountX(f.alice); count != 2 {
		t.Errorf("%d events, want 2", count)
	}

	// The key cannot be reused for another request, on this route or another.
	_, err = f.service.Create(f.alice, "retry", api.CreateTaskRequest{Description: "Pay rent", Tags: []string{"bills"}})
	wantError(t, err, api.CodeConflict, "")
	_, err = f.service.Complete(f.alice, "retry", uuid.MustParse(first.UUID))
	wantError(t, err, api.CodeConflict, "")

	// Keys belong to a user.
	if bobs, err := f.service.Create(f.bob, "retry", input); err != nil || bobs.UUID == first.UUID {
		t.Errorf("Bob's create with Alice's key = %+v, %v", bobs, err)
	}

	// A failed mutation claims nothing.
	_, err = f.service.Start(f.alice, "failed", uuid.MustParse(first.UUID))
	wantError(t, err, api.CodeConflict, "")
	if _, err := f.service.Stop(f.alice, "failed", uuid.MustParse(first.UUID)); err != nil {
		t.Errorf("a failed request's key: %v", err)
	}

	// A key is forgotten after a day.
	f.clock.Advance(keyLifetime + time.Second)
	if later, err := f.service.Create(f.alice, "retry", input); err != nil || later.UUID == first.UUID {
		t.Errorf("create a day later = %+v, %v; want a new task", later, err)
	}

	for _, key := range []string{"", strings.Repeat("k", maxKeyLength+1)} {
		_, err := f.service.Create(f.alice, key, input)
		wantError(t, err, api.CodeInvalid, "")
	}
	if _, err := f.service.Create(f.alice, strings.Repeat("k", maxKeyLength), input); err != nil {
		t.Errorf("the longest key: %v", err)
	}
}

func TestConcurrentRetriesApplyOnce(t *testing.T) {
	f := newFixture(t)
	answers := make([]api.Task, 8)
	errs := make([]error, len(answers))
	var wait sync.WaitGroup
	for i := range answers {
		wait.Go(func() {
			answers[i], errs[i] = f.service.Create(f.alice, "same", api.CreateTaskRequest{Description: "Once"})
		})
	}
	wait.Wait()
	for i := range answers {
		if errs[i] != nil || !reflect.DeepEqual(answers[i], answers[0]) {
			t.Errorf("attempt %d = %+v, %v; want %+v", i, answers[i], errs[i], answers[0])
		}
	}
	if count := f.client.Task.Query().CountX(f.alice); count != 1 {
		t.Errorf("%d tasks, want 1", count)
	}
}

func TestConcurrentChangesToATaskAreAllKept(t *testing.T) {
	f := newFixture(t)
	id := uuid.MustParse(f.create(t, api.CreateTaskRequest{Description: "Shared"}).UUID)
	var wait sync.WaitGroup
	tags := []string{"t0", "t1", "t2", "t3", "t4", "t5", "t6", "t7"}
	for _, tag := range tags {
		wait.Go(func() {
			if _, err := f.service.Modify(f.alice, uuid.NewString(), id, api.ModifyTaskRequest{Tags: &api.TagChange{Add: []string{tag}}}); err != nil {
				t.Errorf("add %s: %v", tag, err)
			}
		})
	}
	wait.Wait()
	got := slices.Sorted(slices.Values(f.client.Task.GetX(f.alice, id).Tags))
	if !reflect.DeepEqual(got, tags) {
		t.Errorf("tags = %v, want all of %v", got, tags)
	}
	if count := len(f.events(t, f.alice, id.String())); count != len(tags)+1 {
		t.Errorf("%d events, want %d", count, len(tags)+1)
	}
}

func TestListByStatus(t *testing.T) {
	f := newFixture(t)
	now := f.clock.now
	entered := func(minutes int) func(*ent.TaskCreate) {
		return func(c *ent.TaskCreate) { c.SetEntry(now.Add(time.Duration(minutes) * time.Minute)) }
	}
	f.insert(t, f.alice, "Plain", entered(1))
	waitingTask := f.insert(t, f.alice, "Waiting", func(c *ent.TaskCreate) { entered(2)(c); c.SetWait(now.Add(24 * time.Hour)) })
	done := f.insert(t, f.alice, "Done", func(c *ent.TaskCreate) {
		entered(3)(c)
		c.SetStatus(task.StatusCompleted).SetEnd(now.Add(-time.Hour))
	})
	f.insert(t, f.alice, "Blocked", func(c *ent.TaskCreate) { entered(4)(c); c.SetDepends([]string{waitingTask.ID.String()}) })
	f.insert(t, f.alice, "Free", func(c *ent.TaskCreate) { entered(5)(c); c.SetDepends([]string{done.ID.String()}) })
	f.insert(t, f.alice, "Old", func(c *ent.TaskCreate) { c.SetStatus(task.StatusCompleted).SetEnd(now.Add(-72 * time.Hour)) })
	f.insert(t, f.alice, "Gone", func(c *ent.TaskCreate) { c.SetStatus(task.StatusDeleted).SetEnd(now) })
	f.insert(t, f.alice, "Series", func(c *ent.TaskCreate) {
		c.SetStatus(task.StatusRecurring).SetRecurrence("weekly").SetDue(now.Add(time.Hour))
	})
	f.insert(t, f.bob, "Bob's", nil)

	type row struct {
		description string
		urgency     float64
	}
	list := func(ctx context.Context, status string, endedAfter time.Time) []row {
		t.Helper()
		listed, err := f.service.List(ctx, status, endedAfter)
		if err != nil {
			t.Fatalf("List(%s): %v", status, err)
		}
		rows := []row{}
		for _, task := range listed {
			rows = append(rows, row{task.Description, task.Urgency})
		}
		return rows
	}
	// The waiting task is left out, but still blocks.
	if got, want := list(f.alice, api.StatusPending, time.Time{}), []row{{"Plain", 0}, {"Blocked", -5}, {"Free", 0}}; !reflect.DeepEqual(got, want) {
		t.Errorf("pending = %v, want %v", got, want)
	}
	if got := list(f.alice, api.StatusCompleted, now.Add(-2*time.Hour)); !reflect.DeepEqual(got, []row{{"Done", 0}}) {
		t.Errorf("completed = %v", got)
	}
	if got := list(f.alice, api.StatusCompleted, *done.End); len(got) != 0 {
		t.Errorf("completed after its end = %v", got)
	}
	if got := list(f.alice, api.StatusRecurring, time.Time{}); len(got) != 1 || got[0].description != "Series" {
		t.Errorf("recurring = %v", got)
	}
	if got := list(f.bob, api.StatusPending, time.Time{}); !reflect.DeepEqual(got, []row{{"Bob's", 0}}) {
		t.Errorf("Bob's pending = %v", got)
	}

	for _, status := range []string{"", "waiting", "deleted"} {
		_, err := f.service.List(f.alice, status, time.Time{})
		wantError(t, err, api.CodeInvalid, api.QueryStatus)
	}
	_, err := f.service.List(f.alice, api.StatusCompleted, time.Time{})
	wantError(t, err, api.CodeInvalid, api.QueryEndedAfter)
}

func TestProjectsAndTagsComeFromPendingTasks(t *testing.T) {
	f := newFixture(t)
	if projects, err := f.service.Projects(f.alice); err != nil || projects == nil || len(projects) != 0 {
		t.Errorf("projects without tasks = %#v, %v", projects, err)
	}
	f.insert(t, f.alice, "One", func(c *ent.TaskCreate) { c.SetProject("work.client").SetTags([]string{"next", "call"}) })
	f.insert(t, f.alice, "Two", func(c *ent.TaskCreate) { c.SetProject("home").SetTags([]string{"call"}) })
	f.insert(t, f.alice, "Three", func(c *ent.TaskCreate) { c.SetProject("work.client") })
	f.insert(t, f.alice, "Waiting", func(c *ent.TaskCreate) {
		c.SetProject("later").SetTags([]string{"someday"}).SetWait(f.clock.now.Add(time.Hour))
	})
	f.insert(t, f.alice, "Done", func(c *ent.TaskCreate) {
		c.SetProject("old").SetTags([]string{"done"}).SetStatus(task.StatusCompleted).SetEnd(f.clock.now)
	})
	f.insert(t, f.bob, "Bob's", func(c *ent.TaskCreate) { c.SetProject("bob").SetTags([]string{"bob"}) })

	projects, err := f.service.Projects(f.alice)
	if want := []string{"home", "work.client"}; err != nil || !reflect.DeepEqual(projects, want) {
		t.Errorf("projects = %v, %v; want %v", projects, err, want)
	}
	tags, err := f.service.Tags(f.alice)
	if want := []string{"call", "next"}; err != nil || !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %v, %v; want %v", tags, err, want)
	}
}
