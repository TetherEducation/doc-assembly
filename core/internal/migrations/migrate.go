package migrations

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"

	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
)

//go:embed sql/*.sql
var sqlFiles embed.FS

// Run applies all pending migrations to the database.
func Run(cfg *config.DatabaseConfig) error {
	src, err := iofs.New(sqlFiles, "sql")
	if err != nil {
		return fmt.Errorf("loading embedded migrations: %w", err)
	}

	connURL := fmt.Sprintf("pgx5://%s:%s@%s:%d/%s?sslmode=%s",
		url.PathEscape(cfg.User), url.PathEscape(cfg.Password), cfg.Host, cfg.Port, cfg.Name, cfg.SSLMode,
	)

	m, err := migrate.NewWithSourceInstance("iofs", src, connURL)
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	v, dirty, _ := m.Version()
	if dirty {
		return fmt.Errorf("migration version %d is dirty — manual intervention required", v)
	}

	fmt.Printf("Migrations applied successfully (version: %d)\n", v)
	return nil
}

// Latest returns the newest migration embedded in this engine: the version a
// database must be at for this engine to serve it.
func Latest() (uint, error) {
	entries, err := fs.ReadDir(sqlFiles, "sql")
	if err != nil {
		return 0, fmt.Errorf("listing embedded migrations: %w", err)
	}
	var latest uint
	for _, entry := range entries {
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		version, err := strconv.ParseUint(prefix, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("embedded migration %q has no numeric version", entry.Name())
		}
		latest = max(latest, uint(version))
	}
	if latest == 0 {
		return 0, errors.New("no embedded migrations")
	}
	return latest, nil
}

// State is the row golang-migrate keeps in schema_migrations.
type State struct {
	Version uint
	Dirty   bool
	// Initialized is false when no migration has completed: the table is
	// missing or empty, or holds golang-migrate's nil version.
	Initialized bool
}

// rowQuerier is the part of a pgx pool or connection ReadState needs.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ReadState reads the migration state through db. The table is resolved on the
// connection's search_path, which is where golang-migrate created it.
func ReadState(ctx context.Context, db rowQuerier) (State, error) {
	var exists bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return State{}, err
	}
	if !exists {
		return State{}, nil
	}
	var version int64
	var dirty bool
	err := db.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty)
	if errors.Is(err, pgx.ErrNoRows) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	if version < 0 { // golang-migrate's nil version: the first migration never completed
		return State{Dirty: dirty}, nil
	}
	return State{Version: uint(version), Dirty: dirty, Initialized: true}, nil
}
