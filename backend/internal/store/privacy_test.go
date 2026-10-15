package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/privacy"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/user"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/storetest"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// twoUsers returns a client and contexts acting for Alice and for Bob.
func twoUsers(t *testing.T) (*ent.Client, context.Context, context.Context) {
	t.Helper()
	client := storetest.Client(t)
	ctx := t.Context()
	alice := client.User.Create().SetEmail("alice@example.com").SaveX(ctx)
	bob := client.User.Create().SetEmail("bob@example.com").SaveX(ctx)
	return client, viewer.NewContext(ctx, alice.ID), viewer.NewContext(ctx, bob.ID)
}

func TestTaskKeepsEveryField(t *testing.T) {
	client, asAlice, _ := twoUsers(t)
	aliceID, _ := viewer.UserID(asAlice)
	at := func(hour int) time.Time { return time.Date(2026, 10, 14, hour, 30, 15, 123456000, time.UTC) }
	parent := uuid.New()
	created, err := client.Task.Create().SetUserID(aliceID).SetDescription("Write the plan").SetStatus(task.StatusPending).
		SetProject("work.client").SetPriority("H").SetEstimateMinutes(90).
		SetDue(at(17)).SetScheduled(at(10)).SetStart(at(11)).SetWait(at(7)).SetUntil(at(23)).SetEnd(at(12)).
		SetTags([]string{"next", "writing"}).SetAnnotations([]api.Annotation{{Entry: at(9), Description: "Outline agreed"}}).
		SetDepends([]string{"0d1c6c58-8d4b-4a8e-9f4e-1b2b3c4d5e6f"}).SetRecurrence("weekly").SetParent(parent).
		SetExtra(map[string]json.RawMessage{"rtype": json.RawMessage(`"periodic"`)}).
		Save(asAlice)
	if err != nil {
		t.Fatal(err)
	}
	read, err := client.Task.Get(asAlice, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		name      string
		got, want any
	}{
		{"description", read.Description, "Write the plan"}, {"status", read.Status, task.StatusPending},
		{"project", read.Project, "work.client"}, {"priority", read.Priority, "H"}, {"estimate", *read.EstimateMinutes, 90},
		{"tags", read.Tags, []string{"next", "writing"}}, {"depends", read.Depends, []string{"0d1c6c58-8d4b-4a8e-9f4e-1b2b3c4d5e6f"}},
		{"recurrence", read.Recurrence, "weekly"}, {"parent", *read.Parent, parent},
		{"extra", string(read.Extra["rtype"]), `"periodic"`},
		{"annotation", read.Annotations[0].Description, "Outline agreed"},
	} {
		if !reflect.DeepEqual(check.got, check.want) {
			t.Errorf("%s = %#v, want %#v", check.name, check.got, check.want)
		}
	}
	for name, pair := range map[string][2]time.Time{
		"due": {*read.Due, at(17)}, "scheduled": {*read.Scheduled, at(10)}, "start": {*read.Start, at(11)},
		"wait": {*read.Wait, at(7)}, "until": {*read.Until, at(23)}, "end": {*read.End, at(12)},
		"annotation entry": {read.Annotations[0].Entry, at(9)},
	} {
		if !pair[0].Equal(pair[1]) {
			t.Errorf("%s = %v, want %v", name, pair[0], pair[1])
		}
	}
	if read.Entry.IsZero() || read.Modified.Before(read.Entry) {
		t.Errorf("entry %v, modified %v", read.Entry, read.Modified)
	}
}

func TestUsersCannotReachEachOthersTasks(t *testing.T) {
	client, asAlice, asBob := twoUsers(t)
	aliceID, _ := viewer.UserID(asAlice)
	bobID, _ := viewer.UserID(asBob)
	own := client.Task.Create().SetUserID(aliceID).SetDescription("Alice's task").SetStatus(task.StatusPending).SaveX(asAlice)
	client.TaskEvent.Create().SetUserID(aliceID).SetTaskID(own.ID).SetChangeID(uuid.New()).SetAction("create").SetAfter(json.RawMessage(`{}`)).SaveX(asAlice)

	if tasks := client.Task.Query().AllX(asBob); len(tasks) != 0 {
		t.Errorf("Bob lists %d of Alice's tasks", len(tasks))
	}
	if _, err := client.Task.Get(asBob, own.ID); !ent.IsNotFound(err) {
		t.Errorf("Bob gets Alice's task: %v", err)
	}
	if tasks := client.User.Query().Where(user.ID(aliceID)).QueryTasks().AllX(asBob); len(tasks) != 0 {
		t.Errorf("Bob reaches %d of Alice's tasks through her user", len(tasks))
	}
	if events := client.TaskEvent.Query().AllX(asBob); len(events) != 0 {
		t.Errorf("Bob lists %d of Alice's task events", len(events))
	}
	if _, err := client.Task.UpdateOneID(own.ID).SetDescription("changed by Bob").Save(asBob); !ent.IsNotFound(err) {
		t.Errorf("Bob updates Alice's task: %v", err)
	}
	if changed := client.Task.Update().SetDescription("changed by Bob").SaveX(asBob); changed != 0 {
		t.Errorf("Bob's bulk update changes %d of Alice's tasks", changed)
	}
	if err := client.Task.DeleteOneID(own.ID).Exec(asBob); !ent.IsNotFound(err) {
		t.Errorf("Bob deletes Alice's task: %v", err)
	}
	if deleted := client.Task.Delete().ExecX(asBob); deleted != 0 {
		t.Errorf("Bob's bulk delete removes %d of Alice's tasks", deleted)
	}
	if deleted := client.TaskEvent.Delete().ExecX(asBob); deleted != 0 {
		t.Errorf("Bob's bulk delete removes %d of Alice's task events", deleted)
	}
	if _, err := client.Task.Create().SetUserID(aliceID).SetDescription("planted by Bob").SetStatus(task.StatusPending).Save(asBob); !errors.Is(err, privacy.Deny) {
		t.Errorf("Bob creates a task for Alice: %v", err)
	}
	if _, err := client.TaskEvent.Create().SetUserID(aliceID).SetTaskID(own.ID).SetChangeID(uuid.New()).SetAction("undo").Save(asBob); !errors.Is(err, privacy.Deny) {
		t.Errorf("Bob creates a task event for Alice: %v", err)
	}
	if _, err := client.Task.Create().SetUserID(bobID).SetDescription("Bob's task").SetStatus(task.StatusPending).Save(asBob); err != nil {
		t.Errorf("Bob creates his own task: %v", err)
	}

	// Alice's task and event are as she left them.
	if read := client.Task.GetX(asAlice, own.ID); read.Description != "Alice's task" {
		t.Errorf("Alice's task is now %q", read.Description)
	}
	if tasks := client.Task.Query().AllX(asAlice); len(tasks) != 1 {
		t.Errorf("Alice lists %d tasks, want only hers", len(tasks))
	}
	if events := client.TaskEvent.Query().AllX(asAlice); len(events) != 1 {
		t.Errorf("Alice lists %d task events, want 1", len(events))
	}
}

func TestTasksNeedASignedInUser(t *testing.T) {
	client, asAlice, _ := twoUsers(t)
	aliceID, _ := viewer.UserID(asAlice)
	client.Task.Create().SetUserID(aliceID).SetDescription("Alice's task").SetStatus(task.StatusPending).SaveX(asAlice)
	nobody := t.Context()

	if _, err := client.Task.Query().All(nobody); !errors.Is(err, privacy.Deny) {
		t.Errorf("query without a user: %v", err)
	}
	if _, err := client.TaskEvent.Query().All(nobody); !errors.Is(err, privacy.Deny) {
		t.Errorf("event query without a user: %v", err)
	}
	if _, err := client.Task.Update().SetDescription("changed").Save(nobody); !errors.Is(err, privacy.Deny) {
		t.Errorf("update without a user: %v", err)
	}
	if _, err := client.Task.Delete().Exec(nobody); !errors.Is(err, privacy.Deny) {
		t.Errorf("delete without a user: %v", err)
	}
	if _, err := client.Task.Create().SetUserID(aliceID).SetDescription("no user").SetStatus(task.StatusPending).Save(nobody); !errors.Is(err, privacy.Deny) {
		t.Errorf("create without a user: %v", err)
	}
}
