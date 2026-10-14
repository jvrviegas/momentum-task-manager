// Package rule holds the store's privacy rules, which keep each user's tasks
// to that user (ADR 0002, P2).
package rule

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/entql"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/privacy"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
)

// FilterByViewer limits queries, updates, and deletes to the viewer's rows,
// and denies them without a viewer.
func FilterByViewer() privacy.QueryMutationRule {
	return privacy.FilterFunc(func(ctx context.Context, f privacy.Filter) error {
		id, ok := viewer.UserID(ctx)
		if !ok {
			return privacy.Denyf("no signed-in user")
		}
		owned, ok := f.(interface{ WhereUserID(entql.ValueP) })
		if !ok {
			return privacy.Denyf("%T has no user_id", f)
		}
		owned.WhereUserID(entql.ValueEQ(id))
		return privacy.Skip
	})
}

// CreateAsViewer denies creating a row that belongs to anyone but the viewer.
func CreateAsViewer() privacy.MutationRule {
	return privacy.MutationRuleFunc(func(ctx context.Context, m ent.Mutation) error {
		if !m.Op().Is(ent.OpCreate) {
			return privacy.Skip
		}
		id, ok := viewer.UserID(ctx)
		if owner, set := m.Field("user_id"); !ok || !set || owner != id {
			return privacy.Denyf("a row may only be created for the signed-in user")
		}
		return privacy.Skip
	})
}
