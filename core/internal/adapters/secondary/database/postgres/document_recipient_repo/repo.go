package documentrecipientrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new document recipient repository.
func New(pool *pgxpool.Pool) port.DocumentRecipientRepository {
	return &Repository{pool: pool}
}

// Repository implements port.DocumentRecipientRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new document recipient.
func (r *Repository) Create(ctx context.Context, recipient *entity.DocumentRecipient) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		recipient.DocumentID,
		recipient.TemplateVersionRoleID,
		recipient.Name,
		recipient.Email,
		recipient.Status,
		recipient.SignedAt,
		recipient.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating document recipient: %w", err)
	}

	return id, nil
}

// CreateBatch creates multiple recipients for a document.
func (r *Repository) CreateBatch(ctx context.Context, recipients []*entity.DocumentRecipient) error {
	if len(recipients) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, recipient := range recipients {
		batch.Queue(queryCreate,
			recipient.DocumentID,
			recipient.TemplateVersionRoleID,
			recipient.Name,
			recipient.Email,
			recipient.Status,
			recipient.SignedAt,
			recipient.CreatedAt,
		)
	}

	results := r.pool.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()

	for i := 0; i < len(recipients); i++ {
		var id string
		if err := results.QueryRow().Scan(&id); err != nil {
			return fmt.Errorf("creating document recipient %d: %w", i, err)
		}
		recipients[i].ID = id
	}

	return nil
}

// FindByID finds a recipient by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.DocumentRecipient, error) {
	recipient := &entity.DocumentRecipient{}
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&recipient.ID,
		&recipient.DocumentID,
		&recipient.TemplateVersionRoleID,
		&recipient.Name,
		&recipient.Email,
		&recipient.Status,
		&recipient.SignedAt,
		&recipient.CreatedAt,
		&recipient.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrDocumentRecipientNotFound
		}
		return nil, fmt.Errorf("finding document recipient %s: %w", id, err)
	}

	return recipient, nil
}

// FindByDocumentID finds all recipients for a document.
func (r *Repository) FindByDocumentID(ctx context.Context, documentID string) ([]*entity.DocumentRecipient, error) {
	rows, err := r.pool.Query(ctx, queryFindByDocumentID, documentID)
	if err != nil {
		return nil, fmt.Errorf("querying document recipients: %w", err)
	}
	defer rows.Close()

	var recipients []*entity.DocumentRecipient
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
		recipients = append(recipients, recipient)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating document recipients: %w", err)
	}

	return recipients, nil
}

// FindByDocumentIDWithRoles finds all recipients for a document with their role information.
func (r *Repository) FindByDocumentIDWithRoles(ctx context.Context, documentID string) ([]*entity.DocumentRecipientWithRole, error) {
	rows, err := r.pool.Query(ctx, queryFindByDocumentIDWithRoles, documentID)
	if err != nil {
		return nil, fmt.Errorf("querying document recipients with roles: %w", err)
	}
	defer rows.Close()

	var recipients []*entity.DocumentRecipientWithRole
	for rows.Next() {
		recipient := &entity.DocumentRecipientWithRole{}
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
			&recipient.RoleName,
			&recipient.SignerOrder,
		); err != nil {
			return nil, fmt.Errorf("scanning document recipient with role: %w", err)
		}
		recipients = append(recipients, recipient)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating document recipients: %w", err)
	}

	return recipients, nil
}

// FindBySignerRecipientID finds a recipient by the external signing provider's recipient ID.
func (r *Repository) FindBySignerRecipientID(ctx context.Context, signerRecipientID string) (*entity.DocumentRecipient, error) {
	recipient := &entity.DocumentRecipient{}
	err := r.pool.QueryRow(ctx, queryFindBySignerRecipientID, signerRecipientID).Scan(
		&recipient.ID,
		&recipient.DocumentID,
		&recipient.TemplateVersionRoleID,
		&recipient.Name,
		&recipient.Email,
		&recipient.Status,
		&recipient.SignedAt,
		&recipient.CreatedAt,
		&recipient.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrDocumentRecipientNotFound
		}
		return nil, fmt.Errorf("finding document recipient by signer recipient ID %s: %w", signerRecipientID, err)
	}

	return recipient, nil
}

// FindByDocumentAndEmail finds a recipient by document ID and email (case-insensitive).
func (r *Repository) FindByDocumentAndEmail(ctx context.Context, documentID, email string) (*entity.DocumentRecipient, error) {
	recipient := &entity.DocumentRecipient{}
	err := r.pool.QueryRow(ctx, queryFindByDocumentAndEmail, documentID, email).Scan(
		&recipient.ID,
		&recipient.DocumentID,
		&recipient.TemplateVersionRoleID,
		&recipient.Name,
		&recipient.Email,
		&recipient.Status,
		&recipient.SignedAt,
		&recipient.CreatedAt,
		&recipient.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrDocumentRecipientNotFound
		}
		return nil, fmt.Errorf("finding document recipient by document and email: %w", err)
	}

	return recipient, nil
}

// FindByDocumentAndRole finds a recipient by document ID and role ID.
func (r *Repository) FindByDocumentAndRole(ctx context.Context, documentID, roleID string) (*entity.DocumentRecipient, error) {
	recipient := &entity.DocumentRecipient{}
	err := r.pool.QueryRow(ctx, queryFindByDocumentAndRole, documentID, roleID).Scan(
		&recipient.ID,
		&recipient.DocumentID,
		&recipient.TemplateVersionRoleID,
		&recipient.Name,
		&recipient.Email,
		&recipient.Status,
		&recipient.SignedAt,
		&recipient.CreatedAt,
		&recipient.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrDocumentRecipientNotFound
		}
		return nil, fmt.Errorf("finding document recipient by document and role: %w", err)
	}

	return recipient, nil
}

// Update updates a document recipient.
func (r *Repository) Update(ctx context.Context, recipient *entity.DocumentRecipient) error {
	result, err := r.pool.Exec(ctx, queryUpdate,
		recipient.ID,
		recipient.Name,
		recipient.Email,
		recipient.Status,
		recipient.SignedAt,
		recipient.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("updating document recipient: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrDocumentRecipientNotFound
	}

	return nil
}

// UpdateStatus updates only the status of a recipient.
func (r *Repository) UpdateStatus(ctx context.Context, id string, status entity.RecipientStatus) error {
	result, err := r.pool.Exec(ctx, queryUpdateStatus, id, status)
	if err != nil {
		return fmt.Errorf("updating document recipient status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrDocumentRecipientNotFound
	}

	return nil
}

// UpdateSignerInfo updates the signer provider recipient ID.
func (r *Repository) UpdateSignerInfo(ctx context.Context, id, signerRecipientID string) error {
	// Provider recipient IDs are attempt-owned in execution.signing_attempt_recipients.
	return nil
}

// Delete deletes a document recipient.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting document recipient: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrDocumentRecipientNotFound
	}

	return nil
}

// DeleteByDocumentID deletes all recipients for a document.
func (r *Repository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	_, err := r.pool.Exec(ctx, queryDeleteByDocumentID, documentID)
	if err != nil {
		return fmt.Errorf("deleting document recipients: %w", err)
	}

	return nil
}

// CountByDocumentAndStatus returns the count of recipients by status for a document.
func (r *Repository) CountByDocumentAndStatus(ctx context.Context, documentID string, status entity.RecipientStatus) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryCountByDocumentAndStatus, documentID, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting document recipients by status: %w", err)
	}

	return count, nil
}

// CountByDocument returns the total number of recipients for a document.
func (r *Repository) CountByDocument(ctx context.Context, documentID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryCountByDocument, documentID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting document recipients: %w", err)
	}

	return count, nil
}

// AllSigned checks if all recipients for a document have signed.
func (r *Repository) AllSigned(ctx context.Context, documentID string) (bool, error) {
	var allSigned bool
	err := r.pool.QueryRow(ctx, queryAllSigned, documentID).Scan(&allSigned)
	if err != nil {
		return false, fmt.Errorf("checking if all recipients signed: %w", err)
	}

	return allSigned, nil
}

// AnyDeclined checks if any recipient has declined.
func (r *Repository) AnyDeclined(ctx context.Context, documentID string) (bool, error) {
	var anyDeclined bool
	err := r.pool.QueryRow(ctx, queryAnyDeclined, documentID).Scan(&anyDeclined)
	if err != nil {
		return false, fmt.Errorf("checking if any recipient declined: %w", err)
	}

	return anyDeclined, nil
}
