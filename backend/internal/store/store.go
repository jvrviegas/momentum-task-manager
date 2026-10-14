// Package store keeps users and their tasks in PostgreSQL through the ent
// client in ./ent. Task reads and writes need a viewer in the context; see
// package viewer.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	_ "github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/runtime" // registers defaults and privacy policies
)

// Open opens the database at url, a PostgreSQL URL or keyword string, through
// pgx.
func Open(url string) (*sql.DB, error) {
	return sql.Open("pgx", url)
}

// NewClient returns an ent client over db.
func NewClient(db *sql.DB) *ent.Client {
	return ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
}

// CheckLocal returns an error unless every host url names is this machine,
// so tests and schema tools cannot reach a real database such as Neon.
func CheckLocal(url string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("no database URL; use a local, disposable PostgreSQL server")
	}
	config, err := pgx.ParseConfig(url)
	if err != nil {
		return fmt.Errorf("parse database URL: %w", err)
	}
	hosts := []string{config.Host}
	for _, fallback := range config.Fallbacks {
		hosts = append(hosts, fallback.Host)
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); host != "localhost" && !strings.HasPrefix(host, "/") && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("refusing database host %q: only a local, disposable PostgreSQL server is allowed", host)
		}
	}
	return nil
}
