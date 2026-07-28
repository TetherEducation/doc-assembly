package signingattemptrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Repository implements SigningAttemptRepository using PostgreSQL.
type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) port.SigningAttemptRepository { return NewConcrete(pool) }
func NewConcrete(pool *pgxpool.Pool) *Repository           { return &Repository{pool: pool} }

func (r *Repository) CreateTx(ctx context.Context, tx pgx.Tx, a *entity.SigningAttempt) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO execution.signing_attempts (
			document_id, sequence, status, render_started_at, pdf_storage_path, pdf_checksum,
			pdf_checksum_algorithm, render_metadata, signature_field_snapshot, provider_upload_payload,
			provider_name, provider_correlation_key, provider_document_id, provider_submit_phase,
			retry_count, next_retry_at, last_error_class, last_error_message,
			reconciliation_count, next_reconciliation_at, cleanup_status, cleanup_action, cleanup_error,
			processing_lease_owner, processing_lease_expires_at, invalidation_reason, terminal_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)
		RETURNING id`,
		a.DocumentID, a.Sequence, a.Status, a.RenderStartedAt, a.PDFStoragePath, a.PDFChecksum,
		a.PDFChecksumAlgorithm, rawOrNil(a.RenderMetadata), rawOrNil(a.SignatureFieldSnapshot), rawOrNil(a.ProviderUploadPayload),
		a.ProviderName, a.ProviderCorrelationKey, a.ProviderDocumentID, a.ProviderSubmitPhase,
		a.RetryCount, a.NextRetryAt, a.LastErrorClass, a.LastErrorMessage,
		a.ReconciliationCount, a.NextReconciliationAt, a.CleanupStatus, a.CleanupAction, a.CleanupError,
		a.ProcessingLeaseOwner, a.ProcessingLeaseExpiresAt, a.InvalidationReason, a.TerminalAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating signing attempt: %w", err)
	}
	a.ID = id
	return id, nil
}

func (r *Repository) CreateRecipientTx(ctx context.Context, tx pgx.Tx, rec *entity.SigningAttemptRecipient) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO execution.signing_attempt_recipients (
			attempt_id, document_recipient_id, template_version_role_id, signer_order, email, name,
			provider_recipient_id, provider_signing_token, signing_url, status, signed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id`,
		rec.AttemptID, rec.DocumentRecipientID, rec.TemplateVersionRoleID, rec.SignerOrder, rec.Email, rec.Name,
		rec.ProviderRecipientID, rec.ProviderSigningToken, rec.SigningURL, rec.Status, rec.SignedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating signing attempt recipient: %w", err)
	}
	rec.ID = id
	return id, nil
}

func (r *Repository) InsertEventTx(ctx context.Context, tx pgx.Tx, ev *entity.SigningAttemptEvent) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO execution.signing_attempt_events (
			attempt_id, document_id, event_type, old_status, new_status, provider_name,
			provider_document_id, correlation_key, river_job_id, error_class, metadata, raw_payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		ev.AttemptID, ev.DocumentID, ev.EventType, ev.OldStatus, ev.NewStatus, ev.ProviderName,
		ev.ProviderDocumentID, ev.CorrelationKey, ev.RiverJobID, ev.ErrorClass, rawOrNil(ev.Metadata), rawOrNil(ev.RawPayload),
	)
	if err != nil {
		return fmt.Errorf("inserting signing attempt event: %w", err)
	}
	return nil
}

func (r *Repository) FindByID(ctx context.Context, attemptID string) (*entity.SigningAttempt, error) {
	return scanAttempt(r.pool.QueryRow(ctx, selectAttemptSQL()+` WHERE id = $1`, attemptID))
}

func (r *Repository) FindActiveByDocumentID(ctx context.Context, documentID string) (*entity.SigningAttempt, error) {
	return scanAttempt(r.pool.QueryRow(ctx, selectAttemptSQL()+`
		WHERE id = (SELECT active_attempt_id FROM execution.documents WHERE id = $1)`, documentID))
}

func (r *Repository) FindByProviderDocumentID(ctx context.Context, providerName, providerDocumentID string) (*entity.SigningAttempt, error) {
	return scanAttempt(r.pool.QueryRow(ctx, selectAttemptSQL()+`
		WHERE provider_name = $1 AND provider_document_id = $2`, providerName, providerDocumentID))
}

func (r *Repository) FindByProviderCorrelationKey(ctx context.Context, providerName, correlationKey string) (*entity.SigningAttempt, error) {
	return scanAttempt(r.pool.QueryRow(ctx, selectAttemptSQL()+`
		WHERE provider_name = $1 AND provider_correlation_key = $2`, providerName, correlationKey))
}

func (r *Repository) FindRecipientsByAttemptID(ctx context.Context, attemptID string) ([]*entity.SigningAttemptRecipient, error) {
	rows, err := r.pool.Query(ctx, selectAttemptRecipientSQL()+` WHERE attempt_id = $1 ORDER BY signer_order ASC`, attemptID)
	if err != nil {
		return nil, fmt.Errorf("querying signing attempt recipients: %w", err)
	}
	defer rows.Close()
	return scanRecipients(rows)
}

func (r *Repository) FindRecipientByAttemptAndDocumentRecipient(ctx context.Context, attemptID, documentRecipientID string) (*entity.SigningAttemptRecipient, error) {
	return scanRecipient(r.pool.QueryRow(ctx, selectAttemptRecipientSQL()+` WHERE attempt_id = $1 AND document_recipient_id = $2`, attemptID, documentRecipientID))
}

func (r *Repository) UpdateTx(ctx context.Context, tx pgx.Tx, a *entity.SigningAttempt) error {
	_, err := tx.Exec(ctx, `
		UPDATE execution.signing_attempts SET
			status=$2, render_started_at=$3, pdf_storage_path=$4, pdf_checksum=$5,
			pdf_checksum_algorithm=$6, render_metadata=$7, signature_field_snapshot=$8,
			provider_upload_payload=$9, provider_name=$10, provider_correlation_key=$11,
			provider_document_id=$12, provider_submit_phase=$13, retry_count=$14, next_retry_at=$15,
			last_error_class=$16, last_error_message=$17, reconciliation_count=$18,
			next_reconciliation_at=$19, cleanup_status=$20, cleanup_action=$21, cleanup_error=$22,
			processing_lease_owner=$23, processing_lease_expires_at=$24,
			invalidation_reason=$25, terminal_at=$26
		WHERE id=$1`,
		a.ID, a.Status, a.RenderStartedAt, a.PDFStoragePath, a.PDFChecksum,
		a.PDFChecksumAlgorithm, rawOrNil(a.RenderMetadata), rawOrNil(a.SignatureFieldSnapshot),
		rawOrNil(a.ProviderUploadPayload), a.ProviderName, a.ProviderCorrelationKey,
		a.ProviderDocumentID, a.ProviderSubmitPhase, a.RetryCount, a.NextRetryAt,
		a.LastErrorClass, a.LastErrorMessage, a.ReconciliationCount,
		a.NextReconciliationAt, a.CleanupStatus, a.CleanupAction, a.CleanupError,
		a.ProcessingLeaseOwner, a.ProcessingLeaseExpiresAt, a.InvalidationReason, a.TerminalAt,
	)
	if err != nil {
		return fmt.Errorf("updating signing attempt: %w", err)
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, a *entity.SigningAttempt) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := r.UpdateTx(ctx, tx, a); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) UpdateRecipientTx(ctx context.Context, tx pgx.Tx, rec *entity.SigningAttemptRecipient) error {
	_, err := tx.Exec(ctx, `
		UPDATE execution.signing_attempt_recipients SET
			provider_recipient_id=$2, provider_signing_token=$3, signing_url=$4,
			status=$5, signed_at=$6
		WHERE id=$1`,
		rec.ID, rec.ProviderRecipientID, rec.ProviderSigningToken, rec.SigningURL, rec.Status, rec.SignedAt,
	)
	if err != nil {
		return fmt.Errorf("updating signing attempt recipient: %w", err)
	}
	return nil
}

func (r *Repository) UpdateRecipient(ctx context.Context, rec *entity.SigningAttemptRecipient) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := r.UpdateRecipientTx(ctx, tx, rec); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) InsertEvent(ctx context.Context, ev *entity.SigningAttemptEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := r.InsertEventTx(ctx, tx, ev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) NextSequenceTx(ctx context.Context, tx pgx.Tx, documentID string) (int, error) {
	var seq int
	err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM execution.signing_attempts WHERE document_id = $1`, documentID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("next signing attempt sequence: %w", err)
	}
	return seq, nil
}

func (r *Repository) BindTokenToAttempt(ctx context.Context, tokenID, attemptID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE execution.document_access_tokens SET attempt_id = $2 WHERE id = $1`, tokenID, attemptID)
	if err != nil {
		return fmt.Errorf("binding token to attempt: %w", err)
	}
	return nil
}

func selectAttemptSQL() string {
	return `
	SELECT id, document_id, sequence, status, render_started_at, pdf_storage_path, pdf_checksum,
	       pdf_checksum_algorithm, render_metadata, signature_field_snapshot, provider_upload_payload,
	       provider_name, provider_correlation_key, provider_document_id, provider_submit_phase,
	       retry_count, next_retry_at, last_error_class, last_error_message,
	       reconciliation_count, next_reconciliation_at, cleanup_status, cleanup_action, cleanup_error,
	       processing_lease_owner, processing_lease_expires_at, invalidation_reason,
	       created_at, updated_at, terminal_at
	FROM execution.signing_attempts`
}

func scanAttempt(row pgx.Row) (*entity.SigningAttempt, error) {
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

func selectAttemptRecipientSQL() string {
	return `
	SELECT id, attempt_id, document_recipient_id, template_version_role_id, signer_order,
	       email, name, provider_recipient_id, provider_signing_token, signing_url,
	       status, signed_at, created_at, updated_at
	FROM execution.signing_attempt_recipients`
}

func scanRecipient(row pgx.Row) (*entity.SigningAttemptRecipient, error) {
	rec := &entity.SigningAttemptRecipient{}
	err := row.Scan(&rec.ID, &rec.AttemptID, &rec.DocumentRecipientID, &rec.TemplateVersionRoleID, &rec.SignerOrder,
		&rec.Email, &rec.Name, &rec.ProviderRecipientID, &rec.ProviderSigningToken, &rec.SigningURL,
		&rec.Status, &rec.SignedAt, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrRecordNotFound
		}
		return nil, fmt.Errorf("scanning signing attempt recipient: %w", err)
	}
	return rec, nil
}

func scanRecipients(rows pgx.Rows) ([]*entity.SigningAttemptRecipient, error) {
	var out []*entity.SigningAttemptRecipient
	for rows.Next() {
		rec := &entity.SigningAttemptRecipient{}
		if err := rows.Scan(&rec.ID, &rec.AttemptID, &rec.DocumentRecipientID, &rec.TemplateVersionRoleID, &rec.SignerOrder,
			&rec.Email, &rec.Name, &rec.ProviderRecipientID, &rec.ProviderSigningToken, &rec.SigningURL,
			&rec.Status, &rec.SignedAt, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func rawOrNil(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

var _ port.SigningAttemptRepository = (*Repository)(nil)
