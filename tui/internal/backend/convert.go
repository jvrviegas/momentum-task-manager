package backend

import (
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// toTask returns task in the shape the apps read from Taskwarrior's export.
// Imported fields the server keeps in extra are raw fields, which the apps
// show read-only.
func toTask(task api.Task) domain.Task {
	converted := domain.Task{
		UUID: task.UUID, Description: task.Description, Status: task.Status, Project: task.Project, Priority: task.Priority,
		Entry: instant(task.Entry), Modified: instant(task.Modified),
		Due: task.Due, Scheduled: task.Scheduled, Start: task.Start, Wait: task.Wait, Until: task.Until, End: task.End,
		Tags: task.Tags, Dependencies: task.Depends, Recurrence: task.Recurrence, Parent: task.Parent, Urgency: task.Urgency,
		RawFields: task.Extra,
	}
	if task.EstimateMinutes > 0 {
		converted.Estimate = &domain.Estimate{Minutes: task.EstimateMinutes}
	}
	for _, annotation := range task.Annotations {
		converted.Annotations = append(converted.Annotations, domain.Annotation{Description: annotation.Description, Entry: instant(annotation.Entry)})
	}
	return converted
}

func instant(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

// createRequest resolves input's date expressions at now, in its zone.
func createRequest(input domain.NewTask, now time.Time) (api.CreateTaskRequest, error) {
	request := api.CreateTaskRequest{
		Description: input.Description, Project: input.Project, Priority: input.Priority,
		Recurrence: input.Recurrence, Tags: input.Tags,
	}
	if input.Estimate != nil {
		request.EstimateMinutes = input.Estimate.Minutes
	}
	var err error
	if request.Due, err = optionalDate("due", input.Due, now); err != nil {
		return api.CreateTaskRequest{}, err
	}
	if request.Scheduled, err = optionalDate("scheduled", input.Scheduled, now); err != nil {
		return api.CreateTaskRequest{}, err
	}
	return request, nil
}

// modifyRequest resolves diff's date expressions at now, in its zone.
func modifyRequest(diff domain.TaskDiff, now time.Time) (api.ModifyTaskRequest, error) {
	request := api.ModifyTaskRequest{
		Description: text(diff.Description), Project: text(diff.Project), Priority: text(diff.Priority),
		Recurrence: text(diff.Recurrence), EstimateMinutes: api.Change[int]{Kind: diff.Estimate.Kind},
	}
	if diff.Estimate.Value != nil {
		request.EstimateMinutes.Value = diff.Estimate.Value.Minutes
	}
	if diff.Tags.Changed {
		request.Tags = &api.TagChange{Add: diff.Tags.Add, Remove: diff.Tags.Remove}
	}
	var err error
	if request.Due, err = dateChange("due", diff.Due, now); err != nil {
		return api.ModifyTaskRequest{}, err
	}
	if request.Scheduled, err = dateChange("scheduled", diff.Scheduled, now); err != nil {
		return api.ModifyTaskRequest{}, err
	}
	return request, nil
}

func text(change domain.FieldChange) api.Change[string] {
	return api.Change[string]{Kind: change.Kind, Value: change.Value}
}

func optionalDate(field, expression string, now time.Time) (*time.Time, error) {
	if expression == "" {
		return nil, nil
	}
	resolved, err := resolve(field, expression, now)
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}

func dateChange(field string, change domain.FieldChange, now time.Time) (api.Change[time.Time], error) {
	if change.Kind != domain.Set {
		return api.Change[time.Time]{Kind: change.Kind}, nil
	}
	resolved, err := resolve(field, change.Value, now)
	return api.Change[time.Time]{Kind: domain.Set, Value: resolved}, err
}

// resolve returns the instant expression names, in UTC, or an error on field
// in the shape the server's would have.
func resolve(field, expression string, now time.Time) (time.Time, error) {
	resolved, err := domain.ResolveDate(expression, now)
	if err != nil {
		return time.Time{}, &api.Error{Code: api.CodeInvalid, Message: err.Error(), Field: field}
	}
	return resolved.UTC(), nil
}
