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

// CalendarLink is a pending Google Calendar connect (ADR 0003, C2). The app
// asks for it while signed in; only the hash of the one-time link is stored.
// Opening the link in the browser starts the Google round trip, and the row
// then holds its state hash, PKCE verifier, and nonce. The callback finds the
// row by its state, and the row is deleted once it is used or expired.
//
// The browser has no signed-in user, so finding the row by its hashes needs
// the service to step past the privacy policy for that lookup alone.
type CalendarLink struct {
	ent.Schema
}

func (CalendarLink) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.UUID{}).Immutable(),
		field.Bytes("link_hash").Unique().NotEmpty().Sensitive().Immutable(),
		field.Bytes("state_hash").Optional().Nillable().Unique().Sensitive(),
		field.String("code_verifier").Optional().Sensitive(),
		field.String("nonce").Optional().Sensitive(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("expires_at").Immutable(),
	}
}

func (CalendarLink) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("calendar_links").Field("user_id").Unique().Required().Immutable(),
	}
}

// Policy limits every read and write to the signed-in user's rows.
func (CalendarLink) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{rule.CreateAsViewer(), rule.FilterByViewer()},
		Query:    privacy.QueryPolicy{rule.FilterByViewer()},
	}
}
