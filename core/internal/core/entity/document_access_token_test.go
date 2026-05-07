package entity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDocumentAccessToken_ViewOnlyType(t *testing.T) {
	token := &DocumentAccessToken{TokenType: TokenTypeViewOnly}

	require.True(t, token.IsViewOnly())
	require.False(t, token.IsSigning())
	require.False(t, token.IsPreSigning())
}
