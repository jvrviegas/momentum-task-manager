package store_test

import (
	"errors"
	"testing"

	"ariga.io/atlas/sql/migrate"
	entschema "entgo.io/ent/dialect/sql/schema"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/storetest"
	"github.com/jvrviegas/momentum-task-manager/backend/migrations"
)

func TestMigrationsBuildTheEntSchemaOnAnEmptyDatabase(t *testing.T) {
	db := storetest.EmptyDatabase(t)
	ctx := t.Context()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// What is left to change from the migrated database to the ent schema.
	missing := func() error {
		options := append(store.DiffOptions(&migrate.MemDir{}, entschema.ModeInspect), entschema.WithErrNoPlan(true))
		return store.NewClient(db).Schema.NamedDiff(ctx, "missing", options...)
	}
	if err := missing(); !errors.Is(err, migrate.ErrNoPlan) {
		t.Fatalf("the migrations do not build the ent schema: %v", err)
	}

	// Every file is recorded, and running again changes nothing.
	dir, err := migrations.Dir()
	if err != nil {
		t.Fatal(err)
	}
	files, err := dir.Files()
	if err != nil || len(files) == 0 {
		t.Fatalf("migration files: %d, %v", len(files), err)
	}
	var recorded int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM momentum_migrations.revisions").Scan(&recorded); err != nil || recorded != len(files) {
		t.Fatalf("recorded %d revisions (%v), want %d", recorded, err, len(files))
	}
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	// The comparison notices a difference.
	if _, err := db.ExecContext(ctx, `ALTER TABLE tasks DROP COLUMN extra`); err != nil {
		t.Fatal(err)
	}
	if err := missing(); err != nil {
		t.Fatalf("NamedDiff after dropping a column: %v", err)
	}
}
