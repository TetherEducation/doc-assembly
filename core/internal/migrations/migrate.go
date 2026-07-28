package migrations

import (
	"embed"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

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
