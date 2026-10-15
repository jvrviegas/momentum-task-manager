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

// IdempotencyKey is a mutation a user's client has made, under the key it
// sent in api.IdempotencyKeyHeader. It is written in the mutation's
// transaction with the response, so a retry gets that response instead of
// applying the mutation again. Fingerprint identifies the request, so a key
// sent with a different one is refused.
type IdempotencyKey struct {
	ent.Schema
}

func (IdempotencyKey) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.UUID{}).Immutable(),
		field.String("key").NotEmpty().MaxLen(255).Immutable(),
		field.Bytes("fingerprint").NotEmpty().Immutable(),
		field.JSON("response", json.RawMessage{}).Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (IdempotencyKey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("idempotency_keys").Field("user_id").Unique().Required().Immutable(),
	}
}

func (IdempotencyKey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "key").Unique(),
	}
}

// Policy limits every read and write to the signed-in user's rows.
func (IdempotencyKey) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{rule.CreateAsViewer(), rule.FilterByViewer()},
		Query:    privacy.QueryPolicy{rule.FilterByViewer()},
	}
}
