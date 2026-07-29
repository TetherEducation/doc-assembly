package document

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Per-ID fakes: the shared readOnlyViewDocumentRepoFake answers with one document
// regardless of the id requested, which cannot express a batch.
type signingStateDocumentRepoFake struct {
	port.DocumentRepository
	byID      map[string]*entity.Document
	errByID   map[string]error
	requested []string
}

func (f *signingStateDocumentRepoFake) FindByID(_ context.Context, id string) (*entity.Document, error) {
	f.requested = append(f.requested, id)
	if err, ok := f.errByID[id]; ok {
		return nil, err
	}
	doc, ok := f.byID[id]
	if !ok {
		return nil, entity.ErrDocumentNotFound
	}
	return doc, nil
}

type signingStateRecipientRepoFake struct {
	port.DocumentRecipientRepository
	byDocumentID map[string][]*entity.DocumentRecipientWithRole
	err          error
}

func (f *signingStateRecipientRepoFake) FindByDocumentIDWithRoles(
	_ context.Context,
	documentID string,
) ([]*entity.DocumentRecipientWithRole, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byDocumentID[documentID], nil
}

func newSigningStateService(
	docs *signingStateDocumentRepoFake,
	recipients *signingStateRecipientRepoFake,
	workspaces *readOnlyViewWorkspaceRepoFake,
) *ReadOnlyViewService {
	service := NewReadOnlyViewService(
		docs, &readOnlyViewAccessTokenRepoFake{}, recipients,
		&readOnlyViewVersionRepoFake{}, nil, nil, nil, nil, false, 48,
		"https://public.example.test/",
	)
	return service.SetWorkspaceRepository(workspaces)
}

func TestReadOnlyViewService_GetSigningStateByWorkspaceCode(t *testing.T) {
	ctx := context.Background()
	signedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	workspaces := &readOnlyViewWorkspaceRepoFake{byID: map[string]*entity.Workspace{
		"workspace-uuid": {ID: "workspace-uuid", Code: "CAMPUS_A"},
		"other-uuid":     {ID: "other-uuid", Code: "CAMPUS_B"},
	}}

	t.Run("reports an unsigned document as not signed", func(t *testing.T) {
		docs := &signingStateDocumentRepoFake{byID: map[string]*entity.Document{
			"doc-1": {ID: "doc-1", WorkspaceID: "workspace-uuid", Status: entity.DocumentStatusReadyToSign},
		}}
		recipients := &signingStateRecipientRepoFake{byDocumentID: map[string][]*entity.DocumentRecipientWithRole{
			"doc-1": {
				{
					DocumentRecipient: entity.DocumentRecipient{
						Name:   "Guardian",
						Email:  "guardian@example.test",
						Status: entity.RecipientStatusPending,
					},
					RoleName:    "GUARDIAN",
					SignerOrder: 1,
				},
			},
		}}

		result, err := newSigningStateService(docs, recipients, workspaces).
			GetSigningStateByWorkspaceCode(ctx, "CAMPUS_A", []string{"doc-1"})

		require.NoError(t, err)
		require.Len(t, result.Documents, 1)
		assert.False(t, result.Documents[0].Signed)
		assert.Equal(t, entity.DocumentStatusReadyToSign, result.Documents[0].Status)
		assert.Empty(t, result.Unavailable)
		require.Len(t, result.Documents[0].Recipients, 1)
		assert.False(t, result.Documents[0].Recipients[0].Signed)
		assert.Equal(t, "GUARDIAN", result.Documents[0].Recipients[0].RoleName)
	})

	t.Run("reports a completed document as signed with recipient timestamps", func(t *testing.T) {
		docs := &signingStateDocumentRepoFake{byID: map[string]*entity.Document{
			"doc-1": {ID: "doc-1", WorkspaceID: "workspace-uuid", Status: entity.DocumentStatusCompleted},
		}}
		recipients := &signingStateRecipientRepoFake{byDocumentID: map[string][]*entity.DocumentRecipientWithRole{
			"doc-1": {
				{
					DocumentRecipient: entity.DocumentRecipient{
						Email:    "guardian@example.test",
						Status:   entity.RecipientStatusSigned,
						SignedAt: &signedAt,
					},
					RoleName: "GUARDIAN",
				},
			},
		}}

		result, err := newSigningStateService(docs, recipients, workspaces).
			GetSigningStateByWorkspaceCode(ctx, "CAMPUS_A", []string{"doc-1"})

		require.NoError(t, err)
		require.Len(t, result.Documents, 1)
		assert.True(t, result.Documents[0].Signed)
		require.Len(t, result.Documents[0].Recipients, 1)
		assert.True(t, result.Documents[0].Recipients[0].Signed)
		require.NotNil(t, result.Documents[0].Recipients[0].SignedAt)
		assert.Equal(t, signedAt, *result.Documents[0].Recipients[0].SignedAt)
	})

	// Unlike the link/print flows, a dead or expired document must be reported
	// rather than rejected: "this needs regenerating" is the useful answer.
	t.Run("reports invalidated, cancelled and expired documents instead of erroring", func(t *testing.T) {
		past := time.Now().UTC().Add(-time.Hour)
		docs := &signingStateDocumentRepoFake{byID: map[string]*entity.Document{
			"doc-invalidated": {ID: "doc-invalidated", WorkspaceID: "workspace-uuid", Status: entity.DocumentStatusInvalidated},
			"doc-cancelled":   {ID: "doc-cancelled", WorkspaceID: "workspace-uuid", Status: entity.DocumentStatusCancelled},
			"doc-expired": {
				ID: "doc-expired", WorkspaceID: "workspace-uuid",
				Status: entity.DocumentStatusReadyToSign, ExpiresAt: &past,
			},
		}}

		result, err := newSigningStateService(docs, &signingStateRecipientRepoFake{}, workspaces).
			GetSigningStateByWorkspaceCode(ctx, "CAMPUS_A",
				[]string{"doc-invalidated", "doc-cancelled", "doc-expired"})

		require.NoError(t, err)
		require.Len(t, result.Documents, 3)
		assert.Empty(t, result.Unavailable)

		byID := map[string]bool{}
		for _, doc := range result.Documents {
			byID[doc.DocumentID] = doc.Expired
			assert.False(t, doc.Signed)
		}
		assert.False(t, byID["doc-invalidated"])
		assert.True(t, byID["doc-expired"], "expired document must be flagged so callers regenerate instead of reminding")
	})

	// The security property: a document in another workspace must be
	// indistinguishable from one that does not exist.
	t.Run("hides documents from other workspaces as unavailable", func(t *testing.T) {
		docs := &signingStateDocumentRepoFake{byID: map[string]*entity.Document{
			"doc-mine":   {ID: "doc-mine", WorkspaceID: "workspace-uuid", Status: entity.DocumentStatusReadyToSign},
			"doc-theirs": {ID: "doc-theirs", WorkspaceID: "other-uuid", Status: entity.DocumentStatusCompleted},
		}}

		result, err := newSigningStateService(docs, &signingStateRecipientRepoFake{}, workspaces).
			GetSigningStateByWorkspaceCode(ctx, "CAMPUS_A", []string{"doc-mine", "doc-theirs", "doc-ghost"})

		require.NoError(t, err)
		require.Len(t, result.Documents, 1)
		assert.Equal(t, "doc-mine", result.Documents[0].DocumentID)
		assert.ElementsMatch(t, []string{"doc-theirs", "doc-ghost"}, result.Unavailable,
			"another workspace's document must look exactly like a missing one")
	})

	t.Run("one unknown id does not fail the batch", func(t *testing.T) {
		docs := &signingStateDocumentRepoFake{
			byID: map[string]*entity.Document{
				"doc-1": {ID: "doc-1", WorkspaceID: "workspace-uuid", Status: entity.DocumentStatusCompleted},
			},
			errByID: map[string]error{"doc-bad": entity.ErrRecordNotFound},
		}

		result, err := newSigningStateService(docs, &signingStateRecipientRepoFake{}, workspaces).
			GetSigningStateByWorkspaceCode(ctx, "CAMPUS_A", []string{"doc-bad", "doc-1"})

		require.NoError(t, err)
		require.Len(t, result.Documents, 1)
		assert.Equal(t, []string{"doc-bad"}, result.Unavailable)
	})

	t.Run("propagates unexpected repository failures", func(t *testing.T) {
		boom := errors.New("connection reset")
		docs := &signingStateDocumentRepoFake{errByID: map[string]error{"doc-1": boom}}

		_, err := newSigningStateService(docs, &signingStateRecipientRepoFake{}, workspaces).
			GetSigningStateByWorkspaceCode(ctx, "CAMPUS_A", []string{"doc-1"})

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("skips blank and duplicated ids", func(t *testing.T) {
		docs := &signingStateDocumentRepoFake{byID: map[string]*entity.Document{
			"doc-1": {ID: "doc-1", WorkspaceID: "workspace-uuid", Status: entity.DocumentStatusCompleted},
		}}

		result, err := newSigningStateService(docs, &signingStateRecipientRepoFake{}, workspaces).
			GetSigningStateByWorkspaceCode(ctx, "CAMPUS_A", []string{"doc-1", " doc-1 ", "", "  "})

		require.NoError(t, err)
		require.Len(t, result.Documents, 1)
		assert.Equal(t, []string{"doc-1"}, docs.requested,
			"trimming happens before dedupe, so ' doc-1 ' is one lookup, not two; blanks are never looked up")
	})

	t.Run("returns nothing when the workspace code is empty", func(t *testing.T) {
		docs := &signingStateDocumentRepoFake{byID: map[string]*entity.Document{
			"doc-1": {ID: "doc-1", WorkspaceID: "workspace-uuid", Status: entity.DocumentStatusCompleted},
		}}

		result, err := newSigningStateService(docs, &signingStateRecipientRepoFake{}, workspaces).
			GetSigningStateByWorkspaceCode(ctx, "", []string{"doc-1"})

		require.NoError(t, err)
		assert.Empty(t, result.Documents)
		assert.Equal(t, []string{"doc-1"}, result.Unavailable)
	})
}
