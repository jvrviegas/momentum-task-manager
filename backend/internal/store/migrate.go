package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/postgres"
	"ariga.io/atlas/sql/schema"
	"entgo.io/ent/dialect"
	entschema "entgo.io/ent/dialect/sql/schema"

	"github.com/jvrviegas/momentum-task-manager/backend/migrations"
)

// Migrate applies the migrations db has not had yet, each file in its own
// transaction, so a failed file leaves nothing behind and runs again next
// time. Applied files are recorded in momentum_migrations.revisions.
func Migrate(ctx context.Context, db *sql.DB) error {
	dir, err := migrations.Dir()
	if err != nil {
		return err
	}
	for _, statement := range []string{
		"CREATE SCHEMA IF NOT EXISTS " + revisionsIdent.Schema,
		`CREATE TABLE IF NOT EXISTS momentum_migrations.revisions (
			version text PRIMARY KEY, description text NOT NULL, type bigint NOT NULL, applied bigint NOT NULL,
			total bigint NOT NULL, executed_at timestamptz NOT NULL, execution_time bigint NOT NULL, error text NOT NULL,
			error_stmt text NOT NULL, hash text NOT NULL, partial_hashes jsonb NOT NULL, operator_version text NOT NULL)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	for {
		done, err := migrateOne(ctx, db, dir)
		if done || err != nil {
			return err
		}
	}
}

// migrateOne applies the next pending file, and reports whether there was
// none.
func migrateOne(ctx context.Context, db *sql.DB, dir migrate.Dir) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	driver, err := postgres.Open(tx)
	if err != nil {
		return false, err
	}
	executor, err := migrate.NewExecutor(driver, dir, revisionTable{tx})
	if err != nil {
		return false, err
	}
	switch err := executor.ExecuteN(ctx, 1); {
	case errors.Is(err, migrate.ErrNoPendingFiles):
		return true, nil
	case err != nil:
		return false, err
	}
	return false, tx.Commit()
}

// DiffOptions compare the ent schema with dir in mode, as migration-diff does
// to write a migration and the tests do to check that none is missing. Drops
// are included, so a migration also removes what the schema no longer has.
func DiffOptions(dir migrate.Dir, mode entschema.Mode) []entschema.MigrateOption {
	return []entschema.MigrateOption{
		entschema.WithDir(dir),
		entschema.WithMigrationMode(mode),
		entschema.WithDialect(dialect.Postgres),
		entschema.WithFormatter(migrate.DefaultFormatter),
		entschema.WithDropColumn(true),
		entschema.WithDropIndex(true),
	}
}

// revisionsIdent is in a schema of its own, so the application's schema holds
// only the application's tables.
var revisionsIdent = &migrate.TableIdent{Schema: "momentum_migrations", Name: "revisions"}

const revisionColumns = "version, description, type, applied, total, executed_at, execution_time, error, error_stmt, hash, partial_hashes, operator_version"

// revisionTable stores Atlas revisions, one column per field.
type revisionTable struct {
	db schema.ExecQuerier
}

func (r revisionTable) Ident() *migrate.TableIdent { return revisionsIdent }

func (r revisionTable) ReadRevisions(ctx context.Context) ([]*migrate.Revision, error) {
	return r.query(ctx, "ORDER BY version")
}

func (r revisionTable) ReadRevision(ctx context.Context, version string) (*migrate.Revision, error) {
	revisions, err := r.query(ctx, "WHERE version = $1", version)
	if err != nil {
		return nil, err
	}
	if len(revisions) == 0 {
		return nil, migrate.ErrRevisionNotExist
	}
	return revisions[0], nil
}

func (r revisionTable) query(ctx context.Context, clause string, args ...any) ([]*migrate.Revision, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+revisionColumns+" FROM momentum_migrations.revisions "+clause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var revisions []*migrate.Revision
	for rows.Next() {
		var revision migrate.Revision
		var partialHashes []byte
		if err := rows.Scan(&revision.Version, &revision.Description, &revision.Type, &revision.Applied, &revision.Total,
			&revision.ExecutedAt, &revision.ExecutionTime, &revision.Error, &revision.ErrorStmt, &revision.Hash,
			&partialHashes, &revision.OperatorVersion); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(partialHashes, &revision.PartialHashes); err != nil {
			return nil, err
		}
		revisions = append(revisions, &revision)
	}
	return revisions, rows.Err()
}

func (r revisionTable) WriteRevision(ctx context.Context, revision *migrate.Revision) error {
	partialHashes, err := json.Marshal(revision.PartialHashes)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "INSERT INTO momentum_migrations.revisions ("+revisionColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (version) DO UPDATE SET description = $2, type = $3, applied = $4, total = $5, executed_at = $6,
			execution_time = $7, error = $8, error_stmt = $9, hash = $10, partial_hashes = $11, operator_version = $12`,
		revision.Version, revision.Description, revision.Type, revision.Applied, revision.Total, revision.ExecutedAt,
		revision.ExecutionTime, revision.Error, revision.ErrorStmt, revision.Hash, partialHashes, revision.OperatorVersion)
	return err
}

func (r revisionTable) DeleteRevision(ctx context.Context, version string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM momentum_migrations.revisions WHERE version = $1", version)
	return err
}
