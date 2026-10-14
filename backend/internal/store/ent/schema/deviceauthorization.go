package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// DeviceAuthorization is a pending device sign-in (RFC 8628, backend plan
// D1). The device polls with its device code, of which only the hash is
// stored; the person types the user code in the browser. UserID is set when
// they sign in there.
type DeviceAuthorization struct {
	ent.Schema
}

func (DeviceAuthorization) Fields() []ent.Field {
	return []ent.Field{
		field.Bytes("device_code_hash").Unique().NotEmpty().Sensitive().Immutable(),
		field.String("user_code").Unique().NotEmpty().Immutable(),
		field.UUID("user_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("expires_at").Immutable(),
	}
}

func (DeviceAuthorization) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("device_authorizations").Field("user_id").Unique(),
	}
}
