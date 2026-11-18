package template

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// docTypeTemplateRepoFake holds one template and the document types its
// workspace's tenant can use, and records the types assigned.
type docTypeTemplateRepoFake struct {
	port.TemplateRepository
	template  *entity.Template
	available map[string]bool
	assigned  []string
}

func (f *docTypeTemplateRepoFake) FindByID(_ context.Context, id string) (*entity.Template, error) {
	if f.template != nil && f.template.ID == id {
		copied := *f.template
		return &copied, nil
	}
	return nil, entity.ErrTemplateNotFound
}

func (f *docTypeTemplateRepoFake) DocumentTypeAvailable(_ context.Context, workspaceID, documentTypeID string) (bool, error) {
	return workspaceID == f.template.WorkspaceID && f.available[documentTypeID], nil
}

func (f *docTypeTemplateRepoFake) FindByDocumentType(context.Context, string, string, string) (*entity.Template, error) {
	return nil, nil
}

func (f *docTypeTemplateRepoFake) UpdateDocumentType(_ context.Context, _ string, documentTypeID *string) error {
	f.assigned = append(f.assigned, *documentTypeID)
	return nil
}

// A template takes a document type its workspace's tenant can use. Any other,
// like an ID that is not a UUID, answers exactly as a missing type does, and
// the template keeps the type it had.
func TestAssignDocumentType_TypeOfTheTemplatesTenant(t *testing.T) {
	const (
		templateID    = "template-1"
		workspaceID   = "ws-caller"
		usableType    = "6f1c2a9e-3b4d-4e8f-9a01-2b3c4d5e6f70"
		otherTenant   = "7a2d3b0f-4c5e-4f90-8b12-3c4d5e6f7a81"
		missingTypeID = "8b3e4c1a-5d6f-4a01-9c23-4d5e6f7a8b92"
	)
	assign := func(documentTypeID string) (*templateuc.AssignDocumentTypeResult, []string, error) {
		repo := &docTypeTemplateRepoFake{
			template:  &entity.Template{ID: templateID, WorkspaceID: workspaceID, Process: entity.DefaultProcess},
			available: map[string]bool{usableType: true},
		}
		svc := &TemplateService{templateRepo: repo}
		result, err := svc.AssignDocumentType(context.Background(), templateuc.AssignDocumentTypeCommand{
			TemplateID: templateID, WorkspaceID: workspaceID, DocumentTypeID: &documentTypeID,
		})
		return result, repo.assigned, err
	}

	result, assigned, err := assign(usableType)
	require.NoError(t, err)
	require.NotNil(t, result.Template)
	assert.Equal(t, []string{usableType}, assigned)

	_, _, missingErr := assign(missingTypeID)
	require.ErrorIs(t, missingErr, entity.ErrDocumentTypeNotFound)

	for name, documentTypeID := range map[string]string{"type of another tenant": otherTenant, "not a UUID": "type-1"} {
		t.Run(name, func(t *testing.T) {
			result, assigned, err := assign(documentTypeID)
			require.ErrorIs(t, err, entity.ErrDocumentTypeNotFound)
			assert.EqualError(t, err, missingErr.Error(), "the answer must be the one a missing type gets")
			assert.Nil(t, result)
			assert.Empty(t, assigned, "no type may be assigned")
		})
	}
}
