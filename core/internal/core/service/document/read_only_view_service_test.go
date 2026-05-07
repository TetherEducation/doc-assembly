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

func TestReadOnlyViewService_CreateReadOnlyViewLink_DocumentNotFound(t *testing.T) {
	documentRepo := &readOnlyViewDocumentRepoFake{err: entity.ErrDocumentNotFound}
	service := NewReadOnlyViewService(documentRepo, &readOnlyViewAccessTokenRepoFake{}, 48, "")

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
			service := NewReadOnlyViewService(&readOnlyViewDocumentRepoFake{doc: tt.doc}, accessTokenRepo, 48, "")

			result, err := service.CreateReadOnlyViewLink(context.Background(), tt.doc.ID)

			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, entity.ErrInvalidDocumentState)
			assert.Nil(t, accessTokenRepo.created)
		})
	}
}

func TestReadOnlyViewService_CreateReadOnlyViewLink_TokenCreateFailure(t *testing.T) {
	createErr := errors.New("insert token failed")
	accessTokenRepo := &readOnlyViewAccessTokenRepoFake{err: createErr}
	service := NewReadOnlyViewService(
		&readOnlyViewDocumentRepoFake{doc: &entity.Document{ID: "doc-123", Status: entity.DocumentStatusReadyToSign}},
		accessTokenRepo,
		48,
		"",
	)

	result, err := service.CreateReadOnlyViewLink(context.Background(), "doc-123")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, createErr)
	assert.ErrorContains(t, err, "create read-only view token")
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
}

func (f *readOnlyViewAccessTokenRepoFake) Create(_ context.Context, token *entity.DocumentAccessToken) error {
	if f.err != nil {
		return f.err
	}
	f.created = token
	return nil
}
