package documentrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// DBTX is satisfied by both *pgxpool.Pool and pgx.Tx, allowing shared
// query implementations across regular and transactional paths.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// New creates a new document repository.
func New(pool *pgxpool.Pool) port.DocumentRepository {
	return NewConcrete(pool)
}

// NewConcrete creates a new document repository returning the concrete type.
// Use this when you need access to transactional methods like UpdateTx.
func NewConcrete(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Repository implements port.DocumentRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// scanDocument scans a full document row into a Document entity.
func scanDocument(row pgx.Row) (*entity.Document, error) {
	doc := &entity.Document{}
	err := row.Scan(
		&doc.ID,
		&doc.WorkspaceID,
		&doc.TemplateVersionID,
		&doc.DocumentTypeID,
		&doc.Title,
		&doc.ClientExternalReferenceID,
		&doc.TransactionalID,
		&doc.OperationType,
		&doc.RelatedDocumentID,
		&doc.ActiveAttemptID,
		&doc.Status,
		&doc.InjectedValuesSnapshot,
		&doc.CompletedPDFURL,
		&doc.IsActive,
		&doc.SupersededAt,
		&doc.SupersededByDocumentID,
		&doc.SupersedeReason,
		&doc.ExpiresAt,
		&doc.Metadata,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)
	return doc, err
}

// scanDocumentRows scans multiple document rows into a slice.
func scanDocumentRows(rows pgx.Rows) ([]*entity.Document, error) {
	var documents []*entity.Document
	for rows.Next() {
		doc := &entity.Document{}
		if err := rows.Scan(
			&doc.ID,
			&doc.WorkspaceID,
			&doc.TemplateVersionID,
			&doc.DocumentTypeID,
			&doc.Title,
			&doc.ClientExternalReferenceID,
			&doc.TransactionalID,
			&doc.OperationType,
			&doc.RelatedDocumentID,
			&doc.ActiveAttemptID,
			&doc.Status,
			&doc.InjectedValuesSnapshot,
			&doc.CompletedPDFURL,
			&doc.IsActive,
			&doc.SupersededAt,
			&doc.SupersededByDocumentID,
			&doc.SupersedeReason,
			&doc.ExpiresAt,
			&doc.Metadata,
			&doc.CreatedAt,
			&doc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		documents = append(documents, doc)
	}
	return documents, rows.Err()
}

func scanDocumentListItem(row pgx.Row) (*entity.DocumentListItem, error) {
	item := &entity.DocumentListItem{}
	var recipientsJSON []byte

	if err := row.Scan(
		&item.ID,
		&item.WorkspaceID,
		&item.TemplateVersionID,
		&item.DocumentTypeID,
		&item.DocumentTypeName,
		&item.TemplateName,
		&item.Title,
		&item.ClientExternalReferenceID,
		&item.SignerProvider,
		&recipientsJSON,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if len(recipientsJSON) > 0 {
		if err := json.Unmarshal(recipientsJSON, &item.Recipients); err != nil {
			return nil, fmt.Errorf("unmarshaling document recipients: %w", err)
		}
	}

	return item, nil
}

// Create creates a new document.
func (r *Repository) Create(ctx context.Context, document *entity.Document) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		document.WorkspaceID,
		document.TemplateVersionID,
		document.DocumentTypeID,
		document.Title,
		document.ClientExternalReferenceID,
		document.TransactionalID,
		document.OperationType,
		document.RelatedDocumentID,
		document.ActiveAttemptID,
		document.Status,
		document.InjectedValuesSnapshot,
		document.CompletedPDFURL,
		document.IsActive,
		document.SupersededAt,
		document.SupersededByDocumentID,
		document.SupersedeReason,
		document.ExpiresAt,
		document.Metadata,
		document.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating document: %w", err)
	}

	return id, nil
}

// FindByID finds a document by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.Document, error) {
	doc, err := scanDocument(r.pool.QueryRow(ctx, queryFindByID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrDocumentNotFound
		}
		return nil, fmt.Errorf("finding document %s: %w", id, err)
	}

	return doc, nil
}

// FindByIDWithRecipients finds a document by ID with all recipients.
func (r *Repository) FindByIDWithRecipients(ctx context.Context, id string) (*entity.DocumentWithRecipients, error) {
	doc, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	result := &entity.DocumentWithRecipients{
		Document: *doc,
	}

	// Get recipients
	rows, err := r.pool.Query(ctx, `
		SELECT id, document_id, template_version_role_id, name, email,
			   status, signed_at, created_at, updated_at
		FROM execution.document_recipients
		WHERE document_id = $1
		ORDER BY created_at ASC
	`, id)
	if err != nil {
		return nil, fmt.Errorf("querying document recipients: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		recipient := &entity.DocumentRecipient{}
		if err := rows.Scan(
			&recipient.ID,
			&recipient.DocumentID,
			&recipient.TemplateVersionRoleID,
			&recipient.Name,
			&recipient.Email,
			&recipient.Status,
			&recipient.SignedAt,
			&recipient.CreatedAt,
			&recipient.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning document recipient: %w", err)
		}
		result.Recipients = append(result.Recipients, recipient)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating document recipients: %w", err)
	}

	return result, nil
}

// appendDocumentSearchFilter appends title/recipient-email ILIKE search to query and args.
func appendDocumentSearchFilter(query string, args []any, argPos int, search string) (string, []any, int) {
	if search == "" {
		return query, args, argPos
	}
	query += fmt.Sprintf(
		` AND (
				d.title ILIKE $%[1]d
				OR EXISTS (
					SELECT 1
					FROM execution.document_recipients dr
					WHERE dr.document_id = d.id
						AND dr.email ILIKE $%[1]d
				)
			)`,
		argPos,
	)
	args = append(args, "%"+search+"%")
	argPos++
	return query, args, argPos
}

// buildDocumentFilters builds the WHERE clause and args for document filters.
// Returns the query suffix and args to append.
func buildDocumentFilters(filters port.DocumentFilters, startArgPos int) (string, []any) {
	var query string
	var args []any
	argPos := startArgPos

	if len(filters.Statuses) > 0 {
		statusStrs := make([]string, len(filters.Statuses))
		for i, s := range filters.Statuses {
			statusStrs[i] = string(s)
		}
		query += fmt.Sprintf(" AND status = ANY($%d::document_status[])", argPos)
		args = append(args, statusStrs)
		argPos++
	}

	if filters.ClientExternalReferenceID != nil {
		query += fmt.Sprintf(" AND client_external_reference_id = $%d", argPos)
		args = append(args, *filters.ClientExternalReferenceID)
		argPos++
	}

	if filters.TemplateVersionID != nil {
		query += fmt.Sprintf(" AND template_version_id = $%d", argPos)
		args = append(args, *filters.TemplateVersionID)
		argPos++
	}

	if len(filters.DocumentTypeIDs) > 0 {
		query += fmt.Sprintf(" AND d.document_type_id = ANY($%d)", argPos)
		args = append(args, filters.DocumentTypeIDs)
		argPos++
	}

	query, args, argPos = appendDocumentSearchFilter(query, args, argPos, filters.Search)

	query += " ORDER BY created_at DESC"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argPos)
		args = append(args, filters.Limit)
		argPos++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argPos)
		args = append(args, filters.Offset)
	}

	return query, args
}

// FindByWorkspace lists all documents in a workspace with optional filters.
func (r *Repository) FindByWorkspace(ctx context.Context, workspaceID string, filters port.DocumentFilters) ([]*entity.DocumentListItem, error) {
	querySuffix, filterArgs := buildDocumentFilters(filters, 2)
	query := queryFindByWorkspaceBase + querySuffix + queryFindByWorkspaceProjection
	args := append([]any{workspaceID}, filterArgs...)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying documents: %w", err)
	}
	defer rows.Close()

	var documents []*entity.DocumentListItem
	for rows.Next() {
		item, err := scanDocumentListItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning document: %w", err)
		}
		documents = append(documents, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating documents: %w", err)
	}

	return documents, nil
}

// ListDistinctDocumentTypesForWorkspace returns distinct document types present on workspace documents.
func (r *Repository) ListDistinctDocumentTypesForWorkspace(ctx context.Context, workspaceID string) ([]*entity.DocumentTypeFilterOption, error) {
	rows, err := r.pool.Query(ctx, queryListDistinctDocumentTypesForWorkspace, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("querying document type options: %w", err)
	}
	defer rows.Close()

	var out []*entity.DocumentTypeFilterOption
	for rows.Next() {
		var opt entity.DocumentTypeFilterOption
		if err := rows.Scan(&opt.ID, &opt.Name); err != nil {
			return nil, fmt.Errorf("scanning document type option: %w", err)
		}
		out = append(out, &opt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating document type options: %w", err)
	}
	return out, nil
}

// FindByClientExternalRef finds documents by the client's external reference ID.
func (r *Repository) FindByClientExternalRef(ctx context.Context, workspaceID, clientExternalRef string) ([]*entity.Document, error) {
	rows, err := r.pool.Query(ctx, queryFindByClientExternalRef, workspaceID, clientExternalRef)
	if err != nil {
		return nil, fmt.Errorf("querying documents by client external ref: %w", err)
	}
	defer rows.Close()

	documents, err := scanDocumentRows(rows)
	if err != nil {
		return nil, fmt.Errorf("scanning documents by client external ref: %w", err)
	}

	return documents, nil
}

// FindByTemplateVersion finds all documents generated from a specific template version.
func (r *Repository) FindByTemplateVersion(ctx context.Context, templateVersionID string) ([]*entity.DocumentListItem, error) {
	rows, err := r.pool.Query(ctx, queryFindByTemplateVersion, templateVersionID)
	if err != nil {
		return nil, fmt.Errorf("querying documents by template version: %w", err)
	}
	defer rows.Close()

	var documents []*entity.DocumentListItem
	for rows.Next() {
		item := &entity.DocumentListItem{}
		if err := rows.Scan(
			&item.ID,
			&item.WorkspaceID,
			&item.TemplateVersionID,
			&item.Title,
			&item.ClientExternalReferenceID,
			&item.SignerProvider,
			&item.Status,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning document: %w", err)
		}
		documents = append(documents, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating documents: %w", err)
	}

	return documents, nil
}

// Update updates a document using the connection pool.
func (r *Repository) Update(ctx context.Context, document *entity.Document) error {
	return r.updateWith(ctx, r.pool, document)
}

// UpdateTx updates a document within an existing transaction.
func (r *Repository) UpdateTx(ctx context.Context, tx pgx.Tx, document *entity.Document) error {
	return r.updateWith(ctx, tx, document)
}

// updateWith is the shared implementation for both Update and UpdateTx.
func (r *Repository) updateWith(ctx context.Context, db DBTX, document *entity.Document) error {
	result, err := db.Exec(ctx, queryUpdate,
		document.ID,
		document.DocumentTypeID,
		document.Title,
		document.ClientExternalReferenceID,
		document.TransactionalID,
		document.OperationType,
		document.RelatedDocumentID,
		document.ActiveAttemptID,
		document.Status,
		document.InjectedValuesSnapshot,
		document.CompletedPDFURL,
		document.IsActive,
		document.SupersededAt,
		document.SupersededByDocumentID,
		document.SupersedeReason,
		document.ExpiresAt,
		document.Metadata,
		document.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("updating document: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrDocumentNotFound
	}

	return nil
}

// UpdateStatus updates only the status of a document.
func (r *Repository) UpdateStatus(ctx context.Context, id string, status entity.DocumentStatus) error {
	result, err := r.pool.Exec(ctx, queryUpdateStatus, id, status)
	if err != nil {
		return fmt.Errorf("updating document status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrDocumentNotFound
	}

	return nil
}

// Delete deletes a document and all its recipients (cascade).
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting document: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrDocumentNotFound
	}

	return nil
}

// CountByWorkspace returns the total number of documents in a workspace.
func (r *Repository) CountByWorkspace(ctx context.Context, workspaceID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryCountByWorkspace, workspaceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting documents in workspace: %w", err)
	}

	return count, nil
}

// CountByStatus returns the count of documents by status in a workspace.
func (r *Repository) CountByStatus(ctx context.Context, workspaceID string, status entity.DocumentStatus) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryCountByStatus, workspaceID, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting documents by status: %w", err)
	}

	return count, nil
}

// FindExpired finds documents that have passed their expiration time and are still active.
func (r *Repository) FindExpired(ctx context.Context, limit int) ([]*entity.Document, error) {
	rows, err := r.pool.Query(ctx, queryFindExpired, limit)
	if err != nil {
		return nil, fmt.Errorf("querying expired documents: %w", err)
	}
	defer rows.Close()

	documents, err := scanDocumentRows(rows)
	if err != nil {
		return nil, fmt.Errorf("scanning expired documents: %w", err)
	}

	return documents, nil
}
