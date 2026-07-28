package document

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

func newPrintPDFService(deps readOnlyViewPDFServiceDeps, workspaces map[string]*entity.Workspace) *ReadOnlyViewService {
	service := newReadOnlyViewPDFService(deps)
	if workspaces != nil {
		service.SetWorkspaceRepository(&readOnlyViewWorkspaceRepoFake{byID: workspaces})
	}
	return service
}

func TestReadOnlyViewService_GetPrintPDF_RendersFilledPDFForAwaitingInput(t *testing.T) {
	renderer := &readOnlyViewPDFRendererFake{
		result: &port.RenderPreviewResult{PDF: []byte("%PDF-print"), Filename: "renderer.pdf"},
	}
	title := "Contrato Matricula"
	service := newPrintPDFService(readOnlyViewPDFServiceDeps{
		doc: &entity.Document{
			ID:                     "doc-123",
			Title:                  &title,
			WorkspaceID:            "workspace-uuid",
			TemplateVersionID:      "version-123",
			Status:                 entity.DocumentStatusAwaitingInput,
			InjectedValuesSnapshot: json.RawMessage(`{"legalguardian_first_name":"Jane"}`),
		},
		recipients: []*entity.DocumentRecipient{{
			ID:                    "recipient-1",
			DocumentID:            "doc-123",
			TemplateVersionRoleID: "role-1",
			Name:                  "Jane Doe",
			Email:                 "jane@example.test",
		}},
		signerRoles: []*entity.TemplateVersionSignerRole{{
			ID:                "role-1",
			TemplateVersionID: "version-123",
			AnchorString:      portabledoc.GenerateAnchorString("Signer"),
		}},
		pdfRenderer: renderer,
	}, nil)

	pdf, filename, err := service.GetPrintPDF(context.Background(), "workspace-uuid", "doc-123", false)

	require.NoError(t, err)
	assert.Equal(t, []byte("%PDF-print"), pdf)
	assert.Equal(t, "Contrato Matricula-print.pdf", filename)
	require.NotNil(t, renderer.request)
	assert.Equal(t, "Jane", renderer.request.Injectables["legalguardian_first_name"])
	assert.Equal(t, port.SignerRoleValue{Name: "Jane Doe", Email: "jane@example.test"}, renderer.request.SignerRoleValues["portable-signer"])
}

func TestReadOnlyViewService_GetPrintPDF_BlankOmitsValuesAndResponses(t *testing.T) {
	renderer := &readOnlyViewPDFRendererFake{
		result: &port.RenderPreviewResult{PDF: []byte("%PDF-blank")},
	}
	service := newPrintPDFService(readOnlyViewPDFServiceDeps{
		doc: &entity.Document{
			ID:                     "doc-123",
			WorkspaceID:            "workspace-uuid",
			TemplateVersionID:      "version-123",
			Status:                 entity.DocumentStatusAwaitingInput,
			InjectedValuesSnapshot: json.RawMessage(`{"legalguardian_first_name":"Jane"}`),
		},
		recipients: []*entity.DocumentRecipient{{
			ID:                    "recipient-1",
			DocumentID:            "doc-123",
			TemplateVersionRoleID: "role-1",
			Name:                  "Jane Doe",
			Email:                 "jane@example.test",
		}},
		fieldResponses: []entity.DocumentFieldResponse{{
			DocumentID: "doc-123",
			FieldID:    "field-1",
			Response:   json.RawMessage(`{"text":"accepted"}`),
		}},
		pdfRenderer: renderer,
	}, nil)

	pdf, filename, err := service.GetPrintPDF(context.Background(), "workspace-uuid", "doc-123", true)

	require.NoError(t, err)
	assert.Equal(t, []byte("%PDF-blank"), pdf)
	assert.Equal(t, "document-doc-123-blank.pdf", filename)
	require.NotNil(t, renderer.request)
	assert.Empty(t, renderer.request.Injectables)
	assert.Empty(t, renderer.request.SignerRoleValues)
	assert.Empty(t, renderer.request.FieldResponses)
}

func TestReadOnlyViewService_GetPrintPDF_WorkspaceMismatchIsForbidden(t *testing.T) {
	service := newPrintPDFService(readOnlyViewPDFServiceDeps{
		doc: &entity.Document{
			ID:                "doc-123",
			WorkspaceID:       "workspace-uuid",
			TemplateVersionID: "version-123",
			Status:            entity.DocumentStatusAwaitingInput,
		},
	}, nil)

	pdf, filename, err := service.GetPrintPDF(context.Background(), "other-workspace", "doc-123", false)

	require.ErrorIs(t, err, entity.ErrForbidden)
	assert.Nil(t, pdf)
	assert.Empty(t, filename)
}

func TestReadOnlyViewService_GetPrintPDFByWorkspaceCode_MatchesDocumentWorkspaceCode(t *testing.T) {
	renderer := &readOnlyViewPDFRendererFake{
		result: &port.RenderPreviewResult{PDF: []byte("%PDF-print")},
	}
	service := newPrintPDFService(readOnlyViewPDFServiceDeps{
		doc: &entity.Document{
			ID:                "doc-123",
			WorkspaceID:       "workspace-uuid",
			TemplateVersionID: "version-123",
			Status:            entity.DocumentStatusReadyToSign,
		},
		recipients:  []*entity.DocumentRecipient{{ID: "recipient-1", DocumentID: "doc-123"}},
		pdfRenderer: renderer,
	}, map[string]*entity.Workspace{
		"workspace-uuid": {ID: "workspace-uuid", Code: "2036400001"},
	})

	pdf, _, err := service.GetPrintPDFByWorkspaceCode(context.Background(), "2036400001", "doc-123", false)

	require.NoError(t, err)
	assert.Equal(t, []byte("%PDF-print"), pdf)

	pdf, _, err = service.GetPrintPDFByWorkspaceCode(context.Background(), "9999999999", "doc-123", false)
	require.ErrorIs(t, err, entity.ErrForbidden)
	assert.Nil(t, pdf)
}

func TestReadOnlyViewService_GetPrintPDF_CompletedRejectsFilledAllowsBlank(t *testing.T) {
	renderer := &readOnlyViewPDFRendererFake{
		result: &port.RenderPreviewResult{PDF: []byte("%PDF-blank")},
	}
	deps := readOnlyViewPDFServiceDeps{
		doc: &entity.Document{
			ID:                "doc-123",
			WorkspaceID:       "workspace-uuid",
			TemplateVersionID: "version-123",
			Status:            entity.DocumentStatusCompleted,
		},
		pdfRenderer: renderer,
	}

	_, _, err := newPrintPDFService(deps, nil).GetPrintPDF(context.Background(), "workspace-uuid", "doc-123", false)
	require.ErrorIs(t, err, entity.ErrInvalidDocumentState)

	pdf, filename, err := newPrintPDFService(deps, nil).GetPrintPDF(context.Background(), "workspace-uuid", "doc-123", true)
	require.NoError(t, err)
	assert.Equal(t, []byte("%PDF-blank"), pdf)
	assert.Equal(t, "document-doc-123-blank.pdf", filename)
}

func TestReadOnlyViewService_GetPrintPDF_CancelledAndInvalidatedRejectBothModes(t *testing.T) {
	for _, status := range []entity.DocumentStatus{entity.DocumentStatusCancelled, entity.DocumentStatusInvalidated} {
		deps := readOnlyViewPDFServiceDeps{
			doc: &entity.Document{
				ID:                "doc-123",
				WorkspaceID:       "workspace-uuid",
				TemplateVersionID: "version-123",
				Status:            status,
			},
		}
		for _, blank := range []bool{false, true} {
			_, _, err := newPrintPDFService(deps, nil).GetPrintPDF(context.Background(), "workspace-uuid", "doc-123", blank)
			require.ErrorIs(t, err, entity.ErrInvalidDocumentState, "status=%s blank=%v", status, blank)
		}
	}
}
