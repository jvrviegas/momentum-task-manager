package tasks

import (
	"context"
	"encoding/json"
	"time"

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
)

// Undo reverts the latest change the signed-in device made, if its task has
// not changed since (backend plan D7), under the idempotency key. Undo is one
// step: after an undo, the device has nothing to undo until its next change.
// Undoing a create removes the task, as Taskwarrior's undo does, and answers
// with the task's last state, deleted.
func (s *Service) Undo(ctx context.Context, key string) (api.Task, error) {
	return s.mutate(ctx, key, "undo", uuid.Nil, nil, func(tx *ent.Client, now time.Time) (*ent.Task, error) {
		session, ok := auth.FromContext(ctx)
		if !ok {
			return nil, errNothingToUndo
		}
		last, err := tx.TaskEvent.Query().Where(taskevent.SessionID(session.ID)).Order(taskevent.ByID(sql.OrderDesc())).First(ctx)
		if ent.IsNotFound(err) {
			return nil, errNothingToUndo
		} else if err != nil {
			return nil, err
		}
		if last.Action == "undo" {
			return nil, errNothingToUndo
		}
		// Every change locks its task and writes an event, so with the lock
		// held, the task's latest event is its latest change.
		row, err := tx.Task.Query().Where(task.ID(last.TaskID)).ForUpdate().Only(ctx)
		if ent.IsNotFound(err) {
			return nil, errChanged
		} else if err != nil {
			return nil, err
		}
		latest, err := tx.TaskEvent.Query().Where(taskevent.TaskID(last.TaskID)).Order(taskevent.ByID(sql.OrderDesc())).FirstID(ctx)
		if err != nil {
			return nil, err
		}
		if latest != last.ID {
			return nil, errChanged
		}

		current := toAPI(row)
		if last.Before == nil {
			if err := tx.Task.DeleteOne(row).Exec(ctx); err != nil {
				return nil, err
			}
			row.Status = task.StatusDeleted
			return row, record(ctx, tx, now, "undo", row.ID, &current, nil)
		}
		var reverted api.Task
		if err := json.Unmarshal(last.Before, &reverted); err != nil {
			return nil, err
		}
		saved, err := save(ctx, tx.Task.UpdateOne(row), reverted)
		if err != nil {
			return nil, err
		}
		result := toAPI(saved)
		return saved, record(ctx, tx, now, "undo", row.ID, &current, &result)
	})
}
