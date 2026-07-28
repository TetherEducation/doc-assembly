package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

// NewTagService creates a new tag service.
func NewTagService(tagRepo port.TagRepository) cataloguc.TagUseCase {
	return &TagService{
		tagRepo: tagRepo,
	}
}

// TagService implements tag business logic.
type TagService struct {
	tagRepo port.TagRepository
}

// CreateTag creates a new tag.
func (s *TagService) CreateTag(ctx context.Context, cmd cataloguc.CreateTagCommand) (*entity.Tag, error) {
	// Normalize tag name
	normalizedName := entity.NormalizeTagName(cmd.Name)

	// Check for duplicate name
	exists, err := s.tagRepo.ExistsByName(ctx, cmd.WorkspaceID, normalizedName)
	if err != nil {
		return nil, fmt.Errorf("checking tag existence: %w", err)
	}
	if exists {
		return nil, entity.ErrTagAlreadyExists
	}

	tag := &entity.Tag{
		ID:          uuid.NewString(),
		WorkspaceID: cmd.WorkspaceID,
		Name:        normalizedName,
		Color:       cmd.Color,
		CreatedAt:   time.Now().UTC(),
	}

	if err := tag.Validate(); err != nil {
		return nil, fmt.Errorf("validating tag: %w", err)
	}

	id, err := s.tagRepo.Create(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("creating tag: %w", err)
	}
	tag.ID = id

	slog.InfoContext(ctx, "tag created",
		slog.String("tag_id", tag.ID),
		slog.String("name", tag.Name),
		slog.String("workspace_id", tag.WorkspaceID),
	)

	return tag, nil
}

// GetTag retrieves a tag by ID.
func (s *TagService) GetTag(ctx context.Context, id string) (*entity.Tag, error) {
	tag, err := s.tagRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding tag %s: %w", id, err)
	}
	return tag, nil
}

// ListTags lists all tags in a workspace.
func (s *TagService) ListTags(ctx context.Context, workspaceID string) ([]*entity.Tag, error) {
	tags, err := s.tagRepo.FindByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	return tags, nil
}

// ListTagsWithCount lists all tags with their template counts.
func (s *TagService) ListTagsWithCount(ctx context.Context, workspaceID string) ([]*entity.TagWithCount, error) {
	tags, err := s.tagRepo.FindByWorkspaceWithCount(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing tags with count: %w", err)
	}
	return tags, nil
}

// UpdateTag updates a tag's details.
func (s *TagService) UpdateTag(ctx context.Context, cmd cataloguc.UpdateTagCommand) (*entity.Tag, error) {
	tag, err := s.tagRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding tag: %w", err)
	}

	// Normalize tag name
	normalizedName := entity.NormalizeTagName(cmd.Name)

	// Check for duplicate name if name changed
	if tag.Name != normalizedName {
		exists, err := s.tagRepo.ExistsByNameExcluding(ctx, tag.WorkspaceID, normalizedName, tag.ID)
		if err != nil {
			return nil, fmt.Errorf("checking tag name: %w", err)
		}
		if exists {
			return nil, entity.ErrTagAlreadyExists
		}
	}

	tag.Name = normalizedName
	tag.Color = cmd.Color
	now := time.Now().UTC()
	tag.UpdatedAt = &now

	if err := tag.Validate(); err != nil {
		return nil, fmt.Errorf("validating tag: %w", err)
	}

	if err := s.tagRepo.Update(ctx, tag); err != nil {
		return nil, fmt.Errorf("updating tag: %w", err)
	}

	slog.InfoContext(ctx, "tag updated",
		slog.String("tag_id", tag.ID),
		slog.String("name", tag.Name),
	)

	return tag, nil
}

// DeleteTag deletes a tag.
func (s *TagService) DeleteTag(ctx context.Context, id string) error {
	// Check if tag is in use
	inUse, err := s.tagRepo.IsInUse(ctx, id)
	if err != nil {
		return fmt.Errorf("checking tag usage: %w", err)
	}
	if inUse {
		return entity.ErrTagInUse
	}

	if err := s.tagRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting tag: %w", err)
	}

	slog.InfoContext(ctx, "tag deleted", slog.String("tag_id", id))
	return nil
}

// FindTagByName finds a tag by name within a workspace.
func (s *TagService) FindTagByName(ctx context.Context, workspaceID, name string) (*entity.Tag, error) {
	tag, err := s.tagRepo.FindByName(ctx, workspaceID, name)
	if err != nil {
		return nil, fmt.Errorf("finding tag by name: %w", err)
	}
	return tag, nil
}
