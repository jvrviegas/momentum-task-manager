// Package storetest gives each test its own database on the local PostgreSQL
// server that MOMENTUM_TEST_DATABASE_URL names, and drops it when the test
// ends. Tests skip when the variable is unset and fail when it names a server
// that is not local.
package storetest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
)

// Variable names the server's URL, for example
// postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable.
const Variable = "MOMENTUM_TEST_DATABASE_URL"

// EmptyDatabase returns a connection to a new database with nothing in it.
func EmptyDatabase(t testing.TB) *sql.DB {
	t.Helper()
	url := os.Getenv(Variable)
	if url == "" {
		t.Skipf("%s is unset; set it to a local PostgreSQL server to run the store tests", Variable)
	}
	if err := store.CheckLocal(url); err != nil {
		t.Fatalf("%s: %v", Variable, err)
	}
	config, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	admin := stdlib.OpenDB(*config)
	t.Cleanup(func() { admin.Close() })
	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	name := "momentum_test_" + hex.EncodeToString(suffix)
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop %s: %v", name, err)
		}
	})
	config.Database = name
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { db.Close() })
	return db
}

// Client returns a client over a new database with every migration applied.
func Client(t testing.TB) *ent.Client {
	t.Helper()
	db := EmptyDatabase(t)
	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.NewClient(db)
}
