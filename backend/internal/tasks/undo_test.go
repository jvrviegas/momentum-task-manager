package tasks

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// device returns a context acting for the user ctx acts for, through a new
// session of theirs, as another of their devices does.
func (f *fixture) device(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	userID, _ := viewer.UserID(ctx)
	session := f.client.Session.Create().SetUserID(userID).SetTokenHash([]byte(uuid.NewString())).
		SetExpiresAt(time.Now().Add(time.Hour)).SaveX(t.Context())
	return auth.NewContext(ctx, auth.Session{ID: session.ID, UserID: userID})
}

func (f *fixture) undo(t *testing.T, ctx context.Context) api.Task {
	t.Helper()
	undone, err := f.service.Undo(ctx, uuid.NewString())
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	return undone
}

// stored returns task id as it is stored, without urgency.
func (f *fixture) stored(t *testing.T, id uuid.UUID) api.Task {
	t.Helper()
	return toAPI(f.client.Task.GetX(f.alice, id))
}

func TestUndoRevertsTheLatestChange(t *testing.T) {
	f := newFixture(t)
	key := uuid.NewString
	start := func(id uuid.UUID) (api.Task, error) { return f.service.Start(f.alice, key(), id) }
	for _, test := range []struct {
		name   string
		setup  func(id uuid.UUID) (api.Task, error)
		change func(id uuid.UUID) (api.Task, error)
	}{
		{"modify", nil, func(id uuid.UUID) (api.Task, error) {
			return f.service.Modify(f.alice, key(), id, api.ModifyTaskRequest{
				Description: set("Final"), Project: cleared[string](), Priority: set("H"), Due: cleared[time.Time](),
				Scheduled: set(time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)), EstimateMinutes: cleared[int](),
				Tags: &api.TagChange{Add: []string{"c"}, Remove: []string{"a"}},
			})
		}},
		{"start", nil, start},
		{"stop", start, func(id uuid.UUID) (api.Task, error) { return f.service.Stop(f.alice, key(), id) }},
		{"complete", start, func(id uuid.UUID) (api.Task, error) { return f.service.Complete(f.alice, key(), id) }},
		{"delete", start, func(id uuid.UUID) (api.Task, error) { return f.service.Delete(f.alice, key(), id) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := uuid.MustParse(f.create(t, api.CreateTaskRequest{
				Description: "Draft", Project: "work", Due: at("2026-10-20T12:00:00Z"), EstimateMinutes: 30, Tags: []string{"a", "b"},
			}).UUID)
			if test.setup != nil {
				f.clock.Advance(time.Minute)
				if _, err := test.setup(id); err != nil {
					t.Fatal(err)
				}
			}
			before := f.stored(t, id)
			f.clock.Advance(time.Hour)
			changed, err := test.change(id)
			if err != nil {
				t.Fatal(err)
			}
			f.clock.Advance(time.Hour)

			// Every field returns to its value before the change, modified
			// included, as Taskwarrior's undo does.
			undone := f.undo(t, f.alice)
			if !reflect.DeepEqual(withoutUrgency(undone), before) {
				t.Errorf("undone\n%+v, want\n%+v", undone, before)
			}
			if stored := f.stored(t, id); !reflect.DeepEqual(stored, before) {
				t.Errorf("stored\n%+v, want\n%+v", stored, before)
			}
			listed, err := f.service.List(f.alice, api.StatusPending, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			for _, task := range listed {
				if task.UUID == undone.UUID && task.Urgency != undone.Urgency {
					t.Errorf("urgency = %v, listed with %v", undone.Urgency, task.Urgency)
				}
			}

			events := f.events(t, f.alice, id.String())
			last := events[len(events)-1]
			if last.Action != "undo" || last.SessionID == nil || *last.SessionID != f.aliceSession || !last.CreatedAt.Equal(f.clock.now) {
				t.Errorf("undo event = %+v", last)
			}
			if got := snapshot(t, last.Before); !reflect.DeepEqual(got, withoutUrgency(changed)) {
				t.Errorf("undo event before\n%+v, want\n%+v", got, withoutUrgency(changed))
			}
			if got := snapshot(t, last.After); !reflect.DeepEqual(got, before) {
				t.Errorf("undo event after\n%+v, want\n%+v", got, before)
			}
		})
	}
}

func TestUndoOfACreateRemovesTheTask(t *testing.T) {
	f := newFixture(t)
	created := f.create(t, api.CreateTaskRequest{Description: "Typo", Tags: []string{"x"}})
	f.clock.Advance(time.Minute)

	// The task no longer exists, so the answer is its last state, deleted.
	undone := f.undo(t, f.alice)
	want := withoutUrgency(created)
	want.Status = api.StatusDeleted
	if !reflect.DeepEqual(withoutUrgency(undone), want) {
		t.Errorf("undone\n%+v, want\n%+v", undone, want)
	}
	if count := f.client.Task.Query().CountX(f.alice); count != 0 {
		t.Errorf("%d tasks, want none", count)
	}
	events := f.events(t, f.alice, created.UUID)
	if len(events) != 2 || events[1].Action != "undo" || events[1].After != nil {
		t.Fatalf("events = %+v", events)
	}
	if before := snapshot(t, events[1].Before); !reflect.DeepEqual(before, withoutUrgency(created)) {
		t.Errorf("undo event before = %+v", before)
	}
	_, err := f.service.Undo(f.alice, uuid.NewString())
	if !errors.Is(err, errNothingToUndo) {
		t.Errorf("second undo: %v", err)
	}
}

func TestUndoIsOneStep(t *testing.T) {
	f := newFixture(t)
	id := uuid.MustParse(f.create(t, api.CreateTaskRequest{Description: "Kept"}).UUID)
	if _, err := f.service.Start(f.alice, uuid.NewString(), id); err != nil {
		t.Fatal(err)
	}
	f.undo(t, f.alice)

	// The create before the start stays.
	_, err := f.service.Undo(f.alice, uuid.NewString())
	if !errors.Is(err, errNothingToUndo) {
		t.Errorf("second undo: %v", err)
	}
	if stored := f.stored(t, id); stored.Status != api.StatusPending || stored.Start != nil {
		t.Errorf("stored = %+v", stored)
	}

	// The next change can be undone.
	if _, err := f.service.Complete(f.alice, uuid.NewString(), id); err != nil {
		t.Fatal(err)
	}
	if undone := f.undo(t, f.alice); undone.Status != api.StatusPending {
		t.Errorf("undone = %+v", undone)
	}
}

func TestUndoRefusesWhenTheTaskChangedSince(t *testing.T) {
	f := newFixture(t)
	laptop, phone := f.alice, f.device(t, f.alice)
	shared := uuid.MustParse(f.create(t, api.CreateTaskRequest{Description: "Shared"}).UUID)
	other := uuid.MustParse(f.create(t, api.CreateTaskRequest{Description: "Other"}).UUID)
	modify := func(ctx context.Context, id uuid.UUID, request api.ModifyTaskRequest) {
		t.Helper()
		if _, err := f.service.Modify(ctx, uuid.NewString(), id, request); err != nil {
			t.Fatal(err)
		}
	}

	modify(laptop, shared, api.ModifyTaskRequest{Priority: set("H")})
	modify(phone, shared, api.ModifyTaskRequest{Description: set("Shared, edited on the phone")})
	_, err := f.service.Undo(laptop, uuid.NewString())
	if !errors.Is(err, errChanged) {
		t.Errorf("laptop undo after the phone's change: %v", err)
	}
	if stored := f.stored(t, shared); stored.Priority != "H" || stored.Description != "Shared, edited on the phone" {
		t.Errorf("stored after a refused undo = %+v", stored)
	}

	// The phone undoes its own change only.
	if undone := f.undo(t, phone); undone.Description != "Shared" || undone.Priority != "H" {
		t.Errorf("phone undo = %+v", undone)
	}
	// The phone's undo changed the task too.
	_, err = f.service.Undo(laptop, uuid.NewString())
	if !errors.Is(err, errChanged) {
		t.Errorf("laptop undo after the phone's undo: %v", err)
	}

	// Changes to other tasks do not matter.
	modify(laptop, other, api.ModifyTaskRequest{Priority: set("L")})
	modify(phone, shared, api.ModifyTaskRequest{Priority: set("M")})
	if undone := f.undo(t, laptop); undone.UUID != other.String() || undone.Priority != "" {
		t.Errorf("laptop undo = %+v", undone)
	}

	// A device without changes, and another user, have nothing to undo.
	for name, ctx := range map[string]context.Context{"new device": f.device(t, f.alice), "Bob": f.bob} {
		if _, err := f.service.Undo(ctx, uuid.NewString()); !errors.Is(err, errNothingToUndo) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if stored := f.stored(t, shared); stored.Priority != "M" {
		t.Errorf("shared = %+v", stored)
	}
}

func TestUndoRetriesApplyOnce(t *testing.T) {
	f := newFixture(t)
	id := uuid.MustParse(f.create(t, api.CreateTaskRequest{Description: "Twice"}).UUID)
	if _, err := f.service.Start(f.alice, uuid.NewString(), id); err != nil {
		t.Fatal(err)
	}
	first, err := f.service.Undo(f.alice, "undo")
	if err != nil {
		t.Fatal(err)
	}
	if again, err := f.service.Undo(f.alice, "undo"); err != nil || !reflect.DeepEqual(again, first) {
		t.Errorf("retry = %+v, %v; want %+v", again, err, first)
	}

	// Undo pressed twice sends two keys; one of them undoes. The undos meet
	// at the task, held here until they have had time to reach it; a slower
	// start only lets fewer of them overlap.
	if _, err := f.service.Complete(f.alice, uuid.NewString(), id); err != nil {
		t.Fatal(err)
	}
	holder, err := f.client.Tx(f.alice)
	if err != nil {
		t.Fatal(err)
	}
	holder.Task.Query().Where(task.ID(id)).ForUpdate().OnlyX(f.alice)
	errs := make([]error, 8)
	var wait sync.WaitGroup
	for i := range errs {
		wait.Go(func() { _, errs[i] = f.service.Undo(f.alice, uuid.NewString()) })
	}
	time.Sleep(250 * time.Millisecond)
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	wait.Wait()
	undone := 0
	for _, err := range errs {
		var apiErr *api.Error
		switch {
		case err == nil:
			undone++
		case !errors.As(err, &apiErr) || apiErr.Code != api.CodeConflict:
			t.Errorf("concurrent undo: %v", err)
		}
	}
	if undone != 1 {
		t.Errorf("%d undos applied, want 1", undone)
	}
	var actions []string
	for _, event := range f.events(t, f.alice, id.String()) {
		actions = append(actions, event.Action)
	}
	if want := []string{"create", "start", "undo", "complete", "undo"}; !reflect.DeepEqual(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
	if stored := f.stored(t, id); stored.Status != api.StatusPending {
		t.Errorf("stored = %+v", stored)
	}
}
