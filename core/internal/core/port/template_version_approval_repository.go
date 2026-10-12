package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TemplateVersionApprovalRepository stores schools' decisions about contract text.
//
// There is deliberately no Update: a decided approval is a historical fact and a
// change of mind is a new proposal. Keeping the table append-only is what makes it
// usable as an audit trail years later.
type TemplateVersionApprovalRepository interface {
	// Create records a new proposal.
	Create(ctx context.Context, approval *entity.TemplateVersionApproval) (string, error)

	// FindByID finds one approval.
	FindByID(ctx context.Context, id string) (*entity.TemplateVersionApproval, error)

	// FindLatestForVersion returns the newest approval for a version, which is the
	// only one the publish gate consults. Returns entity.ErrApprovalNotFound when a
	// version has never been proposed.
	FindLatestForVersion(ctx context.Context, versionID string) (*entity.TemplateVersionApproval, error)

	// FindPendingForVersion returns the outstanding proposal for a version, if any.
	// At most one can exist - the schema enforces it.
	FindPendingForVersion(ctx context.Context, versionID string) (*entity.TemplateVersionApproval, error)

	// ListForVersion returns a version's approval history, newest first.
	ListForVersion(ctx context.Context, versionID string) ([]*entity.TemplateVersionApproval, error)

	// Withdraw removes a proposal that is still PENDING.
	//
	// Without this a proposal sent by mistake can only be escaped by asking someone
	// at the school to decide something we never meant to send them - or by deleting
	// the row in SQL, which is the side-door this codebase keeps closing. Deleting
	// rather than marking withdrawn is deliberate: an undecided proposal carries no
	// governance fact worth keeping, and a WITHDRAWN row would complicate every
	// "newest approval" lookup for nothing.
	//
	// Returns entity.ErrApprovalNotPending when the row was already decided.
	Withdraw(ctx context.Context, id string) error

	// RecordDecision writes a decision onto a proposal that is still PENDING.
	//
	// The status guard is in the UPDATE itself rather than a read-then-write, so two
	// people deciding at the same moment cannot both succeed. Returns
	// entity.ErrApprovalAlreadyDecided when the row was no longer pending.
	RecordDecision(ctx context.Context, approval *entity.TemplateVersionApproval) error
}
