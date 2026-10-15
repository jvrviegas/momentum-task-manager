package tasks

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/taskevent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// setZone sets the time zone of the user ctx acts for.
func (f *fixture) setZone(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	userID, _ := viewer.UserID(ctx)
	f.client.User.UpdateOneID(userID).SetTimeZone(name).ExecX(t.Context())
}

// template returns the stored template of instance.
func (f *fixture) template(t *testing.T, instance api.Task) api.Task {
	t.Helper()
	if instance.Parent == "" {
		t.Fatalf("%+v is not an instance", instance)
	}
	return f.stored(t, uuid.MustParse(instance.Parent))
}

// open returns the stored pending instances of template.
func (f *fixture) open(t *testing.T, template api.Task) []api.Task {
	t.Helper()
	rows := f.client.Task.Query().Where(task.ParentEQ(uuid.MustParse(template.UUID)), task.StatusEQ(task.StatusPending)).AllX(f.alice)
	open := make([]api.Task, len(rows))
	for i, row := range rows {
		open[i] = toAPI(row)
	}
	return open
}

// next returns the one pending instance of template.
func (f *fixture) next(t *testing.T, template api.Task) api.Task {
	t.Helper()
	open := f.open(t, template)
	if len(open) != 1 {
		t.Fatalf("%d pending instances, want 1: %+v", len(open), open)
	}
	return open[0]
}

func (f *fixture) complete(t *testing.T, ctx context.Context, id string) api.Task {
	t.Helper()
	completed, err := f.service.Complete(ctx, uuid.NewString(), uuid.MustParse(id))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	return completed
}

func (f *fixture) modify(t *testing.T, ctx context.Context, id string, input api.ModifyTaskRequest) api.Task {
	t.Helper()
	modified, err := f.service.Modify(ctx, uuid.NewString(), uuid.MustParse(id), input)
	if err != nil {
		t.Fatalf("Modify: %v", err)
	}
	return modified
}

// wantOneChange checks that each task's latest event has its action and that
// they all belong to one change, written in the order given.
func (f *fixture) wantOneChange(t *testing.T, tasks []api.Task, actions []string) {
	t.Helper()
	var change uuid.UUID
	var previous int
	for i, task := range tasks {
		events := f.events(t, f.alice, task.UUID)
		last := events[len(events)-1]
		if last.Action != actions[i] || (i > 0 && (last.ChangeID != change || last.ID < previous)) {
			t.Errorf("event %d of the change = %+v, want %s", i, last, actions[i])
		}
		change, previous = last.ChangeID, last.ID
	}
}

func TestCreatingARecurringTaskStartsASeries(t *testing.T) {
	f := newFixture(t)
	instance := f.create(t, api.CreateTaskRequest{
		Description: "Water plants", Project: "home", Priority: "L", Due: at("2026-10-17T09:00:00Z"),
		Scheduled: at("2026-10-16T18:00:00Z"), Recurrence: "2 weeks", EstimateMinutes: 15, Tags: []string{"chores"},
	})
	template := f.template(t, instance)

	want := api.Task{
		UUID: template.UUID, Description: "Water plants", Status: api.StatusRecurring, Project: "home", Priority: "L",
		EstimateMinutes: 15, Entry: f.clock.now, Modified: f.clock.now, Due: at("2026-10-17T09:00:00Z"),
		Scheduled: at("2026-10-16T18:00:00Z"), Tags: []string{"chores"}, Recurrence: "2wks",
	}
	if !reflect.DeepEqual(template, want) {
		t.Errorf("template\n%+v, want\n%+v", template, want)
	}
	want.UUID, want.Status, want.Parent, want.Urgency = instance.UUID, api.StatusPending, template.UUID, instance.Urgency
	if !reflect.DeepEqual(instance, want) {
		t.Errorf("instance\n%+v, want\n%+v", instance, want)
	}
	f.wantOneChange(t, []api.Task{instance, template}, []string{"create", "create"})

	for status, want := range map[string]string{api.StatusPending: instance.UUID, api.StatusRecurring: template.UUID} {
		listed, err := f.service.List(f.alice, status, time.Time{})
		if err != nil || len(listed) != 1 || listed[0].UUID != want {
			t.Errorf("%s = %+v, %v; want only %s", status, listed, err, want)
		}
	}
}

func TestClosingAnInstanceCreatesTheNext(t *testing.T) {
	f := newFixture(t)
	f.setZone(t, f.alice, "Europe/Lisbon")
	f.clock.now = time.Date(2027, 3, 27, 7, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		action string
		close  func(ctx context.Context, key string, id uuid.UUID) (api.Task, error)
		status string
	}{
		{"complete", f.service.Complete, api.StatusCompleted},
		{"delete", f.service.Delete, api.StatusDeleted},
	} {
		t.Run(test.action, func(t *testing.T) {
			instance := f.create(t, api.CreateTaskRequest{
				Description: "Stretch", Project: "health", Due: at("2027-03-27T09:00:00Z"),
				Scheduled: at("2027-03-27T08:00:00Z"), Recurrence: "daily", Tags: []string{"habit"},
			})
			template := f.template(t, instance)
			// A change to an instance stays with it, as in Taskwarrior.
			f.modify(t, f.alice, instance.UUID, api.ModifyTaskRequest{Description: set("Stretch twice")})
			f.clock.Advance(time.Hour)

			closed, err := test.close(f.alice, uuid.NewString(), uuid.MustParse(instance.UUID))
			if err != nil || closed.UUID != instance.UUID || closed.Status != test.status {
				t.Fatalf("%s = %+v, %v", test.action, closed, err)
			}
			// Lisbon's clocks go forward on 2027-03-28; the series stays at
			// 09:00 and 08:00.
			next := f.next(t, template)
			want := api.Task{
				UUID: next.UUID, Description: "Stretch", Status: api.StatusPending, Project: "health",
				Entry: f.clock.now, Modified: f.clock.now, Due: at("2027-03-28T08:00:00Z"), Scheduled: at("2027-03-28T07:00:00Z"),
				Tags: []string{"habit"}, Recurrence: "daily", Parent: template.UUID,
			}
			if !reflect.DeepEqual(next, want) {
				t.Errorf("next\n%+v, want\n%+v", next, want)
			}
			f.wantOneChange(t, []api.Task{closed, next}, []string{test.action, "recur"})
			if events := f.events(t, f.alice, next.UUID); len(events) != 1 || events[0].Before != nil ||
				!reflect.DeepEqual(snapshot(t, events[0].After), next) {
				t.Errorf("next's events = %+v", events)
			}
		})
	}
}

func TestTheNextInstanceFollowsTheSeries(t *testing.T) {
	f := newFixture(t) // 2026-10-15 09:30 UTC
	for _, test := range []struct {
		name     string
		zone     string
		due      *time.Time
		instance *api.ModifyTaskRequest
		want     *time.Time
	}{
		{name: "a late close skips to today", zone: "UTC", due: at("2026-10-10T09:00:00Z"), want: at("2026-10-15T09:00:00Z")},
		{name: "an early close moves past the closed instance", zone: "UTC", due: at("2026-10-16T09:00:00Z"), want: at("2026-10-17T09:00:00Z")},
		{name: "a moved instance moves the next past it", zone: "UTC", due: at("2026-10-15T09:00:00Z"),
			instance: &api.ModifyTaskRequest{Due: set(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))}, want: at("2026-10-19T09:00:00Z")},
		{name: "an instance without a due date is followed after now", zone: "UTC", due: at("2026-10-14T09:00:00Z"),
			instance: &api.ModifyTaskRequest{Due: cleared[time.Time]()}, want: at("2026-10-16T09:00:00Z")},
		{name: "an instance moved before the first is followed after the first", zone: "UTC", due: at("2026-10-20T09:00:00Z"),
			instance: &api.ModifyTaskRequest{Due: set(time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC))}, want: at("2026-10-21T09:00:00Z")},
		// It is 2026-10-15 22:30 in Auckland: the series is due at 09:00
		// there, and today's has passed.
		{name: "today is the user's", zone: "Pacific/Auckland", due: at("2026-10-10T20:00:00Z"), want: at("2026-10-14T20:00:00Z")},
		{name: "today in UTC", zone: "UTC", due: at("2026-10-10T20:00:00Z"), want: at("2026-10-15T20:00:00Z")},
	} {
		t.Run(test.name, func(t *testing.T) {
			f.setZone(t, f.alice, test.zone)
			instance := f.create(t, api.CreateTaskRequest{Description: "Journal", Due: test.due, Recurrence: "daily"})
			if test.instance != nil {
				f.modify(t, f.alice, instance.UUID, *test.instance)
			}
			f.complete(t, f.alice, instance.UUID)
			if next := f.next(t, f.template(t, instance)); !next.Due.Equal(*test.want) {
				t.Errorf("next due %v, want %v", next.Due, test.want)
			}
		})
	}
}

func TestStoppingASeries(t *testing.T) {
	f := newFixture(t)
	f.setZone(t, f.alice, "Europe/Lisbon")
	f.clock.now = time.Date(2027, 7, 10, 10, 0, 0, 0, time.UTC)
	instance := f.create(t, api.CreateTaskRequest{Description: "Review", Due: at("2027-07-10T08:00:00Z"), Recurrence: "weekly"})
	template := f.template(t, instance)
	f.clock.Advance(time.Minute)

	// Through an instance; the apps send the template, as below.
	stopped, err := f.service.StopRecurrence(f.alice, uuid.NewString(), uuid.MustParse(instance.UUID))
	if err != nil {
		t.Fatal(err)
	}
	// The start of today in Lisbon, as Taskwarrior's until:today.
	want := template
	want.Until, want.Modified = at("2027-07-09T23:00:00Z"), f.clock.now
	if !reflect.DeepEqual(withoutUrgency(stopped), want) {
		t.Errorf("stopped\n%+v, want\n%+v", stopped, want)
	}
	if stored := f.stored(t, uuid.MustParse(instance.UUID)); !reflect.DeepEqual(stored, withoutUrgency(instance)) {
		t.Errorf("the pending instance changed: %+v", stored)
	}
	f.wantOneChange(t, []api.Task{stopped}, []string{"stop-recurrence"})

	plain := f.create(t, api.CreateTaskRequest{Description: "Once"})
	for _, test := range []struct {
		name string
		ctx  context.Context
		id   string
		code string
	}{
		{"a stopped series", f.alice, template.UUID, api.CodeConflict},
		{"a task that does not repeat", f.alice, plain.UUID, api.CodeConflict},
		{"an unknown task", f.alice, uuid.NewString(), api.CodeNotFound},
		{"another user's series", f.bob, template.UUID, api.CodeNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.service.StopRecurrence(test.ctx, uuid.NewString(), uuid.MustParse(test.id))
			wantError(t, err, test.code, "")
		})
	}

	// The pending instance is the last.
	completed := f.complete(t, f.alice, instance.UUID)
	if open := f.open(t, template); len(open) != 0 {
		t.Errorf("a stopped series made %+v", open)
	}
	events := f.events(t, f.alice, completed.UUID)
	if count := f.client.TaskEvent.Query().Where(taskevent.ChangeID(events[len(events)-1].ChangeID)).CountX(f.alice); count != 1 {
		t.Errorf("completing the last instance wrote %d events, want 1", count)
	}
}

func TestASeriesEndsBeforeItsUntilDate(t *testing.T) {
	f := newFixture(t) // 2026-10-15 09:30 UTC
	day := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
	// An imported series, until the 16th at 09:00.
	template := f.insert(t, f.alice, "Pay rent", func(c *ent.TaskCreate) {
		c.SetStatus(task.StatusRecurring).SetRecurrence("daily").SetDue(day(14)).SetUntil(day(16))
	})
	first := f.insert(t, f.alice, "Pay rent", func(c *ent.TaskCreate) { c.SetParent(template.ID).SetRecurrence("daily").SetDue(day(14)) })

	f.complete(t, f.alice, first.ID.String())
	second := f.next(t, toAPI(template))
	if !second.Due.Equal(day(15)) {
		t.Errorf("second due %v, want the 15th", second.Due)
	}
	// The next would be due at the until date.
	f.complete(t, f.alice, second.UUID)
	if open := f.open(t, toAPI(template)); len(open) != 0 {
		t.Errorf("the series went on: %+v", open)
	}
}

func TestASeriesHasOneOpenInstance(t *testing.T) {
	f := newFixture(t)
	day := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
	// Taskwarrior can leave several pending instances, which an import keeps.
	template := f.insert(t, f.alice, "Log hours", func(c *ent.TaskCreate) {
		c.SetStatus(task.StatusRecurring).SetRecurrence("daily").SetDue(day(13))
	})
	var instances []*ent.Task
	for _, due := range []int{13, 14} {
		instances = append(instances, f.insert(t, f.alice, "Log hours", func(c *ent.TaskCreate) {
			c.SetParent(template.ID).SetRecurrence("daily").SetDue(day(due))
		}))
	}

	f.complete(t, f.alice, instances[0].ID.String())
	if open := f.open(t, toAPI(template)); len(open) != 1 || open[0].UUID != instances[1].ID.String() {
		t.Errorf("open = %+v, want only the other instance", open)
	}
	f.complete(t, f.alice, instances[1].ID.String())
	if next := f.next(t, toAPI(template)); !next.Due.Equal(day(15)) {
		t.Errorf("next due %v, want the 15th", next.Due)
	}
}

func TestModifyStartsASeries(t *testing.T) {
	f := newFixture(t)
	created := f.create(t, api.CreateTaskRequest{Description: "Backup", Project: "ops", Due: at("2026-10-16T21:00:00Z"), Tags: []string{"server"}})
	f.clock.Advance(time.Hour)

	modified := f.modify(t, f.alice, created.UUID, api.ModifyTaskRequest{Recurrence: set("week"), Description: set("Back up")})
	// The task becomes the first instance, so it keeps its id and history.
	template := f.template(t, modified)
	want := withoutUrgency(created)
	want.Description, want.Recurrence, want.Parent, want.Modified = "Back up", "weekly", template.UUID, f.clock.now
	if !reflect.DeepEqual(withoutUrgency(modified), want) {
		t.Errorf("modified\n%+v, want\n%+v", modified, want)
	}
	want = api.Task{
		UUID: template.UUID, Description: "Back up", Status: api.StatusRecurring, Project: "ops",
		Entry: f.clock.now, Modified: f.clock.now, Due: at("2026-10-16T21:00:00Z"), Tags: []string{"server"}, Recurrence: "weekly",
	}
	if !reflect.DeepEqual(template, want) {
		t.Errorf("template\n%+v, want\n%+v", template, want)
	}
	f.wantOneChange(t, []api.Task{modified, template}, []string{"modify", "create"})
	if events := f.events(t, f.alice, created.UUID); len(events) != 2 {
		t.Errorf("events = %+v, want the create and the modify", events)
	}
}

func TestModifyChangesTheRuleOfASeries(t *testing.T) {
	f := newFixture(t) // 2026-10-15 09:30 UTC
	instance := f.create(t, api.CreateTaskRequest{Description: "Standup", Due: at("2026-10-16T09:00:00Z"), Recurrence: "daily"})
	templateID := instance.Parent

	// Through an instance, with another field for the instance only.
	modified := f.modify(t, f.alice, instance.UUID, api.ModifyTaskRequest{Recurrence: set("weekdays"), Priority: set("H")})
	template := f.template(t, modified)
	if modified.Recurrence != "weekdays" || modified.Priority != "H" || template.Recurrence != "weekdays" || template.Priority != "" {
		t.Errorf("instance %+v, template %+v", modified, template)
	}
	f.wantOneChange(t, []api.Task{modified, template}, []string{"modify", "modify"})

	// Through the template, as the TUI sends it.
	changed := f.modify(t, f.alice, templateID, api.ModifyTaskRequest{Recurrence: set("2 weeks")})
	instance = f.stored(t, uuid.MustParse(instance.UUID))
	if changed.UUID != templateID || changed.Recurrence != "2wks" || instance.Recurrence != "2wks" {
		t.Errorf("template %+v, instance %+v", changed, instance)
	}
	f.wantOneChange(t, []api.Task{changed, instance}, []string{"modify", "modify"})

	// A template's other fields stay as they are.
	for _, input := range []api.ModifyTaskRequest{
		{Description: set("Sync")},
		{Recurrence: set("weekly"), Description: set("Sync")},
	} {
		_, err := f.service.Modify(f.alice, uuid.NewString(), uuid.MustParse(templateID), input)
		wantError(t, err, api.CodeConflict, "")
	}

	// The next instance follows the new rule.
	f.complete(t, f.alice, instance.UUID)
	if next := f.next(t, changed); !next.Due.Equal(*at("2026-10-30T09:00:00Z")) {
		t.Errorf("next due %v, want two weeks after the first", next.Due)
	}
}

func TestUndoRevertsEveryTaskOfAChange(t *testing.T) {
	f := newFixture(t)
	series := func() api.Task {
		return f.create(t, api.CreateTaskRequest{Description: "Inbox zero", Due: at("2026-10-15T17:00:00Z"), Recurrence: "daily"})
	}
	// all returns Alice's tasks as they are stored, by id.
	all := func() map[string]api.Task {
		result := map[string]api.Task{}
		for _, row := range f.client.Task.Query().AllX(f.alice) {
			result[row.ID.String()] = toAPI(row)
		}
		return result
	}
	stop := func(id string) api.Task {
		stopped, err := f.service.StopRecurrence(f.alice, uuid.NewString(), uuid.MustParse(id))
		if err != nil {
			t.Fatal(err)
		}
		return stopped
	}
	for _, test := range []struct {
		name  string
		setup func() api.Task
		// change makes the change to undo to the task setup returned, and
		// returns its answer.
		change  func(api.Task) api.Task
		reached int
	}{
		{"a created series", nil, func(api.Task) api.Task { return series() }, 2},
		{"a closed instance", series, func(instance api.Task) api.Task { return f.complete(t, f.alice, instance.UUID) }, 2},
		{"a started series", func() api.Task {
			return f.create(t, api.CreateTaskRequest{Description: "Plain", Due: at("2026-10-15T17:00:00Z")})
		}, func(plain api.Task) api.Task {
			return f.modify(t, f.alice, plain.UUID, api.ModifyTaskRequest{Recurrence: set("daily")})
		}, 2},
		{"a rule change through the template", series, func(instance api.Task) api.Task {
			return f.modify(t, f.alice, instance.Parent, api.ModifyTaskRequest{Recurrence: set("weekly")})
		}, 2},
		{"a rule change through an instance", series, func(instance api.Task) api.Task {
			return f.modify(t, f.alice, instance.UUID, api.ModifyTaskRequest{Recurrence: set("weekly"), Priority: set("H")})
		}, 2},
		{"a stop", series, func(instance api.Task) api.Task { return stop(instance.UUID) }, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var target api.Task
			if test.setup != nil {
				target = test.setup()
			}
			f.clock.Advance(time.Hour)
			before := all()
			answer := test.change(target)
			f.clock.Advance(time.Minute)

			undone := f.undo(t, f.alice)
			if undone.UUID != answer.UUID {
				t.Errorf("undo answered with %s, want %s", undone.UUID, answer.UUID)
			}
			if after := all(); !reflect.DeepEqual(after, before) {
				t.Errorf("after undo\n%+v, want\n%+v", after, before)
			}
			last := f.client.TaskEvent.Query().Where(taskevent.SessionID(f.aliceSession)).Order(taskevent.ByID(sql.OrderDesc())).FirstX(f.alice)
			undos := f.client.TaskEvent.Query().Where(taskevent.ChangeID(last.ChangeID)).AllX(f.alice)
			for _, event := range undos {
				if event.Action != "undo" {
					t.Errorf("undo event %+v", event)
				}
			}
			if len(undos) != test.reached {
				t.Errorf("%d undo events, want %d", len(undos), test.reached)
			}
		})
	}
}

func TestUndoOfAClosedInstanceWaitsForTheNextOne(t *testing.T) {
	f := newFixture(t)
	instance := f.create(t, api.CreateTaskRequest{Description: "Inbox zero", Due: at("2026-10-15T17:00:00Z"), Recurrence: "daily"})
	f.complete(t, f.alice, instance.UUID)
	next := f.next(t, f.template(t, instance))

	// Once another device changes the next instance, undo would lose that
	// change, so it is refused and nothing is reverted.
	f.modify(t, f.device(t, f.alice), next.UUID, api.ModifyTaskRequest{Priority: set("H")})
	_, err := f.service.Undo(f.alice, uuid.NewString())
	wantError(t, err, api.CodeConflict, "")
	if stored := f.stored(t, uuid.MustParse(instance.UUID)); stored.Status != api.StatusCompleted {
		t.Errorf("the closed instance is %s", stored.Status)
	}
}

func TestConcurrentClosesMakeOneNextInstance(t *testing.T) {
	f := newFixture(t)
	instance := f.create(t, api.CreateTaskRequest{Description: "Inbox zero", Due: at("2026-10-15T17:00:00Z"), Recurrence: "daily"})
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() { _, errs[i] = f.service.Complete(f.alice, uuid.NewString(), uuid.MustParse(instance.UUID)) })
	}
	wg.Wait()
	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Errorf("%d completes succeeded: %v", succeeded, errs)
	}
	f.next(t, f.template(t, instance))
}

func TestClosingAnInstanceWaitsForItsTemplate(t *testing.T) {
	f := newFixture(t) // 2026-10-15 09:30 UTC
	instance := f.create(t, api.CreateTaskRequest{Description: "Inbox zero", Due: at("2026-10-15T17:00:00Z"), Recurrence: "daily"})
	templateID := uuid.MustParse(instance.Parent)

	// A rule change is under way while the instance is completed. The complete
	// waits for it, held here, so the next instance follows the new rule.
	holder, err := f.client.Tx(f.alice)
	if err != nil {
		t.Fatal(err)
	}
	holder.Task.Query().Where(task.ID(templateID)).ForUpdate().OnlyX(f.alice)
	holder.Task.UpdateOneID(templateID).SetRecurrence("weekly").ExecX(f.alice)
	var completeErr error
	var wait sync.WaitGroup
	wait.Go(func() { _, completeErr = f.service.Complete(f.alice, uuid.NewString(), uuid.MustParse(instance.UUID)) })
	time.Sleep(250 * time.Millisecond)
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	wait.Wait()
	if completeErr != nil {
		t.Fatal(completeErr)
	}
	if next := f.next(t, f.template(t, instance)); next.Recurrence != "weekly" || !next.Due.Equal(*at("2026-10-22T17:00:00Z")) {
		t.Errorf("next = %+v, want weekly and a week later", next)
	}
}
