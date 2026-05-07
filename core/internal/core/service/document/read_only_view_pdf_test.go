package document

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

func TestReadOnlyViewService_GetReadOnlyViewPDF_RejectsContentMode(t *testing.T) {
	service := newReadOnlyViewPDFService(readOnlyViewPDFServiceDeps{
		doc: &entity.Document{
			ID:                "doc-content",
			TemplateVersionID: "version-123",
			Status:            entity.DocumentStatusDraft,
		},
	})

	pdf, filename, err := service.GetReadOnlyViewPDF(context.Background(), "view-token")

	require.Error(t, err)
	assert.Nil(t, pdf)
	assert.Empty(t, filename)
	assert.ErrorContains(t, err, "PDF is not available")
}

func TestReadOnlyViewService_GetReadOnlyViewPDF_ReturnsCompletedPDFFromStorage(t *testing.T) {
	completedPDFURL := "completed/doc-123.pdf"
	title := "Signed Contract"
	storage := &readOnlyViewStorageFake{data: []byte("%PDF-completed")}
	service := newReadOnlyViewPDFService(readOnlyViewPDFServiceDeps{
		doc: &entity.Document{
			ID:              "doc-123",
			Title:           &title,
			Status:          entity.DocumentStatusCompleted,
			CompletedPDFURL: &completedPDFURL,
		},
		storageAdapter: storage,
		storageEnabled: true,
	})

	pdf, filename, err := service.GetReadOnlyViewPDF(context.Background(), "view-token")

	require.NoError(t, err)
	assert.Equal(t, []byte("%PDF-completed"), pdf)
	assert.Equal(t, "Signed Contract-signed.pdf", filename)
	assert.Equal(t, completedPDFURL, storage.downloadKey)
}

func TestReadOnlyViewService_GetReadOnlyViewPDF_RendersPreviewPDFForSigningState(t *testing.T) {
	renderer := &readOnlyViewPDFRendererFake{
		result: &port.RenderPreviewResult{
			PDF:      []byte("%PDF-preview"),
			Filename: "preview.pdf",
		},
	}
	fieldResponse := json.RawMessage(`{"text":"accepted"}`)
	service := newReadOnlyViewPDFService(readOnlyViewPDFServiceDeps{
		doc: &entity.Document{
			ID:                     "doc-123",
			TemplateVersionID:      "version-123",
			Status:                 entity.DocumentStatusSigning,
			InjectedValuesSnapshot: json.RawMessage(`{"customerName":"Jane Doe"}`),
		},
		version: &entity.TemplateVersion{
			ID:               "version-123",
			ContentStructure: mustReadOnlyViewPDFPortableDocContent(),
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
		fieldResponses: []entity.DocumentFieldResponse{{
			DocumentID: "doc-123",
			FieldID:    "field-1",
			Response:   fieldResponse,
		}},
		pdfRenderer: renderer,
	})

	pdf, filename, err := service.GetReadOnlyViewPDF(context.Background(), "view-token")

	require.NoError(t, err)
	assert.Equal(t, []byte("%PDF-preview"), pdf)
	assert.Equal(t, "preview.pdf", filename)
	require.NotNil(t, renderer.request)
	assert.Equal(t, "Jane Doe", renderer.request.Injectables["customerName"])
	assert.Equal(t, fieldResponse, renderer.request.FieldResponses["field-1"])
	assert.Equal(t, port.SignerRoleValue{Name: "Jane Doe", Email: "jane@example.test"}, renderer.request.SignerRoleValues["portable-signer"])
}

func TestReadOnlyViewService_GetReadOnlyViewPDF_RejectsWrongTokenType(t *testing.T) {
	service := newReadOnlyViewPDFService(readOnlyViewPDFServiceDeps{
		tokenType: entity.TokenTypeSigning,
		doc: &entity.Document{
			ID:     "doc-123",
			Status: entity.DocumentStatusCompleted,
		},
	})

	pdf, filename, err := service.GetReadOnlyViewPDF(context.Background(), "view-token")

	require.Error(t, err)
	assert.Nil(t, pdf)
	assert.Empty(t, filename)
	assert.ErrorIs(t, err, entity.ErrInvalidToken)
}

func TestReadOnlyViewService_GetReadOnlyViewPDF_StorageDisabledForCompleted(t *testing.T) {
	completedPDFURL := "completed/doc-123.pdf"
	service := newReadOnlyViewPDFService(readOnlyViewPDFServiceDeps{
		doc: &entity.Document{
			ID:              "doc-123",
			Status:          entity.DocumentStatusCompleted,
			CompletedPDFURL: &completedPDFURL,
		},
		storageAdapter: &readOnlyViewStorageFake{data: []byte("%PDF-completed")},
		storageEnabled: false,
	})

	pdf, filename, err := service.GetReadOnlyViewPDF(context.Background(), "view-token")

	require.Error(t, err)
	assert.Nil(t, pdf)
	assert.Empty(t, filename)
	assert.ErrorContains(t, err, "signed PDF storage is disabled")
}

type readOnlyViewPDFServiceDeps struct {
	tokenType      string
	doc            *entity.Document
	version        *entity.TemplateVersion
	recipients     []*entity.DocumentRecipient
	signerRoles    []*entity.TemplateVersionSignerRole
	fieldResponses []entity.DocumentFieldResponse
	pdfRenderer    port.PDFRenderer
	storageAdapter port.StorageAdapter
	storageEnabled bool
}

func newReadOnlyViewPDFService(deps readOnlyViewPDFServiceDeps) *ReadOnlyViewService {
	tokenType := deps.tokenType
	if tokenType == "" {
		tokenType = entity.TokenTypeViewOnly
	}
	docID := "doc-123"
	if deps.doc != nil && deps.doc.ID != "" {
		docID = deps.doc.ID
	}
	if deps.version == nil {
		deps.version = &entity.TemplateVersion{
			ID:               "version-123",
			ContentStructure: mustReadOnlyViewPDFPortableDocContent(),
		}
	}
	if deps.pdfRenderer == nil {
		deps.pdfRenderer = &readOnlyViewPDFRendererFake{result: &port.RenderPreviewResult{PDF: []byte("%PDF")}}
	}

	return NewReadOnlyViewService(
		&readOnlyViewDocumentRepoFake{doc: deps.doc},
		&readOnlyViewAccessTokenRepoFake{found: readOnlyViewToken(docID, "view-token", time.Now().UTC().Add(time.Hour), tokenType)},
		&readOnlyViewRecipientRepoFake{recipients: deps.recipients},
		&readOnlyViewVersionRepoFake{version: deps.version},
		&readOnlyViewSignerRoleRepoFake{roles: deps.signerRoles},
		&readOnlyViewFieldResponseRepoFake{responses: deps.fieldResponses},
		deps.pdfRenderer,
		deps.storageAdapter,
		deps.storageEnabled,
		48,
		"",
	)
}

func mustReadOnlyViewPDFPortableDocContent() json.RawMessage {
	text := "Sign here"
	data, err := json.Marshal(portabledoc.Document{
		Version: portabledoc.CurrentVersion,
		Content: &portabledoc.ProseMirrorDoc{
			Type: portabledoc.NodeTypeDoc,
			Content: []portabledoc.Node{{
				Type: portabledoc.NodeTypeParagraph,
				Content: []portabledoc.Node{{
					Type: portabledoc.NodeTypeText,
					Text: &text,
				}},
			}},
		},
		SignerRoles: []portabledoc.SignerRole{{
			ID:    "portable-signer",
			Label: "Signer",
		}},
	})
	if err != nil {
		panic(err)
	}
	return data
}

type readOnlyViewSignerRoleRepoFake struct {
	port.TemplateVersionSignerRoleRepository
	roles           []*entity.TemplateVersionSignerRole
	err             error
	findByVersionID string
}

func (f *readOnlyViewSignerRoleRepoFake) FindByVersionID(_ context.Context, versionID string) ([]*entity.TemplateVersionSignerRole, error) {
	f.findByVersionID = versionID
	return f.roles, f.err
}

type readOnlyViewFieldResponseRepoFake struct {
	port.DocumentFieldResponseRepository
	responses        []entity.DocumentFieldResponse
	err              error
	findByDocumentID string
}

func (f *readOnlyViewFieldResponseRepoFake) FindByDocumentID(_ context.Context, documentID string) ([]entity.DocumentFieldResponse, error) {
	f.findByDocumentID = documentID
	return f.responses, f.err
}

type readOnlyViewPDFRendererFake struct {
	port.PDFRenderer
	result  *port.RenderPreviewResult
	err     error
	request *port.RenderPreviewRequest
}

func (f *readOnlyViewPDFRendererFake) RenderPreview(_ context.Context, req *port.RenderPreviewRequest) (*port.RenderPreviewResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.request = req
	return f.result, nil
}

type readOnlyViewStorageFake struct {
	port.StorageAdapter
	data        []byte
	err         error
	downloadKey string
}

func (f *readOnlyViewStorageFake) Download(_ context.Context, req *port.StorageRequest) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if req == nil {
		return nil, errors.New("missing storage request")
	}
	f.downloadKey = req.Key
	return f.data, nil
}
