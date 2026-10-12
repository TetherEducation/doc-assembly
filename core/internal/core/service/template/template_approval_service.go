package template

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

var (
	// ErrWorkspaceScopeRequired is returned when a caller names no workspace. A
	// missing claim must not be a way to skip the check.
	ErrWorkspaceScopeRequired = errors.New("a workspace code is required")

	// ErrWorkspaceMismatch is returned when a caller acts on a version belonging to
	// a different workspace than the one it claimed.
	ErrWorkspaceMismatch = errors.New("template version does not belong to the claimed workspace")
)

// NewTemplateApprovalService creates the school-approval service.
//
// Deliberately separate from TemplateVersionService rather than more methods on it:
// approval is a governance concern with its own lifecycle, and that service already
// carries eight dependencies.
func NewTemplateApprovalService(
	approvalRepo port.TemplateVersionApprovalRepository,
	versionRepo port.TemplateVersionRepository,
) templateuc.TemplateApprovalUseCase {
	return &TemplateApprovalService{
		approvalRepo: approvalRepo,
		versionRepo:  versionRepo,
	}
}

// versionContentReader is the single method this service needs from the template
// version repository. Depending on the narrow slice rather than the whole interface
// keeps the coupling honest and makes the service testable without a 20-method fake.
type versionContentReader interface {
	FindByID(ctx context.Context, id string) (*entity.TemplateVersion, error)

	// FindByIDWithDetailsAndTemplateWorkspace is how the service learns which
	// workspace a version actually belongs to, so a caller's claim can be checked
	// rather than believed.
	FindByIDWithDetailsAndTemplateWorkspace(ctx context.Context, id string) (*port.TemplateVersionContext, error)
}

// TemplateApprovalService runs the propose/decide lifecycle.
type TemplateApprovalService struct {
	approvalRepo port.TemplateVersionApprovalRepository
	versionRepo  versionContentReader
}

// assertVersionInWorkspace refuses to act on a version the caller does not own.
//
// The internal API key is shared by every service-to-service caller, so on its own
// it says "a Tether service is asking", not "this school's service is asking". Every
// sibling internal endpoint narrows that with tenant and workspace headers; without
// the same check here, a caller could record that one school's director approved
// another school's contract and nothing in the stack would object. For a table whose
// only purpose is answering who agreed to what, that is the wrong place to be
// permissive.
//
// An empty claim is refused rather than waved through: a missing header must not be
// a way to opt out of the check.
func (s *TemplateApprovalService) assertVersionInWorkspace(
	ctx context.Context,
	versionID, workspaceCode string,
) error {
	claimed := strings.TrimSpace(workspaceCode)
	if claimed == "" {
		return ErrWorkspaceScopeRequired
	}

	versionCtx, err := s.versionRepo.FindByIDWithDetailsAndTemplateWorkspace(ctx, versionID)
	if err != nil {
		return fmt.Errorf("finding template version context: %w", err)
	}
	if versionCtx == nil || versionCtx.Workspace == nil {
		return fmt.Errorf("template version %s has no workspace: %w", versionID, ErrWorkspaceMismatch)
	}
	if !strings.EqualFold(versionCtx.Workspace.Code, claimed) {
		return fmt.Errorf(
			"template version %s belongs to workspace %s, not %s: %w",
			versionID, versionCtx.Workspace.Code, claimed, ErrWorkspaceMismatch,
		)
	}
	return nil
}

// currentChecksum hashes a version's content as it stands now.
func (s *TemplateApprovalService) currentChecksum(
	ctx context.Context,
	versionID string,
) (string, error) {
	version, err := s.versionRepo.FindByID(ctx, versionID)
	if err != nil {
		return "", fmt.Errorf("finding template version: %w", err)
	}
	checksum, err := entity.ChecksumOfContent(version.ContentStructure)
	if err != nil {
		return "", fmt.Errorf("hashing content: %w", err)
	}
	return checksum, nil
}

// Propose puts the version's current content to the school for approval.
//
// Refuses when a proposal is already outstanding. Two pending proposals would make
// "the newest approval" ambiguous exactly when the publish gate consults it; the
// schema enforces this too, and this check exists to return a comprehensible error
// rather than a constraint violation.
func (s *TemplateApprovalService) Propose(
	ctx context.Context,
	cmd templateuc.ProposeApprovalCommand,
) (*entity.TemplateVersionApproval, error) {
	if err := s.assertVersionInWorkspace(ctx, cmd.TemplateVersionID, cmd.WorkspaceCode); err != nil {
		return nil, err
	}

	existing, err := s.approvalRepo.FindPendingForVersion(ctx, cmd.TemplateVersionID)
	if err != nil && !errors.Is(err, entity.ErrApprovalNotFound) {
		return nil, fmt.Errorf("checking for an outstanding proposal: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf(
			"version %s already has an outstanding proposal: %w",
			cmd.TemplateVersionID, entity.ErrApprovalAlreadyDecided,
		)
	}

	checksum, err := s.currentChecksum(ctx, cmd.TemplateVersionID)
	if err != nil {
		return nil, err
	}

	// Re-proposing text that is already approved would move the latest approval back
	// to PENDING and silently revoke a standing approval of the identical document.
	// Refusing makes that impossible by accident; a deliberate re-approval of
	// unchanged text has no reader-visible purpose anyway.
	latest, err := s.approvalRepo.FindLatestForVersion(ctx, cmd.TemplateVersionID)
	if err != nil && !errors.Is(err, entity.ErrApprovalNotFound) {
		return nil, err
	}
	if latest.AuthorizesContent(checksum) {
		return nil, entity.ErrApprovalContentUnchanged
	}

	approval := entity.NewTemplateVersionApproval(cmd.TemplateVersionID, cmd.ProposedBy, checksum)
	if _, err := s.approvalRepo.Create(ctx, approval); err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "contract version proposed for approval",
		slog.String("version_id", cmd.TemplateVersionID),
		slog.String("approval_id", approval.ID),
		slog.String("proposed_by", cmd.ProposedBy),
	)
	return approval, nil
}

// Decide records the school's decision on a pending proposal.
func (s *TemplateApprovalService) Decide(
	ctx context.Context,
	cmd templateuc.DecideApprovalCommand,
) (*entity.TemplateVersionApproval, error) {
	approval, err := s.approvalRepo.FindByID(ctx, cmd.ApprovalID)
	if err != nil {
		return nil, err
	}
	if err := s.assertVersionInWorkspace(ctx, approval.TemplateVersionID, cmd.WorkspaceCode); err != nil {
		return nil, err
	}

	status := entity.ApprovalStatusChangesRequested
	if cmd.Approved {
		status = entity.ApprovalStatusApproved
	}

	if err := approval.Decide(status, cmd.Email, cmd.Name, cmd.Campus, cmd.Comment); err != nil {
		return nil, err
	}
	if err := s.approvalRepo.RecordDecision(ctx, approval); err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "contract version decision recorded",
		slog.String("version_id", approval.TemplateVersionID),
		slog.String("approval_id", approval.ID),
		slog.String("status", string(approval.Status)),
		slog.String("decided_by", cmd.Email),
		slog.String("campus", cmd.Campus),
	)
	return approval, nil
}

// GetState answers whether a version may be published right now.
//
// A version that was never proposed is not an error: it is simply unapproved, which
// the caller needs to render differently from a failure.
func (s *TemplateApprovalService) GetState(
	ctx context.Context,
	versionID string,
) (*templateuc.ApprovalState, error) {
	checksum, err := s.currentChecksum(ctx, versionID)
	if err != nil {
		return nil, err
	}

	latest, err := s.approvalRepo.FindLatestForVersion(ctx, versionID)
	if err != nil && !errors.Is(err, entity.ErrApprovalNotFound) {
		return nil, err
	}

	return &templateuc.ApprovalState{
		Latest:          latest,
		CurrentChecksum: checksum,
		Authorized:      latest.AuthorizesContent(checksum),
		Stale:           latest.IsStale(checksum),
	}, nil
}

// Withdraw retracts a proposal that has not been decided.
func (s *TemplateApprovalService) Withdraw(
	ctx context.Context,
	approvalID, workspaceCode string,
) error {
	approval, err := s.approvalRepo.FindByID(ctx, approvalID)
	if err != nil {
		return err
	}
	if err := s.assertVersionInWorkspace(ctx, approval.TemplateVersionID, workspaceCode); err != nil {
		return err
	}
	if err := s.approvalRepo.Withdraw(ctx, approvalID); err != nil {
		return err
	}

	slog.InfoContext(ctx, "contract version proposal withdrawn",
		slog.String("version_id", approval.TemplateVersionID),
		slog.String("approval_id", approvalID),
	)
	return nil
}

// ListHistory returns a version's approvals, newest first.
func (s *TemplateApprovalService) ListHistory(
	ctx context.Context,
	versionID string,
) ([]*entity.TemplateVersionApproval, error) {
	return s.approvalRepo.ListForVersion(ctx, versionID)
}
