package store_test

import (
	"testing"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store"
)

func TestCheckLocalAcceptsOnlyThisMachine(t *testing.T) {
	for _, url := range []string{
		"postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable",
		"postgres://127.0.0.1/momentum",
		"postgres://[::1]:5432/momentum",
		"postgres://localhost:5432,127.0.0.1:5433/momentum",
		"host=/tmp dbname=momentum",
	} {
		if err := store.CheckLocal(url); err != nil {
			t.Errorf("CheckLocal(%q) = %v, want nil", url, err)
		}
	}
	for _, url := range []string{
		"",
		"postgres://user:secret@ep-cool-name-123456.eu-central-1.aws.neon.tech/neondb?sslmode=require",
		"postgres://user:secret@ep-cool-name-123456-pooler.eu-central-1.aws.neon.tech/neondb?sslmode=require",
		"host=db.example.com dbname=momentum",
		"postgres://localhost:5432,db.example.com:5432/momentum",
		"postgres://10.0.0.5/momentum",
		"postgres://localhost.example.com/momentum",
		"not a url ://",
	} {
		if err := store.CheckLocal(url); err == nil {
			t.Errorf("CheckLocal(%q) = nil, want an error", url)
		}
	}
}
