package tasks

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/taskevent"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

const (
	importedID  = "1b4e28ba-2fa1-11d2-883f-0016d3cca427"
	completedID = "2c5f39cb-3fb2-12e3-994f-0127e4ddb538"
	deletedID   = "3d6f4adc-40c3-13f4-a55f-0238f5eec649"
	templateID  = "4e7f5bed-51d4-1405-b66f-0349f6ffd75a"
	instanceID  = "5f8f6cfe-62e5-1516-877f-045af7f0e86b"
	pastID      = "6f9f7dff-73f6-1627-988f-056bf8f1f97c"
)

// taskwarriorTasks are tasks as Taskwarrior exports them, converted to
// api.Task: every status, a weekly series with a pending and a completed
// instance, and every field, some not in the store's canonical form.
func taskwarriorTasks() []api.Task {
	extra := map[string]json.RawMessage{"rtype": json.RawMessage(`"periodic"`), "client": json.RawMessage(`{ "name" : "acme" }`)}
	return []api.Task{
		{UUID: strings.ToUpper(importedID), Description: "Send the invoice", Status: api.StatusPending, Project: "work.client",
			Priority: "H", EstimateMinutes: 2000, Entry: *at("2026-07-03T08:00:00.75Z"), Modified: *at("2026-07-04T10:00:00Z"),
			Due: at("2026-10-20T16:00:00+01:00"), Scheduled: at("2026-10-19T00:00:00Z"), Start: at("2026-10-15T09:00:00Z"),
			Wait: at("2026-10-16T00:00:00Z"), Until: at("2026-12-31T00:00:00Z"), Tags: []string{"next", "calls", "next"},
			Annotations: []api.Annotation{{Entry: *at("2026-07-03T08:05:00.5Z"), Description: "Ask for the PO"}},
			Depends:     []string{strings.ToUpper(completedID)}, Urgency: 12.5, Extra: extra},
		{UUID: completedID, Description: "Book the room", Status: api.StatusCompleted, Entry: *at("2026-07-05T08:00:00Z"),
			Modified: *at("2026-07-06T08:00:00Z"), End: at("2026-07-06T08:00:00Z")},
		{UUID: deletedID, Description: "Old idea", Status: api.StatusDeleted, Priority: "L", Entry: *at("2026-07-07T08:00:00Z"),
			Modified: *at("2026-07-08T08:00:00Z"), End: at("2026-07-08T08:00:00Z")},
		{UUID: templateID, Description: "Water the plants", Status: api.StatusRecurring, Recurrence: "weekly",
			Entry: *at("2026-09-01T08:00:00Z"), Modified: *at("2026-09-01T08:00:00Z"), Due: at("2026-09-01T08:00:00Z"),
			Extra: map[string]json.RawMessage{"mask": json.RawMessage(`"X-"`), "rtype": json.RawMessage(`"periodic"`)}},
		{UUID: pastID, Description: "Water the plants", Status: api.StatusCompleted, Recurrence: "week", Parent: templateID,
			Entry: *at("2026-09-01T08:00:00Z"), Modified: *at("2026-09-02T08:00:00Z"), Due: at("2026-09-01T08:00:00Z"),
			End: at("2026-09-02T08:00:00Z"), Extra: map[string]json.RawMessage{"imask": json.RawMessage(`0`)}},
		{UUID: instanceID, Description: "Water the plants", Status: api.StatusPending, Recurrence: "weekly", Parent: templateID,
			Entry: *at("2026-09-08T08:00:00Z"), Modified: *at("2026-09-08T08:00:00Z"), Due: at("2026-09-08T08:00:00Z"),
			Extra: map[string]json.RawMessage{"imask": json.RawMessage(`1`)}},
	}
}

func (f *fixture) imported(t *testing.T, id string) api.Task {
	t.Helper()
	row, err := f.client.Task.Get(f.alice, uuid.MustParse(id))
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return toAPI(row)
}

func TestImportKeepsEveryFieldAndRunsOnce(t *testing.T) {
	f := newFixture(t)
	request := api.ImportRequest{Tasks: taskwarriorTasks()}
	ids := []string{importedID, completedID, deletedID, templateID, pastID, instanceID}

	preview, err := f.service.PreviewImport(f.alice, request)
	want := api.ImportResult{Created: ids, Unchanged: []string{}, Different: []api.ImportDifference{}}
	if err != nil || !reflect.DeepEqual(preview, want) {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	if count := f.client.Task.Query().CountX(f.alice); count != 0 {
		t.Fatalf("the preview stored %d tasks", count)
	}
	result, err := f.service.ApplyImport(f.alice, "import-1", request)
	if err != nil || !reflect.DeepEqual(result, want) {
		t.Fatalf("apply = %+v, %v", result, err)
	}
	if request.Tasks[0].Depends[0] != strings.ToUpper(completedID) || request.Tasks[0].Annotations[0].Entry != *at("2026-07-03T08:05:00.5Z") {
		t.Error("validation changed the request")
	}

	first := f.imported(t, importedID)
	wantFirst := api.Task{UUID: importedID, Description: "Send the invoice", Status: api.StatusPending, Project: "work.client",
		Priority: "H", EstimateMinutes: 2000, Entry: *at("2026-07-03T08:00:00Z"), Modified: *at("2026-07-04T10:00:00Z"),
		Due: at("2026-10-20T15:00:00Z"), Scheduled: at("2026-10-19T00:00:00Z"), Start: at("2026-10-15T09:00:00Z"),
		Wait: at("2026-10-16T00:00:00Z"), Until: at("2026-12-31T00:00:00Z"), Tags: []string{"next", "calls"},
		Annotations: []api.Annotation{{Entry: *at("2026-07-03T08:05:00Z"), Description: "Ask for the PO"}},
		Depends:     []string{completedID}}
	gotExtra := first.Extra
	first.Extra = nil
	if !reflect.DeepEqual(first, wantFirst) {
		t.Errorf("stored task:\n got %+v\nwant %+v", first, wantFirst)
	}
	var client map[string]string
	if json.Unmarshal(gotExtra["client"], &client); client["name"] != "acme" || string(gotExtra["rtype"]) != `"periodic"` || len(gotExtra) != 2 {
		t.Errorf("extra = %s", gotExtra)
	}
	for id, status := range map[string]string{completedID: "completed", deletedID: "deleted", templateID: "recurring", pastID: "completed", instanceID: "pending"} {
		if got := f.imported(t, id); got.Status != status {
			t.Errorf("%s: status %s, want %s", id, got.Status, status)
		}
	}
	if past := f.imported(t, pastID); past.Recurrence != "weekly" || past.Parent != templateID || string(past.Extra["imask"]) != "0" {
		t.Errorf("past instance = %+v", past)
	}

	events := f.client.TaskEvent.Query().Order(taskevent.ByID()).AllX(f.alice)
	if len(events) != len(ids) {
		t.Fatalf("%d events, want %d", len(events), len(ids))
	}
	for i, event := range events {
		after := snapshot(t, event.After)
		if event.Action != importAction || event.ChangeID != events[0].ChangeID || event.SessionID == nil || *event.SessionID != f.aliceSession ||
			event.Before != nil || event.TaskID.String() != ids[i] || after.UUID != ids[i] {
			t.Errorf("event %d = %+v", i, event)
		}
	}

	// Importing again, or previewing, changes nothing and finds every task equal.
	again, err := f.service.ApplyImport(f.alice, "import-2", api.ImportRequest{Tasks: taskwarriorTasks()})
	unchanged := api.ImportResult{Created: []string{}, Unchanged: ids, Different: []api.ImportDifference{}}
	if err != nil || !reflect.DeepEqual(again, unchanged) {
		t.Errorf("second import = %+v, %v", again, err)
	}
	if preview, err := f.service.PreviewImport(f.alice, api.ImportRequest{Tasks: taskwarriorTasks()}); err != nil || !reflect.DeepEqual(preview, unchanged) {
		t.Errorf("preview after the import = %+v, %v", preview, err)
	}
	if count := f.client.TaskEvent.Query().CountX(f.alice); count != len(ids) {
		t.Errorf("%d events after importing again", count)
	}
	// A retry with the first key answers as the first attempt did.
	if retry, err := f.service.ApplyImport(f.alice, "import-1", api.ImportRequest{Tasks: taskwarriorTasks()}); err != nil || !reflect.DeepEqual(retry, want) {
		t.Errorf("retry = %+v, %v", retry, err)
	}
}

func TestImportLeavesTheUsersTasksAsTheyAre(t *testing.T) {
	f := newFixture(t)
	tasks := taskwarriorTasks()[1:3]
	if _, err := f.service.ApplyImport(f.alice, "import-1", api.ImportRequest{Tasks: tasks}); err != nil {
		t.Fatal(err)
	}
	changed := taskwarriorTasks()[1:3]
	changed[0].Project, changed[0].Tags = "home", []string{"errands"}
	changed[1].Extra = map[string]json.RawMessage{"client": json.RawMessage(`"acme"`)}
	result, err := f.service.ApplyImport(f.alice, "import-2", api.ImportRequest{Tasks: changed})
	want := api.ImportResult{Created: []string{}, Unchanged: []string{}, Different: []api.ImportDifference{
		{UUID: completedID, Fields: []string{"project", "tags"}}, {UUID: deletedID, Fields: []string{"extra"}},
	}}
	if err != nil || !reflect.DeepEqual(result, want) {
		t.Fatalf("import = %+v, %v", result, err)
	}
	if got := f.imported(t, completedID); got.Project != "" || got.Tags != nil {
		t.Errorf("the import changed %+v", got)
	}
}

func TestImportRefusesInvalidTasks(t *testing.T) {
	f := newFixture(t)
	valid := func(change func(*api.Task)) []api.Task {
		task := taskwarriorTasks()[1]
		change(&task)
		return []api.Task{task}
	}
	many := make([]api.Task, maxImport+1)
	for i := range many {
		many[i] = taskwarriorTasks()[1]
		many[i].UUID = uuid.NewString()
	}
	for name, tasks := range map[string][]api.Task{
		"no tasks":                   {},
		"too many":                   many,
		"a bad uuid":                 valid(func(t *api.Task) { t.UUID = "nope" }),
		"a repeated uuid":            append(valid(func(*api.Task) {}), valid(func(t *api.Task) { t.UUID = strings.ToUpper(t.UUID) })...),
		"an unknown status":          valid(func(t *api.Task) { t.Status = "waiting" }),
		"an empty description":       valid(func(t *api.Task) { t.Description = " " }),
		"a bad priority":             valid(func(t *api.Task) { t.Priority = "high" }),
		"a negative estimate":        valid(func(t *api.Task) { t.EstimateMinutes = -1 }),
		"a tag with a space":         valid(func(t *api.Task) { t.Tags = []string{"two words"} }),
		"an unsupported rule":        valid(func(t *api.Task) { t.Recurrence = "every full moon" }),
		"a template without a rule":  valid(func(t *api.Task) { t.Status = api.StatusRecurring; t.Due = at("2026-09-01T08:00:00Z") }),
		"a template without a due":   valid(func(t *api.Task) { t.Status = api.StatusRecurring; t.Recurrence = "weekly" }),
		"a bad parent":               valid(func(t *api.Task) { t.Parent = "nope" }),
		"a bad dependency":           valid(func(t *api.Task) { t.Depends = []string{"nope"} }),
		"no entry":                   valid(func(t *api.Task) { t.Entry = time.Time{} }),
		"no modified":                valid(func(t *api.Task) { t.Modified = time.Time{} }),
		"a valid task and a bad one": append(taskwarriorTasks()[:1], valid(func(t *api.Task) { t.Status = "" })...),
	} {
		_, err := f.service.PreviewImport(f.alice, api.ImportRequest{Tasks: tasks})
		wantError(t, err, api.CodeInvalid, "tasks")
		_, err = f.service.ApplyImport(f.alice, "import-"+name, api.ImportRequest{Tasks: tasks})
		wantError(t, err, api.CodeInvalid, "tasks")
	}
	if count := f.client.Task.Query().CountX(f.alice); count != 0 {
		t.Errorf("%d tasks stored", count)
	}
	if count := f.client.IdempotencyKey.Query().CountX(f.alice); count != 0 {
		t.Errorf("%d keys claimed", count)
	}
	_, err := f.service.ApplyImport(f.alice, "", api.ImportRequest{Tasks: taskwarriorTasks()})
	wantError(t, err, api.CodeInvalid, "")
	if _, err := f.service.PreviewImport(f.alice, api.ImportRequest{Tasks: valid(func(t *api.Task) { t.Status = "waiting" })}); err == nil ||
		!strings.Contains(err.Error(), completedID) || strings.Contains(err.Error(), "Book the room") {
		t.Errorf("error names the task by its uuid only: %v", err)
	}
}

func TestImportCannotBeUndone(t *testing.T) {
	f := newFixture(t)
	if _, err := f.service.ApplyImport(f.alice, "import-1", api.ImportRequest{Tasks: taskwarriorTasks()}); err != nil {
		t.Fatal(err)
	}
	_, err := f.service.Undo(f.alice, "undo-1")
	if wantError(t, err, api.CodeConflict, ""); err == nil || err.Error() != "an import cannot be undone" {
		t.Errorf("undo = %v", err)
	}
	if count := f.client.Task.Query().CountX(f.alice); count != len(taskwarriorTasks()) {
		t.Errorf("%d tasks after the refused undo", count)
	}
}

func TestImportedSeriesGoesOn(t *testing.T) {
	f := newFixture(t)
	if _, err := f.service.ApplyImport(f.alice, "import-1", api.ImportRequest{Tasks: taskwarriorTasks()}); err != nil {
		t.Fatal(err)
	}
	f.clock.now = *at("2026-09-09T10:00:00Z")
	if _, err := f.service.Complete(f.alice, "complete-1", uuid.MustParse(instanceID)); err != nil {
		t.Fatal(err)
	}
	next := f.client.Task.Query().Where(task.ParentEQ(uuid.MustParse(templateID)), task.StatusEQ(task.StatusPending)).OnlyX(f.alice)
	if !next.Due.Equal(*at("2026-09-15T08:00:00Z")) || next.Recurrence != "weekly" || next.Description != "Water the plants" {
		t.Errorf("next instance = %+v", next)
	}
}

func TestImportIsPerUser(t *testing.T) {
	f := newFixture(t)
	if _, err := f.service.ApplyImport(f.alice, "import-1", api.ImportRequest{Tasks: taskwarriorTasks()}); err != nil {
		t.Fatal(err)
	}
	tasks := taskwarriorTasks()[1:2]
	preview, err := f.service.PreviewImport(f.bob, api.ImportRequest{Tasks: tasks})
	if err != nil || !reflect.DeepEqual(preview.Created, []string{completedID}) {
		t.Errorf("bob's preview = %+v, %v", preview, err)
	}
	_, err = f.service.ApplyImport(f.bob, "import-1", api.ImportRequest{Tasks: tasks})
	wantError(t, err, api.CodeConflict, "")
	if err == nil || err.Error() != fmt.Sprintf("task %s cannot be imported: its UUID is taken", completedID) {
		t.Errorf("bob's import = %v", err)
	}
	if count := f.client.Task.Query().CountX(f.bob); count != 0 {
		t.Errorf("bob has %d tasks", count)
	}
	if got := f.imported(t, completedID); got.Description != "Book the room" {
		t.Errorf("alice's task = %+v", got)
	}
}
