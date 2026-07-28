package catalog

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// CreateProcessCommand represents the command to create a process.
type CreateProcessCommand struct {
	TenantID    string
	Code        string
	ProcessType entity.ProcessType
	Name        entity.I18nText
	Description entity.I18nText
}

// UpdateProcessCommand represents the command to update a process.
type UpdateProcessCommand struct {
	ID          string
	TenantID    string // Required to verify ownership (cannot modify global processes)
	Name        entity.I18nText
	Description entity.I18nText
}

// DeleteProcessCommand represents the command to delete a process.
type DeleteProcessCommand struct {
	ID              string
	TenantID        string  // Required to verify ownership (cannot delete global processes)
	Force           bool    // If true, delete even if templates are assigned (sets them to default)
	ReplaceWithCode *string // If set, replace process code in templates with this code before deleting
}

// DeleteProcessResult represents the result of attempting to delete a process.
type DeleteProcessResult struct {
	Deleted    bool                          // True if deletion was performed
	Templates  []*entity.ProcessTemplateInfo // Templates using this process (if not deleted)
	CanReplace bool                          // True if replacement is possible
}

// ProcessUseCase defines the input port for process operations.
type ProcessUseCase interface {
	// CreateProcess creates a new process.
	CreateProcess(ctx context.Context, cmd CreateProcessCommand) (*entity.Process, error)

	// GetProcess retrieves a process by ID.
	GetProcess(ctx context.Context, id string) (*entity.Process, error)

	// GetProcessByCode retrieves a process by code within a tenant.
	GetProcessByCode(ctx context.Context, tenantID, code string) (*entity.Process, error)

	// ListProcesses lists all processes for a tenant with pagination.
	ListProcesses(ctx context.Context, tenantID string, filters port.ProcessFilters) ([]*entity.Process, int64, error)

	// ListProcessesWithCount lists processes with template usage count.
	ListProcessesWithCount(ctx context.Context, tenantID string, filters port.ProcessFilters) ([]*entity.ProcessListItem, int64, error)

	// UpdateProcess updates a process's details (name and description only).
	UpdateProcess(ctx context.Context, cmd UpdateProcessCommand) (*entity.Process, error)

	// DeleteProcess attempts to delete a process.
	// If templates are assigned and Force is false, returns templates list without deleting.
	// If ReplaceWithCode is set, replaces the process in all templates before deleting.
	DeleteProcess(ctx context.Context, cmd DeleteProcessCommand) (*DeleteProcessResult, error)
}
