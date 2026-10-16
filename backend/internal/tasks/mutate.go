package tasks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/idempotencykey"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

const (
	// maxKeyLength bounds an idempotency key.
	maxKeyLength = 255
	// keyLifetime is how long a key is remembered. Clients retry within
	// seconds; a key sent again after this applies its mutation again.
	keyLifetime = 24 * time.Hour
)

// Create adds a pending task, under the idempotency key. With a recurrence,
// it adds a series: a recurring template and its first instance, due when
// the template is, and answers with the instance (backend plan D5).
func (s *Service) Create(ctx context.Context, key string, input api.CreateTaskRequest) (api.Task, error) {
	rule, err := validateCreate(input)
	if err != nil {
		return api.Task{}, err
	}
	return s.mutate(ctx, key, "create", uuid.Nil, input, func(c *change) (*ent.Task, error) {
		userID, _ := viewer.UserID(ctx)
		status := task.StatusPending
		if rule != "" {
			status = task.StatusRecurring
		}
		create := c.tx.Task.Create().SetUserID(userID).SetStatus(status).
			SetDescription(input.Description).SetProject(input.Project).SetPriority(input.Priority).
			SetEntry(c.now).SetModified(c.now).SetRecurrence(rule).
			SetNillableDue(instantOf(input.Due)).SetNillableScheduled(instantOf(input.Scheduled))
		if input.EstimateMinutes != 0 {
			create.SetEstimateMinutes(input.EstimateMinutes)
		}
		if tags := uniqueTags(input.Tags); len(tags) > 0 {
			create.SetTags(tags)
		}
		created, err := create.Save(ctx)
		if err != nil {
			return nil, err
		}
		if rule == "" {
			return created, c.recordCreate(ctx, "create", created)
		}
		instance, err := c.createInstance(ctx, created, *created.Due, created.Scheduled, created.Wait)
		if err != nil {
			return nil, err
		}
		// The answer's event comes first, as undo expects.
		if err := c.recordCreate(ctx, "create", instance); err != nil {
			return nil, err
		}
		return instance, c.recordCreate(ctx, "create", created)
	})
}

// Modify changes the fields input names on a pending task, as a
// domain.TaskDiff does, under the idempotency key. A recurrence turns a task
// into the first instance of a new series, or changes the rule of the series
// an instance or template belongs to.
func (s *Service) Modify(ctx context.Context, key string, id uuid.UUID, input api.ModifyTaskRequest) (api.Task, error) {
	rule, err := validateModify(input)
	if err != nil {
		return api.Task{}, err
	}
	edit := func(t *api.Task, _ time.Time) error {
		t.Description = changed(t.Description, input.Description)
		t.Project = changed(t.Project, input.Project)
		t.Priority = changed(t.Priority, input.Priority)
		t.EstimateMinutes = changed(t.EstimateMinutes, input.EstimateMinutes)
		t.Due = changedTime(t.Due, input.Due)
		t.Scheduled = changedTime(t.Scheduled, input.Scheduled)
		if input.Tags != nil {
			kept := slices.DeleteFunc(slices.Clone(t.Tags), func(tag string) bool { return slices.Contains(input.Tags.Remove, tag) })
			t.Tags = uniqueTags(append(kept, input.Tags.Add...))
		}
		if rule != "" {
			t.Recurrence = rule
		}
		return nil
	}
	return s.mutate(ctx, key, "modify", id, input, func(c *change) (*ent.Task, error) {
		row, err := c.lock(ctx, id)
		if err != nil {
			return nil, err
		}
		switch {
		case rule == "":
			return c.updatePending(ctx, "modify", row, edit)
		case row.Status == task.StatusRecurring && onlyRecurrence(input):
			return c.changeSeries(ctx, row, rule, uuid.Nil)
		case row.Parent != nil:
			return c.changeInstanceSeries(ctx, row, rule, edit)
		default:
			return c.startSeries(ctx, row, edit)
		}
	})
}

// Complete completes a pending task and stops it, as Taskwarrior's done
// does, under the idempotency key. Completing an instance creates the next
// one of its series.
func (s *Service) Complete(ctx context.Context, key string, id uuid.UUID) (api.Task, error) {
	return s.close(ctx, key, "complete", id, func(t *api.Task, now time.Time) error {
		t.Status, t.End, t.Start = api.StatusCompleted, &now, nil
		return nil
	})
}

// Delete deletes a pending task under the idempotency key. Like
// Taskwarrior's delete, it keeps the start time of a started task. Deleting
// an instance creates the next one of its series.
func (s *Service) Delete(ctx context.Context, key string, id uuid.UUID) (api.Task, error) {
	return s.close(ctx, key, "delete", id, func(t *api.Task, now time.Time) error {
		t.Status, t.End = api.StatusDeleted, &now
		return nil
	})
}

// close ends the pending task id with edit, recorded as action, and creates
// the next instance if the task is one.
func (s *Service) close(ctx context.Context, key, action string, id uuid.UUID, edit func(t *api.Task, now time.Time) error) (api.Task, error) {
	return s.mutate(ctx, key, action, id, nil, func(c *change) (*ent.Task, error) {
		row, err := c.lock(ctx, id)
		if err != nil {
			return nil, err
		}
		closed, err := c.updatePending(ctx, action, row, edit)
		if err != nil {
			return nil, err
		}
		return closed, c.next(ctx, closed)
	})
}

// Start starts a pending task under the idempotency key.
func (s *Service) Start(ctx context.Context, key string, id uuid.UUID) (api.Task, error) {
	return s.update(ctx, key, "start", id, nil, func(t *api.Task, now time.Time) error {
		if t.Start != nil {
			return conflict("the task is already started")
		}
		t.Start = &now
		return nil
	})
}

// Stop stops a started task under the idempotency key.
func (s *Service) Stop(ctx context.Context, key string, id uuid.UUID) (api.Task, error) {
	return s.update(ctx, key, "stop", id, nil, func(t *api.Task, _ time.Time) error {
		if t.Start == nil {
			return conflict("the task is not started")
		}
		t.Start = nil
		return nil
	})
}

// update applies edit to the pending task id and records it as action.
func (s *Service) update(ctx context.Context, key, action string, id uuid.UUID, input any, edit func(t *api.Task, now time.Time) error) (api.Task, error) {
	return s.mutate(ctx, key, action, id, input, func(c *change) (*ent.Task, error) {
		row, err := c.lock(ctx, id)
		if err != nil {
			return nil, err
		}
		return c.updatePending(ctx, action, row, edit)
	})
}

// change is one mutation in progress: its transaction, its time, and the id
// that every task_events row it writes shares.
type change struct {
	tx  *ent.Client
	now time.Time
	id  uuid.UUID
}

var errNoSuchTask = &api.Error{Code: api.CodeNotFound, Message: "no such task"}

// lock returns the task id, locked until the change ends, so concurrent
// changes to it apply one after the other.
func (c *change) lock(ctx context.Context, id uuid.UUID) (*ent.Task, error) {
	row, err := c.tx.Task.Query().Where(task.ID(id)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return nil, errNoSuchTask
	}
	return row, err
}

// updatePending applies edit to row, which must be pending, as update does.
func (c *change) updatePending(ctx context.Context, action string, row *ent.Task, edit func(t *api.Task, now time.Time) error) (*ent.Task, error) {
	if row.Status != task.StatusPending {
		return nil, conflict(fmt.Sprintf("the task is %s; only pending tasks can change", row.Status))
	}
	return c.update(ctx, action, row, edit)
}

// update applies edit to the locked row, stores the result, and records it
// as action.
func (c *change) update(ctx context.Context, action string, row *ent.Task, edit func(t *api.Task, now time.Time) error) (*ent.Task, error) {
	before := toAPI(row)
	after := before
	if err := edit(&after, c.now); err != nil {
		return nil, err
	}
	after.Modified = c.now
	saved, err := save(ctx, c.tx.Task.UpdateOne(row), after)
	if err != nil {
		return nil, err
	}
	result := toAPI(saved)
	return saved, c.record(ctx, action, row.ID, &before, &result)
}

// save stores the fields of t that a mutation can change.
func save(ctx context.Context, update *ent.TaskUpdateOne, t api.Task) (*ent.Task, error) {
	update.SetDescription(t.Description).SetStatus(task.Status(t.Status)).
		SetProject(t.Project).SetPriority(t.Priority).SetModified(t.Modified).SetRecurrence(t.Recurrence)
	if t.EstimateMinutes != 0 {
		update.SetEstimateMinutes(t.EstimateMinutes)
	} else {
		update.ClearEstimateMinutes()
	}
	setOrClear(t.Due, update.SetDue, update.ClearDue)
	setOrClear(t.Scheduled, update.SetScheduled, update.ClearScheduled)
	setOrClear(t.Start, update.SetStart, update.ClearStart)
	setOrClear(t.Until, update.SetUntil, update.ClearUntil)
	setOrClear(t.End, update.SetEnd, update.ClearEnd)
	if len(t.Tags) > 0 {
		update.SetTags(t.Tags)
	} else {
		update.ClearTags()
	}
	if parent, err := uuid.Parse(t.Parent); err == nil {
		update.SetParent(parent)
	} else {
		update.ClearParent()
	}
	return update.Save(ctx)
}

// mutate runs change once per idempotency key, as once does, and answers
// with the task change returns, with its urgency.
func (s *Service) mutate(ctx context.Context, key, action string, id uuid.UUID, input any, run func(c *change) (*ent.Task, error)) (api.Task, error) {
	return once(ctx, s, key, action, id, input, func(c *change) (api.Task, error) {
		saved, err := run(c)
		if err != nil {
			return api.Task{}, err
		}
		pending, err := pendingTasks(ctx, c.tx)
		if err != nil {
			return api.Task{}, err
		}
		return withUrgency([]*ent.Task{saved}, pending, c.now)[0], nil
	})
}

// once runs change once per idempotency key. In one transaction, it claims
// key for the user, runs change, and stores its answer. A request that sends
// the key again gets that answer instead. Claiming a key waits for a
// transaction that has claimed it already, so a retry sent while the first
// attempt is still running does not run change again. A key sent with a
// different request is a conflict. A failed change claims nothing.
func once[T any](ctx context.Context, s *Service, key, action string, id uuid.UUID, input any, run func(c *change) (T, error)) (T, error) {
	var none T
	if key == "" || len(key) > maxKeyLength {
		return none, invalid("", fmt.Sprintf("send an %s header of 1 to %d characters", api.IdempotencyKeyHeader, maxKeyLength))
	}
	request, err := json.Marshal(struct {
		Action string
		Task   uuid.UUID
		Input  any
	}{action, id, input})
	if err != nil {
		return none, err
	}
	fingerprint := sha256.Sum256(request)
	now := s.clock()
	userID, _ := viewer.UserID(ctx)

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return none, err
	}
	defer tx.Rollback()
	if _, err := tx.IdempotencyKey.Delete().Where(idempotencykey.CreatedAtLT(now.Add(-keyLifetime))).Exec(ctx); err != nil {
		return none, err
	}
	claim, err := tx.IdempotencyKey.Create().SetUserID(userID).SetKey(key).SetFingerprint(fingerprint[:]).SetCreatedAt(now).Save(ctx)
	if ent.IsConstraintError(err) {
		_ = tx.Rollback()
		return replay[T](ctx, s, key, fingerprint[:])
	} else if err != nil {
		return none, err
	}
	answer, err := run(&change{tx: tx.Client(), now: now, id: uuid.New()})
	if err != nil {
		return none, err
	}
	response, err := json.Marshal(answer)
	if err != nil {
		return none, err
	}
	if err := tx.IdempotencyKey.UpdateOne(claim).SetResponse(response).Exec(ctx); err != nil {
		return none, err
	}
	return answer, tx.Commit()
}

// replay returns the answer stored under key, if the same request claimed it.
func replay[T any](ctx context.Context, s *Service, key string, fingerprint []byte) (T, error) {
	var answer T
	claim, err := s.client.IdempotencyKey.Query().Where(idempotencykey.Key(key)).Only(ctx)
	if err != nil {
		return answer, err
	}
	if !bytes.Equal(claim.Fingerprint, fingerprint) {
		return answer, conflict(fmt.Sprintf("this %s was already used for a different request", api.IdempotencyKeyHeader))
	}
	err = json.Unmarshal(claim.Response, &answer)
	return answer, err
}

// recordCreate records the creation of row as action.
func (c *change) recordCreate(ctx context.Context, action string, row *ent.Task) error {
	after := toAPI(row)
	return c.record(ctx, action, row.ID, nil, &after)
}

// record writes the task_events row for this change to the task id, with the
// session that made it. Before is nil for a created task, and after for an
// undone create.
func (c *change) record(ctx context.Context, action string, id uuid.UUID, before, after *api.Task) error {
	userID, _ := viewer.UserID(ctx)
	event := c.tx.TaskEvent.Create().SetUserID(userID).SetTaskID(id).SetChangeID(c.id).SetAction(action).SetCreatedAt(c.now)
	if session, ok := auth.FromContext(ctx); ok {
		event.SetSessionID(session.ID)
	}
	if before != nil {
		data, err := json.Marshal(before)
		if err != nil {
			return err
		}
		event.SetBefore(data)
	}
	if after != nil {
		data, err := json.Marshal(after)
		if err != nil {
			return err
		}
		event.SetAfter(data)
	}
	return event.Exec(ctx)
}

// validateCreate checks input and returns its recurrence rule in canonical
// form, or "" for a task that does not repeat.
func validateCreate(input api.CreateTaskRequest) (string, error) {
	if strings.TrimSpace(input.Description) == "" {
		return "", invalid("description", "description cannot be empty")
	}
	rule, err := checkRecurrence(input.Recurrence)
	if err != nil {
		return "", err
	}
	if rule != "" && input.Due == nil {
		return "", errNoFirstDue
	}
	if err := checkPriority(input.Priority); err != nil {
		return "", err
	}
	if input.EstimateMinutes != 0 {
		if err := checkEstimate(input.EstimateMinutes); err != nil {
			return "", err
		}
	}
	return rule, checkTags(input.Tags)
}

// validateModify checks input and returns the recurrence rule it sets in
// canonical form, or "" if it sets none.
func validateModify(input api.ModifyTaskRequest) (string, error) {
	if onlyRecurrence(input) && input.Recurrence.IsZero() {
		return "", invalid("", "the request changes nothing")
	}
	if input.Description.Kind == domain.Clear || (input.Description.Kind == domain.Set && strings.TrimSpace(input.Description.Value) == "") {
		return "", invalid("description", "description cannot be empty")
	}
	var rule string
	if input.Recurrence.Kind != domain.Unchanged {
		var err error
		if rule, err = checkRecurrence(input.Recurrence.Value); err != nil {
			return "", err
		}
		// As in Taskwarrior, which refuses to remove a recurrence.
		if rule == "" {
			return "", invalid("recurrence", "a recurrence cannot be removed; stop the series instead")
		}
	}
	if input.Priority.Kind == domain.Set {
		if err := checkPriority(input.Priority.Value); err != nil {
			return "", err
		}
	}
	if input.EstimateMinutes.Kind == domain.Set {
		if err := checkEstimate(input.EstimateMinutes.Value); err != nil {
			return "", err
		}
	}
	if input.Tags != nil {
		return rule, checkTags(input.Tags.Add)
	}
	return rule, nil
}

// onlyRecurrence reports whether input changes no field but the recurrence.
func onlyRecurrence(input api.ModifyTaskRequest) bool {
	return input.Description.IsZero() && input.Project.IsZero() && input.Priority.IsZero() &&
		input.Due.IsZero() && input.Scheduled.IsZero() && input.EstimateMinutes.IsZero() &&
		(input.Tags == nil || len(input.Tags.Add)+len(input.Tags.Remove) == 0)
}

var errNoFirstDue = invalid("due", "recurring tasks need a first due date")

// checkRecurrence returns the canonical form of a recurrence rule, "" for
// none.
func checkRecurrence(value string) (string, error) {
	rule, err := domain.ParseRecurrence(value)
	if err != nil {
		return "", invalid("recurrence", err.Error())
	}
	return rule, nil
}

func checkPriority(priority string) error {
	switch priority {
	case "", "H", "M", "L":
		return nil
	}
	return invalid("priority", "priority must be H, M, L, or empty")
}

func checkEstimate(minutes int) error {
	if err := (domain.Estimate{Minutes: minutes}).ValidateForCreation(); err != nil {
		return invalid("estimateMinutes", err.Error())
	}
	return nil
}

func checkTags(tags []string) error {
	for _, tag := range tags {
		if tag == "" || strings.ContainsFunc(tag, unicode.IsSpace) {
			return invalid("tags", "tags cannot be empty or contain spaces")
		}
	}
	return nil
}

// changed returns value with change applied; a cleared field is empty.
func changed[T any](value T, change api.Change[T]) T {
	switch change.Kind {
	case domain.Set:
		return change.Value
	case domain.Clear:
		var empty T
		return empty
	}
	return value
}

func changedTime(value *time.Time, change api.Change[time.Time]) *time.Time {
	switch change.Kind {
	case domain.Set:
		return instantOf(&change.Value)
	case domain.Clear:
		return nil
	}
	return value
}

func instantOf(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	value := instant(*t)
	return &value
}

func setOrClear[T any](value *T, set func(T) *ent.TaskUpdateOne, clear func() *ent.TaskUpdateOne) {
	if value != nil {
		set(*value)
	} else {
		clear()
	}
}

// uniqueTags returns tags without repeats, in their order.
func uniqueTags(tags []string) []string {
	var unique []string
	for _, tag := range tags {
		if !slices.Contains(unique, tag) {
			unique = append(unique, tag)
		}
	}
	return unique
}

func invalid(field, message string) *api.Error {
	return &api.Error{Code: api.CodeInvalid, Message: message, Field: field}
}

func conflict(message string) *api.Error {
	return &api.Error{Code: api.CodeConflict, Message: message}
}
