package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/privacy"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/rule"
)

// TaskEvent records one change to a task, written in the same transaction:
// the task before and after, and the session that made it (backend plan D7).
// Before is absent for a created task. TaskID is not a foreign key, so the
// history outlives the task. The events of one request share a ChangeID, as
// completing an instance and creating the next one do, so undo reverts them
// together (T11).
type TaskEvent struct {
	ent.Schema
}

func (TaskEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.UUID{}).Immutable(),
		field.Int("session_id").Optional().Nillable().Immutable(),
		field.UUID("task_id", uuid.UUID{}).Immutable(),
		field.UUID("change_id", uuid.UUID{}).Immutable(),
		field.String("action").NotEmpty().Immutable(),
		field.JSON("before", json.RawMessage{}).Optional().Immutable(),
		field.JSON("after", json.RawMessage{}).Optional().Immutable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (TaskEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("task_events").Field("user_id").Unique().Required().Immutable(),
		edge.From("session", Session.Type).Ref("task_events").Field("session_id").Unique().Immutable(),
	}
}

func (TaskEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "task_id"),
		index.Fields("session_id"),
		index.Fields("change_id"),
	}
}

// Policy limits every read and write to the signed-in user's rows.
func (TaskEvent) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{rule.CreateAsViewer(), rule.FilterByViewer()},
		Query:    privacy.QueryPolicy{rule.FilterByViewer()},
	}
}
