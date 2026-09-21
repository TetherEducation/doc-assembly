package entity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChecksumIsStableAcrossKeyOrder(t *testing.T) {
	t.Parallel()

	// The same document read back from jsonb can differ byte-for-byte from what was
	// written. If the checksum tracked those differences, approvals would expire
	// spontaneously and people would learn to ignore the warning.
	a := map[string]any{"type": "doc", "content": []any{"x"}, "title": "Contrato"}
	b := map[string]any{"title": "Contrato", "content": []any{"x"}, "type": "doc"}

	sumA, err := ChecksumOfContent(a)
	require.NoError(t, err)
	sumB, err := ChecksumOfContent(b)
	require.NoError(t, err)

	assert.Equal(t, sumA, sumB, "key order must not change the checksum")
}

func TestChecksumChangesWithContent(t *testing.T) {
	t.Parallel()

	before, err := ChecksumOfContent(map[string]any{"clause": "original"})
	require.NoError(t, err)
	after, err := ChecksumOfContent(map[string]any{"clause": "edited"})
	require.NoError(t, err)

	assert.NotEqual(t, before, after, "an edited clause must invalidate the approval")
}

// The whole feature exists to make "approved, then edited" impossible to publish.
func TestApprovedThenEditedDoesNotAuthorise(t *testing.T) {
	t.Parallel()

	approval := NewTemplateVersionApproval("v1", "chris@tether.education", "checksum-at-approval")
	require.NoError(t, approval.Decide(
		ApprovalStatusApproved, "director@colegio.cl", "La Directora", "2036400001", nil))

	assert.True(t, approval.AuthorizesContent("checksum-at-approval"),
		"unchanged content must publish")
	assert.False(t, approval.AuthorizesContent("checksum-after-an-edit"),
		"edited content must NOT publish on an approval of the earlier text")
	assert.True(t, approval.IsStale("checksum-after-an-edit"),
		"and the UI must be able to say it went stale rather than was never approved")
}

func TestOnlyApprovedAuthorises(t *testing.T) {
	t.Parallel()

	pending := NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	assert.False(t, pending.AuthorizesContent("sum"), "a pending proposal is not permission")
	assert.False(t, pending.IsStale("other"), "pending is not stale, it is undecided")

	rejected := NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	comment := "clause 4 names the wrong legal entity"
	require.NoError(t, rejected.Decide(
		ApprovalStatusChangesRequested, "director@colegio.cl", "La Directora", "2036400001", &comment))
	assert.False(t, rejected.AuthorizesContent("sum"), "changes requested is not permission")

	var missing *TemplateVersionApproval
	assert.False(t, missing.AuthorizesContent("sum"), "never proposed is not permission")
}

func TestChangesRequestedNeedsAComment(t *testing.T) {
	t.Parallel()

	blank := "   "
	for name, comment := range map[string]*string{
		"nil":             nil,
		"whitespace only": &blank,
	} {
		t.Run(name, func(t *testing.T) {
			a := NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
			err := a.Decide(ApprovalStatusChangesRequested,
				"director@colegio.cl", "La Directora", "2036400001", comment)
			// A rejection nobody can act on wastes the round trip the feature exists for.
			require.ErrorIs(t, err, ErrApprovalCommentRequired)
			assert.Equal(t, ApprovalStatusPending, a.Status, "a refused decision must not mutate")
		})
	}
}

func TestApprovalNeedsNoComment(t *testing.T) {
	t.Parallel()

	a := NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	require.NoError(t, a.Decide(
		ApprovalStatusApproved, "director@colegio.cl", "La Directora", "2036400001", nil))
	assert.Equal(t, ApprovalStatusApproved, a.Status)
}

func TestDecisionsAreNotOverwritten(t *testing.T) {
	t.Parallel()

	a := NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	require.NoError(t, a.Decide(
		ApprovalStatusApproved, "director@colegio.cl", "La Directora", "2036400001", nil))

	// A change of mind is a new proposal. Overwriting would erase who agreed to what,
	// which is the only thing this table is for.
	err := a.Decide(ApprovalStatusChangesRequested,
		"otro@colegio.cl", "Otro", "2036400001", strPtr("actually no"))
	require.ErrorIs(t, err, ErrApprovalAlreadyDecided)
	assert.Equal(t, "director@colegio.cl", *a.DecidedByEmail, "the original decider stands")
}

func TestDecisionRecordsWhoAndWhere(t *testing.T) {
	t.Parallel()

	a := NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	require.NoError(t, a.Decide(
		ApprovalStatusApproved, "director@colegio.cl", "La Directora", "2036400001", nil))

	require.NotNil(t, a.DecidedByEmail)
	require.NotNil(t, a.DecidedByName)
	require.NotNil(t, a.DecidedAt)
	require.NotNil(t, a.DecidedAtCampus)
	// Capabilities are per-campus but a network approves its shared template once,
	// so which campus the decision was made from is part of the record.
	assert.Equal(t, "2036400001", *a.DecidedAtCampus)
}

func TestCommentIsTrimmed(t *testing.T) {
	t.Parallel()

	a := NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	require.NoError(t, a.Decide(ApprovalStatusChangesRequested,
		"director@colegio.cl", "La Directora", "2036400001", strPtr("  fix clause 4  ")))
	assert.Equal(t, "fix clause 4", *a.Comment)
}

func strPtr(s string) *string { return &s }
