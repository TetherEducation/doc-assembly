package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigningDocumentFilenameFromTitle_DefaultsOff(t *testing.T) {
	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
`))

	require.NoError(t, err)
	assert.False(t, cfg.Signing.DocumentFilenameFromTitle)
}

func TestSigningDocumentFilenameFromTitle_FromYAML(t *testing.T) {
	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
  document_filename_from_title: true
`))

	require.NoError(t, err)
	assert.True(t, cfg.Signing.DocumentFilenameFromTitle)
}

func TestSigningDocumentFilenameFromTitle_EnvOverridesYAML(t *testing.T) {
	t.Setenv("DOC_ENGINE_SIGNING_DOCUMENT_FILENAME_FROM_TITLE", "true")

	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
  document_filename_from_title: false
`))

	require.NoError(t, err)
	assert.True(t, cfg.Signing.DocumentFilenameFromTitle)
}

func TestSigningDocumentFilenameFromTitle_MalformedEnvIgnored(t *testing.T) {
	t.Setenv("DOC_ENGINE_SIGNING_DOCUMENT_FILENAME_FROM_TITLE", "yes-please")

	cfg := SigningConfig{DocumentFilenameFromTitle: true}
	applySigningEnvOverrides(&cfg)

	assert.True(t, cfg.DocumentFilenameFromTitle)
}
