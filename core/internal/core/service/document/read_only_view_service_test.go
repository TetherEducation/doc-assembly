package document

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
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
	service := NewReadOnlyViewService(documentRepo, accessTokenRepo, 48, "https://public.example.test/")

	before := time.Now().UTC()
	result, err := service.CreateReadOnlyViewLink(ctx, docID)
	after := time.Now().UTC()

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, accessTokenRepo.created)
	assert.Equal(t, docID, documentRepo.findByID)
	assert.Equal(t, docID, accessTokenRepo.created.DocumentID)
	assert.Empty(t, accessTokenRepo.created.RecipientID)
	assert.Nil(t, accessTokenRepo.created.AttemptID)
	assert.Equal(t, entity.TokenTypeViewOnly, accessTokenRepo.created.TokenType)
	assert.NotEmpty(t, accessTokenRepo.created.Token)
	assert.Equal(t, accessTokenRepo.created.Token, result.Token)
	assert.Equal(t, "https://public.example.test/public/view/"+result.Token, result.URL)
	assert.Equal(t, accessTokenRepo.created.ExpiresAt, result.ExpiresAt)
	assert.WithinDuration(t, before.Add(48*time.Hour), result.ExpiresAt, after.Sub(before)+time.Second)
	assert.WithinDuration(t, before, accessTokenRepo.created.CreatedAt, after.Sub(before)+time.Second)
}

type readOnlyViewDocumentRepoFake struct {
	doc      *entity.Document
	findByID string
}

func (f *readOnlyViewDocumentRepoFake) Create(context.Context, *entity.Document) (string, error) {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) FindByID(_ context.Context, id string) (*entity.Document, error) {
	f.findByID = id
	return f.doc, nil
}

func (f *readOnlyViewDocumentRepoFake) FindByIDWithRecipients(context.Context, string) (*entity.DocumentWithRecipients, error) {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) FindByWorkspace(context.Context, string, port.DocumentFilters) ([]*entity.DocumentListItem, error) {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) FindByClientExternalRef(context.Context, string, string) ([]*entity.Document, error) {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) FindByTemplateVersion(context.Context, string) ([]*entity.DocumentListItem, error) {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) FindExpired(context.Context, int) ([]*entity.Document, error) {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) Update(context.Context, *entity.Document) error {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) UpdateStatus(context.Context, string, entity.DocumentStatus) error {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) Delete(context.Context, string) error {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) CountByWorkspace(context.Context, string) (int, error) {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) CountByStatus(context.Context, string, entity.DocumentStatus) (int, error) {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) FindInternalCreateReplay(context.Context, string, string, string, string) (string, bool, error) {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) FindActiveByLogicalKey(context.Context, string, string, string) (string, bool, error) {
	panic("not implemented")
}

func (f *readOnlyViewDocumentRepoFake) InternalCreateOrReplay(context.Context, *port.InternalCreateRequest) (*port.InternalCreateResult, error) {
	panic("not implemented")
}

type readOnlyViewAccessTokenRepoFake struct {
	created *entity.DocumentAccessToken
}

func (f *readOnlyViewAccessTokenRepoFake) Create(_ context.Context, token *entity.DocumentAccessToken) error {
	f.created = token
	return nil
}

func (f *readOnlyViewAccessTokenRepoFake) FindByToken(context.Context, string) (*entity.DocumentAccessToken, error) {
	panic("not implemented")
}

func (f *readOnlyViewAccessTokenRepoFake) FindActiveByRecipientAndType(context.Context, string, string) (*entity.DocumentAccessToken, error) {
	panic("not implemented")
}

func (f *readOnlyViewAccessTokenRepoFake) FindActiveByDocumentAndRecipientAndType(context.Context, string, string, string) (*entity.DocumentAccessToken, error) {
	panic("not implemented")
}

func (f *readOnlyViewAccessTokenRepoFake) MarkAsUsed(context.Context, string) error {
	panic("not implemented")
}

func (f *readOnlyViewAccessTokenRepoFake) InvalidateByDocumentID(context.Context, string) error {
	panic("not implemented")
}

func (f *readOnlyViewAccessTokenRepoFake) CountRecentByDocumentAndRecipient(context.Context, string, string, time.Time) (int, error) {
	panic("not implemented")
}
