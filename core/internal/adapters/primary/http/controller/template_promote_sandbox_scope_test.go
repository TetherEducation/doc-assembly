//go:build integration

package controller_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// TestTemplateVersionController_PromoteFromAnotherWorkspacesSandbox_NotFound
// covers the source of a promotion: promote runs in the caller's production
// workspace on a template of that workspace's own sandbox. A published version
// in another workspace's sandbox answers exactly as a missing version does, in
// both modes, and nothing is copied into either workspace.
func TestTemplateVersionController_PromoteFromAnotherWorkspacesSandbox_NotFound(t *testing.T) {
	env := setupTemplateScopeEnv(t)
	missing := missingScopedTemplate()

	victimSandboxID := testhelper.CreateTestSandboxWorkspace(t, env.pool, env.victimWorkspaceID)
	t.Cleanup(func() { cleanupWorkspaceTemplates(t, env.pool, victimSandboxID) })
	sourceTemplateID := testhelper.CreateTestTemplate(t, env.pool, victimSandboxID, "Victim Sandbox Source", nil)
	sourceVersionID := testhelper.CreateTestTemplateVersion(t, env.pool, sourceTemplateID, 1, "Sandbox Published", entity.VersionStatusPublished)

	// The caller's own production template, the target when promoting as a new version.
	target := env.createTemplate(t, env.outsiderWorkspaceID, env.outsiderDocTypeID)
	workspaces := []string{env.victimWorkspaceID, victimSandboxID, env.outsiderWorkspaceID, env.outsiderSandboxID}

	modes := map[string]map[string]any{
		"NEW_TEMPLATE": {"mode": "NEW_TEMPLATE", "versionName": "Copied"},
		"NEW_VERSION":  {"mode": "NEW_VERSION", "targetTemplateId": target.templateID, "versionName": "Copied"},
	}
	for mode, body := range modes {
		t.Run(mode, func(t *testing.T) {
			promote := func(templateID, versionID string) (*http.Response, []byte) {
				return env.outsider.WithHeader("X-Sandbox-Mode", "false").POST(
					templatesPath+templateID+"/versions/"+versionID+"/promote", body)
			}
			before := templateRows(t, env.pool, workspaces...)

			resp, respBody := promote(sourceTemplateID, sourceVersionID)
			assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(respBody))

			missingResp, missingBody := promote(missing.templateID, missing.publishedID)
			assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing version's")
			assert.Equal(t, string(missingBody), string(respBody), "body differs from a missing version's")

			assert.JSONEq(t, before, templateRows(t, env.pool, workspaces...), "a template row changed")
		})
	}
}
