//go:build integration

package testhelper

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // Register pgx driver with database/sql for Snapshot/Restore
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
	"github.com/TetherEducation/doc-assembly/core/internal/migrations"
)

var (
	testContainer *postgres.PostgresContainer
	testPool      *pgxpool.Pool
	once          sync.Once
	initErr       error
)

// GetTestPool returns a connection pool to a PostgreSQL testcontainer
// with all embedded SQL migrations applied via golang-migrate.
// Uses singleton pattern - container is shared across all tests.
// Tests are responsible for cleaning up their own data.
func GetTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	once.Do(func() {
		testContainer, testPool, initErr = setupTestContainer()
	})

	if initErr != nil {
		t.Skipf("Skipping integration test: %v", initErr)
	}

	return testPool
}

func setupTestContainer() (*postgres.PostgresContainer, *pgxpool.Pool, error) {
	ctx := context.Background()

	// Start PostgreSQL container
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("doc_engine_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithSQLDriver("pgx"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("starting postgres: %w", err)
	}

	// Get connection info
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, nil, fmt.Errorf("getting connection string: %w", err)
	}

	host, err := pgContainer.Host(ctx)
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, nil, fmt.Errorf("getting host: %w", err)
	}

	port, err := pgContainer.MappedPort(ctx, "5432/tcp")
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, nil, fmt.Errorf("getting port: %w", err)
	}

	// Run embedded SQL migrations
	dbCfg := &config.DatabaseConfig{
		Host:     host,
		Port:     port.Int(),
		User:     "test",
		Password: "test",
		Name:     "doc_engine_test",
		SSLMode:  "disable",
	}
	if migErr := migrations.Run(dbCfg); migErr != nil {
		pgContainer.Terminate(ctx)
		return nil, nil, fmt.Errorf("running migrations: %w", migErr)
	}

	// Create connection pool
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, nil, fmt.Errorf("creating pool: %w", err)
	}

	return pgContainer, pool, nil
}

// CleanupContainers terminates all test containers.
// Call from TestMain or use t.Cleanup() in individual tests.
func CleanupContainers(ctx context.Context) {
	if testPool != nil {
		testPool.Close()
	}
	if testContainer != nil {
		testContainer.Terminate(ctx)
	}
}
