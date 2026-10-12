package template

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// ProposeApprovalCommand puts a version's current content to the school.
type ProposeApprovalCommand struct {
	TemplateVersionID string
	ProposedBy        string

	// WorkspaceCode the caller claims the version belongs to. Checked, not trusted:
	// every sibling internal endpoint scopes by tenant and workspace, and without it
	// any holder of the internal key could act on any school's contract.
	WorkspaceCode string
}

// DecideApprovalCommand records what the school decided.
//
// Name and email are stored rather than a user id alone: the user record can
// change or be deleted, the legal fact cannot. Campus is recorded because
// capabilities are per-campus while a network approves its shared template once,
// so the context the decision was made in is not derivable from the template.
type DecideApprovalCommand struct {
	ApprovalID string

	// WorkspaceCode the caller claims the approval's version belongs to. Checked
	// against the template's actual workspace before anything is written, so the
	// record cannot say one school's director approved another school's contract.
	WorkspaceCode string
	Approved      bool
	Email         string
	Name          string
	Campus        string
	Comment       *string
}

// ApprovalState is the answer to "may this version be published, and why not?".
type ApprovalState struct {
	// Latest is the newest approval, or nil when the version was never proposed.
	Latest *entity.TemplateVersionApproval

	// CurrentChecksum is the version's content as it stands now.
	CurrentChecksum string

	// Authorized reports whether publishing is permitted: an APPROVED latest whose
	// checksum still matches.
	Authorized bool

	// Stale separates "was approved, then edited" from "never approved". They look
	// the same to a gate but need different words in front of a person.
	Stale bool
}

// TemplateApprovalUseCase is the school-approval lifecycle for a template version.
type TemplateApprovalUseCase interface {
	// Propose puts the version's current content to the school for approval.
	// Refuses when a proposal is already outstanding.
	Propose(ctx context.Context, cmd ProposeApprovalCommand) (*entity.TemplateVersionApproval, error)

	// Decide records the school's decision on a pending proposal.
	Decide(ctx context.Context, cmd DecideApprovalCommand) (*entity.TemplateVersionApproval, error)

	// GetState answers whether a version may be published right now.
	GetState(ctx context.Context, versionID string) (*ApprovalState, error)

	// ListHistory returns a version's approvals, newest first.
	ListHistory(ctx context.Context, versionID string) ([]*entity.TemplateVersionApproval, error)

	// Withdraw retracts a proposal that has not been decided, so a proposal sent by
	// mistake does not have to be answered by the school to get rid of it.
	Withdraw(ctx context.Context, approvalID, workspaceCode string) error
}
