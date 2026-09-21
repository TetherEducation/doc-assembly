package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ApprovalStatus is where a proposal stands.
type ApprovalStatus string

const (
	ApprovalStatusPending          ApprovalStatus = "PENDING"
	ApprovalStatusApproved         ApprovalStatus = "APPROVED"
	ApprovalStatusChangesRequested ApprovalStatus = "CHANGES_REQUESTED"
)

var (
	// ErrApprovalCommentRequired is returned when changes are requested without
	// saying what is wrong. A rejection with no reason cannot be acted on, which
	// makes the round trip pointless.
	ErrApprovalCommentRequired = errors.New("a comment is required when requesting changes")

	// ErrApprovalAlreadyDecided is returned when deciding an approval that is no
	// longer pending. Decisions are never overwritten - a change of mind is a new
	// proposal, so the history stays readable.
	ErrApprovalAlreadyDecided = errors.New("approval has already been decided")

	// ErrApprovalNotFound is returned when no approval exists for a version.
	ErrApprovalNotFound = errors.New("no approval found for template version")
)

// TemplateVersionApproval records a school's decision about one exact contract text.
//
// The row is never updated after a decision: a re-proposal is a new row, so the
// table is its own audit trail.
type TemplateVersionApproval struct {
	ID                string         `json:"id"`
	TemplateVersionID string         `json:"templateVersionId"`
	ContentChecksum   string         `json:"contentChecksum"`
	Status            ApprovalStatus `json:"status"`

	ProposedBy string    `json:"proposedBy"`
	ProposedAt time.Time `json:"proposedAt"`

	DecidedByEmail  *string    `json:"decidedByEmail,omitempty"`
	DecidedByName   *string    `json:"decidedByName,omitempty"`
	DecidedAtCampus *string    `json:"decidedAtCampus,omitempty"`
	DecidedAt       *time.Time `json:"decidedAt,omitempty"`

	Comment         *string `json:"comment,omitempty"`
	ApprovedPDFPath *string `json:"approvedPdfPath,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
}

// ChecksumOfContent hashes a content structure so an approval can be tied to the
// exact text it approved.
//
// The same document read back from jsonb can differ byte-for-byte from what was
// written - key order and whitespace are not preserved - while being the same
// document. Hashing those bytes would make approvals expire spontaneously, and a
// warning that fires for no reason is one people learn to ignore.
//
// So raw JSON is decoded before it is hashed. encoding/json passes a
// json.RawMessage straight through untouched, which means marshalling one is
// hashing the raw bytes; decoding first and re-encoding sorts object keys and
// drops insignificant whitespace, giving the same hash for the same document
// however it was stored.
func ChecksumOfContent(content any) (string, error) {
	if raw, ok := rawJSON(content); ok {
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return "", fmt.Errorf("decoding content before hashing: %w", err)
		}
		content = decoded
	}

	canonical, err := json.Marshal(content)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// rawJSON reports content that is already encoded JSON rather than a Go value.
func rawJSON(content any) ([]byte, bool) {
	switch v := content.(type) {
	case json.RawMessage:
		return v, len(v) > 0
	case []byte:
		return v, len(v) > 0
	default:
		return nil, false
	}
}

// NewTemplateVersionApproval starts a proposal for a version's current content.
func NewTemplateVersionApproval(versionID, proposedBy, checksum string) *TemplateVersionApproval {
	now := time.Now().UTC()
	return &TemplateVersionApproval{
		TemplateVersionID: versionID,
		ContentChecksum:   checksum,
		Status:            ApprovalStatusPending,
		ProposedBy:        proposedBy,
		ProposedAt:        now,
		CreatedAt:         now,
	}
}

// Decide records a school's decision on a pending proposal.
func (a *TemplateVersionApproval) Decide(
	status ApprovalStatus,
	email, name, campus string,
	comment *string,
) error {
	if a.Status != ApprovalStatusPending {
		return ErrApprovalAlreadyDecided
	}
	if status != ApprovalStatusApproved && status != ApprovalStatusChangesRequested {
		return errors.New("a decision must be APPROVED or CHANGES_REQUESTED")
	}
	if status == ApprovalStatusChangesRequested &&
		(comment == nil || strings.TrimSpace(*comment) == "") {
		return ErrApprovalCommentRequired
	}

	now := time.Now().UTC()
	a.Status = status
	a.DecidedByEmail = &email
	a.DecidedByName = &name
	a.DecidedAt = &now
	if strings.TrimSpace(campus) != "" {
		a.DecidedAtCampus = &campus
	}
	if comment != nil && strings.TrimSpace(*comment) != "" {
		trimmed := strings.TrimSpace(*comment)
		a.Comment = &trimmed
	}
	return nil
}

// AuthorizesContent reports whether this approval permits publishing the given
// content right now.
//
// Both halves matter. An APPROVED row whose checksum no longer matches means the
// text was edited after sign-off: the school approved something else. Treating that
// as approved is the exact failure this feature exists to prevent, so a stale
// approval is worth no more than none at all.
func (a *TemplateVersionApproval) AuthorizesContent(currentChecksum string) bool {
	if a == nil || a.Status != ApprovalStatusApproved {
		return false
	}
	return a.ContentChecksum == currentChecksum
}

// IsStale reports an approval that was granted and then invalidated by an edit.
// Distinguished from "never approved" so the UI can say which of the two it is -
// they need different actions from the reader.
func (a *TemplateVersionApproval) IsStale(currentChecksum string) bool {
	if a == nil || a.Status != ApprovalStatusApproved {
		return false
	}
	return a.ContentChecksum != currentChecksum
}
