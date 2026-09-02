package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

func TestEngine_CreateReadOnlyViewLinkByWorkspaceCode_BeforeInitialize(t *testing.T) {
	engine := New()

	result, err := engine.CreateReadOnlyViewLinkByWorkspaceCode(context.Background(), "CAMPUS_A", "doc-1")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrEngineNotInitialized)
}

func TestEngine_CreateReadOnlyViewLinkByWorkspaceCode_NilUseCaseAfterInitialize(t *testing.T) {
	engine := New()
	engine.initialized = true

	result, err := engine.CreateReadOnlyViewLinkByWorkspaceCode(context.Background(), "CAMPUS_A", "doc-1")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrReadOnlyViewNotWired)
}

func TestEngine_CreateReadOnlyViewLinkByWorkspaceCode_DelegatesToUseCase(t *testing.T) {
	expiresAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	engine := initializedEngineWithReadOnlyView(t, &readOnlyViewUseCaseStub{
		createByCode: func(_ context.Context, workspaceCode, documentID string) (*documentuc.CreateReadOnlyViewLinkResult, error) {
			assert.Equal(t, "CAMPUS_A", workspaceCode)
			assert.Equal(t, "doc-1", documentID)
			return &documentuc.CreateReadOnlyViewLinkResult{
				URL:       "https://public.example.test/public/view/view-token",
				Token:     "view-token",
				ExpiresAt: expiresAt,
			}, nil
		},
	})

	result, err := engine.CreateReadOnlyViewLinkByWorkspaceCode(context.Background(), "CAMPUS_A", "doc-1")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "https://public.example.test/public/view/view-token", result.URL)
	assert.Equal(t, "view-token", result.Token)
	assert.Equal(t, expiresAt, result.ExpiresAt)
}

func TestEngine_CreateReadOnlyViewLinkByWorkspaceCode_UseCaseError(t *testing.T) {
	want := errors.New("workspace mismatch")
	engine := initializedEngineWithReadOnlyView(t, &readOnlyViewUseCaseStub{
		createByCode: func(_ context.Context, workspaceCode, documentID string) (*documentuc.CreateReadOnlyViewLinkResult, error) {
			assert.Equal(t, "9999999999", workspaceCode)
			assert.Equal(t, "doc-1", documentID)
			return nil, want
		},
	})

	result, err := engine.CreateReadOnlyViewLinkByWorkspaceCode(context.Background(), "9999999999", "doc-1")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, want)
}

func initializedEngineWithReadOnlyView(t *testing.T, useCase documentuc.ReadOnlyViewUseCase) *Engine {
	t.Helper()
	engine := New()
	engine.initialized = true
	engine.readOnlyViewUseCase = useCase
	return engine
}

type readOnlyViewUseCaseStub struct {
	createByCode func(ctx context.Context, workspaceCode, documentID string) (*documentuc.CreateReadOnlyViewLinkResult, error)
}

func (s *readOnlyViewUseCaseStub) CreateReadOnlyViewLink(context.Context, string, string) (*documentuc.CreateReadOnlyViewLinkResult, error) {
	return nil, errors.New("unexpected CreateReadOnlyViewLink")
}

func (s *readOnlyViewUseCaseStub) CreateReadOnlyViewLinkByWorkspaceCode(
	ctx context.Context,
	workspaceCode, documentID string,
) (*documentuc.CreateReadOnlyViewLinkResult, error) {
	if s.createByCode == nil {
		return nil, errors.New("unexpected CreateReadOnlyViewLinkByWorkspaceCode")
	}
	return s.createByCode(ctx, workspaceCode, documentID)
}

func (s *readOnlyViewUseCaseStub) GetReadOnlyView(context.Context, string) (*documentuc.ReadOnlyViewResponse, error) {
	return nil, errors.New("unexpected GetReadOnlyView")
}

func (s *readOnlyViewUseCaseStub) GetReadOnlyViewPDF(context.Context, string) ([]byte, string, error) {
	return nil, "", errors.New("unexpected GetReadOnlyViewPDF")
}

func (s *readOnlyViewUseCaseStub) GetPrintPDF(context.Context, string, string, bool) ([]byte, string, error) {
	return nil, "", errors.New("unexpected GetPrintPDF")
}

func (s *readOnlyViewUseCaseStub) GetPrintPDFByWorkspaceCode(context.Context, string, string, bool) ([]byte, string, error) {
	return nil, "", errors.New("unexpected GetPrintPDFByWorkspaceCode")
}

func (s *readOnlyViewUseCaseStub) GetSigningStateByWorkspaceCode(
	context.Context,
	string,
	[]string,
) (*documentuc.SigningStateResult, error) {
	return nil, errors.New("unexpected GetSigningStateByWorkspaceCode")
}

var _ documentuc.ReadOnlyViewUseCase = (*readOnlyViewUseCaseStub)(nil)
