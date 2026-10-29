package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/privacy"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/rule"
)

// CalendarConnection is a user's Google Calendar connection (ADR 0003, D1),
// at most one per user. Besides the Google account's subject and email it
// keeps only the refresh token, sealed under CALENDAR_TOKEN_KEY (D5): never an
// event or a title. Status is needs_reconnect while Google refuses the token.
type CalendarConnection struct {
	ent.Schema
}

func (CalendarConnection) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.UUID{}).Immutable(),
		// Subject is Google's stable identifier for the account.
		field.String("subject").NotEmpty(),
		field.String("email").NotEmpty(),
		field.Bytes("sealed_token").NotEmpty().Sensitive(),
		field.Enum("status").Values("connected", "needs_reconnect").Default("connected"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (CalendarConnection) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("calendar_connection").Field("user_id").Unique().Required().Immutable(),
	}
}

// Policy limits every read and write to the signed-in user's row.
func (CalendarConnection) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{rule.CreateAsViewer(), rule.FilterByViewer()},
		Query:    privacy.QueryPolicy{rule.FilterByViewer()},
	}
}
