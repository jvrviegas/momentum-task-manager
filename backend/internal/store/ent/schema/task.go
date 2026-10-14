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
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// Task is a task, a recurring template, or one of its instances, as
// api.Task describes. An imported task keeps its Taskwarrior UUID as its id,
// and the Taskwarrior fields Momentum does not use in Extra.
type Task struct {
	ent.Schema
}

func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("user_id", uuid.UUID{}).Immutable(),
		field.String("description").NotEmpty(),
		field.Enum("status").Values(api.StatusPending, api.StatusCompleted, api.StatusDeleted, api.StatusRecurring),
		field.String("project").Default(""),
		field.String("priority").Default(""),
		field.Int("estimate_minutes").Optional().Nillable().Positive(),
		field.Time("entry").Default(time.Now).Immutable(),
		field.Time("modified").Default(time.Now).UpdateDefault(time.Now),
		field.Time("due").Optional().Nillable(),
		field.Time("scheduled").Optional().Nillable(),
		field.Time("start").Optional().Nillable(),
		field.Time("wait").Optional().Nillable(),
		field.Time("until").Optional().Nillable(),
		field.Time("end").Optional().Nillable(),
		field.Strings("tags").Optional(),
		field.JSON("annotations", []api.Annotation{}).Optional(),
		field.Strings("depends").Optional(),
		field.String("recurrence").Default(""),
		// Parent is the recurring template of an instance.
		field.UUID("parent", uuid.UUID{}).Optional().Nillable(),
		field.JSON("extra", map[string]json.RawMessage{}).Optional(),
	}
}

func (Task) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("tasks").Field("user_id").Unique().Required().Immutable(),
	}
}

func (Task) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("parent"),
	}
}

// Policy limits every read and write to the signed-in user's rows.
func (Task) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{rule.CreateAsViewer(), rule.FilterByViewer()},
		Query:    privacy.QueryPolicy{rule.FilterByViewer()},
	}
}
