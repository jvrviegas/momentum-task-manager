package tasks

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// Recurring series follow backend plan D5. A template, with status
// recurring, holds a series' rule and the fields it repeats. It has one
// pending instance at a time, which names it as its parent and copies its
// rule. The template's due date anchors the series in the user's time zone,
// and closing an instance creates the next one, due at the series' next
// occurrence (domain.NextInstanceDue). A request that changes several tasks
// records an event for each under one change, so undo reverts them together.
//
// A change locks an instance before its template, except a rule change sent
// to the template, which locks the template first. Two requests that meet
// in between deadlock, and PostgreSQL fails one of them.

// StopRecurrence ends the series of id, a template or one of its instances,
// under the idempotency key, as Taskwarrior's until:today does: the
// template's until date becomes the start of today, so no instance follows
// the pending one, which stays. It answers with the template.
func (s *Service) StopRecurrence(ctx context.Context, key string, id uuid.UUID) (api.Task, error) {
	return s.mutate(ctx, key, "stop-recurrence", id, nil, func(c *change) (*ent.Task, error) {
		row, err := c.lock(ctx, id)
		if err == nil && row.Parent != nil {
			row, err = c.lock(ctx, *row.Parent)
		}
		if err != nil {
			return nil, err
		}
		if row.Status != task.StatusRecurring {
			return nil, conflict("the task does not repeat")
		}
		zone, err := c.zone(ctx)
		if err != nil {
			return nil, err
		}
		today, err := domain.ResolveDate("today", c.now.In(zone))
		if err != nil {
			return nil, err
		}
		if row.Until != nil && !row.Until.After(today) {
			return nil, conflict("the series is already stopped")
		}
		return c.update(ctx, "stop-recurrence", row, func(t *api.Task, _ time.Time) error {
			t.Until = instantOf(&today)
			return nil
		})
	})
}

// startSeries applies edit, which sets a rule, to row, a pending task that
// does not repeat, and makes it the first instance of a new series. The
// template repeats the task's fields, its due date included.
func (c *change) startSeries(ctx context.Context, row *ent.Task, edit func(t *api.Task, now time.Time) error) (*ent.Task, error) {
	templateID := uuid.New()
	instance, err := c.updatePending(ctx, "modify", row, func(t *api.Task, now time.Time) error {
		if err := edit(t, now); err != nil {
			return err
		}
		if t.Due == nil {
			return errNoFirstDue
		}
		t.Parent = templateID.String()
		return nil
	})
	if err != nil {
		return nil, err
	}
	template, err := repeat(c.tx.Task.Create(), instance).SetID(templateID).SetStatus(task.StatusRecurring).
		SetEntry(c.now).SetModified(c.now).
		SetNillableDue(instance.Due).SetNillableScheduled(instance.Scheduled).SetNillableWait(instance.Wait).Save(ctx)
	if err != nil {
		return nil, err
	}
	return instance, c.recordCreate(ctx, "create", template)
}

// changeInstanceSeries applies edit to row, a pending instance, and gives its
// series rule.
func (c *change) changeInstanceSeries(ctx context.Context, row *ent.Task, rule string, edit func(t *api.Task, now time.Time) error) (*ent.Task, error) {
	instance, err := c.updatePending(ctx, "modify", row, edit)
	if err != nil {
		return nil, err
	}
	template, err := c.lock(ctx, *row.Parent)
	if err != nil {
		return nil, err
	}
	_, err = c.changeSeries(ctx, template, rule, instance.ID)
	return instance, err
}

// changeSeries gives template, and its pending instances other than except,
// rule, and returns the template.
func (c *change) changeSeries(ctx context.Context, template *ent.Task, rule string, except uuid.UUID) (*ent.Task, error) {
	setRule := func(t *api.Task, _ time.Time) error {
		t.Recurrence = rule
		return nil
	}
	saved, err := c.update(ctx, "modify", template, setRule)
	if err != nil {
		return nil, err
	}
	instances, err := c.tx.Task.Query().
		Where(task.ParentEQ(template.ID), task.StatusEQ(task.StatusPending), task.IDNEQ(except)).ForUpdate().All(ctx)
	if err != nil {
		return nil, err
	}
	for _, instance := range instances {
		if _, err := c.update(ctx, "modify", instance, setRule); err != nil {
			return nil, err
		}
	}
	return saved, nil
}

// next creates the instance that follows closed, if closed is an instance of
// a series that goes on: its template still repeats, it has no other
// pending instance, and its next occurrence comes before its until date.
// The template stays locked, so a stop or a rule change applies before or
// after, not during.
func (c *change) next(ctx context.Context, closed *ent.Task) error {
	if closed.Parent == nil {
		return nil
	}
	template, err := c.tx.Task.Query().
		Where(task.ID(*closed.Parent), task.StatusEQ(task.StatusRecurring), task.DueNotNil()).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	if open, err := c.tx.Task.Query().Where(task.ParentEQ(template.ID), task.StatusEQ(task.StatusPending)).Exist(ctx); err != nil || open {
		return err
	}
	zone, err := c.zone(ctx)
	if err != nil {
		return err
	}
	// The next instance comes after the closed one, or after now if it had
	// no due date, and after the first, which is due at the anchor.
	after := c.now
	if closed.Due != nil {
		after = *closed.Due
	}
	if after.Before(*template.Due) {
		after = *template.Due
	}
	due, ok := domain.NextInstanceDue(*template.Due, template.Recurrence, after, c.now.In(zone))
	if !ok || (template.Until != nil && !due.Before(*template.Until)) {
		return nil
	}
	instance, err := c.createInstance(ctx, template, instant(due),
		moved(template.Scheduled, *template.Due, due), moved(template.Wait, *template.Due, due))
	if err != nil {
		return err
	}
	return c.recordCreate(ctx, "recur", instance)
}

// createInstance adds a pending instance of template, due at due.
func (c *change) createInstance(ctx context.Context, template *ent.Task, due time.Time, scheduled, wait *time.Time) (*ent.Task, error) {
	return repeat(c.tx.Task.Create(), template).SetStatus(task.StatusPending).SetParent(template.ID).
		SetEntry(c.now).SetModified(c.now).SetDue(due).SetNillableScheduled(scheduled).SetNillableWait(wait).Save(ctx)
}

// repeat sets on create the fields a series repeats from source, other than
// its dates. Taskwarrior also copies annotations, dependencies, and
// unrecognized fields; Momentum's series do not.
func repeat(create *ent.TaskCreate, source *ent.Task) *ent.TaskCreate {
	create.SetUserID(source.UserID).SetDescription(source.Description).SetProject(source.Project).
		SetPriority(source.Priority).SetNillableEstimateMinutes(source.EstimateMinutes).SetRecurrence(source.Recurrence)
	if len(source.Tags) > 0 {
		create.SetTags(source.Tags)
	}
	return create
}

// moved returns value, a template's scheduled or wait date, moved with its
// due date from anchor to due, as Taskwarrior moves it.
func moved(value *time.Time, anchor, due time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := instant(domain.MoveWithDue(*value, anchor, due))
	return &result
}

// zone returns the user's time zone, which anchors their series. It is UTC
// until an app sets it with SetTimeZone.
func (c *change) zone(ctx context.Context) (*time.Location, error) {
	userID, _ := viewer.UserID(ctx)
	user, err := c.tx.User.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return time.LoadLocation(user.TimeZone)
}

// SetTimeZone answers api.RouteSetTimeZone: name, an IANA time zone, becomes
// the user's, and their series follow it from their next change.
func (s *Service) SetTimeZone(ctx context.Context, name string) (api.TimeZone, error) {
	if _, err := time.LoadLocation(name); err != nil || name == "" || name == "Local" {
		return api.TimeZone{}, invalid("timeZone", "timeZone must be an IANA time zone, such as Europe/Lisbon")
	}
	userID, _ := viewer.UserID(ctx)
	if err := s.client.User.UpdateOneID(userID).SetTimeZone(name).Exec(ctx); err != nil {
		return api.TimeZone{}, err
	}
	return api.TimeZone{TimeZone: name}, nil
}
