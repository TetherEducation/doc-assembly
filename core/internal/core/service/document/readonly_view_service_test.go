package document

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

func TestDocumentAccessToken_ViewOnlyType(t *testing.T) {
	token := &entity.DocumentAccessToken{TokenType: entity.TokenTypeViewOnly}

	require.True(t, token.IsViewOnly())
	require.False(t, token.IsSigning())
	require.False(t, token.IsPreSigning())
}
