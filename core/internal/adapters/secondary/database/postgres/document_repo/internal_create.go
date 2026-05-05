package documentrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

const (
	uniqueViolationCode              = "23505"
	constraintWorkspaceTransactional = "uq_documents_workspace_transactional_id"
	constraintActiveLogicalKey       = "uq_documents_active_logical_key"
)

const (
	queryAdvisoryLockInternalCreate = `
		SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
	`

	queryFindByWorkspaceTransactionalForUpdate = `
		SELECT id
		FROM execution.documents
		WHERE workspace_id = $1 AND transactional_id = $2
		ORDER BY created_at ASC, id ASC
		LIMIT 1
		FOR UPDATE
	`

	queryFindActiveByLogicalKeyForUpdate = `
		SELECT id
		FROM execution.documents
		WHERE workspace_id = $1
		  AND document_type_id = $2
		  AND client_external_reference_id = $3
		  AND is_active = TRUE
		ORDER BY created_at DESC, id DESC
		LIMIT 1
		FOR UPDATE
	`

	queryMarkSuperseded = `
		UPDATE execution.documents
		SET is_active = FALSE,
		    superseded_at = NOW(),
		    supersede_reason = $2,
		    updated_at = NOW()
		WHERE id = $1
	`

	querySetSupersededByDocument = `
		UPDATE execution.documents
		SET superseded_by_document_id = $2,
		    updated_at = NOW()
		WHERE id = $1
	`

	queryCreateInternalDocument = `
		INSERT INTO execution.documents (
			workspace_id, template_version_id, document_type_id, title, client_external_reference_id,
			transactional_id, operation_type, related_document_id, active_attempt_id,
			status, injected_values_snapshot, completed_pdf_url, is_active, superseded_at,
			superseded_by_document_id, supersede_reason, expires_at, metadata, created_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11, $12,
			$13, $14, $15,
			$16, $17, $18, $19
		)
		RETURNING id
	`

	queryCreateInternalRecipient = `
		INSERT INTO execution.document_recipients (
			document_id, template_version_role_id, name, email,
			status, signed_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`

	queryFindByWorkspaceTransactional = `
		SELECT id
		FROM execution.documents
		WHERE workspace_id = $1 AND transactional_id = $2
		ORDER BY created_at ASC, id ASC
		LIMIT 1
	`

	queryFindActiveByLogicalKey = `
		SELECT id
		FROM execution.documents
		WHERE workspace_id = $1
		  AND document_type_id = $2
		  AND client_external_reference_id = $3
		  AND is_active = TRUE
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`

	queryFindInternalCreateReplay = `
		WITH transactional AS (
			SELECT id
			FROM execution.documents
			WHERE workspace_id = $1
			  AND transactional_id = $4
			ORDER BY created_at ASC, id ASC
			LIMIT 1
		), active AS (
			SELECT id
			FROM execution.documents
			WHERE NOT EXISTS (SELECT 1 FROM transactional)
			  AND workspace_id = $1
			  AND document_type_id = $2
			  AND client_external_reference_id = $3
			  AND is_active = TRUE
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		)
		SELECT id FROM transactional
		UNION ALL
		SELECT id FROM active
		LIMIT 1
	`

	queryInternalCreateOrReplayFast = `
		WITH locked AS (
			SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
		), transactional AS (
			SELECT d.id, TRUE AS replay
			FROM execution.documents d, locked
			WHERE d.workspace_id = $2
			  AND d.transactional_id = $6
			ORDER BY d.created_at ASC, d.id ASC
			LIMIT 1
		), active AS (
			SELECT d.id, TRUE AS replay
			FROM execution.documents d, locked
			WHERE NOT EXISTS (SELECT 1 FROM transactional)
			  AND d.workspace_id = $2
			  AND d.document_type_id = $3
			  AND d.client_external_reference_id = $4
			  AND d.is_active = TRUE
			ORDER BY d.created_at DESC, d.id DESC
			LIMIT 1
		), inserted_doc AS (
			INSERT INTO execution.documents (
				workspace_id, template_version_id, document_type_id, title, client_external_reference_id,
				transactional_id, operation_type, related_document_id, active_attempt_id,
				status, injected_values_snapshot, completed_pdf_url, is_active, superseded_at,
				superseded_by_document_id, supersede_reason, expires_at, metadata, created_at
			)
			SELECT
				$2, $5, $3, $7, $4,
				$6, $8, $9, $10,
				$11, $12, $13, TRUE, NULL,
				NULL, NULL, $14, $15, $16
			WHERE NOT EXISTS (SELECT 1 FROM transactional)
			  AND NOT EXISTS (SELECT 1 FROM active)
			RETURNING id, FALSE AS replay
		), recipient_input AS (
			SELECT *
			FROM jsonb_to_recordset($17::jsonb) AS r(
				template_version_role_id uuid,
				name text,
				email text,
				status recipient_status,
				signed_at timestamptz,
				created_at timestamptz
			)
		), inserted_recipients AS (
			INSERT INTO execution.document_recipients (
				document_id, template_version_role_id, name, email,
				status, signed_at, created_at
			)
			SELECT d.id, r.template_version_role_id, r.name, r.email,
			       r.status, r.signed_at, r.created_at
			FROM inserted_doc d
			JOIN recipient_input r ON TRUE
			RETURNING id, document_id, template_version_role_id, name, email,
			          status, signed_at, created_at, updated_at
		), selected_doc AS (
			SELECT id, replay FROM transactional
			UNION ALL
			SELECT id, replay FROM active
			UNION ALL
			SELECT id, replay FROM inserted_doc
			LIMIT 1
		)
		SELECT id, replay,
		       COALESCE((
		           SELECT jsonb_agg(to_jsonb(ir) ORDER BY ir.created_at ASC)
		           FROM inserted_recipients ir
		       ), '[]'::jsonb) AS recipients
		FROM selected_doc
	`
)

// FindInternalCreateReplay finds an already-created document for an internal-create request.
func (r *Repository) FindInternalCreateReplay(
	ctx context.Context,
	workspaceID string,
	documentTypeID string,
	externalID string,
	transactionalID string,
) (string, bool, error) {
	var docID string
	err := r.pool.QueryRow(ctx, queryFindInternalCreateReplay, workspaceID, documentTypeID, externalID, transactionalID).Scan(&docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("querying internal create replay: %w", err)
	}
	return docID, true, nil
}

// InternalCreateOrReplay executes transactional create/replay/supersede logic for internal create.
func (r *Repository) InternalCreateOrReplay(ctx context.Context, req *port.InternalCreateRequest) (*port.InternalCreateResult, error) {
	if req == nil || req.Document == nil {
		return nil, fmt.Errorf("invalid internal create request")
	}
	prepareInternalCreateDocument(req)
	prepareInternalCreateRecipients(req.Recipients)

	if !req.ForceCreate {
		result, err := r.internalCreateOrReplayFast(ctx, req)
		if err == nil {
			return result, nil
		}
		if !isRecoverableInternalCreateConflict(err) {
			return nil, err
		}
		// Fall through to the explicit transaction path for rare unique-conflict recovery.
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("starting internal create transaction: %w", err)
	}

	result, err := r.internalCreateOrReplayTx(ctx, tx, req)
	if err != nil {
		_ = tx.Rollback(ctx)
		return r.recoverInternalCreateConflict(ctx, req, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing internal create transaction: %w", err)
	}

	return result, nil
}

func isRecoverableInternalCreateConflict(err error) bool {
	return isConstraintViolation(err, constraintWorkspaceTransactional) ||
		isConstraintViolation(err, constraintActiveLogicalKey)
}

func (r *Repository) recoverInternalCreateConflict(
	ctx context.Context,
	req *port.InternalCreateRequest,
	err error,
) (*port.InternalCreateResult, error) {
	switch {
	case isConstraintViolation(err, constraintWorkspaceTransactional):
		docID, findErr := r.findByWorkspaceTransactional(ctx, req.WorkspaceID, req.TransactionalID)
		if findErr != nil {
			return nil, fmt.Errorf("recovering transactional replay after conflict: %w", findErr)
		}
		return &port.InternalCreateResult{DocumentID: docID, IdempotentReplay: true}, nil
	case isConstraintViolation(err, constraintActiveLogicalKey):
		docID, findErr := r.findActiveByLogicalKey(ctx, req.WorkspaceID, req.DocumentTypeID, req.ExternalID)
		if findErr != nil {
			return nil, fmt.Errorf("recovering active replay after conflict: %w", findErr)
		}
		return &port.InternalCreateResult{DocumentID: docID, IdempotentReplay: true}, nil
	default:
		return nil, err
	}
}

type internalCreateRecipientRow struct {
	TemplateVersionRoleID string                 `json:"template_version_role_id"`
	Name                  string                 `json:"name"`
	Email                 string                 `json:"email"`
	Status                entity.RecipientStatus `json:"status"`
	SignedAt              *time.Time             `json:"signed_at"`
	CreatedAt             time.Time              `json:"created_at"`
}

type internalCreatedRecipientRow struct {
	ID                    string                 `json:"id"`
	DocumentID            string                 `json:"document_id"`
	TemplateVersionRoleID string                 `json:"template_version_role_id"`
	Name                  string                 `json:"name"`
	Email                 string                 `json:"email"`
	Status                entity.RecipientStatus `json:"status"`
	SignedAt              *time.Time             `json:"signed_at"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             *time.Time             `json:"updated_at"`
}

func (r *Repository) internalCreateOrReplayFast(ctx context.Context, req *port.InternalCreateRequest) (*port.InternalCreateResult, error) {
	recipientsJSON, err := marshalInternalCreateRecipients(req.Recipients)
	if err != nil {
		return nil, err
	}

	lockKey := fmt.Sprintf("%s|%s|%s", req.WorkspaceID, req.ExternalID, req.DocumentTypeID)
	doc := req.Document
	var docID string
	var replay bool
	var createdRecipientsJSON []byte
	if err := r.pool.QueryRow(ctx, queryInternalCreateOrReplayFast,
		lockKey,
		req.WorkspaceID,
		req.DocumentTypeID,
		req.ExternalID,
		doc.TemplateVersionID,
		req.TransactionalID,
		doc.Title,
		doc.OperationType,
		doc.RelatedDocumentID,
		doc.ActiveAttemptID,
		doc.Status,
		doc.InjectedValuesSnapshot,
		doc.CompletedPDFURL,
		doc.ExpiresAt,
		doc.Metadata,
		doc.CreatedAt,
		recipientsJSON,
	).Scan(&docID, &replay, &createdRecipientsJSON); err != nil {
		return nil, fmt.Errorf("executing fast internal create/replay: %w", err)
	}

	result := &port.InternalCreateResult{DocumentID: docID, IdempotentReplay: replay}
	if !replay {
		createdRecipients, err := unmarshalInternalCreatedRecipients(createdRecipientsJSON)
		if err != nil {
			return nil, err
		}
		result.Document = buildInternalCreateStoredDocument(req.Document, docID, createdRecipients)
	}

	return result, nil
}

func prepareInternalCreateDocument(req *port.InternalCreateRequest) {
	doc := req.Document
	doc.DocumentTypeID = req.DocumentTypeID
	doc.IsActive = true
	if doc.ClientExternalReferenceID == nil {
		doc.SetExternalReference(req.ExternalID)
	}
	if doc.TransactionalID == nil {
		doc.SetTransactionalID(req.TransactionalID)
	}
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
}

func prepareInternalCreateRecipients(recipients []*entity.DocumentRecipient) {
	for _, recipient := range recipients {
		if recipient == nil {
			continue
		}
		if recipient.CreatedAt.IsZero() {
			recipient.CreatedAt = time.Now().UTC()
		}
		if !recipient.Status.IsValid() {
			recipient.Status = entity.RecipientStatusPending
		}
	}
}

func marshalInternalCreateRecipients(recipients []*entity.DocumentRecipient) ([]byte, error) {
	rows := make([]internalCreateRecipientRow, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient == nil {
			continue
		}
		rows = append(rows, internalCreateRecipientRow{
			TemplateVersionRoleID: recipient.TemplateVersionRoleID,
			Name:                  recipient.Name,
			Email:                 recipient.Email,
			Status:                recipient.Status,
			SignedAt:              recipient.SignedAt,
			CreatedAt:             recipient.CreatedAt,
		})
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("marshalling internal create recipients: %w", err)
	}
	return data, nil
}

func unmarshalInternalCreatedRecipients(data []byte) ([]*entity.DocumentRecipient, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var rows []internalCreatedRecipientRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("unmarshalling created internal recipients: %w", err)
	}
	recipients := make([]*entity.DocumentRecipient, 0, len(rows))
	for _, row := range rows {
		recipients = append(recipients, &entity.DocumentRecipient{
			ID:                    row.ID,
			DocumentID:            row.DocumentID,
			TemplateVersionRoleID: row.TemplateVersionRoleID,
			Name:                  row.Name,
			Email:                 row.Email,
			Status:                row.Status,
			SignedAt:              row.SignedAt,
			CreatedAt:             row.CreatedAt,
			UpdatedAt:             row.UpdatedAt,
		})
	}
	return recipients, nil
}

func buildInternalCreateStoredDocument(
	doc *entity.Document,
	docID string,
	recipients []*entity.DocumentRecipient,
) *entity.DocumentWithRecipients {
	if doc == nil {
		return nil
	}
	stored := *doc
	stored.ID = docID
	stored.UpdatedAt = nil
	for _, recipient := range recipients {
		if recipient != nil {
			recipient.DocumentID = docID
		}
	}
	return &entity.DocumentWithRecipients{
		Document:   stored,
		Recipients: recipients,
	}
}

//nolint:funlen,gocognit,gocyclo,nestif // Transaction flow keeps conflict/replay branches explicit.
func (r *Repository) internalCreateOrReplayTx(ctx context.Context, tx pgx.Tx, req *port.InternalCreateRequest) (*port.InternalCreateResult, error) {
	lockKey := fmt.Sprintf("%s|%s|%s", req.WorkspaceID, req.ExternalID, req.DocumentTypeID)
	if _, err := tx.Exec(ctx, queryAdvisoryLockInternalCreate, lockKey); err != nil {
		return nil, fmt.Errorf("acquiring advisory lock: %w", err)
	}

	if replayDocID, ok, err := r.findTransactionalReplayTx(ctx, tx, req.WorkspaceID, req.TransactionalID); err != nil {
		return nil, err
	} else if ok {
		return &port.InternalCreateResult{
			DocumentID:       replayDocID,
			IdempotentReplay: true,
		}, nil
	}

	activeDocID, hasActive, err := r.findActiveByLogicalKeyTx(ctx, tx, req.WorkspaceID, req.DocumentTypeID, req.ExternalID)
	if err != nil {
		return nil, err
	}

	if hasActive && !req.ForceCreate {
		return &port.InternalCreateResult{
			DocumentID:       activeDocID,
			IdempotentReplay: true,
		}, nil
	}

	if hasActive {
		if _, err := tx.Exec(ctx, queryMarkSuperseded, activeDocID, req.SupersedeReason); err != nil {
			return nil, fmt.Errorf("marking active document superseded: %w", err)
		}
	}

	doc := req.Document

	var createdDocID string
	if err := tx.QueryRow(ctx, queryCreateInternalDocument,
		doc.WorkspaceID,
		doc.TemplateVersionID,
		doc.DocumentTypeID,
		doc.Title,
		doc.ClientExternalReferenceID,
		doc.TransactionalID,
		doc.OperationType,
		doc.RelatedDocumentID,
		doc.ActiveAttemptID,
		doc.Status,
		doc.InjectedValuesSnapshot,
		doc.CompletedPDFURL,
		doc.IsActive,
		doc.SupersededAt,
		doc.SupersededByDocumentID,
		doc.SupersedeReason,
		doc.ExpiresAt,
		doc.Metadata,
		doc.CreatedAt,
	).Scan(&createdDocID); err != nil {
		return nil, fmt.Errorf("creating internal document: %w", err)
	}

	for _, recipient := range req.Recipients {
		if recipient == nil {
			continue
		}
		recipient.DocumentID = createdDocID

		if err := tx.QueryRow(ctx, queryCreateInternalRecipient,
			recipient.DocumentID,
			recipient.TemplateVersionRoleID,
			recipient.Name,
			recipient.Email,
			recipient.Status,
			recipient.SignedAt,
			recipient.CreatedAt,
		).Scan(&recipient.ID); err != nil {
			return nil, fmt.Errorf("creating internal recipient: %w", err)
		}
	}

	if hasActive {
		if _, err := tx.Exec(ctx, querySetSupersededByDocument, activeDocID, createdDocID); err != nil {
			return nil, fmt.Errorf("linking superseded document: %w", err)
		}
	}

	result := &port.InternalCreateResult{
		DocumentID: createdDocID,
		Document:   buildInternalCreateStoredDocument(req.Document, createdDocID, req.Recipients),
	}
	if hasActive {
		result.SupersededPreviousDocumentID = &activeDocID
	}

	return result, nil
}

func (r *Repository) findTransactionalReplayTx(ctx context.Context, tx pgx.Tx, workspaceID, transactionalID string) (string, bool, error) {
	var docID string
	err := tx.QueryRow(ctx, queryFindByWorkspaceTransactionalForUpdate, workspaceID, transactionalID).Scan(&docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("querying transactional replay: %w", err)
	}
	return docID, true, nil
}

func (r *Repository) findActiveByLogicalKeyTx(ctx context.Context, tx pgx.Tx, workspaceID, documentTypeID, externalID string) (string, bool, error) {
	var docID string
	err := tx.QueryRow(ctx, queryFindActiveByLogicalKeyForUpdate, workspaceID, documentTypeID, externalID).Scan(&docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("querying active document by logical key: %w", err)
	}
	return docID, true, nil
}

func (r *Repository) findByWorkspaceTransactional(ctx context.Context, workspaceID, transactionalID string) (string, error) {
	var docID string
	err := r.pool.QueryRow(ctx, queryFindByWorkspaceTransactional, workspaceID, transactionalID).Scan(&docID)
	if err != nil {
		return "", err
	}
	return docID, nil
}

func (r *Repository) findActiveByLogicalKey(ctx context.Context, workspaceID, documentTypeID, externalID string) (string, error) {
	var docID string
	err := r.pool.QueryRow(ctx, queryFindActiveByLogicalKey, workspaceID, documentTypeID, externalID).Scan(&docID)
	if err != nil {
		return "", err
	}
	return docID, nil
}

func isConstraintViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == constraint
}
