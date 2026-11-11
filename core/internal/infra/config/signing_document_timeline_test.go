package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigningDocumentTimeline_DefaultsOff(t *testing.T) {
	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
`))

	require.NoError(t, err)
	assert.False(t, cfg.Signing.DocumentTimeline)
}

func TestSigningDocumentTimeline_FromYAML(t *testing.T) {
	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
  document_timeline: true
`))

	require.NoError(t, err)
	assert.True(t, cfg.Signing.DocumentTimeline)
}

func TestSigningDocumentTimeline_EnvOverridesYAML(t *testing.T) {
	t.Setenv("DOC_ENGINE_SIGNING_DOCUMENT_TIMELINE", "true")

	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
  document_timeline: false
`))

	require.NoError(t, err)
	assert.True(t, cfg.Signing.DocumentTimeline)
}

func TestSigningDocumentTimeline_MalformedEnvIgnored(t *testing.T) {
	t.Setenv("DOC_ENGINE_SIGNING_DOCUMENT_TIMELINE", "yes-please")

	cfg := SigningConfig{DocumentTimeline: true}
	applySigningEnvOverrides(&cfg)

	assert.True(t, cfg.DocumentTimeline)
}
