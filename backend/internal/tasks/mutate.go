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

// Create adds a pending task, under the idempotency key.
func (s *Service) Create(ctx context.Context, key string, input api.CreateTaskRequest) (api.Task, error) {
	if err := validateCreate(input); err != nil {
		return api.Task{}, err
	}
	return s.mutate(ctx, key, "create", uuid.Nil, input, func(tx *ent.Client, now time.Time) (*ent.Task, error) {
		userID, _ := viewer.UserID(ctx)
		create := tx.Task.Create().SetUserID(userID).SetStatus(task.StatusPending).
			SetDescription(input.Description).SetProject(input.Project).SetPriority(input.Priority).
			SetEntry(now).SetModified(now).
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
		return created, record(ctx, tx, now, "create", created.ID, nil, toAPI(created))
	})
}

// Modify changes the fields input names on a pending task, as a
// domain.TaskDiff does, under the idempotency key.
func (s *Service) Modify(ctx context.Context, key string, id uuid.UUID, input api.ModifyTaskRequest) (api.Task, error) {
	if err := validateModify(input); err != nil {
		return api.Task{}, err
	}
	return s.update(ctx, key, "modify", id, input, func(t *api.Task, _ time.Time) error {
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
		return nil
	})
}

// Complete completes a pending task and stops it, as Taskwarrior's done
// does, under the idempotency key.
func (s *Service) Complete(ctx context.Context, key string, id uuid.UUID) (api.Task, error) {
	return s.update(ctx, key, "complete", id, nil, func(t *api.Task, now time.Time) error {
		t.Status, t.End, t.Start = api.StatusCompleted, &now, nil
		return nil
	})
}

// Delete deletes a pending task under the idempotency key. Like
// Taskwarrior's delete, it keeps the start time of a started task.
func (s *Service) Delete(ctx context.Context, key string, id uuid.UUID) (api.Task, error) {
	return s.update(ctx, key, "delete", id, nil, func(t *api.Task, now time.Time) error {
		t.Status, t.End = api.StatusDeleted, &now
		return nil
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

// update applies edit to the pending task id and records it as action. The
// task stays locked until the transaction ends, so concurrent changes apply
// one after the other.
func (s *Service) update(ctx context.Context, key, action string, id uuid.UUID, input any, edit func(t *api.Task, now time.Time) error) (api.Task, error) {
	return s.mutate(ctx, key, action, id, input, func(tx *ent.Client, now time.Time) (*ent.Task, error) {
		row, err := tx.Task.Query().Where(task.ID(id)).ForUpdate().Only(ctx)
		if ent.IsNotFound(err) {
			return nil, &api.Error{Code: api.CodeNotFound, Message: "no such task"}
		} else if err != nil {
			return nil, err
		}
		if row.Status != task.StatusPending {
			return nil, conflict(fmt.Sprintf("the task is %s; only pending tasks can change", row.Status))
		}
		before := toAPI(row)
		after := before
		if err := edit(&after, now); err != nil {
			return nil, err
		}
		after.Modified = now
		update := tx.Task.UpdateOne(row).
			SetDescription(after.Description).SetStatus(task.Status(after.Status)).
			SetProject(after.Project).SetPriority(after.Priority).SetModified(after.Modified)
		if after.EstimateMinutes != 0 {
			update.SetEstimateMinutes(after.EstimateMinutes)
		} else {
			update.ClearEstimateMinutes()
		}
		setOrClear(after.Due, update.SetDue, update.ClearDue)
		setOrClear(after.Scheduled, update.SetScheduled, update.ClearScheduled)
		setOrClear(after.Start, update.SetStart, update.ClearStart)
		setOrClear(after.End, update.SetEnd, update.ClearEnd)
		if len(after.Tags) > 0 {
			update.SetTags(after.Tags)
		} else {
			update.ClearTags()
		}
		saved, err := update.Save(ctx)
		if err != nil {
			return nil, err
		}
		return saved, record(ctx, tx, now, action, id, &before, toAPI(saved))
	})
}

// mutate runs change once per idempotency key. In one transaction, it claims
// key for the user, runs change, and stores the answer: the changed task
// with its urgency. A request that sends the key again gets that answer
// instead. Claiming a key waits for a transaction that has claimed it
// already, so a retry sent while the first attempt is still running does not
// run change again. A key sent with a different request is a conflict. A
// failed change claims nothing.
func (s *Service) mutate(ctx context.Context, key, action string, id uuid.UUID, input any, change func(tx *ent.Client, now time.Time) (*ent.Task, error)) (api.Task, error) {
	if key == "" || len(key) > maxKeyLength {
		return api.Task{}, invalid("", fmt.Sprintf("send an %s header of 1 to %d characters", api.IdempotencyKeyHeader, maxKeyLength))
	}
	request, err := json.Marshal(struct {
		Action string
		Task   uuid.UUID
		Input  any
	}{action, id, input})
	if err != nil {
		return api.Task{}, err
	}
	fingerprint := sha256.Sum256(request)
	now := s.clock()
	userID, _ := viewer.UserID(ctx)

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return api.Task{}, err
	}
	defer tx.Rollback()
	if _, err := tx.IdempotencyKey.Delete().Where(idempotencykey.CreatedAtLT(now.Add(-keyLifetime))).Exec(ctx); err != nil {
		return api.Task{}, err
	}
	claim, err := tx.IdempotencyKey.Create().SetUserID(userID).SetKey(key).SetFingerprint(fingerprint[:]).SetCreatedAt(now).Save(ctx)
	if ent.IsConstraintError(err) {
		_ = tx.Rollback()
		return s.replay(ctx, key, fingerprint[:])
	} else if err != nil {
		return api.Task{}, err
	}
	saved, err := change(tx.Client(), now)
	if err != nil {
		return api.Task{}, err
	}
	pending, err := pendingTasks(ctx, tx.Client())
	if err != nil {
		return api.Task{}, err
	}
	answer := withUrgency([]*ent.Task{saved}, pending, now)[0]
	response, err := json.Marshal(answer)
	if err != nil {
		return api.Task{}, err
	}
	if err := tx.IdempotencyKey.UpdateOne(claim).SetResponse(response).Exec(ctx); err != nil {
		return api.Task{}, err
	}
	return answer, tx.Commit()
}

// replay returns the answer stored under key, if the same request claimed it.
func (s *Service) replay(ctx context.Context, key string, fingerprint []byte) (api.Task, error) {
	claim, err := s.client.IdempotencyKey.Query().Where(idempotencykey.Key(key)).Only(ctx)
	if err != nil {
		return api.Task{}, err
	}
	if !bytes.Equal(claim.Fingerprint, fingerprint) {
		return api.Task{}, conflict(fmt.Sprintf("this %s was already used for a different request", api.IdempotencyKeyHeader))
	}
	var answer api.Task
	err = json.Unmarshal(claim.Response, &answer)
	return answer, err
}

// record writes the task_events row for one change to the task id, with the
// session that made it. Before is nil for a created task.
func record(ctx context.Context, tx *ent.Client, now time.Time, action string, id uuid.UUID, before *api.Task, after api.Task) error {
	userID, _ := viewer.UserID(ctx)
	event := tx.TaskEvent.Create().SetUserID(userID).SetTaskID(id).SetAction(action).SetCreatedAt(now)
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
	data, err := json.Marshal(after)
	if err != nil {
		return err
	}
	return event.SetAfter(data).Exec(ctx)
}

func validateCreate(input api.CreateTaskRequest) error {
	if strings.TrimSpace(input.Description) == "" {
		return invalid("description", "description cannot be empty")
	}
	if input.Recurrence != "" {
		return errRecurrence
	}
	if err := checkPriority(input.Priority); err != nil {
		return err
	}
	if input.EstimateMinutes != 0 {
		if err := checkEstimate(input.EstimateMinutes); err != nil {
			return err
		}
	}
	return checkTags(input.Tags)
}

func validateModify(input api.ModifyTaskRequest) error {
	if input.Description.IsZero() && input.Project.IsZero() && input.Priority.IsZero() &&
		input.Due.IsZero() && input.Scheduled.IsZero() && input.Recurrence.IsZero() &&
		input.EstimateMinutes.IsZero() && (input.Tags == nil || len(input.Tags.Add)+len(input.Tags.Remove) == 0) {
		return invalid("", "the request changes nothing")
	}
	if input.Description.Kind == domain.Clear || (input.Description.Kind == domain.Set && strings.TrimSpace(input.Description.Value) == "") {
		return invalid("description", "description cannot be empty")
	}
	if !input.Recurrence.IsZero() {
		return errRecurrence
	}
	if input.Priority.Kind == domain.Set {
		if err := checkPriority(input.Priority.Value); err != nil {
			return err
		}
	}
	if input.EstimateMinutes.Kind == domain.Set {
		if err := checkEstimate(input.EstimateMinutes.Value); err != nil {
			return err
		}
	}
	if input.Tags != nil {
		return checkTags(input.Tags.Add)
	}
	return nil
}

// errRecurrence refuses recurring series until the server generates their
// instances (backend plan T11).
var errRecurrence = invalid("recurrence", "recurring tasks are not available on the server yet")

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
