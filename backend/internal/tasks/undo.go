package tasks

import (
	"context"
	"encoding/json"
	"slices"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/taskevent"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

var (
	// errNothingToUndo refuses an undo from a device that has made no change
	// since its last undo, or none at all.
	errNothingToUndo = conflict("there is no change to undo")
	// errChanged refuses an undo when the task changed after the change it
	// would revert.
	errChanged = conflict("the task changed since; undo is not available")
	// errMigrationUndo refuses to undo a project migration, as ADR 0001 has
	// no batch undo: the apps' catalog would keep the new project.
	errMigrationUndo = conflict("a project migration cannot be undone")
)

// Undo reverts the latest change the signed-in device made, if none of its
// tasks has changed since (backend plan D7), under the idempotency key. A
// change can reach several tasks, such as completing an instance and
// creating the next one; undo reverts all of them or none. Undo is one step:
// after an undo, the device has nothing to undo until its next change. Nor
// after a project migration, which undo refuses (ADR 0001 has no batch
// undo). Undoing a create removes the task, as Taskwarrior's undo does. The
// answer is the task the change answered with, deleted if it was created.
func (s *Service) Undo(ctx context.Context, key string) (api.Task, error) {
	return s.mutate(ctx, key, "undo", uuid.Nil, nil, func(c *change) (*ent.Task, error) {
		session, ok := auth.FromContext(ctx)
		if !ok {
			return nil, errNothingToUndo
		}
		last, err := c.tx.TaskEvent.Query().Where(taskevent.SessionID(session.ID)).Order(taskevent.ByID(sql.OrderDesc())).First(ctx)
		if ent.IsNotFound(err) {
			return nil, errNothingToUndo
		} else if err != nil {
			return nil, err
		}
		switch last.Action {
		case "undo":
			return nil, errNothingToUndo
		case migrateAction:
			return nil, errMigrationUndo
		}
		// The change's first event is the task it answered with.
		events, err := c.tx.TaskEvent.Query().Where(taskevent.ChangeID(last.ChangeID)).Order(taskevent.ByID()).All(ctx)
		if err != nil {
			return nil, err
		}
		ids := make([]uuid.UUID, len(events))
		for i, event := range events {
			ids[i] = event.TaskID
		}
		// Every change locks its tasks and writes an event for each, so with
		// the locks held, a task's latest event is its latest change.
		rows, err := c.tx.Task.Query().Where(task.IDIn(ids...)).Order(task.ByID()).ForUpdate().All(ctx)
		if err != nil {
			return nil, err
		}
		if len(rows) != len(events) {
			return nil, errChanged
		}
		for _, event := range events {
			latest, err := c.tx.TaskEvent.Query().Where(taskevent.TaskID(event.TaskID)).Order(taskevent.ByID(sql.OrderDesc())).FirstID(ctx)
			if err != nil {
				return nil, err
			}
			if latest != event.ID {
				return nil, errChanged
			}
		}

		var answer *ent.Task
		for _, event := range events {
			row := rows[slices.IndexFunc(rows, func(row *ent.Task) bool { return row.ID == event.TaskID })]
			reverted, err := c.revert(ctx, row, event)
			if err != nil {
				return nil, err
			}
			if answer == nil {
				answer = reverted
			}
		}
		return answer, nil
	})
}

// revert returns row to its state before event and records the undo. A
// created task is removed, and the result is its last state, deleted.
func (c *change) revert(ctx context.Context, row *ent.Task, event *ent.TaskEvent) (*ent.Task, error) {
	current := toAPI(row)
	if event.Before == nil {
		if err := c.tx.Task.DeleteOne(row).Exec(ctx); err != nil {
			return nil, err
		}
		row.Status = task.StatusDeleted
		return row, c.record(ctx, "undo", row.ID, &current, nil)
	}
	var reverted api.Task
	if err := json.Unmarshal(event.Before, &reverted); err != nil {
		return nil, err
	}
	saved, err := save(ctx, c.tx.Task.UpdateOne(row), reverted)
	if err != nil {
		return nil, err
	}
	result := toAPI(saved)
	return saved, c.record(ctx, "undo", row.ID, &current, &result)
}
