package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// An import creates the user's Taskwarrior tasks as they were exported,
// whatever their status, with their UUIDs (backend plan T16). It never
// changes a task the user already has, so importing again changes nothing.

// importAction names the task_events rows of an import.
const importAction = "import"

// maxImport bounds one request; the apps send a larger import in batches.
const maxImport = 1000

// errImportUndo refuses to undo an import, which would delete the tasks it
// brought.
var errImportUndo = conflict("an import cannot be undone")

// importedFields are the JSON names of the fields an import compares, in the
// order a difference lists them.
var importedFields = []string{
	"description", "status", "project", "priority", "estimateMinutes", "entry", "modified", "due", "scheduled",
	"start", "wait", "until", "end", "tags", "annotations", "depends", "recurrence", "parent", "extra",
}

// PreviewImport answers api.RoutePreviewImport with what ApplyImport would do
// with request, and changes nothing.
func (s *Service) PreviewImport(ctx context.Context, request api.ImportRequest) (api.ImportResult, error) {
	tasks, ids, err := validateImport(request)
	if err != nil {
		return api.ImportResult{}, err
	}
	existing, err := s.client.Task.Query().Where(task.IDIn(ids...)).All(ctx)
	if err != nil {
		return api.ImportResult{}, err
	}
	return compareImport(tasks, existing)
}

// ApplyImport creates the tasks of request that the user does not have, in
// one change under the idempotency key, and answers what it did with each.
func (s *Service) ApplyImport(ctx context.Context, key string, request api.ImportRequest) (api.ImportResult, error) {
	tasks, ids, err := validateImport(request)
	if err != nil {
		return api.ImportResult{}, err
	}
	return once(ctx, s, key, importAction, uuid.Nil, request, func(c *change) (api.ImportResult, error) {
		existing, err := c.tx.Task.Query().Where(task.IDIn(ids...)).Order(task.ByID()).ForUpdate().All(ctx)
		if err != nil {
			return api.ImportResult{}, err
		}
		result, err := compareImport(tasks, existing)
		if err != nil {
			return api.ImportResult{}, err
		}
		created := make(map[string]bool, len(result.Created))
		for _, id := range result.Created {
			created[id] = true
		}
		userID, _ := viewer.UserID(ctx)
		for _, t := range tasks {
			if !created[t.UUID] {
				continue
			}
			row, err := importedTask(c.tx, userID, t).Save(ctx)
			if ent.IsConstraintError(err) {
				// Only another user's task can hold the UUID: the user's own
				// were found above.
				return api.ImportResult{}, conflict(fmt.Sprintf("task %s cannot be imported: its UUID is taken", t.UUID))
			} else if err != nil {
				return api.ImportResult{}, err
			}
			if err := c.recordCreate(ctx, importAction, row); err != nil {
				return api.ImportResult{}, err
			}
		}
		return result, nil
	})
}

// validateImport checks every task of request and returns them in canonical
// form, with their ids.
func validateImport(request api.ImportRequest) ([]api.Task, []uuid.UUID, error) {
	if len(request.Tasks) == 0 || len(request.Tasks) > maxImport {
		return nil, nil, invalid("tasks", fmt.Sprintf("send from 1 to %d tasks at a time", maxImport))
	}
	tasks := make([]api.Task, len(request.Tasks))
	ids := make([]uuid.UUID, len(request.Tasks))
	seen := make(map[uuid.UUID]bool, len(request.Tasks))
	for i, t := range request.Tasks {
		id, err := uuid.Parse(t.UUID)
		if err != nil {
			return nil, nil, invalid("tasks", fmt.Sprintf("task %d has no valid uuid", i+1))
		}
		if seen[id] {
			return nil, nil, invalid("tasks", fmt.Sprintf("task %s appears twice", id))
		}
		seen[id] = true
		if tasks[i], err = canonicalImport(t); err != nil {
			return nil, nil, invalid("tasks", fmt.Sprintf("task %s: %s", id, err))
		}
		ids[i] = id
	}
	return tasks, ids, nil
}

// canonicalImport checks t and returns it as the store keeps it: UUIDs in
// canonical form, times in UTC to the second, tags without repeats, the
// recurrence rule in canonical form, and no urgency.
func canonicalImport(t api.Task) (api.Task, error) {
	t.UUID = uuid.MustParse(t.UUID).String()
	switch t.Status {
	case api.StatusPending, api.StatusCompleted, api.StatusDeleted, api.StatusRecurring:
	default:
		return api.Task{}, fmt.Errorf("status %q is not pending, completed, deleted, or recurring", t.Status)
	}
	if strings.TrimSpace(t.Description) == "" {
		return api.Task{}, fmt.Errorf("description cannot be empty")
	}
	if err := checkPriority(t.Priority); err != nil {
		return api.Task{}, err
	}
	if t.EstimateMinutes < 0 {
		return api.Task{}, fmt.Errorf("estimate cannot be negative")
	}
	if err := checkTags(t.Tags); err != nil {
		return api.Task{}, err
	}
	if len(t.Tags) > 0 {
		t.Tags = uniqueTags(t.Tags)
	}
	rule, err := domain.ParseRecurrence(t.Recurrence)
	if err != nil {
		return api.Task{}, err
	}
	t.Recurrence = rule
	if t.Status == api.StatusRecurring && (rule == "" || t.Due == nil) {
		return api.Task{}, fmt.Errorf("a recurring template needs a recurrence and a due date")
	}
	if t.Parent != "" {
		parent, err := uuid.Parse(t.Parent)
		if err != nil {
			return api.Task{}, fmt.Errorf("parent is not a uuid")
		}
		t.Parent = parent.String()
	}
	// The request's slices stay as they were sent.
	t.Depends, t.Annotations = slices.Clone(t.Depends), slices.Clone(t.Annotations)
	for i, dependency := range t.Depends {
		id, err := uuid.Parse(dependency)
		if err != nil {
			return api.Task{}, fmt.Errorf("a dependency is not a uuid")
		}
		t.Depends[i] = id.String()
	}
	if t.Entry.IsZero() || t.Modified.IsZero() {
		return api.Task{}, fmt.Errorf("entry and modified are required")
	}
	t.Entry, t.Modified = instant(t.Entry), instant(t.Modified)
	t.Due, t.Scheduled, t.Start = instantOf(t.Due), instantOf(t.Scheduled), instantOf(t.Start)
	t.Wait, t.Until, t.End = instantOf(t.Wait), instantOf(t.Until), instantOf(t.End)
	for i := range t.Annotations {
		t.Annotations[i].Entry = instant(t.Annotations[i].Entry)
	}
	if len(t.Extra) == 0 {
		t.Extra = nil
	}
	t.Urgency = 0
	return t, nil
}

// importedTask creates t for the user userID.
func importedTask(client *ent.Client, userID uuid.UUID, t api.Task) *ent.TaskCreate {
	create := client.Task.Create().SetID(uuid.MustParse(t.UUID)).SetUserID(userID).
		SetDescription(t.Description).SetStatus(task.Status(t.Status)).SetProject(t.Project).SetPriority(t.Priority).
		SetEntry(t.Entry).SetModified(t.Modified).SetRecurrence(t.Recurrence).
		SetNillableDue(t.Due).SetNillableScheduled(t.Scheduled).SetNillableStart(t.Start).
		SetNillableWait(t.Wait).SetNillableUntil(t.Until).SetNillableEnd(t.End)
	if t.EstimateMinutes > 0 {
		create.SetEstimateMinutes(t.EstimateMinutes)
	}
	if len(t.Tags) > 0 {
		create.SetTags(t.Tags)
	}
	if len(t.Annotations) > 0 {
		create.SetAnnotations(t.Annotations)
	}
	if len(t.Depends) > 0 {
		create.SetDepends(t.Depends)
	}
	if t.Parent != "" {
		create.SetParent(uuid.MustParse(t.Parent))
	}
	if t.Extra != nil {
		create.SetExtra(t.Extra)
	}
	return create
}

// compareImport sorts tasks by what an import does with each, given the
// user's existing tasks with the same ids.
func compareImport(tasks []api.Task, existing []*ent.Task) (api.ImportResult, error) {
	stored := make(map[string]api.Task, len(existing))
	for _, row := range existing {
		stored[row.ID.String()] = toAPI(row)
	}
	result := api.ImportResult{Created: []string{}, Unchanged: []string{}, Different: []api.ImportDifference{}}
	for _, t := range tasks {
		have, ok := stored[t.UUID]
		if !ok {
			result.Created = append(result.Created, t.UUID)
			continue
		}
		fields, err := differences(have, t)
		if err != nil {
			return api.ImportResult{}, err
		}
		if len(fields) == 0 {
			result.Unchanged = append(result.Unchanged, t.UUID)
		} else {
			result.Different = append(result.Different, api.ImportDifference{UUID: t.UUID, Fields: fields})
		}
	}
	return result, nil
}

// differences returns the fields in which have and want differ, compared as
// JSON, so that extra fields compare by value whatever their spacing.
func differences(have, want api.Task) ([]string, error) {
	left, err := jsonFields(have)
	if err != nil {
		return nil, err
	}
	right, err := jsonFields(want)
	if err != nil {
		return nil, err
	}
	var fields []string
	for _, name := range importedFields {
		if !reflect.DeepEqual(left[name], right[name]) {
			fields = append(fields, name)
		}
	}
	return fields, nil
}

func jsonFields(t api.Task) (map[string]any, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	return fields, json.Unmarshal(data, &fields)
}
