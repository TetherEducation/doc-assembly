package document

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

const uploadedPDFBytes = "immutable uploaded PDF bytes"

// previewSourceRepo behaves as an uploaded version unless authored or err is set.
type previewSourceRepo struct {
	port.UploadedSourceRepository
	authored bool
	err      error
}

func (r previewSourceRepo) FindByVersionID(_ context.Context, versionID string) (*entity.UploadedSource, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.authored {
		return nil, entity.ErrUploadedSourceNotFound
	}
	return &entity.UploadedSource{ObjectKey: "source/" + versionID + ".pdf"}, nil
}

type previewStorage struct {
	port.StorageAdapter
	requests []*port.StorageRequest
	err      error
}

func (s *previewStorage) Download(_ context.Context, req *port.StorageRequest) ([]byte, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return nil, s.err
	}
	return []byte(uploadedPDFBytes), nil
}

func newUploadedPreSigningService(tokenType string, sources port.UploadedSourceRepository, storage port.StorageAdapter) *PreSigningService {
	return &PreSigningService{
		accessTokenRepo: &viewOnlyTokenAccessTokenRepoFake{token: &entity.DocumentAccessToken{
			DocumentID: "doc-1",
			TokenType:  tokenType,
			ExpiresAt:  time.Now().Add(time.Hour),
		}},
		documentRepo:    &viewOnlyTokenPreSigningDocumentRepoFake{},
		versionRepo:     &viewOnlyTokenVersionRepoFake{},
		pdfRenderer:     &viewOnlyTokenPDFRendererFake{},
		uploadedSources: sources,
		storageAdapter:  storage,
	}
}

func TestUploadedSignerPreviewUsesSourceAfterTokenValidation(t *testing.T) {
	for _, tokenType := range []string{entity.TokenTypeSigning, entity.TokenTypePreSigning, entity.TokenTypeViewOnly} {
		t.Run(tokenType, func(t *testing.T) {
			storage := &previewStorage{}
			service := newUploadedPreSigningService(tokenType, previewSourceRepo{}, storage)

			pdf, err := service.RenderPreviewPDF(context.Background(), "token")

			if tokenType == entity.TokenTypeViewOnly {
				require.ErrorIs(t, err, entity.ErrInvalidToken)
				require.Empty(t, storage.requests, "an invalid token must not reach storage")
				return
			}
			require.NoError(t, err)
			require.Equal(t, []byte(uploadedPDFBytes), pdf)
			require.Len(t, storage.requests, 1)
			require.Equal(t, "source/version-1.pdf", storage.requests[0].Key)
			require.Equal(t, entity.EnvironmentProd, storage.requests[0].Environment)
		})
	}
}

func TestAuthoredSignerPreviewStillRendersFromTemplate(t *testing.T) {
	storage := &previewStorage{}
	service := newUploadedPreSigningService(entity.TokenTypeSigning, previewSourceRepo{authored: true}, storage)

	_, err := service.RenderPreviewPDF(context.Background(), "token")

	// The version fake fails on purpose: reaching it proves the authored path
	// fell through to the Typst render instead of stopping at the upload check.
	require.ErrorContains(t, err, "finding template version")
	require.Empty(t, storage.requests)
}

func TestUploadedReadOnlyViewShowsPDFBeforeSigning(t *testing.T) {
	for _, status := range []entity.DocumentStatus{entity.DocumentStatusDraft, entity.DocumentStatusAwaitingInput} {
		t.Run(string(status), func(t *testing.T) {
			service := newReadOnlyViewGetService(status, time.Now().Add(time.Hour), "Contract").
				SetUploadedSourceRepository(previewSourceRepo{})
			service.storageAdapter = &previewStorage{}

			view, err := service.GetReadOnlyView(context.Background(), "view-token")
			require.NoError(t, err)
			require.Equal(t, documentuc.ReadOnlyViewModePDF, view.Mode)
			require.NotNil(t, view.PDFURL)
			require.Nil(t, view.Content)

			pdf, filename, err := service.GetReadOnlyViewPDF(context.Background(), "view-token")
			require.NoError(t, err)
			require.Equal(t, []byte(uploadedPDFBytes), pdf)
			require.NotEmpty(t, filename)
		})
	}
}

func TestAuthoredReadOnlyViewKeepsContentBeforeSigning(t *testing.T) {
	storage := &previewStorage{}
	service := newReadOnlyViewGetService(entity.DocumentStatusAwaitingInput, time.Now().Add(time.Hour), "Contract").
		SetUploadedSourceRepository(previewSourceRepo{authored: true})
	service.storageAdapter = storage

	view, err := service.GetReadOnlyView(context.Background(), "view-token")
	require.NoError(t, err)
	require.Equal(t, documentuc.ReadOnlyViewModeContent, view.Mode)
	require.NotEmpty(t, view.Content)

	_, _, err = service.GetReadOnlyViewPDF(context.Background(), "view-token")
	require.ErrorContains(t, err, "PDF is not available")
	require.Empty(t, storage.requests)
}

func TestUploadedReadOnlyPreviewDoesNotRequireTypst(t *testing.T) {
	// No renderer or version repository is configured: only the upload path can succeed.
	service := (&ReadOnlyViewService{storageAdapter: &previewStorage{}}).SetUploadedSourceRepository(previewSourceRepo{})

	pdf, _, err := service.renderReadOnlyViewPreviewPDF(context.Background(), &entity.Document{TemplateVersionID: "version-1"})
	require.NoError(t, err)
	require.Equal(t, []byte(uploadedPDFBytes), pdf)
}

func TestUploadedBlankPrintIsTheSourcePDF(t *testing.T) {
	service := (&ReadOnlyViewService{storageAdapter: &previewStorage{}}).SetUploadedSourceRepository(previewSourceRepo{})

	pdf, filename, err := service.renderBlankPrintPDF(context.Background(), &entity.Document{ID: "doc-1", TemplateVersionID: "version-1"})
	require.NoError(t, err)
	require.Equal(t, []byte(uploadedPDFBytes), pdf)
	require.Equal(t, "document-doc-1-blank.pdf", filename)
}

func TestUploadedPreviewFailsClosedWhenSourceUnavailable(t *testing.T) {
	doc := &entity.Document{TemplateVersionID: "version-1"}

	t.Run("storage error", func(t *testing.T) {
		service := (&ReadOnlyViewService{storageAdapter: &previewStorage{err: errors.New("storage unavailable")}}).
			SetUploadedSourceRepository(previewSourceRepo{})
		pdf, filename, err := service.renderReadOnlyViewPreviewPDF(context.Background(), doc)
		require.ErrorContains(t, err, "storage unavailable")
		require.Empty(t, pdf)
		require.Empty(t, filename)
	})

	t.Run("repository error", func(t *testing.T) {
		storage := &previewStorage{}
		service := (&ReadOnlyViewService{storageAdapter: storage}).
			SetUploadedSourceRepository(previewSourceRepo{err: errors.New("repository unavailable")})
		_, _, err := service.renderReadOnlyViewPreviewPDF(context.Background(), doc)
		require.ErrorContains(t, err, "repository unavailable")
		require.Empty(t, storage.requests)
	})

	t.Run("storage not configured", func(t *testing.T) {
		service := (&ReadOnlyViewService{}).SetUploadedSourceRepository(previewSourceRepo{})
		_, _, err := service.renderReadOnlyViewPreviewPDF(context.Background(), doc)
		require.ErrorContains(t, err, "storage is not configured")
	})
}
