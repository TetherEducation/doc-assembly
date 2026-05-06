package template

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	templateuc "github.com/rendis/doc-assembly/core/internal/core/usecase/template"
)

// NewTemplateVersionService creates a new template version service.
func NewTemplateVersionService(
	versionRepo port.TemplateVersionRepository,
	injectableRepo port.TemplateVersionInjectableRepository,
	signerRoleRepo port.TemplateVersionSignerRoleRepository,
	systemInjectableRepo port.SystemInjectableRepository,
	templateRepo port.TemplateRepository,
	tagRepo port.TemplateTagRepository,
	contentValidator port.ContentValidator,
	workspaceRepo port.WorkspaceRepository,
) templateuc.TemplateVersionUseCase {
	return &TemplateVersionService{
		versionRepo:          versionRepo,
		injectableRepo:       injectableRepo,
		signerRoleRepo:       signerRoleRepo,
		systemInjectableRepo: systemInjectableRepo,
		templateRepo:         templateRepo,
		tagRepo:              tagRepo,
		contentValidator:     contentValidator,
		workspaceRepo:        workspaceRepo,
	}
}

// TemplateVersionService implements template version business logic.
type TemplateVersionService struct {
	versionRepo          port.TemplateVersionRepository
	injectableRepo       port.TemplateVersionInjectableRepository
	signerRoleRepo       port.TemplateVersionSignerRoleRepository
	systemInjectableRepo port.SystemInjectableRepository
	templateRepo         port.TemplateRepository
	tagRepo              port.TemplateTagRepository
	contentValidator     port.ContentValidator
	workspaceRepo        port.WorkspaceRepository
}

// CreateVersion creates a new version for a template.
func (s *TemplateVersionService) CreateVersion(ctx context.Context, cmd templateuc.CreateVersionCommand) (*entity.TemplateVersion, error) {
	if _, err := s.templateRepo.FindByID(ctx, cmd.TemplateID); err != nil {
		return nil, fmt.Errorf("finding template: %w", err)
	}

	if err := s.checkVersionNameUnique(ctx, cmd.TemplateID, cmd.Name); err != nil {
		return nil, err
	}

	versionNumber, err := s.versionRepo.GetNextVersionNumber(ctx, cmd.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("getting next version number: %w", err)
	}

	version := s.buildNewVersion(cmd.TemplateID, versionNumber, cmd.Name, cmd.Description, cmd.CreatedBy, nil)
	if err := version.Validate(); err != nil {
		return nil, fmt.Errorf("validating version: %w", err)
	}

	id, err := s.versionRepo.Create(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("creating version: %w", err)
	}
	version.ID = id

	slog.InfoContext(ctx, "template version created",
		slog.String("version_id", version.ID),
		slog.String("template_id", cmd.TemplateID),
		slog.Int("version_number", versionNumber),
		slog.String("name", cmd.Name),
	)

	return version, nil
}

// CreateVersionFromExisting creates a new version copying content from an existing version.
func (s *TemplateVersionService) CreateVersionFromExisting(ctx context.Context, sourceVersionID string, name string, description *string, createdBy *string) (*entity.TemplateVersion, error) {
	source, err := s.versionRepo.FindByID(ctx, sourceVersionID)
	if err != nil {
		return nil, fmt.Errorf("finding source version: %w", err)
	}

	if err := s.checkVersionNameUnique(ctx, source.TemplateID, name); err != nil {
		return nil, err
	}

	versionNumber, err := s.versionRepo.GetNextVersionNumber(ctx, source.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("getting next version number: %w", err)
	}

	version := s.buildNewVersion(source.TemplateID, versionNumber, name, description, createdBy, source.ContentStructure)

	id, err := s.versionRepo.Create(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("creating version: %w", err)
	}
	version.ID = id

	s.copyVersionRelatedData(ctx, sourceVersionID, version.ID)

	slog.InfoContext(ctx, "template version created from existing",
		slog.String("version_id", version.ID),
		slog.String("source_version_id", sourceVersionID),
		slog.Int("version_number", versionNumber),
	)

	return version, nil
}

// GetVersion retrieves a template version by ID.
func (s *TemplateVersionService) GetVersion(ctx context.Context, id string) (*entity.TemplateVersion, error) {
	version, err := s.versionRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding version %s: %w", id, err)
	}
	return version, nil
}

// GetVersionWithDetails retrieves a version with all related data.
func (s *TemplateVersionService) GetVersionWithDetails(ctx context.Context, id string) (*entity.TemplateVersionWithDetails, error) {
	details, err := s.versionRepo.FindByIDWithDetails(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding version details %s: %w", id, err)
	}
	return details, nil
}

// ListVersions lists all versions for a template.
func (s *TemplateVersionService) ListVersions(ctx context.Context, templateID string) ([]*entity.TemplateVersion, error) {
	versions, err := s.versionRepo.FindByTemplateID(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("listing versions: %w", err)
	}
	return versions, nil
}

// GetPublishedVersion gets the currently published version for a template.
func (s *TemplateVersionService) GetPublishedVersion(ctx context.Context, templateID string) (*entity.TemplateVersionWithDetails, error) {
	details, err := s.versionRepo.FindPublishedByTemplateIDWithDetails(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("finding published version: %w", err)
	}
	return details, nil
}

// UpdateVersion updates a template version.
func (s *TemplateVersionService) UpdateVersion(ctx context.Context, cmd templateuc.UpdateVersionCommand) (*entity.TemplateVersion, error) {
	version, err := s.versionRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding version: %w", err)
	}

	if err := version.CanEdit(); err != nil {
		return nil, err
	}

	if err := s.applyVersionUpdates(ctx, version, cmd); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	version.UpdatedAt = &now

	if err := version.Validate(); err != nil {
		return nil, fmt.Errorf("validating version: %w", err)
	}

	if err := s.versionRepo.Update(ctx, version); err != nil {
		return nil, fmt.Errorf("updating version: %w", err)
	}

	slog.InfoContext(ctx, "template version updated", slog.String("version_id", version.ID))
	return version, nil
}

// PublishVersion publishes a version (archives current published if exists).
func (s *TemplateVersionService) PublishVersion(ctx context.Context, id string, userID string) error {
	version, err := s.versionRepo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("finding version: %w", err)
	}

	if err := version.CanPublish(); err != nil {
		return err
	}

	template, err := s.templateRepo.FindByID(ctx, version.TemplateID)
	if err != nil {
		return fmt.Errorf("finding template: %w", err)
	}

	result := s.contentValidator.ValidateForPublish(ctx, template.WorkspaceID, version.ID, version.ContentStructure)
	if !result.Valid {
		return toContentValidationError(result)
	}

	if err := s.replaceSignerRoles(ctx, version.ID, result.ExtractedSignerRoles); err != nil {
		return err
	}

	if err := s.replaceInjectables(ctx, version.ID, result.ExtractedInjectables); err != nil {
		return err
	}

	if err := s.archiveCurrentPublished(ctx, version.TemplateID, id, userID); err != nil {
		return err
	}

	version.Publish(userID)
	if err := s.versionRepo.Update(ctx, version); err != nil {
		return fmt.Errorf("publishing version: %w", err)
	}

	slog.InfoContext(ctx, "template version published",
		slog.String("version_id", id),
		slog.String("template_id", version.TemplateID),
	)
	return nil
}

// SchedulePublish schedules a version for future publication.
func (s *TemplateVersionService) SchedulePublish(ctx context.Context, cmd templateuc.SchedulePublishCommand) error {
	version, err := s.versionRepo.FindByID(ctx, cmd.VersionID)
	if err != nil {
		return fmt.Errorf("finding version: %w", err)
	}

	if err := version.SchedulePublish(cmd.PublishAt); err != nil {
		return err
	}

	template, err := s.templateRepo.FindByID(ctx, version.TemplateID)
	if err != nil {
		return fmt.Errorf("finding template: %w", err)
	}

	result := s.contentValidator.ValidateForPublish(ctx, template.WorkspaceID, version.ID, version.ContentStructure)
	if !result.Valid {
		return toContentValidationError(result)
	}

	conflict, err := s.versionRepo.ExistsScheduledAtTime(ctx, version.TemplateID, cmd.PublishAt, &cmd.VersionID)
	if err != nil {
		return fmt.Errorf("checking schedule conflict: %w", err)
	}
	if conflict {
		return entity.ErrScheduledTimeConflict
	}

	if err := s.versionRepo.Update(ctx, version); err != nil {
		return fmt.Errorf("scheduling publish: %w", err)
	}

	slog.InfoContext(ctx, "version scheduled for publication",
		slog.String("version_id", cmd.VersionID),
		slog.Time("publish_at", cmd.PublishAt),
	)
	return nil
}

// ScheduleArchive schedules the current published version for future archival.
func (s *TemplateVersionService) ScheduleArchive(ctx context.Context, cmd templateuc.ScheduleArchiveCommand) error {
	version, err := s.versionRepo.FindByID(ctx, cmd.VersionID)
	if err != nil {
		return fmt.Errorf("finding version: %w", err)
	}

	hasScheduled, err := s.versionRepo.HasScheduledVersion(ctx, version.TemplateID)
	if err != nil {
		return fmt.Errorf("checking for scheduled version: %w", err)
	}
	if !hasScheduled {
		return entity.ErrCannotArchiveWithoutReplacement
	}

	if err := version.ScheduleArchive(cmd.ArchiveAt); err != nil {
		return err
	}

	if err := s.versionRepo.Update(ctx, version); err != nil {
		return fmt.Errorf("scheduling archive: %w", err)
	}

	slog.InfoContext(ctx, "version scheduled for archival",
		slog.String("version_id", cmd.VersionID),
		slog.Time("archive_at", cmd.ArchiveAt),
	)
	return nil
}

// CancelSchedule cancels any scheduled publication or archival.
func (s *TemplateVersionService) CancelSchedule(ctx context.Context, versionID string) error {
	version, err := s.versionRepo.FindByID(ctx, versionID)
	if err != nil {
		return fmt.Errorf("finding version: %w", err)
	}

	if err := version.CancelSchedule(); err != nil {
		return err
	}

	if err := s.versionRepo.Update(ctx, version); err != nil {
		return fmt.Errorf("canceling schedule: %w", err)
	}

	slog.InfoContext(ctx, "version schedule canceled", slog.String("version_id", versionID))
	return nil
}

// ArchiveVersion manually archives a published version.
func (s *TemplateVersionService) ArchiveVersion(ctx context.Context, id string, userID string) error {
	version, err := s.versionRepo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("finding version: %w", err)
	}

	if err := version.CanArchive(); err != nil {
		return err
	}

	version.Archive(userID)
	if err := s.versionRepo.Update(ctx, version); err != nil {
		return fmt.Errorf("archiving version: %w", err)
	}

	slog.InfoContext(ctx, "template version archived", slog.String("version_id", id))
	return nil
}

// DeleteVersion deletes a draft version.
func (s *TemplateVersionService) DeleteVersion(ctx context.Context, id string) error {
	version, err := s.versionRepo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("finding version: %w", err)
	}

	if !version.IsDraft() && !version.IsScheduled() {
		return entity.ErrCannotEditPublished
	}

	s.deleteVersionRelatedData(ctx, id)

	if err := s.versionRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting version: %w", err)
	}

	slog.InfoContext(ctx, "template version deleted", slog.String("version_id", id))
	return nil
}

// AddInjectable adds an injectable to a version.
func (s *TemplateVersionService) AddInjectable(ctx context.Context, cmd templateuc.AddVersionInjectableCommand) (*entity.TemplateVersionInjectable, error) {
	version, err := s.versionRepo.FindByID(ctx, cmd.VersionID)
	if err != nil {
		return nil, fmt.Errorf("finding version: %w", err)
	}
	if err := version.CanEdit(); err != nil {
		return nil, err
	}

	exists, err := s.injectableRepo.Exists(ctx, cmd.VersionID, cmd.InjectableDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("checking injectable link: %w", err)
	}
	if exists {
		return nil, entity.ErrInjectableAlreadyExists
	}

	injectable := entity.NewTemplateVersionInjectable(
		cmd.VersionID,
		cmd.InjectableDefinitionID,
		cmd.IsRequired,
		cmd.DefaultValue,
	)
	injectable.ID = uuid.NewString()

	if err := injectable.Validate(); err != nil {
		return nil, fmt.Errorf("validating injectable: %w", err)
	}

	id, err := s.injectableRepo.Create(ctx, injectable)
	if err != nil {
		return nil, fmt.Errorf("adding injectable: %w", err)
	}
	injectable.ID = id

	slog.InfoContext(ctx, "injectable added to version",
		slog.String("version_id", cmd.VersionID),
		slog.String("injectable_id", cmd.InjectableDefinitionID),
	)

	return injectable, nil
}

// RemoveInjectable removes an injectable from a version.
func (s *TemplateVersionService) RemoveInjectable(ctx context.Context, id string) error {
	injectable, err := s.injectableRepo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("finding injectable: %w", err)
	}

	version, err := s.versionRepo.FindByID(ctx, injectable.TemplateVersionID)
	if err != nil {
		return fmt.Errorf("finding version: %w", err)
	}
	if err := version.CanEdit(); err != nil {
		return err
	}

	if err := s.injectableRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("removing injectable: %w", err)
	}

	slog.InfoContext(ctx, "injectable removed from version", slog.String("injectable_id", id))
	return nil
}

// ProcessScheduledPublications publishes all versions whose scheduled time has passed.
func (s *TemplateVersionService) ProcessScheduledPublications(ctx context.Context) error {
	versions, err := s.versionRepo.FindScheduledToPublish(ctx, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("finding scheduled versions: %w", err)
	}

	for _, version := range versions {
		if err := s.PublishVersion(ctx, version.ID, ""); err != nil {
			slog.ErrorContext(ctx, "failed to process scheduled publication",
				slog.String("version_id", version.ID),
				slog.Any("error", err),
			)
			continue
		}
		slog.InfoContext(ctx, "scheduled publication processed", slog.String("version_id", version.ID))
	}

	return nil
}

// ProcessScheduledArchivals archives all published versions whose scheduled archive time has passed.
func (s *TemplateVersionService) ProcessScheduledArchivals(ctx context.Context) error {
	versions, err := s.versionRepo.FindScheduledToArchive(ctx, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("finding scheduled archivals: %w", err)
	}

	for _, version := range versions {
		if err := s.ArchiveVersion(ctx, version.ID, ""); err != nil {
			slog.ErrorContext(ctx, "failed to process scheduled archival",
				slog.String("version_id", version.ID),
				slog.Any("error", err),
			)
			continue
		}
		slog.InfoContext(ctx, "scheduled archival processed", slog.String("version_id", version.ID))
	}

	return nil
}

// UpdateVersionContent updates the content of a DRAFT version after validating format and injectables.
func (s *TemplateVersionService) UpdateVersionContent(ctx context.Context, versionID string, content json.RawMessage) error {
	// 1. Load the version to verify it exists and get workspaceID
	version, err := s.versionRepo.FindByID(ctx, versionID)
	if err != nil {
		return fmt.Errorf("finding version: %w", err)
	}

	// 2. Verify the version can be edited — only DRAFT versions can have content updated
	if err := version.CanEdit(); err != nil {
		return err
	}

	// 3. Load the template to get the workspaceID
	template, err := s.templateRepo.FindByID(ctx, version.TemplateID)
	if err != nil {
		return fmt.Errorf("finding template: %w", err)
	}

	// 4. Validate format and injectables (not full publish validation — signer roles not required yet)
	result := s.contentValidator.ValidateForContentUpdate(ctx, template.WorkspaceID, versionID, content)
	if !result.Valid {
		return toContentValidationError(result)
	}

	// 5. Persist the content
	now := time.Now().UTC()
	version.ContentStructure = content
	version.UpdatedAt = &now
	if err = s.versionRepo.Update(ctx, version); err != nil {
		return fmt.Errorf("updating version content: %w", err)
	}

	slog.InfoContext(ctx, "template version content updated", slog.String("version_id", versionID))
	return nil
}

// PromoteVersion promotes a published version from sandbox to production.
func (s *TemplateVersionService) PromoteVersion(ctx context.Context, cmd templateuc.PromoteVersionCommand) (*templateuc.PromoteVersionResult, error) {
	sourceVersion, err := s.versionRepo.FindByID(ctx, cmd.SourceVersionID)
	if err != nil {
		return nil, fmt.Errorf("finding source version: %w", err)
	}

	if err := s.validatePromotionSource(sourceVersion, cmd.SourceTemplateID); err != nil {
		return nil, err
	}

	if err := s.validatePromotionTarget(ctx, cmd.TargetWorkspaceID); err != nil {
		return nil, err
	}

	switch cmd.Mode {
	case "NEW_TEMPLATE":
		return s.promoteAsNewTemplate(ctx, sourceVersion, cmd)
	case "NEW_VERSION":
		return s.promoteAsNewVersion(ctx, sourceVersion, cmd)
	default:
		return nil, fmt.Errorf("invalid promotion mode: %s", cmd.Mode)
	}
}

// promoteAsNewTemplate creates a new template in production with the promoted version.
func (s *TemplateVersionService) promoteAsNewTemplate(ctx context.Context, sourceVersion *entity.TemplateVersion, cmd templateuc.PromoteVersionCommand) (*templateuc.PromoteVersionResult, error) {
	sourceTemplate, err := s.templateRepo.FindByIDWithDetails(ctx, sourceVersion.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("finding source template: %w", err)
	}

	if err := s.checkTemplateTitleUnique(ctx, cmd.TargetWorkspaceID, sourceTemplate.Title); err != nil {
		return nil, err
	}

	newTemplate := s.buildPromotedTemplate(cmd.TargetWorkspaceID, cmd.TargetFolderID, sourceTemplate.Title)
	if _, err := s.templateRepo.Create(ctx, newTemplate); err != nil {
		return nil, fmt.Errorf("creating template: %w", err)
	}

	versionName := s.resolvePromotionVersionName(cmd.VersionName)
	description := s.buildPromotionDescription(sourceTemplate.Title, sourceVersion.VersionNumber)

	newVersion := s.buildNewVersion(newTemplate.ID, 1, versionName, &description, &cmd.PromotedBy, sourceVersion.ContentStructure)
	if _, err := s.versionRepo.Create(ctx, newVersion); err != nil {
		return nil, fmt.Errorf("creating version: %w", err)
	}

	s.copyVersionRelatedData(ctx, sourceVersion.ID, newVersion.ID)
	s.copyTemplateTags(ctx, sourceTemplate.Tags, newTemplate.ID)

	slog.InfoContext(ctx, "version promoted as new template",
		slog.String("source_version_id", sourceVersion.ID),
		slog.String("source_template_id", sourceVersion.TemplateID),
		slog.String("new_template_id", newTemplate.ID),
		slog.String("new_version_id", newVersion.ID),
		slog.String("target_workspace_id", cmd.TargetWorkspaceID),
	)

	return &templateuc.PromoteVersionResult{
		Template: newTemplate,
		Version:  newVersion,
	}, nil
}

// promoteAsNewVersion adds the promoted version to an existing template.
func (s *TemplateVersionService) promoteAsNewVersion(ctx context.Context, sourceVersion *entity.TemplateVersion, cmd templateuc.PromoteVersionCommand) (*templateuc.PromoteVersionResult, error) {
	if cmd.TargetTemplateID == nil || *cmd.TargetTemplateID == "" {
		return nil, entity.ErrTargetTemplateRequired
	}

	targetTemplate, err := s.templateRepo.FindByID(ctx, *cmd.TargetTemplateID)
	if err != nil {
		return nil, fmt.Errorf("finding target template: %w", err)
	}
	if targetTemplate.WorkspaceID != cmd.TargetWorkspaceID {
		return nil, entity.ErrTargetTemplateNotInWorkspace
	}

	sourceTemplate, err := s.templateRepo.FindByID(ctx, sourceVersion.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("finding source template: %w", err)
	}

	versionNumber, err := s.versionRepo.GetNextVersionNumber(ctx, *cmd.TargetTemplateID)
	if err != nil {
		return nil, fmt.Errorf("getting next version number: %w", err)
	}

	versionName := s.resolvePromotionVersionName(cmd.VersionName)
	if err := s.checkVersionNameUniqueInTemplate(ctx, *cmd.TargetTemplateID, versionName); err != nil {
		return nil, err
	}

	description := s.buildPromotionDescription(sourceTemplate.Title, sourceVersion.VersionNumber)
	newVersion := s.buildNewVersion(*cmd.TargetTemplateID, versionNumber, versionName, &description, &cmd.PromotedBy, sourceVersion.ContentStructure)

	if _, err := s.versionRepo.Create(ctx, newVersion); err != nil {
		return nil, fmt.Errorf("creating version: %w", err)
	}

	s.copyVersionRelatedData(ctx, sourceVersion.ID, newVersion.ID)

	slog.InfoContext(ctx, "version promoted as new version",
		slog.String("source_version_id", sourceVersion.ID),
		slog.String("source_template_id", sourceVersion.TemplateID),
		slog.String("target_template_id", *cmd.TargetTemplateID),
		slog.String("new_version_id", newVersion.ID),
		slog.Int("version_number", versionNumber),
	)

	return &templateuc.PromoteVersionResult{
		Template: nil,
		Version:  newVersion,
	}, nil
}

// --- Private helper methods ---

// checkVersionNameUnique verifies the version name is unique within a template.
func (s *TemplateVersionService) checkVersionNameUnique(ctx context.Context, templateID, name string) error {
	exists, err := s.versionRepo.ExistsByName(ctx, templateID, name)
	if err != nil {
		return fmt.Errorf("checking version name: %w", err)
	}
	if exists {
		return entity.ErrVersionNameExists
	}
	return nil
}

// checkVersionNameUniqueInTemplate verifies the version name is unique (alias for clarity).
func (s *TemplateVersionService) checkVersionNameUniqueInTemplate(ctx context.Context, templateID, name string) error {
	return s.checkVersionNameUnique(ctx, templateID, name)
}

// checkTemplateTitleUnique verifies the template title is unique within a workspace.
func (s *TemplateVersionService) checkTemplateTitleUnique(ctx context.Context, workspaceID, title string) error {
	exists, err := s.templateRepo.ExistsByTitle(ctx, workspaceID, title)
	if err != nil {
		return fmt.Errorf("checking title uniqueness: %w", err)
	}
	if exists {
		return entity.ErrTemplateAlreadyExists
	}
	return nil
}

// buildNewVersion creates a new TemplateVersion entity with the given parameters.
func (s *TemplateVersionService) buildNewVersion(templateID string, versionNumber int, name string, description *string, createdBy *string, content json.RawMessage) *entity.TemplateVersion {
	version := entity.NewTemplateVersion(templateID, versionNumber, name, createdBy)
	version.ID = uuid.NewString()
	version.Description = description
	version.ContentStructure = content
	return version
}

// buildPromotedTemplate creates a new Template entity for promotion.
func (s *TemplateVersionService) buildPromotedTemplate(workspaceID string, folderID *string, title string) *entity.Template {
	return &entity.Template{
		ID:              uuid.NewString(),
		WorkspaceID:     workspaceID,
		FolderID:        folderID,
		Title:           title,
		IsPublicLibrary: false,
		CreatedAt:       time.Now().UTC(),
	}
}

// buildPromotionDescription creates a description for promoted versions.
func (s *TemplateVersionService) buildPromotionDescription(sourceTitle string, sourceVersionNumber int) string {
	return fmt.Sprintf("Promoted from template '%s' version %d", sourceTitle, sourceVersionNumber)
}

// resolvePromotionVersionName determines the version name for promotion.
func (s *TemplateVersionService) resolvePromotionVersionName(providedName *string) string {
	if providedName != nil && *providedName != "" {
		return *providedName
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	return fmt.Sprintf("Promoted from Sandbox %s", timestamp)
}

// copyVersionRelatedData copies injectables and signer roles from one version to another.
func (s *TemplateVersionService) copyVersionRelatedData(ctx context.Context, sourceVersionID, targetVersionID string) {
	if err := s.injectableRepo.CopyFromVersion(ctx, sourceVersionID, targetVersionID); err != nil {
		slog.WarnContext(ctx, "failed to copy injectables",
			slog.String("source_version_id", sourceVersionID),
			slog.String("target_version_id", targetVersionID),
			slog.Any("error", err),
		)
	}
	if err := s.signerRoleRepo.CopyFromVersion(ctx, sourceVersionID, targetVersionID); err != nil {
		slog.WarnContext(ctx, "failed to copy signer roles",
			slog.String("source_version_id", sourceVersionID),
			slog.String("target_version_id", targetVersionID),
			slog.Any("error", err),
		)
	}
}

// deleteVersionRelatedData deletes injectables and signer roles for a version.
func (s *TemplateVersionService) deleteVersionRelatedData(ctx context.Context, versionID string) {
	if err := s.injectableRepo.DeleteByVersionID(ctx, versionID); err != nil {
		slog.WarnContext(ctx, "failed to delete version injectables", slog.Any("error", err))
	}
	if err := s.signerRoleRepo.DeleteByVersionID(ctx, versionID); err != nil {
		slog.WarnContext(ctx, "failed to delete version signer roles", slog.Any("error", err))
	}
}

// copyTemplateTags copies tags from source to target template.
func (s *TemplateVersionService) copyTemplateTags(ctx context.Context, tags []*entity.Tag, targetTemplateID string) {
	for _, tag := range tags {
		if err := s.tagRepo.AddTag(ctx, targetTemplateID, tag.ID); err != nil {
			slog.WarnContext(ctx, "failed to copy tag during promotion",
				slog.String("tag_id", tag.ID),
				slog.String("template_id", targetTemplateID),
				slog.Any("error", err),
			)
		}
	}
}

// applyVersionUpdates applies update command fields to a version.
func (s *TemplateVersionService) applyVersionUpdates(ctx context.Context, version *entity.TemplateVersion, cmd templateuc.UpdateVersionCommand) error {
	if cmd.Name != nil && *cmd.Name != version.Name {
		exists, err := s.versionRepo.ExistsByNameExcluding(ctx, version.TemplateID, *cmd.Name, version.ID)
		if err != nil {
			return fmt.Errorf("checking version name: %w", err)
		}
		if exists {
			return entity.ErrVersionNameExists
		}
		version.Name = *cmd.Name
	}

	if cmd.Description != nil {
		version.Description = cmd.Description
	}

	if cmd.ContentStructure != nil {
		result := s.contentValidator.ValidateForDraft(ctx, cmd.ContentStructure)
		if !result.Valid {
			return toContentValidationError(result)
		}
		version.ContentStructure = cmd.ContentStructure
	}

	return nil
}

// replaceSignerRoles deletes existing signer roles and inserts new ones.
func (s *TemplateVersionService) replaceSignerRoles(ctx context.Context, versionID string, roles []*entity.TemplateVersionSignerRole) error {
	if err := s.signerRoleRepo.DeleteByVersionID(ctx, versionID); err != nil {
		slog.WarnContext(ctx, "failed to delete existing signer roles",
			slog.String("version_id", versionID),
			slog.Any("error", err),
		)
	}

	for _, role := range roles {
		role.ID = uuid.NewString()
		if _, err := s.signerRoleRepo.Create(ctx, role); err != nil {
			return fmt.Errorf("creating signer role %s: %w", role.RoleName, err)
		}
	}

	slog.InfoContext(ctx, "signer roles extracted from content",
		slog.String("version_id", versionID),
		slog.Int("count", len(roles)),
	)

	return nil
}

// replaceInjectables deletes existing injectables and inserts new ones.
func (s *TemplateVersionService) replaceInjectables(ctx context.Context, versionID string, injectables []*entity.TemplateVersionInjectable) error {
	if err := s.injectableRepo.DeleteByVersionID(ctx, versionID); err != nil {
		slog.WarnContext(ctx, "failed to delete existing injectables",
			slog.String("version_id", versionID),
			slog.Any("error", err),
		)
	}

	for _, injectable := range injectables {
		injectable.ID = uuid.NewString()
		if err := s.ensureSystemInjectableDefinition(ctx, injectable); err != nil {
			return err
		}
		if _, err := s.injectableRepo.Create(ctx, injectable); err != nil {
			key := ""
			if injectable.SystemInjectableKey != nil {
				key = *injectable.SystemInjectableKey
			} else if injectable.InjectableDefinitionID != nil {
				key = *injectable.InjectableDefinitionID
			}
			return fmt.Errorf("creating injectable %s: %w", key, err)
		}
	}

	slog.InfoContext(ctx, "injectables extracted from content",
		slog.String("version_id", versionID),
		slog.Int("count", len(injectables)),
	)

	return nil
}

func (s *TemplateVersionService) ensureSystemInjectableDefinition(
	ctx context.Context,
	injectable *entity.TemplateVersionInjectable,
) error {
	if injectable == nil || injectable.SystemInjectableKey == nil || *injectable.SystemInjectableKey == "" {
		return nil
	}
	if s.systemInjectableRepo == nil {
		return nil
	}

	key := *injectable.SystemInjectableKey
	if err := s.systemInjectableRepo.EnsureDefinition(ctx, key); err != nil {
		return fmt.Errorf("ensuring system injectable definition %s: %w", key, err)
	}
	return nil
}

// archiveCurrentPublished archives the currently published version if one exists.
func (s *TemplateVersionService) archiveCurrentPublished(ctx context.Context, templateID, newVersionID, userID string) error {
	currentPublished, err := s.versionRepo.FindPublishedByTemplateID(ctx, templateID)
	if err != nil {
		// No published version exists - this is expected for first publish
		return nil //nolint:nilerr // Not finding a published version is not an error
	}
	if currentPublished == nil {
		return nil
	}

	currentPublished.Archive(userID)
	if err := s.versionRepo.Update(ctx, currentPublished); err != nil {
		return fmt.Errorf("archiving current version: %w", err)
	}

	slog.InfoContext(ctx, "previous version archived",
		slog.String("archived_version_id", currentPublished.ID),
		slog.String("new_version_id", newVersionID),
	)

	return nil
}

// validatePromotionSource validates the source version for promotion.
func (s *TemplateVersionService) validatePromotionSource(sourceVersion *entity.TemplateVersion, sourceTemplateID string) error {
	if sourceVersion.Status != entity.VersionStatusPublished {
		return entity.ErrVersionNotPublished
	}
	if sourceVersion.TemplateID != sourceTemplateID {
		return entity.ErrVersionDoesNotBelongToTemplate
	}
	return nil
}

// validatePromotionTarget validates the target workspace for promotion.
func (s *TemplateVersionService) validatePromotionTarget(ctx context.Context, targetWorkspaceID string) error {
	targetWorkspace, err := s.workspaceRepo.FindByID(ctx, targetWorkspaceID)
	if err != nil {
		return fmt.Errorf("finding target workspace: %w", err)
	}
	if targetWorkspace.IsSandbox {
		return entity.ErrCannotPromoteToSandbox
	}
	return nil
}

// toContentValidationError converts a validation result to an entity.ContentValidationError.
func toContentValidationError(result *port.ContentValidationResult) *entity.ContentValidationError {
	errors := make([]entity.ContentValidationItem, 0, len(result.Errors))
	for _, e := range result.Errors {
		errors = append(errors, entity.ContentValidationItem{
			Code:    e.Code,
			Path:    e.Path,
			Message: e.Message,
		})
	}

	warnings := make([]entity.ContentValidationItem, 0, len(result.Warnings))
	for _, w := range result.Warnings {
		warnings = append(warnings, entity.ContentValidationItem{
			Code:    w.Code,
			Path:    w.Path,
			Message: w.Message,
		})
	}

	return &entity.ContentValidationError{
		Errors:   errors,
		Warnings: warnings,
	}
}
