package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigningSuppressOwnerSignedEmail_DefaultsOff(t *testing.T) {
	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
`))

	require.NoError(t, err)
	assert.False(t, cfg.Signing.SuppressOwnerSignedEmail)
}

func TestSigningSuppressOwnerSignedEmail_FromYAML(t *testing.T) {
	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
  suppress_owner_signed_email: true
`))

	require.NoError(t, err)
	assert.True(t, cfg.Signing.SuppressOwnerSignedEmail)
}

func TestSigningSuppressOwnerSignedEmail_EnvOverridesYAML(t *testing.T) {
	t.Setenv("DOC_ENGINE_SIGNING_SUPPRESS_OWNER_SIGNED_EMAIL", "true")

	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
  suppress_owner_signed_email: false
`))

	require.NoError(t, err)
	assert.True(t, cfg.Signing.SuppressOwnerSignedEmail)
}

func TestSigningSuppressOwnerSignedEmail_MalformedEnvIgnored(t *testing.T) {
	t.Setenv("DOC_ENGINE_SIGNING_SUPPRESS_OWNER_SIGNED_EMAIL", "yes-please")

	cfg := SigningConfig{SuppressOwnerSignedEmail: true}
	applySigningEnvOverrides(&cfg)

	assert.True(t, cfg.SuppressOwnerSignedEmail)
}
