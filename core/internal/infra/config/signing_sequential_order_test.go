package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSigningConfig(t *testing.T, yaml string) string {
	t.Helper()
	configFile := filepath.Join(t.TempDir(), "app.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(yaml), 0o600))
	return configFile
}

func TestSigningSequentialOrder_DefaultsOff(t *testing.T) {
	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
`))

	require.NoError(t, err)
	assert.False(t, cfg.Signing.SequentialOrder)
}

func TestSigningSequentialOrder_FromYAML(t *testing.T) {
	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
  sequential_order: true
`))

	require.NoError(t, err)
	assert.True(t, cfg.Signing.SequentialOrder)
}

func TestSigningSequentialOrder_EnvOverridesYAML(t *testing.T) {
	t.Setenv("DOC_ENGINE_SIGNING_SEQUENTIAL_ORDER", "true")

	cfg, err := LoadFromFile(writeSigningConfig(t, `
signing:
  provider: documenso
  sequential_order: false
`))

	require.NoError(t, err)
	assert.True(t, cfg.Signing.SequentialOrder)
}

func TestSigningSequentialOrder_MalformedEnvIgnored(t *testing.T) {
	t.Setenv("DOC_ENGINE_SIGNING_SEQUENTIAL_ORDER", "yes-please")

	cfg := SigningConfig{SequentialOrder: true}
	applySigningEnvOverrides(&cfg)

	assert.True(t, cfg.SequentialOrder)
}
