package templateversionrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new template version repository.
func New(pool *pgxpool.Pool) port.TemplateVersionRepository {
	return &Repository{pool: pool}
}

// Repository implements port.TemplateVersionRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

type templateVersionRow interface {
	Scan(dest ...any) error
}

// Create creates a new template version.
func (r *Repository) Create(ctx context.Context, version *entity.TemplateVersion) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		version.TemplateID,
		version.VersionNumber,
		version.Name,
		version.Description,
		version.ContentStructure,
		version.Status,
		version.ScheduledPublishAt,
		version.ScheduledArchiveAt,
		version.SigningWorkflowConfig,
		version.CreatedBy,
		version.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating template version: %w", err)
	}

	return id, nil
}

// FindByID finds a template version by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.TemplateVersion, error) {
	version, err := scanTemplateVersion(r.pool.QueryRow(ctx, queryFindByID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrVersionNotFound
		}
		return nil, fmt.Errorf("finding template version %s: %w", id, err)
	}

	return version, nil
}

// FindByIDWithDetails finds a template version by ID with all related data.
func (r *Repository) FindByIDWithDetails(ctx context.Context, id string) (*entity.TemplateVersionWithDetails, error) {
	details, err := scanTemplateVersionWithDetails(r.pool.QueryRow(ctx, queryFindByIDWithDetails, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrVersionNotFound
		}
		return nil, fmt.Errorf("finding template version with details %s: %w", id, err)
	}

	return details, nil
}

// FindByIDWithDetailsAndTemplateWorkspace finds a version plus template/workspace context in one query.
func (r *Repository) FindByIDWithDetailsAndTemplateWorkspace(ctx context.Context, id string) (*port.TemplateVersionContext, error) {
	result, err := scanTemplateVersionContext(r.pool.QueryRow(ctx, queryFindByIDWithDetailsAndTemplateWorkspace, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrVersionNotFound
		}
		return nil, fmt.Errorf("finding template version context %s: %w", id, err)
	}
	return result, nil
}

// FindByTemplateID lists all versions for a template.
func (r *Repository) FindByTemplateID(ctx context.Context, templateID string) ([]*entity.TemplateVersion, error) {
	rows, err := r.pool.Query(ctx, queryFindByTemplateID, templateID)
	if err != nil {
		return nil, fmt.Errorf("querying template versions: %w", err)
	}
	defer rows.Close()

	return collectTemplateVersions(rows, "scanning template version", "iterating template versions")
}

// FindByTemplateIDWithDetails lists all versions for a template with full details.
func (r *Repository) FindByTemplateIDWithDetails(ctx context.Context, templateID string) ([]*entity.TemplateVersionWithDetails, error) {
	versions, err := r.FindByTemplateID(ctx, templateID)
	if err != nil {
		return nil, err
	}

	results := make([]*entity.TemplateVersionWithDetails, 0, len(versions))
	for _, v := range versions {
		details, err := r.FindByIDWithDetails(ctx, v.ID)
		if err != nil {
			return nil, err
		}
		results = append(results, details)
	}

	return results, nil
}

// FindPublishedByTemplateID finds the currently published version for a template.
func (r *Repository) FindPublishedByTemplateID(ctx context.Context, templateID string) (*entity.TemplateVersion, error) {
	version, err := scanTemplateVersion(r.pool.QueryRow(ctx, queryFindPublishedByTemplateID, templateID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrNoPublishedVersion
		}
		return nil, fmt.Errorf("finding published version for template %s: %w", templateID, err)
	}

	return version, nil
}

// FindPublishedByTemplateIDWithDetails finds the published version with all details.
func (r *Repository) FindPublishedByTemplateIDWithDetails(ctx context.Context, templateID string) (*entity.TemplateVersionWithDetails, error) {
	version, err := r.FindPublishedByTemplateID(ctx, templateID)
	if err != nil {
		return nil, err
	}

	return r.FindByIDWithDetails(ctx, version.ID)
}

// FindScheduledToPublish finds all versions scheduled to publish before the given time.
func (r *Repository) FindScheduledToPublish(ctx context.Context, before time.Time) ([]*entity.TemplateVersion, error) {
	rows, err := r.pool.Query(ctx, queryFindScheduledToPublish, before)
	if err != nil {
		return nil, fmt.Errorf("querying scheduled versions to publish: %w", err)
	}
	defer rows.Close()

	return collectTemplateVersions(rows, "scanning scheduled version", "iterating scheduled versions")
}

// FindScheduledToArchive finds all published versions scheduled to archive before the given time.
func (r *Repository) FindScheduledToArchive(ctx context.Context, before time.Time) ([]*entity.TemplateVersion, error) {
	rows, err := r.pool.Query(ctx, queryFindScheduledToArchive, before)
	if err != nil {
		return nil, fmt.Errorf("querying scheduled versions to archive: %w", err)
	}
	defer rows.Close()

	return collectTemplateVersions(rows, "scanning scheduled archive version", "iterating scheduled archive versions")
}

func scanTemplateVersion(row templateVersionRow) (*entity.TemplateVersion, error) {
	version := &entity.TemplateVersion{}
	if err := row.Scan(
		&version.ID,
		&version.TemplateID,
		&version.VersionNumber,
		&version.Name,
		&version.Description,
		&version.ContentStructure,
		&version.Status,
		&version.ScheduledPublishAt,
		&version.ScheduledArchiveAt,
		&version.SigningWorkflowConfig,
		&version.PublishedAt,
		&version.ArchivedAt,
		&version.PublishedBy,
		&version.ArchivedBy,
		&version.CreatedBy,
		&version.CreatedAt,
		&version.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return version, nil
}

func scanTemplateVersionWithDetails(row templateVersionRow) (*entity.TemplateVersionWithDetails, error) {
	version := &entity.TemplateVersion{}
	var injectablesJSON, signerRolesJSON []byte
	if err := row.Scan(
		&version.ID,
		&version.TemplateID,
		&version.VersionNumber,
		&version.Name,
		&version.Description,
		&version.ContentStructure,
		&version.Status,
		&version.ScheduledPublishAt,
		&version.ScheduledArchiveAt,
		&version.SigningWorkflowConfig,
		&version.PublishedAt,
		&version.ArchivedAt,
		&version.PublishedBy,
		&version.ArchivedBy,
		&version.CreatedBy,
		&version.CreatedAt,
		&version.UpdatedAt,
		&injectablesJSON,
		&signerRolesJSON,
	); err != nil {
		return nil, err
	}

	details := &entity.TemplateVersionWithDetails{TemplateVersion: *version}
	if err := json.Unmarshal(injectablesJSON, &details.Injectables); err != nil {
		return nil, fmt.Errorf("unmarshaling version injectables: %w", err)
	}
	if err := json.Unmarshal(signerRolesJSON, &details.SignerRoles); err != nil {
		return nil, fmt.Errorf("unmarshaling version signer roles: %w", err)
	}
	return details, nil
}

//nolint:funlen // Single DB projection scan; keeping columns together makes the SQL/Scan contract auditable.
func scanTemplateVersionContext(row templateVersionRow) (*port.TemplateVersionContext, error) {
	version := &entity.TemplateVersion{}
	template := &entity.Template{}
	workspace := &entity.Workspace{}
	var injectablesJSON, signerRolesJSON []byte
	var workspaceTenantID sql.NullString
	var workspaceSandboxOfID sql.NullString
	if err := row.Scan(
		&version.ID,
		&version.TemplateID,
		&version.VersionNumber,
		&version.Name,
		&version.Description,
		&version.ContentStructure,
		&version.Status,
		&version.ScheduledPublishAt,
		&version.ScheduledArchiveAt,
		&version.SigningWorkflowConfig,
		&version.PublishedAt,
		&version.ArchivedAt,
		&version.PublishedBy,
		&version.ArchivedBy,
		&version.CreatedBy,
		&version.CreatedAt,
		&version.UpdatedAt,
		&injectablesJSON,
		&signerRolesJSON,
		&template.ID,
		&template.WorkspaceID,
		&template.FolderID,
		&template.DocumentTypeID,
		&template.Title,
		&template.IsPublicLibrary,
		&template.Process,
		&template.ProcessType,
		&template.CreatedAt,
		&template.UpdatedAt,
		&workspace.ID,
		&workspaceTenantID,
		&workspace.Name,
		&workspace.Code,
		&workspace.Type,
		&workspace.Status,
		&workspace.IsSandbox,
		&workspaceSandboxOfID,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if workspaceTenantID.Valid {
		workspace.TenantID = &workspaceTenantID.String
	}
	if workspaceSandboxOfID.Valid {
		workspace.SandboxOfID = &workspaceSandboxOfID.String
	}

	details := &entity.TemplateVersionWithDetails{TemplateVersion: *version}
	if err := json.Unmarshal(injectablesJSON, &details.Injectables); err != nil {
		return nil, fmt.Errorf("unmarshaling version injectables: %w", err)
	}
	if err := json.Unmarshal(signerRolesJSON, &details.SignerRoles); err != nil {
		return nil, fmt.Errorf("unmarshaling version signer roles: %w", err)
	}

	return &port.TemplateVersionContext{
		Version:   details,
		Template:  template,
		Workspace: workspace,
	}, nil
}

func collectTemplateVersions(rows pgx.Rows, scanErrMsg, iterateErrMsg string) ([]*entity.TemplateVersion, error) {
	var versions []*entity.TemplateVersion
	for rows.Next() {
		version, err := scanTemplateVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", scanErrMsg, err)
		}
		versions = append(versions, version)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", iterateErrMsg, err)
	}

	return versions, nil
}

// Update updates a template version.
func (r *Repository) Update(ctx context.Context, version *entity.TemplateVersion) error {
	result, err := r.pool.Exec(ctx, queryUpdate,
		version.ID,
		version.Name,
		version.Description,
		version.ContentStructure,
		version.Status,
		version.ScheduledPublishAt,
		version.ScheduledArchiveAt,
		version.SigningWorkflowConfig,
		version.PublishedAt,
		version.ArchivedAt,
		version.PublishedBy,
		version.ArchivedBy,
		version.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("updating template version: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrVersionNotFound
	}

	return nil
}

// UpdateStatus updates a version's status with optional user tracking.
func (r *Repository) UpdateStatus(ctx context.Context, id string, status entity.VersionStatus, userID *string) error {
	var query string
	var args []any

	switch status {
	case entity.VersionStatusPublished:
		query = queryUpdateStatusPublished
		args = []any{id, status, userID}
	case entity.VersionStatusArchived:
		query = queryUpdateStatusArchived
		args = []any{id, status, userID}
	default:
		query = queryUpdateStatusDefault
		args = []any{id, status}
	}

	result, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating version status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrVersionNotFound
	}

	return nil
}

// Delete deletes a template version.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting template version: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrVersionNotFound
	}

	return nil
}

// ExistsByVersionNumber checks if a version number already exists for the template.
func (r *Repository) ExistsByVersionNumber(ctx context.Context, templateID string, versionNumber int) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByVersionNumber, templateID, versionNumber).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking version number existence: %w", err)
	}

	return exists, nil
}

// ExistsByName checks if a version name already exists for the template.
func (r *Repository) ExistsByName(ctx context.Context, templateID, name string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByName, templateID, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking version name existence: %w", err)
	}

	return exists, nil
}

// ExistsByNameExcluding checks if a version name exists excluding a specific version ID.
func (r *Repository) ExistsByNameExcluding(ctx context.Context, templateID, name, excludeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByNameExcluding, templateID, name, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking version name existence: %w", err)
	}

	return exists, nil
}

// GetNextVersionNumber returns the next available version number for a template.
func (r *Repository) GetNextVersionNumber(ctx context.Context, templateID string) (int, error) {
	var nextNum int
	err := r.pool.QueryRow(ctx, queryGetNextVersionNumber, templateID).Scan(&nextNum)
	if err != nil {
		return 0, fmt.Errorf("getting next version number: %w", err)
	}

	return nextNum, nil
}

// HasScheduledVersion checks if the template has a version with SCHEDULED status.
func (r *Repository) HasScheduledVersion(ctx context.Context, templateID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryHasScheduledVersion, templateID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking for scheduled version: %w", err)
	}

	return exists, nil
}

// ExistsScheduledAtTime checks if another version is scheduled at the exact time for the template.
func (r *Repository) ExistsScheduledAtTime(ctx context.Context, templateID string, scheduledAt time.Time, excludeVersionID *string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsScheduledAtTime, templateID, scheduledAt, excludeVersionID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking scheduled time conflict: %w", err)
	}

	return exists, nil
}

// CountByTemplateID returns the number of versions for a template.
func (r *Repository) CountByTemplateID(ctx context.Context, templateID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryCountByTemplateID, templateID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting template versions: %w", err)
	}

	return count, nil
}
