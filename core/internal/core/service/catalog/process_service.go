package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

// NewProcessService creates a new process service.
func NewProcessService(
	processRepo port.ProcessRepository,
	templateRepo port.TemplateRepository,
) cataloguc.ProcessUseCase {
	return &ProcessService{
		processRepo:  processRepo,
		templateRepo: templateRepo,
	}
}

// ProcessService implements process business logic.
type ProcessService struct {
	processRepo  port.ProcessRepository
	templateRepo port.TemplateRepository
}

// CreateProcess creates a new process.
func (s *ProcessService) CreateProcess(ctx context.Context, cmd cataloguc.CreateProcessCommand) (*entity.Process, error) {
	// Check for duplicate code
	exists, err := s.processRepo.ExistsByCode(ctx, cmd.TenantID, cmd.Code)
	if err != nil {
		return nil, fmt.Errorf("checking process existence: %w", err)
	}
	if exists {
		return nil, entity.ErrProcessCodeExists
	}

	process := &entity.Process{
		ID:          uuid.NewString(),
		TenantID:    cmd.TenantID,
		Code:        cmd.Code,
		ProcessType: cmd.ProcessType,
		Name:        cmd.Name,
		Description: cmd.Description,
		CreatedAt:   time.Now().UTC(),
	}

	if err := process.Validate(); err != nil {
		return nil, fmt.Errorf("validating process: %w", err)
	}

	id, err := s.processRepo.Create(ctx, process)
	if err != nil {
		return nil, fmt.Errorf("creating process: %w", err)
	}
	process.ID = id

	slog.InfoContext(ctx, "process created",
		slog.String("process_id", process.ID),
		slog.String("code", process.Code),
		slog.String("tenant_id", process.TenantID),
	)

	return process, nil
}

// GetProcess retrieves a process by ID.
func (s *ProcessService) GetProcess(ctx context.Context, id string) (*entity.Process, error) {
	process, err := s.processRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding process %s: %w", id, err)
	}
	return process, nil
}

// GetProcessByCode retrieves a process by code within a tenant.
// For non-SYS tenants, also checks global (SYS tenant) processes.
func (s *ProcessService) GetProcessByCode(ctx context.Context, tenantID, code string) (*entity.Process, error) {
	// Check if this is the SYS tenant
	isSys, err := s.processRepo.IsSysTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("checking tenant type: %w", err)
	}

	// SYS tenant only sees its own processes
	if isSys {
		process, err := s.processRepo.FindByCode(ctx, tenantID, code)
		if err != nil {
			return nil, fmt.Errorf("finding process by code %s: %w", code, err)
		}
		return process, nil
	}

	// Non-SYS tenants see their processes + global processes (with priority for own)
	process, err := s.processRepo.FindByCodeWithGlobalFallback(ctx, tenantID, code)
	if err != nil {
		return nil, fmt.Errorf("finding process by code %s: %w", code, err)
	}
	return process, nil
}

// ListProcesses lists all processes for a tenant with pagination.
func (s *ProcessService) ListProcesses(ctx context.Context, tenantID string, filters port.ProcessFilters) ([]*entity.Process, int64, error) {
	processes, total, err := s.processRepo.FindByTenant(ctx, tenantID, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("listing processes: %w", err)
	}
	return processes, total, nil
}

// ListProcessesWithCount lists processes with template usage count.
// For non-SYS tenants, includes global (SYS tenant) processes with priority for own processes.
func (s *ProcessService) ListProcessesWithCount(ctx context.Context, tenantID string, filters port.ProcessFilters) ([]*entity.ProcessListItem, int64, error) {
	// Check if this is the SYS tenant
	isSys, err := s.processRepo.IsSysTenant(ctx, tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("checking tenant type: %w", err)
	}

	// SYS tenant only sees its own processes
	if isSys {
		processes, total, err := s.processRepo.FindByTenantWithTemplateCount(ctx, tenantID, filters)
		if err != nil {
			return nil, 0, fmt.Errorf("listing processes with count: %w", err)
		}
		return processes, total, nil
	}

	// Non-SYS tenants see their processes + global processes (with priority for own)
	processes, total, err := s.processRepo.FindByTenantWithTemplateCountAndGlobal(ctx, tenantID, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("listing processes with count and global: %w", err)
	}
	return processes, total, nil
}

// UpdateProcess updates a process's details (name and description only).
// Global processes (from SYS tenant) cannot be modified by other tenants.
func (s *ProcessService) UpdateProcess(ctx context.Context, cmd cataloguc.UpdateProcessCommand) (*entity.Process, error) {
	process, err := s.processRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding process: %w", err)
	}

	// Check ownership: cannot modify global processes
	if process.TenantID != cmd.TenantID {
		return nil, entity.ErrCannotModifyGlobalProcess
	}

	process.Name = cmd.Name
	process.Description = cmd.Description
	now := time.Now().UTC()
	process.UpdatedAt = &now

	if err := process.Validate(); err != nil {
		return nil, fmt.Errorf("validating process: %w", err)
	}

	if err := s.processRepo.Update(ctx, process); err != nil {
		return nil, fmt.Errorf("updating process: %w", err)
	}

	slog.InfoContext(ctx, "process updated",
		slog.String("process_id", process.ID),
		slog.String("code", process.Code),
	)

	return process, nil
}

// DeleteProcess attempts to delete a process.
// Global processes (from SYS tenant) cannot be deleted by other tenants.
func (s *ProcessService) DeleteProcess(ctx context.Context, cmd cataloguc.DeleteProcessCommand) (*cataloguc.DeleteProcessResult, error) {
	process, err := s.processRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding process: %w", err)
	}

	// Check ownership: cannot delete global processes
	if process.TenantID != cmd.TenantID {
		return nil, entity.ErrCannotModifyGlobalProcess
	}

	// Block deletion of the DEFAULT process (codes are normalized to uppercase)
	if strings.EqualFold(process.Code, "DEFAULT") {
		return nil, entity.ErrCannotDeleteDefaultProcess
	}

	templates, err := s.processRepo.FindTemplatesByProcess(ctx, cmd.TenantID, process.Code)
	if err != nil {
		return nil, fmt.Errorf("finding templates by process: %w", err)
	}

	// If templates exist and no action specified, return info without deleting
	if len(templates) > 0 && !cmd.Force && cmd.ReplaceWithCode == nil {
		return &cataloguc.DeleteProcessResult{
			Deleted:    false,
			Templates:  templates,
			CanReplace: true,
		}, nil
	}

	// Handle template updates before deletion
	if err := s.handleTemplatesBeforeDelete(ctx, templates, cmd); err != nil {
		return nil, err
	}

	if err := s.processRepo.Delete(ctx, cmd.ID); err != nil {
		return nil, fmt.Errorf("deleting process: %w", err)
	}

	slog.InfoContext(ctx, "process deleted",
		slog.String("process_id", process.ID),
		slog.String("code", process.Code),
		slog.Int("templates_affected", len(templates)),
	)

	return &cataloguc.DeleteProcessResult{Deleted: true, Templates: templates}, nil
}

// handleTemplatesBeforeDelete updates templates before deleting the process.
func (s *ProcessService) handleTemplatesBeforeDelete(ctx context.Context, templates []*entity.ProcessTemplateInfo, cmd cataloguc.DeleteProcessCommand) error {
	if len(templates) == 0 {
		return nil
	}

	// Replace with another process code
	if cmd.ReplaceWithCode != nil {
		replacementProcess, err := s.processRepo.FindByCode(ctx, cmd.TenantID, *cmd.ReplaceWithCode)
		if err != nil {
			return fmt.Errorf("replacement process not found: %w", err)
		}
		return s.updateTemplatesProcess(ctx, templates, replacementProcess.Code, replacementProcess.ProcessType)
	}

	// Force delete: reset templates to DEFAULT process with CANONICAL_NAME type
	if cmd.Force {
		return s.updateTemplatesProcess(ctx, templates, entity.DefaultProcess, entity.DefaultProcessType)
	}

	return nil
}

// updateTemplatesProcess updates process fields for multiple templates.
func (s *ProcessService) updateTemplatesProcess(ctx context.Context, templates []*entity.ProcessTemplateInfo, newCode string, processType entity.ProcessType) error {
	for _, tmpl := range templates {
		if err := s.templateRepo.UpdateProcessFields(ctx, tmpl.ID, newCode, processType); err != nil {
			return fmt.Errorf("updating process for template %s: %w", tmpl.ID, err)
		}
	}
	return nil
}
