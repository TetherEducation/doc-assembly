package injectable

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	injectableuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/injectable"
)

// NewWorkspaceInjectableService creates a new workspace injectable service.
func NewWorkspaceInjectableService(repo port.WorkspaceInjectableRepository) injectableuc.WorkspaceInjectableUseCase {
	return &WorkspaceInjectableService{
		repo: repo,
	}
}

// WorkspaceInjectableService implements workspace injectable business logic.
type WorkspaceInjectableService struct {
	repo port.WorkspaceInjectableRepository
}

// CreateInjectable creates a new TEXT type injectable for the workspace.
func (s *WorkspaceInjectableService) CreateInjectable(ctx context.Context, cmd injectableuc.CreateWorkspaceInjectableCommand) (*entity.InjectableDefinition, error) {
	// Check for duplicate key
	exists, err := s.repo.ExistsByKey(ctx, cmd.WorkspaceID, cmd.Key)
	if err != nil {
		return nil, fmt.Errorf("checking injectable existence: %w", err)
	}
	if exists {
		return nil, entity.ErrInjectableAlreadyExists
	}

	injectable := &entity.InjectableDefinition{
		ID:           uuid.NewString(),
		WorkspaceID:  &cmd.WorkspaceID,
		Key:          cmd.Key,
		Label:        cmd.Label,
		Description:  cmd.Description,
		DataType:     entity.InjectableDataTypeText, // Only TEXT type allowed
		Metadata:     cmd.Metadata,
		DefaultValue: &cmd.DefaultValue,
		IsActive:     true,
		IsDeleted:    false,
		CreatedAt:    time.Now().UTC(),
	}

	if injectable.Metadata == nil {
		injectable.Metadata = make(map[string]any)
	}

	if err := injectable.ValidateForWorkspace(); err != nil {
		return nil, fmt.Errorf("validating injectable: %w", err)
	}

	id, err := s.repo.Create(ctx, injectable)
	if err != nil {
		return nil, fmt.Errorf("creating injectable: %w", err)
	}
	injectable.ID = id

	slog.InfoContext(ctx, "workspace injectable created",
		slog.String("injectable_id", injectable.ID),
		slog.String("key", injectable.Key),
		slog.String("workspace_id", cmd.WorkspaceID),
	)

	return injectable, nil
}

// GetInjectable retrieves an injectable by ID.
func (s *WorkspaceInjectableService) GetInjectable(ctx context.Context, id, workspaceID string) (*entity.InjectableDefinition, error) {
	injectable, err := s.repo.FindByID(ctx, id, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("finding injectable %s: %w", id, err)
	}
	return injectable, nil
}

// ListInjectables lists all injectables owned by the workspace.
func (s *WorkspaceInjectableService) ListInjectables(ctx context.Context, workspaceID string) ([]*entity.InjectableDefinition, error) {
	injectables, err := s.repo.FindByWorkspaceOwned(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing injectables: %w", err)
	}
	return injectables, nil
}

// UpdateInjectable updates a workspace-owned injectable.
func (s *WorkspaceInjectableService) UpdateInjectable(ctx context.Context, cmd injectableuc.UpdateWorkspaceInjectableCommand) (*entity.InjectableDefinition, error) {
	injectable, err := s.repo.FindByID(ctx, cmd.ID, cmd.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("finding injectable: %w", err)
	}

	// Check for duplicate key if key changed
	if cmd.Key != nil && *cmd.Key != injectable.Key {
		exists, err := s.repo.ExistsByKeyExcluding(ctx, cmd.WorkspaceID, *cmd.Key, cmd.ID)
		if err != nil {
			return nil, fmt.Errorf("checking injectable key: %w", err)
		}
		if exists {
			return nil, entity.ErrInjectableAlreadyExists
		}
		injectable.Key = *cmd.Key
	}

	if cmd.Label != nil {
		injectable.Label = *cmd.Label
	}
	if cmd.Description != nil {
		injectable.Description = *cmd.Description
	}
	if cmd.DefaultValue != nil {
		injectable.DefaultValue = cmd.DefaultValue
	}
	if cmd.Metadata != nil {
		injectable.Metadata = cmd.Metadata
	}

	now := time.Now().UTC()
	injectable.UpdatedAt = &now

	if err := injectable.ValidateForWorkspace(); err != nil {
		return nil, fmt.Errorf("validating injectable: %w", err)
	}

	if err := s.repo.Update(ctx, injectable); err != nil {
		return nil, fmt.Errorf("updating injectable: %w", err)
	}

	slog.InfoContext(ctx, "workspace injectable updated",
		slog.String("injectable_id", injectable.ID),
		slog.String("key", injectable.Key),
	)

	return injectable, nil
}

// DeleteInjectable soft-deletes an injectable.
func (s *WorkspaceInjectableService) DeleteInjectable(ctx context.Context, id, workspaceID string) error {
	if err := s.repo.SoftDelete(ctx, id, workspaceID); err != nil {
		return fmt.Errorf("deleting injectable: %w", err)
	}

	slog.InfoContext(ctx, "workspace injectable deleted",
		slog.String("injectable_id", id),
		slog.String("workspace_id", workspaceID),
	)
	return nil
}

// ActivateInjectable sets is_active=true for an injectable.
func (s *WorkspaceInjectableService) ActivateInjectable(ctx context.Context, id, workspaceID string) (*entity.InjectableDefinition, error) {
	return s.setActiveStatus(ctx, id, workspaceID, true)
}

// DeactivateInjectable sets is_active=false for an injectable.
func (s *WorkspaceInjectableService) DeactivateInjectable(ctx context.Context, id, workspaceID string) (*entity.InjectableDefinition, error) {
	return s.setActiveStatus(ctx, id, workspaceID, false)
}

func (s *WorkspaceInjectableService) setActiveStatus(ctx context.Context, id, workspaceID string, active bool) (*entity.InjectableDefinition, error) {
	if err := s.repo.SetActive(ctx, id, workspaceID, active); err != nil {
		return nil, fmt.Errorf("setting injectable active status: %w", err)
	}

	injectable, err := s.repo.FindByID(ctx, id, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("finding injectable: %w", err)
	}

	action := "deactivated"
	if active {
		action = "activated"
	}
	slog.InfoContext(ctx, "workspace injectable "+action,
		slog.String("injectable_id", id),
		slog.String("workspace_id", workspaceID),
	)

	return injectable, nil
}
