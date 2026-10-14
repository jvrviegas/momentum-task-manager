// Command migration-diff writes the migration that brings the committed
// migrations up to the store's ent schema. It replays them on
// DEV_DATABASE_URL, an empty database on a local PostgreSQL server, and
// writes the difference to migrations/<version>_<name>.sql. Run it from
// backend/, and review the file before committing it:
//
//	DEV_DATABASE_URL=postgres://… go run ./cmd/migration-diff <name>
package main

import (
	"context"
	"fmt"
	"os"

	"ariga.io/atlas/sql/migrate"
	"entgo.io/ent/dialect/sql/schema"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: migration-diff <name>")
		os.Exit(2)
	}
	if err := run(context.Background(), os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "migration-diff:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, name string) error {
	url := os.Getenv("DEV_DATABASE_URL")
	if err := store.CheckLocal(url); err != nil {
		return err
	}
	db, err := store.Open(url)
	if err != nil {
		return err
	}
	defer db.Close()
	dir, err := migrate.NewLocalDir("migrations")
	if err != nil {
		return err
	}
	return store.NewClient(db).Schema.NamedDiff(ctx, name, store.DiffOptions(dir, schema.ModeReplay)...)
}
