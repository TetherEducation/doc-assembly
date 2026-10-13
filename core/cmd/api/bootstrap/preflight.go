package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
	"github.com/TetherEducation/doc-assembly/core/internal/migrations"
)

// preflightChecks runs all startup validations.
func (e *Engine) preflightChecks(ctx context.Context) error {
	if err := checkTypst(ctx, e.config.Typst.BinPath); err != nil {
		return err
	}

	pool, err := checkDatabase(ctx, e)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := checkSchema(ctx, pool); err != nil {
		return err
	}

	// Run OIDC discovery (populates issuer/jwks_url from discovery_url).
	// Already called in config.Load() but re-run if config was loaded from file.
	if err := e.config.Auth.DiscoverAll(ctx); err != nil {
		slog.WarnContext(ctx, "OIDC discovery failed (non-fatal)", slog.String("error", err.Error()))
	}

	checkAuth(ctx, e)
	if err := checkSigningSessionAuth(ctx, e); err != nil {
		return err
	}
	return nil
}

// checkTypst verifies the Typst CLI is installed and accessible.
func checkTypst(ctx context.Context, binPath string) error {
	if binPath == "" {
		binPath = "typst"
	}

	out, err := exec.CommandContext(ctx, binPath, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf(`typst CLI not found (%s)

Typst is required for PDF rendering. Install it:

  macOS:   brew install typst
  Linux:   curl -fsSL https://typst.community | sh
  Cargo:   cargo install typst-cli
  Windows: winget install typst

More info: https://github.com/typst/typst#installation`, binPath)
	}

	version := strings.TrimSpace(string(out))
	slog.InfoContext(ctx, "typst CLI found", slog.String("version", version), slog.String("os", runtime.GOOS))
	return nil
}

// checkDatabase verifies the database is reachable.
func checkDatabase(ctx context.Context, e *Engine) (*pgxpool.Pool, error) {
	pool, err := postgres.NewPool(ctx, &e.config.Database)
	if err != nil {
		return nil, fmt.Errorf("database unreachable: %w\n\nCheck your database configuration:\n  host: %s\n  port: %d\n  name: %s\n  user: %s\n\nMake sure PostgreSQL is running and accessible",
			err,
			e.config.Database.Host,
			e.config.Database.Port,
			e.config.Database.Name,
			e.config.Database.User,
		)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	slog.InfoContext(ctx, "database connection OK",
		slog.String("host", e.config.Database.Host),
		slog.Int("port", e.config.Database.Port),
		slog.String("name", e.config.Database.Name),
	)
	return pool, nil
}

// checkSchema refuses to serve a database this engine's migrations have not
// brought up to date. Checking for one table was not enough: a schema one
// migration behind passed, and failed later at query time.
func checkSchema(ctx context.Context, pool *pgxpool.Pool) error {
	latest, err := migrations.Latest()
	if err != nil {
		return err
	}
	state, err := migrations.ReadState(ctx, pool)
	if err != nil {
		return fmt.Errorf("reading schema_migrations: %w", err)
	}

	ahead, err := evaluateSchema(state, latest)
	if err != nil {
		return err
	}
	if ahead {
		slog.WarnContext(ctx, "database schema is ahead of this engine; expected while a release migrates or after a rollback",
			slog.Uint64("schema_version", uint64(state.Version)),
			slog.Bool("dirty", state.Dirty),
			slog.Uint64("engine_schema_version", uint64(latest)),
		)
		return nil
	}
	slog.InfoContext(ctx, "database schema OK", slog.Uint64("schema_version", uint64(state.Version)))
	return nil
}

// evaluateSchema decides whether an engine whose newest migration is latest may
// serve a database in state.
//
// A database ahead of the engine is served (ahead=true), dirty or not:
// migrations are additive, and golang-migrate marks a migration dirty while it
// runs, so during a release the old revision must keep starting while the new
// image's migrations apply, and again after a rollback. A dirty database at the
// engine's own version is refused: that migration is half-applied.
func evaluateSchema(state migrations.State, latest uint) (ahead bool, err error) {
	const runMigrations = "\n\nRun migrations first:\n  go run ./core/cmd/api migrate   (in a wrapper: go run . migrate)"
	switch {
	case !state.Initialized:
		return false, errors.New("database schema not initialized" + runMigrations)
	case state.Version < latest:
		dirty := ""
		if state.Dirty {
			dirty = " (dirty)"
		}
		return false, fmt.Errorf("database schema is at version %d%s, but this engine requires %d%s",
			state.Version, dirty, latest, runMigrations)
	case state.Version == latest && state.Dirty:
		return false, fmt.Errorf("migration %d is dirty: it failed or is still running.\n\n"+
			"Wait for a running migration to finish; after a failure, repair the schema and force the version", latest)
	case state.Version > latest:
		return true, nil
	}
	return false, nil
}

// checkAuth checks auth configuration and logs dummy mode warning.
func checkAuth(ctx context.Context, e *Engine) {
	panel := e.config.Auth.GetPanelOIDC()

	if panel == nil {
		slog.WarnContext(ctx, "OIDC not configured - running in dummy mode (dev only)")
		e.config.Auth.Dummy = true
		return
	}

	if panel.Issuer == "" {
		slog.WarnContext(ctx, "panel OIDC missing issuer", slog.String("name", panel.Name))
	}
	if panel.JWKSURL == "" {
		slog.WarnContext(ctx, "panel OIDC missing jwks_url", slog.String("name", panel.Name))
	}
	slog.InfoContext(ctx, "panel OIDC configured",
		slog.String("name", panel.Name),
		slog.String("issuer", panel.Issuer))
}

func checkSigningSessionAuth(ctx context.Context, e *Engine) error {
	// In dummy auth mode (dev-only bypass), signing session auth is not enforced.
	if e.config.Auth.IsDummyAuth() {
		slog.WarnContext(ctx, "signing session auth check skipped (dummy auth mode)")
		return nil
	}

	mode := strings.TrimSpace(e.signingSessionMode)
	if mode == "" {
		mode = e.config.SigningSessionAuth.Mode
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return fmt.Errorf("missing required config: signing_session_auth.mode (expected 'oidc' or 'custom')")
	}

	switch mode {
	case config.SigningSessionAuthModeOIDC:
		panel := e.config.Auth.GetPanelOIDC()
		if panel == nil {
			return fmt.Errorf("signing_session_auth.mode=oidc requires auth.panel OIDC configuration")
		}
		requestedProvider := strings.TrimSpace(e.config.SigningSessionAuth.OIDC.Provider)
		if requestedProvider != "" &&
			!strings.EqualFold(requestedProvider, "panel") &&
			!strings.EqualFold(requestedProvider, panel.Name) {
			return fmt.Errorf(
				"unsupported signing_session_auth.oidc.provider=%q (supported: panel or %q)",
				requestedProvider,
				panel.Name,
			)
		}
		slog.InfoContext(ctx, "signing session auth mode configured",
			slog.String("mode", mode),
			slog.String("provider", strings.TrimSpace(e.config.SigningSessionAuth.OIDC.Provider)),
			slog.String("email_claim", strings.TrimSpace(e.config.SigningSessionAuth.OIDC.EmailClaim)),
		)
		return nil
	case config.SigningSessionAuthModeCustom:
		if e.signingSessionAuth == nil {
			return fmt.Errorf("signing_session_auth.mode=custom requires SetSigningSessionAuthenticator(...)")
		}
		slog.InfoContext(ctx, "signing session auth mode configured",
			slog.String("mode", mode),
		)
		return nil
	default:
		return fmt.Errorf("invalid signing_session_auth.mode=%q (expected 'oidc' or 'custom')", mode)
	}
}
