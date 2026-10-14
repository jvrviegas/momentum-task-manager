// Package viewer carries the signed-in user through a request's context. The
// store's privacy rules limit task reads and writes to that user.
package viewer

import (
	"context"

	"github.com/google/uuid"
)

type key struct{}

// NewContext returns ctx acting for the user with id userID.
func NewContext(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, key{}, userID)
}

// UserID returns the user ctx acts for, if any.
func UserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(key{}).(uuid.UUID)
	return id, ok
}
