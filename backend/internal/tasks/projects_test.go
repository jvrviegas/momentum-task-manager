package tasks

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/hook"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/taskevent"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

var workToOffice = api.ProjectMigration{Source: "work", Destination: "office", IncludeSubprojects: true}

// project inserts a pending task of alice's in project, entered a minute
// after the one before, so a preview lists them in order.
func (f *fixture) project(t *testing.T, project string, change func(*ent.TaskCreate)) *ent.Task {
	t.Helper()
	f.clock.Advance(time.Minute)
	return f.insert(t, f.alice, "Task in "+project, func(create *ent.TaskCreate) {
		create.SetProject(project)
		if change != nil {
			change(create)
		}
	})
}

func (f *fixture) preview(t *testing.T, ctx context.Context, migration api.ProjectMigration) []api.ProjectTaskMapping {
	t.Helper()
	preview, err := f.service.PreviewProjectMigration(ctx, migration)
	if err != nil {
		t.Fatalf("PreviewProjectMigration: %v", err)
	}
	return preview.Tasks
}

func (f *fixture) apply(ctx context.Context, key string, migration api.ProjectMigration, mappings []api.ProjectTaskMapping) (api.ProjectMigrationResult, error) {
	return f.service.ApplyProjectMigration(ctx, key, api.ApplyProjectMigrationRequest{ProjectMigration: migration, Tasks: mappings})
}

// migrationEvents counts the task events of project migrations.
func (f *fixture) migrationEvents(t *testing.T) int {
	t.Helper()
	return f.client.TaskEvent.Query().Where(taskevent.Action(migrateAction)).CountX(f.alice)
}

// storedTasks returns alice's tasks as they are stored, by id.
func (f *fixture) storedTasks(t *testing.T) map[uuid.UUID]api.Task {
	t.Helper()
	stored := make(map[uuid.UUID]api.Task)
	for _, row := range f.client.Task.Query().AllX(f.alice) {
		stored[row.ID] = toAPI(row)
	}
	return stored
}

func mapping(row *ent.Task, newValue string) api.ProjectTaskMapping {
	return api.ProjectTaskMapping{UUID: row.ID.String(), OldValue: row.Project, NewValue: newValue}
}

func TestPreviewListsThePendingTasksAMigrationMoves(t *testing.T) {
	f := newFixture(t)
	exact := f.project(t, "work", nil)
	child := f.project(t, "work.client", nil)
	grandchild := f.project(t, "work.client.site", nil)
	f.project(t, "workshop", nil)
	f.project(t, "home.work", nil)
	f.project(t, "", nil)
	waiting := f.project(t, "work", func(create *ent.TaskCreate) { create.SetWait(f.clock.now.Add(time.Hour)) })
	waited := f.project(t, "work", func(create *ent.TaskCreate) { create.SetWait(f.clock.now.Add(-time.Hour)) })
	f.project(t, "work", func(create *ent.TaskCreate) { create.SetStatus(task.StatusCompleted).SetEnd(f.clock.now) })
	f.project(t, "work.client", func(create *ent.TaskCreate) { create.SetStatus(task.StatusDeleted).SetEnd(f.clock.now) })
	f.create(t, api.CreateTaskRequest{Description: "Weekly review", Project: "work", Recurrence: "weekly", Due: at("2026-10-16T09:00:00Z")})
	f.insert(t, f.bob, "Bob's work", func(create *ent.TaskCreate) { create.SetProject("work") })
	events := f.client.TaskEvent.Query().CountX(f.alice)

	if got, want := f.preview(t, f.alice, workToOffice), []api.ProjectTaskMapping{
		mapping(exact, "office"), mapping(child, "office.client"), mapping(grandchild, "office.client.site"), mapping(waited, "office"),
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("with subprojects\n%+v, want\n%+v", got, want)
	}
	exactOnly := api.ProjectMigration{Source: "work", Destination: "office"}
	if got, want := f.preview(t, f.alice, exactOnly), []api.ProjectTaskMapping{mapping(exact, "office"), mapping(waited, "office")}; !reflect.DeepEqual(got, want) {
		t.Errorf("without subprojects\n%+v, want\n%+v", got, want)
	}
	subproject := api.ProjectMigration{Source: "work.client", Destination: "clients", IncludeSubprojects: true}
	if got, want := f.preview(t, f.alice, subproject), []api.ProjectTaskMapping{mapping(child, "clients"), mapping(grandchild, "clients.site")}; !reflect.DeepEqual(got, want) {
		t.Errorf("from a subproject\n%+v, want\n%+v", got, want)
	}

	// A waiting task joins once its wait date passes.
	f.clock.now = *waiting.Wait
	if got := f.preview(t, f.alice, exactOnly); len(got) != 3 || got[1] != mapping(waiting, "office") {
		t.Errorf("after the wait date = %+v", got)
	}
	if got := f.preview(t, f.alice, api.ProjectMigration{Source: "nothing", Destination: "office"}); got == nil || len(got) != 0 {
		t.Errorf("no tasks = %#v, want an empty list", got)
	}
	if got := f.preview(t, f.bob, workToOffice); len(got) != 1 || got[0].NewValue != "office" {
		t.Errorf("Bob's preview = %+v", got)
	}
	if count := f.client.TaskEvent.Query().CountX(f.alice); count != events {
		t.Errorf("previews wrote %d events", count-events)
	}
}

func TestMigrationsRefuseInvalidProjects(t *testing.T) {
	f := newFixture(t)
	for _, test := range []struct {
		migration api.ProjectMigration
		field     string
	}{
		{api.ProjectMigration{Destination: "office"}, "source"},
		{api.ProjectMigration{Source: "work"}, "destination"},
		{api.ProjectMigration{Source: "work", Destination: "work", IncludeSubprojects: true}, "destination"},
	} {
		_, err := f.service.PreviewProjectMigration(f.alice, test.migration)
		wantError(t, err, api.CodeInvalid, test.field)
		_, err = f.apply(f.alice, uuid.NewString(), test.migration, nil)
		wantError(t, err, api.CodeInvalid, test.field)
	}
	_, err := f.apply(f.alice, "", workToOffice, nil)
	wantError(t, err, api.CodeInvalid, "")
}

func TestApplyMovesThePreviewedTasksInOneChange(t *testing.T) {
	f := newFixture(t)
	exact := f.project(t, "work", func(create *ent.TaskCreate) { create.SetPriority("H").SetTags([]string{"next"}) })
	child := f.project(t, "work.client", nil)
	others := []*ent.Task{
		f.project(t, "workshop", nil),
		f.project(t, "work", func(create *ent.TaskCreate) { create.SetWait(f.clock.now.Add(time.Hour)) }),
		f.project(t, "work", func(create *ent.TaskCreate) { create.SetStatus(task.StatusCompleted).SetEnd(f.clock.now) }),
	}
	before := f.storedTasks(t)
	mappings := f.preview(t, f.alice, workToOffice)
	f.clock.Advance(time.Minute)

	result, err := f.apply(f.alice, "migrate", workToOffice, mappings)
	if err != nil || result.Changed != 2 {
		t.Fatalf("apply = %+v, %v; want 2 changed", result, err)
	}
	var change uuid.UUID
	for _, test := range []struct {
		row     *ent.Task
		project string
	}{{exact, "office"}, {child, "office.client"}} {
		before := before[test.row.ID]
		want := before
		want.Project, want.Modified = test.project, f.clock.now
		if stored := f.stored(t, test.row.ID); !reflect.DeepEqual(stored, want) {
			t.Errorf("stored\n%+v, want\n%+v", stored, want)
		}
		events := f.events(t, f.alice, test.row.ID.String())
		if len(events) != 1 || events[0].Action != migrateAction || events[0].SessionID == nil || *events[0].SessionID != f.aliceSession ||
			!reflect.DeepEqual(snapshot(t, events[0].Before), before) || !reflect.DeepEqual(snapshot(t, events[0].After), want) {
			t.Fatalf("events of %s = %+v", test.project, events)
		}
		if change == uuid.Nil {
			change = events[0].ChangeID
		} else if events[0].ChangeID != change {
			t.Errorf("the migration's events have changes %s and %s", change, events[0].ChangeID)
		}
	}
	for _, other := range others {
		if stored := f.stored(t, other.ID); !reflect.DeepEqual(stored, before[other.ID]) {
			t.Errorf("a task the migration leaves out changed to %+v", stored)
		}
	}
	if got := f.preview(t, f.alice, workToOffice); len(got) != 0 {
		t.Errorf("preview after the migration = %+v", got)
	}

	// A retry answers as the first attempt did; applying again would be
	// stale. The key cannot be reused for another request.
	if again, err := f.apply(f.alice, "migrate", workToOffice, mappings); err != nil || again != result {
		t.Errorf("retry = %+v, %v; want %+v", again, err, result)
	}
	_, err = f.apply(f.alice, uuid.NewString(), workToOffice, mappings)
	wantError(t, err, api.CodeConflict, "")
	_, err = f.apply(f.alice, "migrate", workToOffice, nil)
	wantError(t, err, api.CodeConflict, "")
	if count := f.migrationEvents(t); count != 2 {
		t.Errorf("%d migration events, want 2", count)
	}

	// Nothing to move is a migration that changes nothing.
	if result, err := f.apply(f.alice, uuid.NewString(), workToOffice, []api.ProjectTaskMapping{}); err != nil || result.Changed != 0 {
		t.Errorf("an empty migration = %+v, %v", result, err)
	}
}

func TestApplyRefusesAStalePreview(t *testing.T) {
	for _, test := range []struct {
		name string
		// stale changes the tasks, or the mappings sent, after the preview.
		stale func(t *testing.T, f *fixture, first *ent.Task, mappings []api.ProjectTaskMapping) []api.ProjectTaskMapping
	}{
		{"a task joins", func(t *testing.T, f *fixture, _ *ent.Task, mappings []api.ProjectTaskMapping) []api.ProjectTaskMapping {
			f.project(t, "work.new", nil)
			return mappings
		}},
		{"a task completes", func(t *testing.T, f *fixture, first *ent.Task, mappings []api.ProjectTaskMapping) []api.ProjectTaskMapping {
			if _, err := f.service.Complete(f.alice, uuid.NewString(), first.ID); err != nil {
				t.Fatal(err)
			}
			return mappings
		}},
		{"a task moves", func(t *testing.T, f *fixture, first *ent.Task, mappings []api.ProjectTaskMapping) []api.ProjectTaskMapping {
			if _, err := f.service.Modify(f.alice, uuid.NewString(), first.ID, api.ModifyTaskRequest{Project: set("home")}); err != nil {
				t.Fatal(err)
			}
			return mappings
		}},
		{"a waiting task wakes", func(_ *testing.T, f *fixture, _ *ent.Task, mappings []api.ProjectTaskMapping) []api.ProjectTaskMapping {
			f.clock.Advance(2 * time.Hour)
			return mappings
		}},
		{"a mapping differs", func(_ *testing.T, _ *fixture, _ *ent.Task, mappings []api.ProjectTaskMapping) []api.ProjectTaskMapping {
			mappings[1].NewValue = "elsewhere"
			return mappings
		}},
		{"a mapping repeats", func(_ *testing.T, _ *fixture, _ *ent.Task, mappings []api.ProjectTaskMapping) []api.ProjectTaskMapping {
			return []api.ProjectTaskMapping{mappings[0], mappings[0]}
		}},
		{"a mapping is missing", func(_ *testing.T, _ *fixture, _ *ent.Task, mappings []api.ProjectTaskMapping) []api.ProjectTaskMapping {
			return mappings[1:]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			first := f.project(t, "work", nil)
			f.project(t, "work.client", nil)
			f.project(t, "work", func(create *ent.TaskCreate) { create.SetWait(f.clock.now.Add(time.Hour)) })
			mappings := test.stale(t, f, first, f.preview(t, f.alice, workToOffice))
			projects := f.client.Task.Query().Order(task.ByID()).Select(task.FieldProject).StringsX(f.alice)

			_, err := f.apply(f.alice, "migrate", workToOffice, mappings)
			wantError(t, err, api.CodeConflict, "")
			if after := f.client.Task.Query().Order(task.ByID()).Select(task.FieldProject).StringsX(f.alice); !reflect.DeepEqual(after, projects) {
				t.Errorf("projects = %v, want %v", after, projects)
			}
			if count := f.migrationEvents(t); count != 0 {
				t.Errorf("%d migration events", count)
			}
			// The refused request claimed nothing.
			if _, err := f.apply(f.alice, "migrate", workToOffice, f.preview(t, f.alice, workToOffice)); err != nil {
				t.Errorf("apply after a fresh preview: %v", err)
			}
		})
	}
}

func TestApplyChangesNothingIfAWriteFails(t *testing.T) {
	f := newFixture(t)
	for range 3 {
		f.project(t, "work", nil)
	}
	before := f.storedTasks(t)
	mappings := f.preview(t, f.alice, workToOffice)
	f.clock.Advance(time.Minute)

	// The third task's write fails, after the first two were written.
	failing, updates := true, 0
	f.client.Task.Use(func(next ent.Mutator) ent.Mutator {
		return hook.TaskFunc(func(ctx context.Context, m *ent.TaskMutation) (ent.Value, error) {
			if m.Op().Is(ent.OpUpdateOne) && failing {
				if updates++; updates == 3 {
					return nil, errors.New("the write failed")
				}
			}
			return next.Mutate(ctx, m)
		})
	})
	_, err := f.apply(f.alice, "migrate", workToOffice, mappings)
	var apiErr *api.Error
	if err == nil || errors.As(err, &apiErr) || updates != 3 {
		t.Fatalf("apply = %v after %d updates; want an internal error at the third", err, updates)
	}
	if after := f.storedTasks(t); !reflect.DeepEqual(after, before) {
		t.Errorf("tasks after the failure\n%+v, want\n%+v", after, before)
	}
	if count := f.client.TaskEvent.Query().CountX(f.alice); count != 0 {
		t.Errorf("%d events after the failure", count)
	}

	failing = false
	if result, err := f.apply(f.alice, "migrate", workToOffice, mappings); err != nil || result.Changed != 3 {
		t.Errorf("apply with the same key = %+v, %v", result, err)
	}
}

func TestUndoRefusesAProjectMigration(t *testing.T) {
	f := newFixture(t)
	other := f.device(t, f.alice)
	id := uuid.MustParse(f.create(t, api.CreateTaskRequest{Description: "Report", Project: "work"}).UUID)
	if _, err := f.service.Start(other, uuid.NewString(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.apply(f.alice, uuid.NewString(), workToOffice, f.preview(t, f.alice, workToOffice)); err != nil {
		t.Fatal(err)
	}

	_, err := f.service.Undo(f.alice, uuid.NewString())
	wantError(t, err, api.CodeConflict, "")
	if !errors.Is(err, errMigrationUndo) {
		t.Errorf("undo = %v, want %v", err, errMigrationUndo)
	}
	// The other device's start came before the migration, which changed
	// the task since.
	_, err = f.service.Undo(other, uuid.NewString())
	if !errors.Is(err, errChanged) {
		t.Errorf("the other device's undo = %v, want %v", err, errChanged)
	}
	if stored := f.stored(t, id); stored.Project != "office" || stored.Start == nil {
		t.Errorf("stored = %+v", stored)
	}

	// The device's next change can be undone, and the migration stays.
	if _, err := f.service.Modify(f.alice, uuid.NewString(), id, api.ModifyTaskRequest{Priority: set("H")}); err != nil {
		t.Fatal(err)
	}
	if undone := f.undo(t, f.alice); undone.Project != "office" || undone.Priority != "" {
		t.Errorf("undone = %+v", undone)
	}
}

func TestApplyWaitsForTheTasksItMoves(t *testing.T) {
	f := newFixture(t)
	first := f.project(t, "work", nil)
	f.project(t, "work", nil)
	mappings := f.preview(t, f.alice, workToOffice)

	// Another change to a previewed task is under way when the migration
	// applies. The migration waits for it, then sees the task has moved.
	holder, err := f.client.Tx(f.alice)
	if err != nil {
		t.Fatal(err)
	}
	locked := holder.Task.Query().Where(task.ID(first.ID)).ForUpdate().OnlyX(f.alice)
	done := make(chan error)
	go func() {
		_, err := f.apply(f.alice, uuid.NewString(), workToOffice, mappings)
		done <- err
	}()
	time.Sleep(250 * time.Millisecond)
	holder.Task.UpdateOne(locked).SetProject("home").ExecX(f.alice)
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	wantError(t, <-done, api.CodeConflict, "")
	if projects := f.client.Task.Query().Order(task.ByEntry()).Select(task.FieldProject).StringsX(f.alice); !reflect.DeepEqual(projects, []string{"home", "work"}) {
		t.Errorf("projects = %v", projects)
	}
}
