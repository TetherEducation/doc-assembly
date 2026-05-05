package templaterepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// New creates a new template repository.
func New(pool *pgxpool.Pool) port.TemplateRepository {
	return &Repository{pool: pool}
}

// Repository implements port.TemplateRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new template.
func (r *Repository) Create(ctx context.Context, template *entity.Template) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		template.WorkspaceID,
		template.FolderID,
		template.DocumentTypeID,
		template.Title,
		template.IsPublicLibrary,
		template.Process,
		template.ProcessType,
		template.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating template: %w", err)
	}

	return id, nil
}

// FindByID finds a template by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.Template, error) {
	template := &entity.Template{}
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
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
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrTemplateNotFound
		}
		return nil, fmt.Errorf("finding template %s: %w", id, err)
	}

	return template, nil
}

// FindByIDWithDetails finds a template by ID with published version, tags, and folder.
func (r *Repository) FindByIDWithDetails(ctx context.Context, id string) (*entity.TemplateWithDetails, error) {
	template, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	details := &entity.TemplateWithDetails{Template: *template}

	// Load published version with details
	version, err := r.loadPublishedVersion(ctx, id)
	if err == nil && version != nil {
		versionDetails := &entity.TemplateVersionWithDetails{TemplateVersion: *version}
		versionDetails.Injectables, _ = r.loadVersionInjectables(ctx, version.ID)
		versionDetails.SignerRoles, _ = r.loadVersionSignerRoles(ctx, version.ID)
		details.PublishedVersion = versionDetails
	}

	details.Tags, _ = r.loadTemplateTags(ctx, id)
	if template.FolderID != nil {
		details.Folder = r.loadFolder(ctx, *template.FolderID)
	}

	return details, nil
}

// FindByIDWithAllVersions finds a template by ID with all versions.
func (r *Repository) FindByIDWithAllVersions(ctx context.Context, id string) (*entity.TemplateWithAllVersions, error) {
	template, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	result := &entity.TemplateWithAllVersions{Template: *template}

	// Load all versions
	result.Versions, err = r.loadAllVersions(ctx, id)
	if err != nil {
		return nil, err
	}

	result.Tags, _ = r.loadTemplateTags(ctx, id)
	if template.FolderID != nil {
		result.Folder = r.loadFolder(ctx, *template.FolderID)
	}
	if template.DocumentTypeID != nil {
		result.DocumentType = r.loadDocumentType(ctx, *template.DocumentTypeID)
	}

	return result, nil
}

// FindByWorkspace lists all templates in a workspace with filters.
func (r *Repository) FindByWorkspace(ctx context.Context, workspaceID string, filters port.TemplateFilters) ([]*entity.TemplateListItem, error) {
	filterQuery, filterArgs := buildTemplateFilters(filters, 2)
	query := queryFindByWorkspaceBase + filterQuery
	args := append([]any{workspaceID}, filterArgs...)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying templates: %w", err)
	}
	defer rows.Close()

	templates, err := scanTemplateListItems(rows)
	if err != nil {
		return nil, err
	}

	if err := r.loadTagsForTemplates(ctx, templates); err != nil {
		return nil, err
	}

	return templates, nil
}

// FindByFolder lists all templates in a folder.
func (r *Repository) FindByFolder(ctx context.Context, folderID string) ([]*entity.TemplateListItem, error) {
	rows, err := r.pool.Query(ctx, queryFindByFolder, folderID)
	if err != nil {
		return nil, fmt.Errorf("querying templates by folder: %w", err)
	}
	defer rows.Close()

	templates, err := scanTemplateListItems(rows)
	if err != nil {
		return nil, err
	}

	// Load tags in batch for all templates
	if err := r.loadTagsForTemplates(ctx, templates); err != nil {
		return nil, err
	}

	return templates, nil
}

// FindPublicLibrary lists all public library templates (that have a published version).
func (r *Repository) FindPublicLibrary(ctx context.Context, workspaceID string) ([]*entity.TemplateListItem, error) {
	rows, err := r.pool.Query(ctx, queryFindPublicLibrary)
	if err != nil {
		return nil, fmt.Errorf("querying public library templates: %w", err)
	}
	defer rows.Close()

	templates, err := scanTemplateListItems(rows)
	if err != nil {
		return nil, err
	}

	// Load tags in batch for all templates
	if err := r.loadTagsForTemplates(ctx, templates); err != nil {
		return nil, err
	}

	return templates, nil
}

// loadPublishedVersion loads the published version for a template.
func (r *Repository) loadPublishedVersion(ctx context.Context, templateID string) (*entity.TemplateVersion, error) {
	version := &entity.TemplateVersion{}
	err := r.pool.QueryRow(ctx, queryPublishedVersion, templateID).Scan(
		&version.ID,
		&version.TemplateID,
		&version.VersionNumber,
		&version.Name,
		&version.Description,
		&version.ContentStructure,
		&version.Status,
		&version.ScheduledPublishAt,
		&version.ScheduledArchiveAt,
		&version.PublishedAt,
		&version.ArchivedAt,
		&version.PublishedBy,
		&version.ArchivedBy,
		&version.CreatedBy,
		&version.CreatedAt,
		&version.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return version, nil
}

// loadVersionInjectables loads injectables for a version.
func (r *Repository) loadVersionInjectables(ctx context.Context, versionID string) ([]*entity.VersionInjectableWithDefinition, error) {
	rows, err := r.pool.Query(ctx, queryVersionInjectables, versionID)
	if err != nil {
		return nil, fmt.Errorf("querying version injectables: %w", err)
	}
	defer rows.Close()

	var injectables []*entity.VersionInjectableWithDefinition
	for rows.Next() {
		iwd := &entity.VersionInjectableWithDefinition{
			Definition: &entity.InjectableDefinition{},
		}
		if err := rows.Scan(
			&iwd.ID, &iwd.TemplateVersionID, &iwd.InjectableDefinitionID,
			&iwd.IsRequired, &iwd.DefaultValue, &iwd.CreatedAt,
			&iwd.Definition.ID, &iwd.Definition.WorkspaceID, &iwd.Definition.Key,
			&iwd.Definition.Label, &iwd.Definition.Description, &iwd.Definition.DataType,
			&iwd.Definition.CreatedAt, &iwd.Definition.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning version injectable: %w", err)
		}
		injectables = append(injectables, iwd)
	}
	return injectables, rows.Err()
}

// loadVersionSignerRoles loads signer roles for a version.
func (r *Repository) loadVersionSignerRoles(ctx context.Context, versionID string) ([]*entity.TemplateVersionSignerRole, error) {
	rows, err := r.pool.Query(ctx, queryVersionSignerRoles, versionID)
	if err != nil {
		return nil, fmt.Errorf("querying version signer roles: %w", err)
	}
	defer rows.Close()

	var roles []*entity.TemplateVersionSignerRole
	for rows.Next() {
		role := &entity.TemplateVersionSignerRole{}
		if err := rows.Scan(
			&role.ID, &role.TemplateVersionID, &role.RoleName,
			&role.AnchorString, &role.SignerOrder,
			&role.CreatedAt, &role.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning version signer role: %w", err)
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

// loadTemplateTags loads tags for a template.
func (r *Repository) loadTemplateTags(ctx context.Context, templateID string) ([]*entity.Tag, error) {
	rows, err := r.pool.Query(ctx, queryTemplateTags, templateID)
	if err != nil {
		return nil, fmt.Errorf("querying template tags: %w", err)
	}
	defer rows.Close()

	var tags []*entity.Tag
	for rows.Next() {
		tag := &entity.Tag{}
		if err := rows.Scan(
			&tag.ID, &tag.WorkspaceID, &tag.Name,
			&tag.Color, &tag.CreatedAt, &tag.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning template tag: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// loadFolder loads a folder by ID.
func (r *Repository) loadFolder(ctx context.Context, folderID string) *entity.Folder {
	folder := &entity.Folder{}
	err := r.pool.QueryRow(ctx, queryFolder, folderID).Scan(
		&folder.ID, &folder.WorkspaceID, &folder.ParentID,
		&folder.Name, &folder.CreatedAt, &folder.UpdatedAt,
	)
	if err != nil {
		return nil
	}
	return folder
}

// loadDocumentType loads a document type by ID.
func (r *Repository) loadDocumentType(ctx context.Context, documentTypeID string) *entity.DocumentType {
	docType := &entity.DocumentType{}
	err := r.pool.QueryRow(ctx, queryDocumentType, documentTypeID).Scan(
		&docType.ID, &docType.TenantID, &docType.Code,
		&docType.Name, &docType.Description,
		&docType.CreatedAt, &docType.UpdatedAt,
	)
	if err != nil {
		return nil
	}
	return docType
}

// loadAllVersions loads all versions for a template.
func (r *Repository) loadAllVersions(ctx context.Context, templateID string) ([]*entity.TemplateVersionWithDetails, error) {
	rows, err := r.pool.Query(ctx, queryAllVersions, templateID)
	if err != nil {
		return nil, fmt.Errorf("querying template versions: %w", err)
	}
	defer rows.Close()

	var versions []*entity.TemplateVersionWithDetails
	for rows.Next() {
		v := &entity.TemplateVersion{}
		if err := rows.Scan(
			&v.ID, &v.TemplateID, &v.VersionNumber, &v.Name, &v.Description,
			&v.ContentStructure, &v.Status, &v.ScheduledPublishAt, &v.ScheduledArchiveAt,
			&v.PublishedAt, &v.ArchivedAt, &v.PublishedBy, &v.ArchivedBy,
			&v.CreatedBy, &v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning template version: %w", err)
		}
		versions = append(versions, &entity.TemplateVersionWithDetails{TemplateVersion: *v})
	}
	return versions, rows.Err()
}

// buildTemplateFilters builds filter query and args for template listing.
func buildTemplateFilters(filters port.TemplateFilters, startArgPos int) (string, []any) {
	var query string
	var args []any
	argPos := startArgPos

	if filters.RootOnly {
		query += " AND t.folder_id IS NULL"
	} else if filters.FolderID != nil {
		query += fmt.Sprintf(` AND t.folder_id = $%d`, argPos)
		args = append(args, *filters.FolderID)
		argPos++
	}

	if filters.Process != nil {
		query += fmt.Sprintf(` AND t.process = $%d`, argPos)
		args = append(args, *filters.Process)
		argPos++
	}

	if filters.HasPublishedVersion != nil {
		if *filters.HasPublishedVersion {
			query += " AND EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = t.id AND status = 'PUBLISHED')"
		} else {
			query += " AND NOT EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = t.id AND status = 'PUBLISHED')"
		}
	}

	if filters.Search != "" {
		query += fmt.Sprintf(" AND t.title ILIKE $%d", argPos)
		args = append(args, "%"+filters.Search+"%")
		argPos++
	}

	if len(filters.TagIDs) > 0 {
		query += fmt.Sprintf(` AND t.id IN (
			SELECT template_id FROM content.template_tags WHERE tag_id = ANY($%d)
			GROUP BY template_id HAVING COUNT(DISTINCT tag_id) = $%d
		)`, argPos, argPos+1)
		args = append(args, filters.TagIDs, len(filters.TagIDs))
		argPos += 2
	}

	query += " ORDER BY COALESCE(f.path, '') ASC, t.title ASC"

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

// scanTemplateListItems scans template list items from rows.
func scanTemplateListItems(rows pgx.Rows) ([]*entity.TemplateListItem, error) {
	var templates []*entity.TemplateListItem
	for rows.Next() {
		item := &entity.TemplateListItem{}
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.FolderID,
			&item.DocumentTypeID, &item.DocumentTypeCode,
			&item.Title, &item.IsPublicLibrary, &item.Process, &item.ProcessType,
			&item.CreatedAt, &item.UpdatedAt,
			&item.HasPublishedVersion, &item.VersionCount,
			&item.ScheduledVersionCount, &item.PublishedVersionNumber,
		); err != nil {
			return nil, fmt.Errorf("scanning template: %w", err)
		}
		templates = append(templates, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating templates: %w", err)
	}
	return templates, nil
}

// loadTagsForTemplates loads tags for multiple templates in a single batch query.
func (r *Repository) loadTagsForTemplates(ctx context.Context, templates []*entity.TemplateListItem) error {
	if len(templates) == 0 {
		return nil
	}

	// Collect template IDs
	templateIDs := make([]string, len(templates))
	templateMap := make(map[string]*entity.TemplateListItem, len(templates))
	for i, t := range templates {
		templateIDs[i] = t.ID
		templateMap[t.ID] = t
		t.Tags = []*entity.Tag{} // Initialize empty slice
	}

	// Query all tags for these templates in one batch
	rows, err := r.pool.Query(ctx, queryTemplateTagsBatch, templateIDs)
	if err != nil {
		return fmt.Errorf("querying template tags batch: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var templateID string
		tag := &entity.Tag{}
		if err := rows.Scan(
			&templateID,
			&tag.ID,
			&tag.Name,
			&tag.Color,
		); err != nil {
			return fmt.Errorf("scanning template tag: %w", err)
		}

		if template, ok := templateMap[templateID]; ok {
			template.Tags = append(template.Tags, tag)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating template tags: %w", err)
	}

	return nil
}

// Update updates a template.
func (r *Repository) Update(ctx context.Context, template *entity.Template) error {
	result, err := r.pool.Exec(ctx, queryUpdate,
		template.ID,
		template.Title,
		template.FolderID,
		template.DocumentTypeID,
		template.IsPublicLibrary,
		template.Process,
		template.ProcessType,
		template.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("updating template: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrTemplateNotFound
	}

	return nil
}

// Delete deletes a template.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting template: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrTemplateNotFound
	}

	return nil
}

// ExistsByTitle checks if a template with the given title exists in the workspace.
func (r *Repository) ExistsByTitle(ctx context.Context, workspaceID, title string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByTitle, workspaceID, title).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking template title existence: %w", err)
	}

	return exists, nil
}

// ExistsByTitleExcluding checks if a template with the given title exists, excluding a specific ID.
func (r *Repository) ExistsByTitleExcluding(ctx context.Context, workspaceID, title, excludeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByTitleExcluding, workspaceID, title, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking template title existence: %w", err)
	}

	return exists, nil
}

// CountByFolder returns the number of templates in a folder.
func (r *Repository) CountByFolder(ctx context.Context, folderID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryCountByFolder, folderID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting templates in folder: %w", err)
	}

	return count, nil
}

// FindInternalTemplateBaseContext resolves tenant, optional workspace, and document type in one query.
//
//nolint:funlen // Single DB projection scan; splitting the scan list makes column-order drift harder to audit.
func (r *Repository) FindInternalTemplateBaseContext(ctx context.Context, query port.InternalTemplateBaseQuery) (*port.InternalTemplateBaseContext, error) {
	var tenant entity.Tenant
	var workspaceID, workspaceTenantID, workspaceName, workspaceCode, workspaceType, workspaceStatus sql.NullString
	var workspaceIsSandbox sql.NullBool
	var workspaceSandboxOfID sql.NullString
	var workspaceCreatedAt sql.NullTime
	var workspaceUpdatedAt sql.NullTime
	var docTypeID, docTypeTenantID, docTypeCode sql.NullString
	var docTypeName, docTypeDescription entity.I18nText
	var docTypeIsGlobal sql.NullBool
	var docTypeCreatedAt sql.NullTime
	var docTypeUpdatedAt sql.NullTime

	err := r.pool.QueryRow(ctx, queryFindInternalTemplateBaseContext,
		query.TenantCode,
		query.WorkspaceCode,
		query.DocumentType,
	).Scan(
		&tenant.ID,
		&tenant.Code,
		&tenant.Name,
		&tenant.Description,
		&tenant.IsSystem,
		&tenant.Status,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&workspaceID,
		&workspaceTenantID,
		&workspaceName,
		&workspaceCode,
		&workspaceType,
		&workspaceStatus,
		&workspaceIsSandbox,
		&workspaceSandboxOfID,
		&workspaceCreatedAt,
		&workspaceUpdatedAt,
		&docTypeID,
		&docTypeTenantID,
		&docTypeCode,
		&docTypeName,
		&docTypeDescription,
		&docTypeIsGlobal,
		&docTypeCreatedAt,
		&docTypeUpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying internal template base context: %w", err)
	}
	if !docTypeID.Valid {
		return nil, entity.ErrDocumentTypeNotFound
	}

	out := &port.InternalTemplateBaseContext{
		Tenant: &tenant,
		DocumentType: &entity.DocumentType{
			ID:          docTypeID.String,
			TenantID:    docTypeTenantID.String,
			Code:        docTypeCode.String,
			Name:        docTypeName,
			Description: docTypeDescription,
			IsGlobal:    docTypeIsGlobal.Bool,
			CreatedAt:   docTypeCreatedAt.Time,
		},
	}
	if docTypeUpdatedAt.Valid {
		out.DocumentType.UpdatedAt = &docTypeUpdatedAt.Time
	}
	if workspaceID.Valid {
		out.Workspace = &entity.Workspace{
			ID:        workspaceID.String,
			Name:      workspaceName.String,
			Code:      workspaceCode.String,
			Type:      entity.WorkspaceType(workspaceType.String),
			Status:    entity.WorkspaceStatus(workspaceStatus.String),
			IsSandbox: workspaceIsSandbox.Bool,
			CreatedAt: workspaceCreatedAt.Time,
		}
		if workspaceTenantID.Valid {
			out.Workspace.TenantID = &workspaceTenantID.String
		}
		if workspaceSandboxOfID.Valid {
			out.Workspace.SandboxOfID = &workspaceSandboxOfID.String
		}
		if workspaceUpdatedAt.Valid {
			out.Workspace.UpdatedAt = &workspaceUpdatedAt.Time
		}
	}
	return out, nil
}

// FindInternalTemplateContext resolves the full internal-create template context in one query.
func (r *Repository) FindInternalTemplateContext(
	ctx context.Context,
	query port.InternalTemplateContextQuery,
) (*port.InternalTemplateContext, error) {
	result, err := scanInternalTemplateContext(r.pool.QueryRow(ctx, queryFindInternalTemplateContext,
		query.TenantCode,
		query.RequestedWorkspaceCode,
		query.WorkspaceCodes,
		query.DocumentType,
		query.Process,
		query.Tags,
		query.Published,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrInternalTemplateResolutionNotFound
		}
		return nil, fmt.Errorf("finding internal template context: %w", err)
	}
	return result, nil
}

//nolint:funlen,nestif // Single DB projection scan; keeping columns together makes the SQL/Scan contract auditable.
func scanInternalTemplateContext(row pgx.Row) (*port.InternalTemplateContext, error) {
	var tenant entity.Tenant
	var requestedWorkspaceID, requestedWorkspaceTenantID, requestedWorkspaceName, requestedWorkspaceCode sql.NullString
	var requestedWorkspaceType, requestedWorkspaceStatus sql.NullString
	var requestedWorkspaceIsSandbox sql.NullBool
	var requestedWorkspaceSandboxOfID sql.NullString
	var requestedWorkspaceCreatedAt, requestedWorkspaceUpdatedAt sql.NullTime

	var docType entity.DocumentType
	var version entity.TemplateVersion
	var template entity.Template
	var workspace entity.Workspace
	var workspaceTenantID, workspaceSandboxOfID sql.NullString
	var injectablesJSON, signerRolesJSON []byte

	if err := row.Scan(
		&tenant.ID,
		&tenant.Code,
		&tenant.Name,
		&tenant.Description,
		&tenant.IsSystem,
		&tenant.Status,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&requestedWorkspaceID,
		&requestedWorkspaceTenantID,
		&requestedWorkspaceName,
		&requestedWorkspaceCode,
		&requestedWorkspaceType,
		&requestedWorkspaceStatus,
		&requestedWorkspaceIsSandbox,
		&requestedWorkspaceSandboxOfID,
		&requestedWorkspaceCreatedAt,
		&requestedWorkspaceUpdatedAt,
		&docType.ID,
		&docType.TenantID,
		&docType.Code,
		&docType.Name,
		&docType.Description,
		&docType.IsGlobal,
		&docType.CreatedAt,
		&docType.UpdatedAt,
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

	var requestedWorkspace *entity.Workspace
	if requestedWorkspaceID.Valid {
		requestedWorkspace = &entity.Workspace{
			ID:        requestedWorkspaceID.String,
			Name:      requestedWorkspaceName.String,
			Code:      requestedWorkspaceCode.String,
			Type:      entity.WorkspaceType(requestedWorkspaceType.String),
			Status:    entity.WorkspaceStatus(requestedWorkspaceStatus.String),
			IsSandbox: requestedWorkspaceIsSandbox.Bool,
		}
		if requestedWorkspaceTenantID.Valid {
			requestedWorkspace.TenantID = &requestedWorkspaceTenantID.String
		}
		if requestedWorkspaceSandboxOfID.Valid {
			requestedWorkspace.SandboxOfID = &requestedWorkspaceSandboxOfID.String
		}
		if requestedWorkspaceCreatedAt.Valid {
			requestedWorkspace.CreatedAt = requestedWorkspaceCreatedAt.Time
		}
		if requestedWorkspaceUpdatedAt.Valid {
			requestedWorkspace.UpdatedAt = &requestedWorkspaceUpdatedAt.Time
		}
	}

	if workspaceTenantID.Valid {
		workspace.TenantID = &workspaceTenantID.String
	}
	if workspaceSandboxOfID.Valid {
		workspace.SandboxOfID = &workspaceSandboxOfID.String
	}

	versionDetails := &entity.TemplateVersionWithDetails{TemplateVersion: version}
	if err := json.Unmarshal(injectablesJSON, &versionDetails.Injectables); err != nil {
		return nil, fmt.Errorf("unmarshaling version injectables: %w", err)
	}
	if err := json.Unmarshal(signerRolesJSON, &versionDetails.SignerRoles); err != nil {
		return nil, fmt.Errorf("unmarshaling version signer roles: %w", err)
	}

	dbInjectables, activeSystemKeys := generationInjectablesFromVersion(versionDetails.Injectables)

	return &port.InternalTemplateContext{
		Tenant:             &tenant,
		RequestedWorkspace: requestedWorkspace,
		DocumentType:       &docType,
		Template:           &template,
		Workspace:          &workspace,
		Version:            versionDetails,
		DBInjectables:      dbInjectables,
		ActiveSystemKeys:   activeSystemKeys,
	}, nil
}

func generationInjectablesFromVersion(
	versionInjectables []*entity.VersionInjectableWithDefinition,
) ([]*entity.InjectableDefinition, []string) {
	dbInjectables := make([]*entity.InjectableDefinition, 0, len(versionInjectables))
	activeSystemKeys := make([]string, 0, len(versionInjectables))
	seenDB := make(map[string]struct{}, len(versionInjectables))
	seenSystem := make(map[string]struct{}, len(versionInjectables))

	for _, versionInjectable := range versionInjectables {
		dbInjectables = appendUniqueDBInjectable(dbInjectables, seenDB, versionInjectable)
		activeSystemKeys = appendUniqueSystemInjectable(activeSystemKeys, seenSystem, versionInjectable)
	}

	return dbInjectables, activeSystemKeys
}

func appendUniqueDBInjectable(
	dbInjectables []*entity.InjectableDefinition,
	seen map[string]struct{},
	versionInjectable *entity.VersionInjectableWithDefinition,
) []*entity.InjectableDefinition {
	if versionInjectable == nil || versionInjectable.Definition == nil {
		return dbInjectables
	}

	key := versionInjectable.Definition.Key
	if key == "" {
		return dbInjectables
	}
	if _, ok := seen[key]; ok {
		return dbInjectables
	}

	seen[key] = struct{}{}
	return append(dbInjectables, versionInjectable.Definition)
}

func appendUniqueSystemInjectable(
	activeSystemKeys []string,
	seen map[string]struct{},
	versionInjectable *entity.VersionInjectableWithDefinition,
) []string {
	if versionInjectable == nil || versionInjectable.Definition != nil {
		return activeSystemKeys
	}
	if versionInjectable.SystemInjectableKey == nil || *versionInjectable.SystemInjectableKey == "" {
		return activeSystemKeys
	}

	key := *versionInjectable.SystemInjectableKey
	if _, ok := seen[key]; ok {
		return activeSystemKeys
	}

	seen[key] = struct{}{}
	return append(activeSystemKeys, key)
}

// FindTemplateWorkspaceByTemplateID loads a template and its owning workspace in one query.
func (r *Repository) FindTemplateWorkspaceByTemplateID(ctx context.Context, templateID string) (*port.TemplateWorkspace, error) {
	template := &entity.Template{}
	workspace := &entity.Workspace{}
	var workspaceTenantID sql.NullString
	var workspaceSandboxOfID sql.NullString

	err := r.pool.QueryRow(ctx, queryFindTemplateWorkspaceByTemplateID, templateID).Scan(
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
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrTemplateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finding template workspace %s: %w", templateID, err)
	}
	if workspaceTenantID.Valid {
		workspace.TenantID = &workspaceTenantID.String
	}
	if workspaceSandboxOfID.Valid {
		workspace.SandboxOfID = &workspaceSandboxOfID.String
	}
	return &port.TemplateWorkspace{
		Template:  template,
		Workspace: workspace,
	}, nil
}

// FindResolutionCandidates finds lightweight template-version candidates for internal template resolution.
func (r *Repository) FindResolutionCandidates(ctx context.Context, query port.TemplateResolutionQuery) ([]port.TemplateResolutionCandidate, error) {
	var published *bool
	if query.Published != nil {
		value := *query.Published
		published = &value
	}
	rows, err := r.pool.Query(ctx, queryFindResolutionCandidates,
		query.TenantCode,
		query.WorkspaceCodes,
		query.DocumentType,
		query.Process,
		query.Tags,
		published,
	)
	if err != nil {
		return nil, fmt.Errorf("querying template resolution candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]port.TemplateResolutionCandidate, 0)
	for rows.Next() {
		var candidate port.TemplateResolutionCandidate
		if err := rows.Scan(
			&candidate.TenantID,
			&candidate.TenantCode,
			&candidate.WorkspaceID,
			&candidate.WorkspaceCode,
			&candidate.DocumentTypeID,
			&candidate.DocumentTypeCode,
			&candidate.TemplateID,
			&candidate.VersionID,
			&candidate.Tags,
			&candidate.Priority,
		); err != nil {
			return nil, fmt.Errorf("scanning template resolution candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating template resolution candidates: %w", err)
	}
	return candidates, nil
}

// FindByDocumentType finds the template assigned to a document type and process in a workspace.
func (r *Repository) FindByDocumentType(ctx context.Context, workspaceID, documentTypeID, process string) (*entity.Template, error) {
	template := &entity.Template{}
	err := r.pool.QueryRow(ctx, queryFindByDocumentType, workspaceID, documentTypeID, process).Scan(
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
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No template assigned to this type+process in this workspace
		}
		return nil, fmt.Errorf("finding template by document type: %w", err)
	}

	return template, nil
}

// UpdateProcessFields updates the process and processType of a template.
func (r *Repository) UpdateProcessFields(ctx context.Context, templateID string, process string, processType entity.ProcessType) error {
	result, err := r.pool.Exec(ctx, queryUpdateProcessFields, templateID, process, processType)
	if err != nil {
		return fmt.Errorf("updating process fields: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrTemplateNotFound
	}

	return nil
}

// FindByDocumentTypeCode finds templates by document type code across a tenant.
func (r *Repository) FindByDocumentTypeCode(ctx context.Context, tenantID, documentTypeCode string) ([]*entity.TemplateListItem, error) {
	rows, err := r.pool.Query(ctx, queryFindByDocumentTypeCode, tenantID, documentTypeCode)
	if err != nil {
		return nil, fmt.Errorf("querying templates by document type code: %w", err)
	}
	defer rows.Close()

	templates, err := scanTemplateListItems(rows)
	if err != nil {
		return nil, err
	}

	// Load tags in batch for all templates
	if err := r.loadTagsForTemplates(ctx, templates); err != nil {
		return nil, err
	}

	return templates, nil
}

// UpdateDocumentType updates the document type assignment for a template.
func (r *Repository) UpdateDocumentType(ctx context.Context, templateID string, documentTypeID *string) error {
	result, err := r.pool.Exec(ctx, queryUpdateDocumentType, templateID, documentTypeID)
	if err != nil {
		return fmt.Errorf("updating template document type: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrTemplateNotFound
	}

	return nil
}
