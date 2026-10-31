package document

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

type relatedDocumentRepoFake struct {
	port.DocumentRepository
	docs map[string]*entity.Document
}

func (f *relatedDocumentRepoFake) FindByID(_ context.Context, id string) (*entity.Document, error) {
	if doc, ok := f.docs[id]; ok {
		return doc, nil
	}
	return nil, entity.ErrDocumentNotFound
}

func TestValidateOperationType_RelatedDocument(t *testing.T) {
	const (
		workspaceID      = "ws-caller"
		ownCompleted     = "6f1c2a9e-3b4d-4e8f-9a01-2b3c4d5e6f70"
		ownPending       = "7a2d3b0f-4c5e-4f90-8b12-3c4d5e6f7a81"
		foreignCompleted = "8b3e4c1a-5d6f-4a01-9c23-4d5e6f7a8b92"
		missing          = "9c4f5d2b-6e7a-4b12-8d34-5e6f7a8b9ca3"
	)
	svc := &DocumentService{documentRepo: &relatedDocumentRepoFake{docs: map[string]*entity.Document{
		ownCompleted:     {ID: ownCompleted, WorkspaceID: workspaceID, Status: entity.DocumentStatusCompleted},
		ownPending:       {ID: ownPending, WorkspaceID: workspaceID, Status: entity.DocumentStatusAwaitingInput},
		foreignCompleted: {ID: foreignCompleted, WorkspaceID: "ws-other", Status: entity.DocumentStatusCompleted},
	}}}

	tests := []struct {
		name      string
		operation entity.OperationType
		related   *string
		want      error
	}{
		{"create without a related document", "", nil, nil},
		{"create naming the caller's own document", "", strPtr(ownCompleted), entity.ErrRelatedDocumentNotAllowed},
		{"create naming another workspace's document", entity.OperationCreate, strPtr(foreignCompleted), entity.ErrRelatedDocumentNotAllowed},
		{"cancel naming a document", entity.OperationCancel, strPtr(ownCompleted), entity.ErrRelatedDocumentNotAllowed},
		{"preview naming a missing document", entity.OperationPreview, strPtr(missing), entity.ErrRelatedDocumentNotAllowed},
		{"renew without a related document", entity.OperationRenew, nil, entity.ErrRelatedDocumentRequired},
		{"renew of the caller's completed document", entity.OperationRenew, strPtr(ownCompleted), nil},
		{"renew of the caller's pending document", entity.OperationRenew, strPtr(ownPending), entity.ErrDocumentNotCompleted},
		{"renew of another workspace's document", entity.OperationRenew, strPtr(foreignCompleted), entity.ErrRelatedDocumentNotFound},
		{"renew of a missing document", entity.OperationRenew, strPtr(missing), entity.ErrRelatedDocumentNotFound},
		{"renew naming an ID that is not a UUID", entity.OperationRenew, strPtr("not-a-uuid"), entity.ErrRelatedDocumentNotFound},
		{"amend of the caller's completed document", entity.OperationAmend, strPtr(ownCompleted), nil},
		{"amend of the caller's pending document", entity.OperationAmend, strPtr(ownPending), entity.ErrDocumentNotTerminal},
		{"amend of another workspace's document", entity.OperationAmend, strPtr(foreignCompleted), entity.ErrRelatedDocumentNotFound},
		{"amend of a missing document", entity.OperationAmend, strPtr(missing), entity.ErrRelatedDocumentNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.validateOperationType(context.Background(), documentuc.CreateDocumentCommand{
				WorkspaceID:       workspaceID,
				OperationType:     tt.operation,
				RelatedDocumentID: tt.related,
			})

			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.want)
			// Unwrapped: the message becomes the response body, which must not
			// tell another workspace's document from a missing one.
			assert.EqualError(t, err, tt.want.Error())
		})
	}
}
