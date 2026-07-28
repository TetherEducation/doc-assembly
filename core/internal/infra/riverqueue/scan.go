package riverqueue

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

func scanAttemptRow(row pgx.Row) (*entity.SigningAttempt, error) {
	a := &entity.SigningAttempt{}
	var renderMeta, sigFields, payload []byte
	var phase sql.NullString
	var lastClass sql.NullString
	err := row.Scan(&a.ID, &a.DocumentID, &a.Sequence, &a.Status, &a.RenderStartedAt, &a.PDFStoragePath, &a.PDFChecksum,
		&a.PDFChecksumAlgorithm, &renderMeta, &sigFields, &payload, &a.ProviderName, &a.ProviderCorrelationKey,
		&a.ProviderDocumentID, &phase, &a.RetryCount, &a.NextRetryAt, &lastClass, &a.LastErrorMessage,
		&a.ReconciliationCount, &a.NextReconciliationAt, &a.CleanupStatus, &a.CleanupAction, &a.CleanupError,
		&a.ProcessingLeaseOwner, &a.ProcessingLeaseExpiresAt, &a.InvalidationReason, &a.CreatedAt, &a.UpdatedAt, &a.TerminalAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrRecordNotFound
		}
		return nil, fmt.Errorf("scanning signing attempt: %w", err)
	}
	a.RenderMetadata = json.RawMessage(renderMeta)
	a.SignatureFieldSnapshot = json.RawMessage(sigFields)
	a.ProviderUploadPayload = json.RawMessage(payload)
	if phase.Valid {
		v := entity.ProviderSubmitPhase(phase.String)
		a.ProviderSubmitPhase = &v
	}
	if lastClass.Valid {
		v := entity.ProviderErrorClass(lastClass.String)
		a.LastErrorClass = &v
	}
	return a, nil
}
