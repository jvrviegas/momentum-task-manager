package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Session is one device's sign-in. Only the hash of its token is stored
// (backend plan D1).
type Session struct {
	ent.Schema
}

func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.UUID{}).Immutable(),
		field.Bytes("token_hash").Unique().NotEmpty().Sensitive().Immutable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("expires_at"),
		field.Time("revoked_at").Optional().Nillable(),
	}
}

func (Session) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("sessions").Field("user_id").Unique().Required().Immutable(),
		edge.To("task_events", TaskEvent.Type),
	}
}
