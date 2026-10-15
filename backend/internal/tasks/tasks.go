// Package tasks reads and changes the signed-in user's tasks (backend plan
// T09). The store's privacy rules limit every query to the viewer in the
// context. Each mutation runs in one transaction that locks its task, writes
// a task_events row, and remembers its idempotency key.
//
// Times are stored in UTC to the second, as Taskwarrior keeps them.
package tasks

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// Service answers the task routes of core/api.
type Service struct {
	client *ent.Client
	now    func() time.Time
}

// New returns a Service that keeps tasks through client.
func New(client *ent.Client) *Service {
	return &Service{client: client, now: time.Now}
}

// clock returns the current time as it is stored.
func (s *Service) clock() time.Time { return instant(s.now()) }

func instant(t time.Time) time.Time { return t.UTC().Truncate(time.Second) }

// List answers api.RouteListTasks with the user's tasks of status, each with
// its urgency at now. Pending leaves out waiting tasks, as Taskwarrior's
// status:pending does. Completed lists the tasks that ended after endedAfter.
func (s *Service) List(ctx context.Context, status string, endedAfter time.Time) ([]api.Task, error) {
	switch {
	case status != api.StatusPending && status != api.StatusRecurring && status != api.StatusCompleted:
		return nil, invalid(api.QueryStatus, "status must be pending, recurring, or completed")
	case status == api.StatusCompleted && endedAfter.IsZero():
		return nil, invalid(api.QueryEndedAfter, "ended_after is required for completed tasks")
	}
	now := s.clock()
	pending, err := pendingTasks(ctx, s.client)
	if err != nil {
		return nil, err
	}
	if status == api.StatusPending {
		rows := slices.DeleteFunc(slices.Clone(pending), func(row *ent.Task) bool { return waiting(row, now) })
		return withUrgency(rows, pending, now), nil
	}
	query := s.client.Task.Query().Where(task.StatusEQ(task.Status(status)))
	if status == api.StatusCompleted {
		query.Where(task.EndGT(endedAfter))
	}
	rows, err := query.Order(task.ByEntry(), task.ByID()).All(ctx)
	if err != nil {
		return nil, err
	}
	return withUrgency(rows, pending, now), nil
}

// Projects answers api.RouteListProjects with the projects of pending tasks.
func (s *Service) Projects(ctx context.Context) ([]string, error) {
	return s.collect(ctx, func(row *ent.Task) []string { return []string{row.Project} })
}

// Tags answers api.RouteListTags with the tags of pending tasks.
func (s *Service) Tags(ctx context.Context) ([]string, error) {
	return s.collect(ctx, func(row *ent.Task) []string { return row.Tags })
}

// collect returns the distinct, non-empty values of pending tasks, sorted.
func (s *Service) collect(ctx context.Context, values func(*ent.Task) []string) ([]string, error) {
	now := s.clock()
	pending, err := pendingTasks(ctx, s.client)
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, row := range pending {
		if waiting(row, now) {
			continue
		}
		for _, value := range values(row) {
			if value != "" && !slices.Contains(result, value) {
				result = append(result, value)
			}
		}
	}
	slices.Sort(result)
	return result, nil
}

// pendingTasks returns every pending task, waiting ones included.
func pendingTasks(ctx context.Context, client *ent.Client) ([]*ent.Task, error) {
	return client.Task.Query().Where(task.StatusEQ(task.StatusPending)).Order(task.ByEntry(), task.ByID()).All(ctx)
}

func waiting(row *ent.Task, now time.Time) bool {
	return row.Wait != nil && row.Wait.After(now)
}

// withUrgency returns rows as api.Tasks with their urgency at now. A task is
// blocked by, and blocks, the tasks in pending, waiting ones included, as
// Taskwarrior counts them (backend plan T04).
func withUrgency(rows, pending []*ent.Task, now time.Time) []api.Task {
	tasks := make([]api.Task, len(rows))
	scored := make([]domain.Task, 0, len(rows)+len(pending))
	listed := make(map[uuid.UUID]bool, len(rows))
	for i, row := range rows {
		tasks[i] = toAPI(row)
		scored = append(scored, forUrgency(tasks[i]))
		listed[row.ID] = true
	}
	for _, row := range pending {
		if !listed[row.ID] {
			scored = append(scored, forUrgency(toAPI(row)))
		}
	}
	domain.SetUrgency(scored, now)
	for i := range tasks {
		tasks[i].Urgency = scored[i].Urgency
	}
	return tasks
}

// forUrgency returns the fields of t that domain.SetUrgency reads.
func forUrgency(t api.Task) domain.Task {
	return domain.Task{
		UUID: t.UUID, Status: t.Status, Project: t.Project, Priority: t.Priority,
		Entry: &t.Entry, Due: t.Due, Scheduled: t.Scheduled, Start: t.Start, Wait: t.Wait,
		Tags: t.Tags, Annotations: make([]domain.Annotation, len(t.Annotations)), Dependencies: t.Depends,
	}
}

func toAPI(row *ent.Task) api.Task {
	t := api.Task{
		UUID:        row.ID.String(),
		Description: row.Description,
		Status:      string(row.Status),
		Project:     row.Project,
		Priority:    row.Priority,
		Entry:       row.Entry.UTC(),
		Modified:    row.Modified.UTC(),
		Due:         utc(row.Due),
		Scheduled:   utc(row.Scheduled),
		Start:       utc(row.Start),
		Wait:        utc(row.Wait),
		Until:       utc(row.Until),
		End:         utc(row.End),
		Tags:        row.Tags,
		Annotations: row.Annotations,
		Depends:     row.Depends,
		Recurrence:  row.Recurrence,
		Extra:       row.Extra,
	}
	if row.EstimateMinutes != nil {
		t.EstimateMinutes = *row.EstimateMinutes
	}
	if row.Parent != nil {
		t.Parent = row.Parent.String()
	}
	return t
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	value := t.UTC()
	return &value
}
