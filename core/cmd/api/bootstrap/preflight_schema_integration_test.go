//go:build integration

package bootstrap

import (
	"context"
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/migrations"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// TestCheckSchemaAgainstPostgres runs the preflight check against a database the
// engine's own migrations built, rewriting schema_migrations into each state.
func TestCheckSchemaAgainstPostgres(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ctx := context.Background()
	latest, err := migrations.Latest()
	if err != nil {
		t.Fatal(err)
	}

	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	setVersion := func(t *testing.T, version int64, dirty bool) {
		exec(t, `UPDATE schema_migrations SET version = $1, dirty = $2`, version, dirty)
	}
	t.Cleanup(func() {
		// Leave the shared container as the migrations left it.
		_, _ = pool.Exec(ctx, `ALTER TABLE IF EXISTS schema_migrations_hidden RENAME TO schema_migrations`)
		_, _ = pool.Exec(ctx, `DELETE FROM schema_migrations`)
		_, _ = pool.Exec(ctx, `INSERT INTO schema_migrations (version, dirty) VALUES ($1, false)`, int64(latest))
	})

	for _, tc := range []struct {
		name    string
		arrange func(t *testing.T)
		wantErr string // empty: must start
	}{
		{"as migrated", func(t *testing.T) {}, ""},
		{"clean, one behind", func(t *testing.T) { setVersion(t, int64(latest)-1, false) }, "but this engine requires"},
		{"dirty at the engine's version", func(t *testing.T) { setVersion(t, int64(latest), true) }, "is dirty"},
		{"clean, one ahead", func(t *testing.T) { setVersion(t, int64(latest)+1, false) }, ""},
		{"dirty, one ahead (a newer migration running)", func(t *testing.T) { setVersion(t, int64(latest)+1, true) }, ""},
		{"empty table", func(t *testing.T) { exec(t, `DELETE FROM schema_migrations`) }, "not initialized"},
		{"no table", func(t *testing.T) {
			exec(t, `ALTER TABLE schema_migrations RENAME TO schema_migrations_hidden`)
			t.Cleanup(func() { exec(t, `ALTER TABLE schema_migrations_hidden RENAME TO schema_migrations`) })
		}, "not initialized"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec(t, `DELETE FROM schema_migrations`)
			exec(t, `INSERT INTO schema_migrations (version, dirty) VALUES ($1, false)`, int64(latest))
			tc.arrange(t)

			err := checkSchema(ctx, pool)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want start, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
