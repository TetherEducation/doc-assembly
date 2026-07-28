// Package riverqueue implements attempt-scoped background signing jobs using
// River, a PostgreSQL-native durable queue.
package riverqueue

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
)

// RiverService manages the River client lifecycle and exposes signing attempt UoW.
type RiverService struct {
	client         *river.Client[pgx.Tx]
	signingUOW     *SigningExecutionUnitOfWork
	workersEnabled bool
}

type Dependencies struct {
	DocumentRepo      port.DocumentRepository
	RecipientRepo     port.DocumentRecipientRepository
	AttemptRepo       port.SigningAttemptRepository
	VersionRepo       port.TemplateVersionRepository
	SignerRoleRepo    port.TemplateVersionSignerRoleRepository
	FieldResponseRepo port.DocumentFieldResponseRepository
	PDFRenderer       port.PDFRenderer
	SigningProvider   port.SigningProvider
	StorageAdapter    port.StorageAdapter
	StorageEnabled    bool
	CompletionHandler port.DocumentCompletedHandler
}

// New creates a RiverService. When cfg.Enabled is false the client operates in
// insert-only mode: jobs can be transactionally enqueued, but not processed.
func New(ctx context.Context, pool *pgxpool.Pool, cfg config.WorkerConfig, deps Dependencies) (*RiverService, error) {
	driver := riverpgxv5.New(pool)

	migrator, err := rivermigrate.New(driver, nil)
	if err != nil {
		return nil, fmt.Errorf("creating river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return nil, fmt.Errorf("running river migrations: %w", err)
	}

	riverCfg := &river.Config{}
	var executor *SigningAttemptExecutor
	if cfg.Enabled {
		workers := river.NewWorkers()
		executor = NewSigningAttemptExecutor(SigningAttemptExecutorConfig{
			Pool:              pool,
			DocumentRepo:      deps.DocumentRepo,
			RecipientRepo:     deps.RecipientRepo,
			AttemptRepo:       deps.AttemptRepo,
			VersionRepo:       deps.VersionRepo,
			SignerRoleRepo:    deps.SignerRoleRepo,
			FieldResponseRepo: deps.FieldResponseRepo,
			PDFRenderer:       deps.PDFRenderer,
			SigningProvider:   deps.SigningProvider,
			StorageAdapter:    deps.StorageAdapter,
			StorageEnabled:    deps.StorageEnabled,
			CompletionHandler: deps.CompletionHandler,
			Failpoints:        workerFailpoints(cfg),
		})
		river.AddWorker(workers, &RenderAttemptPDFWorker{executor: executor})
		river.AddWorker(workers, &SubmitAttemptToProviderWorker{executor: executor})
		river.AddWorker(workers, &AdvanceProviderSubmissionWorker{executor: executor})
		river.AddWorker(workers, &ReconcileProviderSubmissionWorker{executor: executor})
		river.AddWorker(workers, &RefreshAttemptProviderStatusWorker{executor: executor})
		river.AddWorker(workers, &CleanupProviderAttemptWorker{executor: executor})
		river.AddWorker(workers, &DispatchAttemptCompletionWorker{executor: executor})
		riverCfg.Workers = workers
		riverCfg.Queues = map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.MaxWorkersOrDefault()}}
	}

	client, err := river.NewClient(driver, riverCfg)
	if err != nil {
		return nil, fmt.Errorf("creating river client: %w", err)
	}
	if executor != nil {
		executor.client = client
	}

	signingUOW := NewSigningExecutionUnitOfWork(pool, client, deps.AttemptRepo)
	slog.InfoContext(ctx, "river signing queue initialized", slog.Bool("workers_enabled", cfg.Enabled), slog.Int("max_workers", cfg.MaxWorkersOrDefault()))
	return &RiverService{client: client, signingUOW: signingUOW, workersEnabled: cfg.Enabled}, nil
}

func workerFailpoints(cfg config.WorkerConfig) AttemptFailpoints {
	env := strings.ToLower(strings.TrimSpace(cfg.RuntimeEnvironment))
	if !cfg.FailpointsEnabled || env == "production" || env == "prod" {
		return nil
	}
	return newAttemptFailpoints(cfg.Failpoints)
}

func (r *RiverService) SigningExecutionUOW() port.SigningExecutionUnitOfWork { return r.signingUOW }

func (r *RiverService) Start(ctx context.Context) error {
	if !r.workersEnabled {
		return nil
	}
	return r.client.Start(ctx)
}

func (r *RiverService) Stop(ctx context.Context) error {
	if !r.workersEnabled {
		return nil
	}
	return r.client.Stop(ctx)
}
