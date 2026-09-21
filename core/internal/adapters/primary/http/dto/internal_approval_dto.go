package dto

import "time"

// InternalProposeApprovalRequest puts a version's current content to the school.
type InternalProposeApprovalRequest struct {
	// ProposedBy is who at Tether put it forward. Required: an approval whose
	// proposer is unknown is half an audit trail.
	ProposedBy string `json:"proposedBy"`
}

// InternalDecideApprovalRequest records what the school decided.
type InternalDecideApprovalRequest struct {
	Approved bool `json:"approved"`

	// Who decided. Stored as name and email rather than a user id alone, because
	// the user record can change or be deleted and the legal fact cannot.
	Email string `json:"email"`
	Name  string `json:"name"`

	// Which campus the decision was made from. Capabilities are per-campus while a
	// network approves its shared template once, so this is not derivable.
	Campus string `json:"campus"`

	// Required when requesting changes.
	Comment *string `json:"comment,omitempty"`
}

// InternalApprovalResponse is one approval row.
type InternalApprovalResponse struct {
	ID                string     `json:"id"`
	TemplateVersionID string     `json:"templateVersionId"`
	Status            string     `json:"status"`
	ContentChecksum   string     `json:"contentChecksum"`
	ProposedBy        string     `json:"proposedBy"`
	ProposedAt        time.Time  `json:"proposedAt"`
	DecidedByEmail    *string    `json:"decidedByEmail,omitempty"`
	DecidedByName     *string    `json:"decidedByName,omitempty"`
	DecidedAtCampus   *string    `json:"decidedAtCampus,omitempty"`
	DecidedAt         *time.Time `json:"decidedAt,omitempty"`
	Comment           *string    `json:"comment,omitempty"`
}

// InternalApprovalStateResponse answers "may this version be published, and if
// not, why not?".
//
// Authorized and Stale are separate on purpose. Both mean "do not publish", but
// "approved, then edited" and "never approved" need different words in front of a
// person: one is a warning that something changed under them, the other is simply
// work not yet done.
type InternalApprovalStateResponse struct {
	Authorized      bool                      `json:"authorized"`
	Stale           bool                      `json:"stale"`
	CurrentChecksum string                    `json:"currentChecksum"`
	Latest          *InternalApprovalResponse `json:"latest,omitempty"`
}

// InternalApprovalHistoryResponse lists a version's approvals, newest first.
type InternalApprovalHistoryResponse struct {
	Items []InternalApprovalResponse `json:"items"`
}
