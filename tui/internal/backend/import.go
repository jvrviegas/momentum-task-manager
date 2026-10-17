package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// importBatch is how many tasks one import request carries.
const importBatch = 200

// columns are the export fields the server stores as task fields; every
// other field but the numeric id and the urgency, which Taskwarrior derives,
// goes to the task's extra fields.
var columns = map[string]bool{
	"uuid": true, "description": true, "status": true, "project": true, "priority": true, "estimate": true,
	"entry": true, "modified": true, "end": true, "due": true, "scheduled": true, "start": true, "wait": true,
	"until": true, "tags": true, "annotations": true, "depends": true, "recur": true, "parent": true,
}

// Import is a Taskwarrior export ready to send to the server (backend plan
// T16).
type Import struct {
	Tasks []api.Task
	// Statuses counts the tasks by status.
	Statuses map[string]int
	// Extra counts, for each field Momentum keeps but does not use, the tasks
	// that have it.
	Extra map[string]int
}

// NewImport converts tasks as Taskwarrior exported them. It refuses the
// export if a task repeats by a rule Momentum does not support, naming each.
func NewImport(tasks []domain.Task) (Import, error) {
	result := Import{Tasks: make([]api.Task, 0, len(tasks)), Statuses: map[string]int{}, Extra: map[string]int{}}
	var unsupported []string
	for _, task := range tasks {
		if _, err := domain.ParseRecurrence(task.Recurrence); err != nil {
			unsupported = append(unsupported, fmt.Sprintf("task %s repeats %q", task.UUID, task.Recurrence))
		}
		converted := toImport(task)
		result.Tasks = append(result.Tasks, converted)
		result.Statuses[converted.Status]++
		for name := range converted.Extra {
			result.Extra[name]++
		}
	}
	if len(unsupported) > 0 {
		return Import{}, fmt.Errorf("these tasks repeat by rules Momentum does not support (daily, weekdays, weekly, monthly, quarterly, yearly, or every so many days, weeks, or months); change them in Taskwarrior first:\n  %s",
			strings.Join(unsupported, "\n  "))
	}
	return result, nil
}

// toImport returns task as the server imports it.
func toImport(task domain.Task) api.Task {
	converted := api.Task{
		UUID: task.UUID, Description: task.Description, Status: task.Status, Project: task.Project, Priority: task.Priority,
		Due: task.Due, Scheduled: task.Scheduled, Start: task.Start, Wait: task.Wait, Until: task.Until, End: task.End,
		Tags: task.Tags, Depends: task.Dependencies, Recurrence: task.Recurrence, Parent: task.Parent,
	}
	// Taskwarrior 2 marked a task waiting; Taskwarrior 3 keeps it pending.
	if converted.Status == "waiting" {
		converted.Status = api.StatusPending
	}
	if task.Entry != nil {
		converted.Entry = *task.Entry
	}
	converted.Modified = converted.Entry
	if task.Modified != nil {
		converted.Modified = *task.Modified
	}
	if task.Estimate != nil {
		converted.EstimateMinutes = task.Estimate.Minutes
	}
	for _, annotation := range task.Annotations {
		var entry time.Time
		if annotation.Entry != nil {
			entry = *annotation.Entry
		}
		converted.Annotations = append(converted.Annotations, api.Annotation{Entry: entry, Description: annotation.Description})
	}
	for name, value := range task.RawFields {
		// An estimate the store cannot hold stays as it was exported.
		keep := !columns[name] || (name == "estimate" && task.Estimate == nil)
		if keep && name != "id" && name != "urgency" {
			if converted.Extra == nil {
				converted.Extra = map[string]json.RawMessage{}
			}
			converted.Extra[name] = value
		}
	}
	return converted
}

// Preview asks the server at url, as the user token signs in, what importing
// would do with each task.
func (i Import) Preview(ctx context.Context, url, token string) (api.ImportResult, error) {
	server := api.NewClient(url, token)
	return i.send(func(batch api.ImportRequest) (api.ImportResult, error) { return server.PreviewImport(ctx, batch) })
}

// Apply imports the tasks the user does not have, batch by batch. A batch
// that fails leaves the ones before it imported; importing again finishes the
// rest.
func (i Import) Apply(ctx context.Context, url, token string) (api.ImportResult, error) {
	server := api.NewClient(url, token)
	return i.send(func(batch api.ImportRequest) (api.ImportResult, error) { return server.ApplyImport(ctx, batch) })
}

func (i Import) send(request func(api.ImportRequest) (api.ImportResult, error)) (api.ImportResult, error) {
	total := api.ImportResult{Created: []string{}, Unchanged: []string{}, Different: []api.ImportDifference{}}
	for start := 0; start < len(i.Tasks); start += importBatch {
		result, err := request(api.ImportRequest{Tasks: i.Tasks[start:min(start+importBatch, len(i.Tasks))]})
		if err != nil {
			return total, err
		}
		total.Created = append(total.Created, result.Created...)
		total.Unchanged = append(total.Unchanged, result.Unchanged...)
		total.Different = append(total.Different, result.Different...)
	}
	return total, nil
}
