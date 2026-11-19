package document

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Every path that renders a document from its template renders it for the
// document's workspace. The renderer resolves stored images for that workspace
// only, so a path that left it out would render no stored image at all.

const renderWorkspaceID = "5f0c9a52-7c3e-4c7b-9d55-2f4f6a1f0e11"

func renderWorkspaceDocument(status entity.DocumentStatus) *entity.Document {
	return &entity.Document{
		ID:                "doc-123",
		WorkspaceID:       renderWorkspaceID,
		TemplateVersionID: "version-123",
		Status:            status,
	}
}

func TestPreSigningService_RenderPreviewPDF_RendersForTheDocumentsWorkspace(t *testing.T) {
	renderer := &readOnlyViewPDFRendererFake{result: &port.RenderPreviewResult{PDF: []byte("%PDF-preview")}}
	service := &PreSigningService{
		accessTokenRepo: &viewOnlyTokenAccessTokenRepoFake{token: &entity.DocumentAccessToken{
			DocumentID: "doc-123",
			TokenType:  entity.TokenTypeSigning,
			ExpiresAt:  time.Now().Add(time.Hour),
		}},
		documentRepo: &readOnlyViewDocumentRepoFake{doc: renderWorkspaceDocument(entity.DocumentStatusAwaitingInput)},
		versionRepo: &readOnlyViewVersionRepoFake{version: &entity.TemplateVersion{
			ID:               "version-123",
			ContentStructure: mustReadOnlyViewPDFPortableDocContent(),
		}},
		recipientRepo:     &readOnlyViewRecipientRepoFake{},
		signerRoleRepo:    &readOnlyViewSignerRoleRepoFake{},
		fieldResponseRepo: &readOnlyViewFieldResponseRepoFake{},
		pdfRenderer:       renderer,
	}

	pdf, err := service.RenderPreviewPDF(context.Background(), "token")

	require.NoError(t, err)
	assert.Equal(t, []byte("%PDF-preview"), pdf)
	require.NotNil(t, renderer.request)
	assert.Equal(t, renderWorkspaceID, renderer.request.WorkspaceID)
}

func TestReadOnlyViewService_GetReadOnlyViewPDF_RendersForTheDocumentsWorkspace(t *testing.T) {
	renderer := &readOnlyViewPDFRendererFake{result: &port.RenderPreviewResult{PDF: []byte("%PDF-preview")}}
	service := newReadOnlyViewPDFService(readOnlyViewPDFServiceDeps{
		doc:         renderWorkspaceDocument(entity.DocumentStatusSigning),
		pdfRenderer: renderer,
	})

	pdf, _, err := service.GetReadOnlyViewPDF(context.Background(), "view-token")

	require.NoError(t, err)
	assert.Equal(t, []byte("%PDF-preview"), pdf)
	require.NotNil(t, renderer.request)
	assert.Equal(t, renderWorkspaceID, renderer.request.WorkspaceID)
}

func TestReadOnlyViewService_GetPrintPDF_RendersForTheDocumentsWorkspace(t *testing.T) {
	for name, blank := range map[string]bool{"filled": false, "blank": true} {
		t.Run(name, func(t *testing.T) {
			renderer := &readOnlyViewPDFRendererFake{result: &port.RenderPreviewResult{PDF: []byte("%PDF-print")}}
			service := newPrintPDFService(readOnlyViewPDFServiceDeps{
				doc:         renderWorkspaceDocument(entity.DocumentStatusAwaitingInput),
				pdfRenderer: renderer,
			}, nil)

			pdf, _, err := service.GetPrintPDF(context.Background(), renderWorkspaceID, "doc-123", blank)

			require.NoError(t, err)
			assert.Equal(t, []byte("%PDF-print"), pdf)
			require.NotNil(t, renderer.request)
			assert.Equal(t, renderWorkspaceID, renderer.request.WorkspaceID)
		})
	}
}
