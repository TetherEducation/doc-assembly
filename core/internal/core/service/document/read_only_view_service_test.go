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
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

func TestReadOnlyViewService_CreateReadOnlyViewLink_CreatesFreshViewOnlyToken(t *testing.T) {
	ctx := context.Background()
	docID := "doc-123"
	documentRepo := &readOnlyViewDocumentRepoFake{
		doc: &entity.Document{
			ID:     docID,
			Status: entity.DocumentStatusReadyToSign,
		},
	}
	accessTokenRepo := &readOnlyViewAccessTokenRepoFake{}
	recipientRepo := &readOnlyViewRecipientRepoFake{
		recipients: []*entity.DocumentRecipient{{ID: "recipient-1", DocumentID: docID}},
	}
	service := NewReadOnlyViewService(documentRepo, accessTokenRepo, recipientRepo, &readOnlyViewVersionRepoFake{}, 48, "https://public.example.test/")

	before := time.Now().UTC()
	result, err := service.CreateReadOnlyViewLink(ctx, docID)
	after := time.Now().UTC()

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, accessTokenRepo.created)
	assert.Equal(t, docID, documentRepo.findByID)
	assert.Equal(t, docID, accessTokenRepo.created.DocumentID)
	assert.Equal(t, docID, recipientRepo.findByDocumentID)
	assert.Equal(t, "recipient-1", accessTokenRepo.created.RecipientID)
	assert.Nil(t, accessTokenRepo.created.AttemptID)
	assert.Equal(t, entity.TokenTypeViewOnly, accessTokenRepo.created.TokenType)
	assert.NotEmpty(t, accessTokenRepo.created.Token)
	assert.Equal(t, accessTokenRepo.created.Token, result.Token)
	assert.Equal(t, "https://public.example.test/public/view/"+result.Token, result.URL)
	assert.Equal(t, accessTokenRepo.created.ExpiresAt, result.ExpiresAt)
	assert.WithinDuration(t, before.Add(48*time.Hour), result.ExpiresAt, after.Sub(before)+time.Second)
	assert.WithinDuration(t, before, accessTokenRepo.created.CreatedAt, after.Sub(before)+time.Second)
}

func TestReadOnlyViewService_CreateReadOnlyViewLink_DocumentNotFound(t *testing.T) {
	documentRepo := &readOnlyViewDocumentRepoFake{err: entity.ErrDocumentNotFound}
	service := NewReadOnlyViewService(documentRepo, &readOnlyViewAccessTokenRepoFake{}, &readOnlyViewRecipientRepoFake{}, &readOnlyViewVersionRepoFake{}, 48, "")

	result, err := service.CreateReadOnlyViewLink(context.Background(), "missing-doc")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, entity.ErrDocumentNotFound)
	assert.ErrorContains(t, err, "find document")
}

func TestReadOnlyViewService_CreateReadOnlyViewLink_InvalidStateDoesNotPersistToken(t *testing.T) {
	expiredAt := time.Now().UTC().Add(-time.Hour)
	tests := []struct {
		name string
		doc  *entity.Document
	}{
		{
			name: "cancelled",
			doc: &entity.Document{
				ID:     "doc-cancelled",
				Status: entity.DocumentStatusCancelled,
			},
		},
		{
			name: "invalidated",
			doc: &entity.Document{
				ID:     "doc-invalidated",
				Status: entity.DocumentStatusInvalidated,
			},
		},
		{
			name: "expired",
			doc: &entity.Document{
				ID:        "doc-expired",
				Status:    entity.DocumentStatusReadyToSign,
				ExpiresAt: &expiredAt,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accessTokenRepo := &readOnlyViewAccessTokenRepoFake{}
			recipientRepo := &readOnlyViewRecipientRepoFake{recipients: []*entity.DocumentRecipient{{ID: "recipient-1", DocumentID: tt.doc.ID}}}
			service := NewReadOnlyViewService(&readOnlyViewDocumentRepoFake{doc: tt.doc}, accessTokenRepo, recipientRepo, &readOnlyViewVersionRepoFake{}, 48, "")

			result, err := service.CreateReadOnlyViewLink(context.Background(), tt.doc.ID)

			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, entity.ErrInvalidDocumentState)
			assert.Nil(t, accessTokenRepo.created)
			assert.Empty(t, recipientRepo.findByDocumentID)
		})
	}
}

func TestReadOnlyViewService_CreateReadOnlyViewLink_NoRecipientsDoesNotPersistToken(t *testing.T) {
	accessTokenRepo := &readOnlyViewAccessTokenRepoFake{}
	service := NewReadOnlyViewService(
		&readOnlyViewDocumentRepoFake{doc: &entity.Document{ID: "doc-without-recipients", Status: entity.DocumentStatusReadyToSign}},
		accessTokenRepo,
		&readOnlyViewRecipientRepoFake{},
		&readOnlyViewVersionRepoFake{},
		48,
		"",
	)

	result, err := service.CreateReadOnlyViewLink(context.Background(), "doc-without-recipients")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "find read-only view anchor recipient")
	assert.Nil(t, accessTokenRepo.created)
}

func TestReadOnlyViewService_CreateReadOnlyViewLink_RecipientLookupFailureDoesNotPersistToken(t *testing.T) {
	lookupErr := errors.New("recipient lookup failed")
	accessTokenRepo := &readOnlyViewAccessTokenRepoFake{}
	service := NewReadOnlyViewService(
		&readOnlyViewDocumentRepoFake{doc: &entity.Document{ID: "doc-123", Status: entity.DocumentStatusReadyToSign}},
		accessTokenRepo,
		&readOnlyViewRecipientRepoFake{err: lookupErr},
		&readOnlyViewVersionRepoFake{},
		48,
		"",
	)

	result, err := service.CreateReadOnlyViewLink(context.Background(), "doc-123")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, lookupErr)
	assert.ErrorContains(t, err, "find read-only view anchor recipient")
	assert.Nil(t, accessTokenRepo.created)
}

func TestReadOnlyViewService_CreateReadOnlyViewLink_TokenCreateFailure(t *testing.T) {
	createErr := errors.New("insert token failed")
	accessTokenRepo := &readOnlyViewAccessTokenRepoFake{err: createErr}
	service := NewReadOnlyViewService(
		&readOnlyViewDocumentRepoFake{doc: &entity.Document{ID: "doc-123", Status: entity.DocumentStatusReadyToSign}},
		accessTokenRepo,
		&readOnlyViewRecipientRepoFake{recipients: []*entity.DocumentRecipient{{ID: "recipient-1", DocumentID: "doc-123"}}},
		&readOnlyViewVersionRepoFake{},
		48,
		"",
	)

	result, err := service.CreateReadOnlyViewLink(context.Background(), "doc-123")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, createErr)
	assert.ErrorContains(t, err, "create read-only view token")
}

func TestReadOnlyViewService_GetReadOnlyView_SelectsModeByDocumentStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     entity.DocumentStatus
		wantMode   documentuc.ReadOnlyViewMode
		wantReason *string
	}{
		{name: "draft", status: entity.DocumentStatusDraft, wantMode: documentuc.ReadOnlyViewModeContent},
		{name: "awaiting input", status: entity.DocumentStatusAwaitingInput, wantMode: documentuc.ReadOnlyViewModeContent},
		{name: "preparing signature", status: entity.DocumentStatusPreparingSignature, wantMode: documentuc.ReadOnlyViewModePDF},
		{name: "ready to sign", status: entity.DocumentStatusReadyToSign, wantMode: documentuc.ReadOnlyViewModePDF},
		{name: "signing", status: entity.DocumentStatusSigning, wantMode: documentuc.ReadOnlyViewModePDF},
		{name: "completed", status: entity.DocumentStatusCompleted, wantMode: documentuc.ReadOnlyViewModePDF},
		{name: "cancelled", status: entity.DocumentStatusCancelled, wantMode: documentuc.ReadOnlyViewModeUnavailable, wantReason: ptrString("document_unavailable")},
		{name: "invalidated", status: entity.DocumentStatusInvalidated, wantMode: documentuc.ReadOnlyViewModeUnavailable, wantReason: ptrString("document_unavailable")},
		{name: "error", status: entity.DocumentStatusError, wantMode: documentuc.ReadOnlyViewModeUnavailable, wantReason: ptrString("document_unavailable")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiresAt := time.Now().UTC().Add(time.Hour)
			docTitle := "Contract draft"
			service := newReadOnlyViewGetService(tt.status, expiresAt, docTitle)

			result, err := service.GetReadOnlyView(context.Background(), "view-token")

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantMode, result.Mode)
			assert.Equal(t, "doc-123", result.DocumentID)
			assert.Equal(t, docTitle, result.DocumentTitle)
			assert.Equal(t, tt.status, result.DocumentStatus)
			assert.Equal(t, expiresAt, result.ExpiresAt)
			if tt.wantReason == nil {
				assert.Nil(t, result.Reason)
			} else {
				require.NotNil(t, result.Reason)
				assert.Equal(t, *tt.wantReason, *result.Reason)
			}
		})
	}
}

func TestReadOnlyViewService_GetReadOnlyView_RejectsInvalidMissingExpiredOrWrongTypeToken(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		stored    *entity.DocumentAccessToken
		repoErr   error
		wantError error
	}{
		{name: "missing token", token: "", wantError: entity.ErrMissingToken},
		{name: "token not found", token: "missing", repoErr: errors.New("not found"), wantError: entity.ErrInvalidToken},
		{name: "expired token", token: "expired", stored: readOnlyViewToken("doc-123", "expired", time.Now().UTC().Add(-time.Minute), entity.TokenTypeViewOnly), wantError: entity.ErrTokenExpired},
		{name: "signing token", token: "signing", stored: readOnlyViewToken("doc-123", "signing", time.Now().UTC().Add(time.Hour), entity.TokenTypeSigning), wantError: entity.ErrInvalidToken},
		{name: "pre signing token", token: "pre-signing", stored: readOnlyViewToken("doc-123", "pre-signing", time.Now().UTC().Add(time.Hour), entity.TokenTypePreSigning), wantError: entity.ErrInvalidToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewReadOnlyViewService(
				&readOnlyViewDocumentRepoFake{doc: &entity.Document{ID: "doc-123", Status: entity.DocumentStatusReadyToSign}},
				&readOnlyViewAccessTokenRepoFake{found: tt.stored, findErr: tt.repoErr},
				&readOnlyViewRecipientRepoFake{},
				&readOnlyViewVersionRepoFake{},
				48,
				"",
			)

			result, err := service.GetReadOnlyView(context.Background(), tt.token)

			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, tt.wantError)
		})
	}
}

func TestReadOnlyViewService_GetReadOnlyView_ContentModeReturnsContentWithoutPDFURL(t *testing.T) {
	service := newReadOnlyViewGetService(entity.DocumentStatusDraft, time.Now().UTC().Add(time.Hour), "Draft title")

	result, err := service.GetReadOnlyView(context.Background(), "view-token")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, documentuc.ReadOnlyViewModeContent, result.Mode)
	assert.NotEmpty(t, result.Content)
	assert.Nil(t, result.PDFURL)
}

func TestReadOnlyViewService_GetReadOnlyView_PDFModeReturnsPDFURLWithoutContent(t *testing.T) {
	service := newReadOnlyViewGetServiceWithToken(entity.DocumentStatusCompleted, time.Now().UTC().Add(time.Hour), "Signed title", "token")

	result, err := service.GetReadOnlyView(context.Background(), "token")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, documentuc.ReadOnlyViewModePDF, result.Mode)
	require.NotNil(t, result.PDFURL)
	assert.Equal(t, "/public/view/token/pdf", *result.PDFURL)
	assert.Empty(t, result.Content)
}

func TestReadOnlyViewService_GetReadOnlyView_UnavailableModeReturnsReason(t *testing.T) {
	service := newReadOnlyViewGetService(entity.DocumentStatusCancelled, time.Now().UTC().Add(time.Hour), "Cancelled title")

	result, err := service.GetReadOnlyView(context.Background(), "view-token")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, documentuc.ReadOnlyViewModeUnavailable, result.Mode)
	require.NotNil(t, result.Reason)
	assert.Equal(t, "document_unavailable", *result.Reason)
	assert.Empty(t, result.Content)
	assert.Nil(t, result.PDFURL)
}

func newReadOnlyViewGetService(status entity.DocumentStatus, expiresAt time.Time, title string) *ReadOnlyViewService {
	return newReadOnlyViewGetServiceWithToken(status, expiresAt, title, "view-token")
}

func newReadOnlyViewGetServiceWithToken(status entity.DocumentStatus, expiresAt time.Time, title string, token string) *ReadOnlyViewService {
	const docID = "doc-123"
	const versionID = "version-123"

	return NewReadOnlyViewService(
		&readOnlyViewDocumentRepoFake{doc: &entity.Document{
			ID:                docID,
			TemplateVersionID: versionID,
			Title:             &title,
			Status:            status,
		}},
		&readOnlyViewAccessTokenRepoFake{found: readOnlyViewToken(docID, token, expiresAt, entity.TokenTypeViewOnly)},
		&readOnlyViewRecipientRepoFake{},
		&readOnlyViewVersionRepoFake{version: &entity.TemplateVersion{
			ID:               versionID,
			ContentStructure: mustReadOnlyViewPortableDocContent(),
		}},
		48,
		"",
	)
}

func readOnlyViewToken(documentID, token string, expiresAt time.Time, tokenType string) *entity.DocumentAccessToken {
	return &entity.DocumentAccessToken{
		DocumentID:  documentID,
		RecipientID: "recipient-1",
		Token:       token,
		TokenType:   tokenType,
		ExpiresAt:   expiresAt,
	}
}

func mustReadOnlyViewPortableDocContent() json.RawMessage {
	text := "Read-only contract body"
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
	})
	if err != nil {
		panic(err)
	}
	return data
}

type readOnlyViewDocumentRepoFake struct {
	port.DocumentRepository
	doc      *entity.Document
	err      error
	findByID string
}

func (f *readOnlyViewDocumentRepoFake) FindByID(_ context.Context, id string) (*entity.Document, error) {
	f.findByID = id
	return f.doc, f.err
}

type readOnlyViewAccessTokenRepoFake struct {
	port.DocumentAccessTokenRepository
	created *entity.DocumentAccessToken
	err     error
	found   *entity.DocumentAccessToken
	findErr error
}

func (f *readOnlyViewAccessTokenRepoFake) Create(_ context.Context, token *entity.DocumentAccessToken) error {
	if f.err != nil {
		return f.err
	}
	f.created = token
	return nil
}

func (f *readOnlyViewAccessTokenRepoFake) FindByToken(_ context.Context, token string) (*entity.DocumentAccessToken, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	if f.found == nil || f.found.Token != token {
		return nil, entity.ErrInvalidToken
	}
	return f.found, nil
}

type readOnlyViewRecipientRepoFake struct {
	port.DocumentRecipientRepository
	recipients       []*entity.DocumentRecipient
	err              error
	findByDocumentID string
}

func (f *readOnlyViewRecipientRepoFake) FindByDocumentID(_ context.Context, documentID string) ([]*entity.DocumentRecipient, error) {
	f.findByDocumentID = documentID
	return f.recipients, f.err
}

type readOnlyViewVersionRepoFake struct {
	port.TemplateVersionRepository
	version  *entity.TemplateVersion
	err      error
	findByID string
}

func (f *readOnlyViewVersionRepoFake) FindByID(_ context.Context, id string) (*entity.TemplateVersion, error) {
	f.findByID = id
	return f.version, f.err
}
