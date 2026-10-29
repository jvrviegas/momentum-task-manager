package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// User is a person who signs in. TimeZone, an IANA name, anchors recurrence
// (backend plan D5).
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("email").Unique().NotEmpty(),
		field.String("time_zone").Default("UTC"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("calendar_connection", CalendarConnection.Type).Unique(),
		edge.To("calendar_links", CalendarLink.Type),
		edge.To("device_authorizations", DeviceAuthorization.Type),
		edge.To("identities", Identity.Type),
		edge.To("idempotency_keys", IdempotencyKey.Type),
		edge.To("sessions", Session.Type),
		edge.To("tasks", Task.Type),
		edge.To("task_events", TaskEvent.Type),
	}
}
