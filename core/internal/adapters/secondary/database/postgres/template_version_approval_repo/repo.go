package templateversionapprovalrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new template version approval repository.
func New(pool *pgxpool.Pool) port.TemplateVersionApprovalRepository {
	return &Repository{pool: pool}
}

// Repository stores schools' approvals of contract text.
type Repository struct {
	pool *pgxpool.Pool
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanApproval(row rowScanner) (*entity.TemplateVersionApproval, error) {
	var a entity.TemplateVersionApproval
	err := row.Scan(
		&a.ID, &a.TemplateVersionID, &a.ContentChecksum, &a.Status,
		&a.ProposedBy, &a.ProposedAt,
		&a.DecidedByEmail, &a.DecidedByName, &a.DecidedAtCampus, &a.DecidedAt,
		&a.Comment, &a.ApprovedPDFPath, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Create records a new proposal.
func (r *Repository) Create(
	ctx context.Context,
	approval *entity.TemplateVersionApproval,
) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		approval.TemplateVersionID,
		approval.ContentChecksum,
		approval.Status,
		approval.ProposedBy,
		approval.ProposedAt,
		approval.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating template version approval: %w", err)
	}
	approval.ID = id
	return id, nil
}

// FindByID finds one approval.
func (r *Repository) FindByID(
	ctx context.Context,
	id string,
) (*entity.TemplateVersionApproval, error) {
	approval, err := scanApproval(r.pool.QueryRow(ctx, queryFindByID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrApprovalNotFound
		}
		return nil, fmt.Errorf("finding template version approval: %w", err)
	}
	return approval, nil
}

// FindLatestForVersion returns the newest approval for a version.
func (r *Repository) FindLatestForVersion(
	ctx context.Context,
	versionID string,
) (*entity.TemplateVersionApproval, error) {
	approval, err := scanApproval(r.pool.QueryRow(ctx, queryFindLatestForVersion, versionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrApprovalNotFound
		}
		return nil, fmt.Errorf("finding latest approval: %w", err)
	}
	return approval, nil
}

// FindPendingForVersion returns the outstanding proposal for a version, if any.
func (r *Repository) FindPendingForVersion(
	ctx context.Context,
	versionID string,
) (*entity.TemplateVersionApproval, error) {
	approval, err := scanApproval(r.pool.QueryRow(ctx, queryFindPendingForVersion, versionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrApprovalNotFound
		}
		return nil, fmt.Errorf("finding pending approval: %w", err)
	}
	return approval, nil
}

// ListForVersion returns a version's approval history, newest first.
func (r *Repository) ListForVersion(
	ctx context.Context,
	versionID string,
) ([]*entity.TemplateVersionApproval, error) {
	rows, err := r.pool.Query(ctx, queryListForVersion, versionID)
	if err != nil {
		return nil, fmt.Errorf("listing approvals: %w", err)
	}
	defer rows.Close()

	var approvals []*entity.TemplateVersionApproval
	for rows.Next() {
		approval, scanErr := scanApproval(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scanning approval: %w", scanErr)
		}
		approvals = append(approvals, approval)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterating approvals: %w", rows.Err())
	}
	return approvals, nil
}

// RecordDecision writes a decision onto a proposal that is still PENDING.
//
// Reports ErrApprovalAlreadyDecided when no row matched, which means someone else
// decided first. That is a real outcome rather than an error condition: the guard
// lives in the UPDATE so two simultaneous decisions cannot both land.
func (r *Repository) RecordDecision(
	ctx context.Context,
	approval *entity.TemplateVersionApproval,
) error {
	tag, err := r.pool.Exec(ctx, queryRecordDecision,
		approval.ID,
		approval.Status,
		approval.DecidedByEmail,
		approval.DecidedByName,
		approval.DecidedAtCampus,
		approval.DecidedAt,
		approval.Comment,
	)
	if err != nil {
		return fmt.Errorf("recording approval decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return entity.ErrApprovalAlreadyDecided
	}
	return nil
}
