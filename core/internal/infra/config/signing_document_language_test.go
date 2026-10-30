package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigningDocumentLanguage_DefaultsOff(t *testing.T) {
	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
`))

	require.NoError(t, err)
	assert.False(t, cfg.Signing.DocumentLanguage)
}

func TestSigningDocumentLanguage_FromYAML(t *testing.T) {
	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
  document_language: true
`))

	require.NoError(t, err)
	assert.True(t, cfg.Signing.DocumentLanguage)
}

func TestSigningDocumentLanguage_EnvOverridesYAML(t *testing.T) {
	t.Setenv("DOC_ENGINE_SIGNING_DOCUMENT_LANGUAGE", "true")

	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
  document_language: false
`))

	require.NoError(t, err)
	assert.True(t, cfg.Signing.DocumentLanguage)
}

func TestSigningDocumentLanguage_MalformedEnvIgnored(t *testing.T) {
	t.Setenv("DOC_ENGINE_SIGNING_DOCUMENT_LANGUAGE", "yes-please")

	cfg := SigningConfig{DocumentLanguage: true}
	applySigningEnvOverrides(&cfg)

	assert.True(t, cfg.DocumentLanguage)
}
