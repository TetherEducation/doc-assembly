package docs

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyDocumentProxySwagger_DoesNotNarrowDynamicPayloads(t *testing.T) {
	raw, err := os.ReadFile("swagger.yaml")
	require.NoError(t, err)

	section := legacyDocumentProxySwaggerSection(string(raw))
	require.NotEmpty(t, section)

	assert.NotContains(t, section, "consumes:")
	assert.NotContains(t, section, "in: body")
	assert.Contains(t, section, "optional raw data owned by the host handler")
	assert.Contains(t, section, "Host-defined JSON response (object, array, scalar, or null)")
	assert.Contains(t, section, "name: X-Workspace-Code")
	assert.Contains(t, section, "name: X-Environment")
}

func legacyDocumentProxySwaggerSection(swagger string) string {
	start := strings.Index(swagger, "  /legacy-documents/proxy:")
	if start < 0 {
		return ""
	}

	rest := swagger[start+len("  /legacy-documents/proxy:"):]
	nextPath := strings.Index(rest, "\n  /")
	if nextPath < 0 {
		return swagger[start:]
	}
	return swagger[start : start+len("  /legacy-documents/proxy:")+nextPath]
}
