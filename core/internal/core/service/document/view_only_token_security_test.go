package document

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

func TestPreSigningService_RejectsViewOnlyToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	accessTokenRepo := &viewOnlyTokenAccessTokenRepoFake{
		token: &entity.DocumentAccessToken{
			ID:          "access-token-1",
			DocumentID:  "doc-1",
			RecipientID: "recipient-1",
			Token:       "view-only-token",
			TokenType:   entity.TokenTypeViewOnly,
			ExpiresAt:   time.Now().UTC().Add(time.Hour),
			CreatedAt:   time.Now().UTC(),
		},
	}
	signingUOW := &viewOnlyTokenSigningUOWFake{}
	service := &PreSigningService{
		accessTokenRepo: accessTokenRepo,
		documentRepo:    &viewOnlyTokenPreSigningDocumentRepoFake{},
		recipientRepo:   &viewOnlyTokenRecipientRepoFake{},
		versionRepo:     &viewOnlyTokenVersionRepoFake{},
		signerRoleRepo:  &viewOnlyTokenSignerRoleRepoFake{},
		pdfRenderer:     &viewOnlyTokenPDFRendererFake{},
		signingUOW:      signingUOW,
	}

	t.Run("GetPublicSigningPage", func(t *testing.T) {
		resp, err := service.GetPublicSigningPage(ctx, "view-only-token")

		require.ErrorIs(t, err, entity.ErrInvalidToken)
		assert.Nil(t, resp)
	})

	t.Run("ProceedToSigning", func(t *testing.T) {
		resp, err := service.ProceedToSigning(ctx, "view-only-token")

		require.ErrorIs(t, err, entity.ErrInvalidToken)
		assert.Nil(t, resp)
		assert.False(t, signingUOW.createAttemptCalled, "VIEW_ONLY token must not bind or create signing attempts")
	})

	t.Run("RenderPreviewPDF", func(t *testing.T) {
		pdf, err := service.RenderPreviewPDF(ctx, "view-only-token")

		require.ErrorIs(t, err, entity.ErrInvalidToken)
		assert.Nil(t, pdf)
	})
}

func TestDocumentAccessService_RequestAccessByToken_RejectsViewOnlyToken(t *testing.T) {
	t.Parallel()

	documentRepo := &viewOnlyTokenDocumentRepoFake{}
	service := &DocumentAccessService{
		documentRepo: documentRepo,
		accessTokenRepo: &viewOnlyTokenAccessTokenRepoFake{
			token: &entity.DocumentAccessToken{
				ID:          "access-token-1",
				DocumentID:  "doc-1",
				RecipientID: "recipient-1",
				Token:       "view-only-token",
				TokenType:   entity.TokenTypeViewOnly,
				ExpiresAt:   time.Now().UTC().Add(time.Hour),
				CreatedAt:   time.Now().UTC(),
			},
		},
	}

	err := service.RequestAccessByToken(context.Background(), "view-only-token", "signer@example.test", "en")

	require.NoError(t, err)
	assert.False(t, documentRepo.findByIDCalled, "VIEW_ONLY token must not be usable for signer access recovery")
}

type viewOnlyTokenAccessTokenRepoFake struct {
	port.DocumentAccessTokenRepository
	token *entity.DocumentAccessToken
}

func (f *viewOnlyTokenAccessTokenRepoFake) FindByToken(_ context.Context, _ string) (*entity.DocumentAccessToken, error) {
	if f.token == nil {
		return nil, entity.ErrInvalidToken
	}
	return f.token, nil
}

type viewOnlyTokenSigningUOWFake struct {
	port.SigningExecutionUnitOfWork
	createAttemptCalled bool
}

func (f *viewOnlyTokenSigningUOWFake) CreateAttemptAndEnqueueRender(
	context.Context,
	string,
	[]*entity.DocumentRecipient,
	map[string]int,
) (*entity.SigningAttempt, error) {
	f.createAttemptCalled = true
	return nil, errors.New("unexpected signing attempt creation")
}

type viewOnlyTokenDocumentRepoFake struct {
	port.DocumentRepository
	findByIDCalled bool
}

func (f *viewOnlyTokenDocumentRepoFake) FindByID(context.Context, string) (*entity.Document, error) {
	f.findByIDCalled = true
	return nil, errors.New("unexpected document lookup")
}

type viewOnlyTokenPreSigningDocumentRepoFake struct {
	port.DocumentRepository
}

func (f *viewOnlyTokenPreSigningDocumentRepoFake) FindByID(context.Context, string) (*entity.Document, error) {
	return &entity.Document{
		ID:                "doc-1",
		TemplateVersionID: "version-1",
		Status:            entity.DocumentStatusAwaitingInput,
	}, nil
}

type viewOnlyTokenRecipientRepoFake struct {
	port.DocumentRecipientRepository
}

func (f *viewOnlyTokenRecipientRepoFake) FindByID(context.Context, string) (*entity.DocumentRecipient, error) {
	return &entity.DocumentRecipient{
		ID:                    "recipient-1",
		DocumentID:            "doc-1",
		TemplateVersionRoleID: "role-1",
		Name:                  "Signer",
		Email:                 "signer@example.test",
		Status:                entity.RecipientStatusPending,
	}, nil
}

func (f *viewOnlyTokenRecipientRepoFake) FindByDocumentID(context.Context, string) ([]*entity.DocumentRecipient, error) {
	return []*entity.DocumentRecipient{{
		ID:                    "recipient-1",
		DocumentID:            "doc-1",
		TemplateVersionRoleID: "role-1",
		Name:                  "Signer",
		Email:                 "signer@example.test",
		Status:                entity.RecipientStatusPending,
	}}, nil
}

type viewOnlyTokenVersionRepoFake struct {
	port.TemplateVersionRepository
}

func (f *viewOnlyTokenVersionRepoFake) FindByID(context.Context, string) (*entity.TemplateVersion, error) {
	return nil, errors.New("unexpected template version lookup")
}

type viewOnlyTokenSignerRoleRepoFake struct {
	port.TemplateVersionSignerRoleRepository
}

func (f *viewOnlyTokenSignerRoleRepoFake) FindByVersionID(context.Context, string) ([]*entity.TemplateVersionSignerRole, error) {
	return []*entity.TemplateVersionSignerRole{{ID: "role-1", SignerOrder: 1}}, nil
}

type viewOnlyTokenPDFRendererFake struct {
	port.PDFRenderer
}

func (f *viewOnlyTokenPDFRendererFake) RenderPreview(context.Context, *port.RenderPreviewRequest) (*port.RenderPreviewResult, error) {
	return nil, errors.New("unexpected PDF preview render")
}
