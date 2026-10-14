// Package migrations holds the database migrations. cmd/migration-diff
// writes each from the store's ent schema; review it before committing, and
// never edit one that has been committed: add another.
package migrations

import (
	"embed"
	"path"

	"ariga.io/atlas/sql/migrate"
)

//go:embed *
var files embed.FS

// Dir returns the migration files and their atlas.sum as an Atlas migration
// directory.
func Dir() (migrate.Dir, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	dir := &migrate.MemDir{}
	for _, entry := range entries {
		name := entry.Name()
		if path.Ext(name) != ".sql" && name != migrate.HashFileName {
			continue
		}
		data, err := files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		if err := dir.WriteFile(name, data); err != nil {
			return nil, err
		}
	}
	return dir, nil
}
