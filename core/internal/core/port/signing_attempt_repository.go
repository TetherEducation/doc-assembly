package port

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// SigningAttemptRepository persists attempt-owned signing execution state.
type SigningAttemptRepository interface {
	CreateTx(ctx context.Context, tx pgx.Tx, attempt *entity.SigningAttempt) (string, error)
	CreateRecipientTx(ctx context.Context, tx pgx.Tx, recipient *entity.SigningAttemptRecipient) (string, error)
	InsertEventTx(ctx context.Context, tx pgx.Tx, event *entity.SigningAttemptEvent) error
	FindByID(ctx context.Context, attemptID string) (*entity.SigningAttempt, error)
	FindActiveByDocumentID(ctx context.Context, documentID string) (*entity.SigningAttempt, error)
	FindByProviderDocumentID(ctx context.Context, providerName, providerDocumentID string) (*entity.SigningAttempt, error)
	FindByProviderCorrelationKey(ctx context.Context, providerName, correlationKey string) (*entity.SigningAttempt, error)
	FindRecipientsByAttemptID(ctx context.Context, attemptID string) ([]*entity.SigningAttemptRecipient, error)
	FindRecipientByAttemptAndDocumentRecipient(ctx context.Context, attemptID, documentRecipientID string) (*entity.SigningAttemptRecipient, error)
	UpdateTx(ctx context.Context, tx pgx.Tx, attempt *entity.SigningAttempt) error
	Update(ctx context.Context, attempt *entity.SigningAttempt) error
	UpdateRecipientTx(ctx context.Context, tx pgx.Tx, recipient *entity.SigningAttemptRecipient) error
	UpdateRecipient(ctx context.Context, recipient *entity.SigningAttemptRecipient) error
	InsertEvent(ctx context.Context, event *entity.SigningAttemptEvent) error
	NextSequenceTx(ctx context.Context, tx pgx.Tx, documentID string) (int, error)
	BindTokenToAttempt(ctx context.Context, tokenID, attemptID string) error
}
