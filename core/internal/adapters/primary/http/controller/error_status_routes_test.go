//go:build integration

package controller_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/controller"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	systeminjectablerepo "github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/system_injectable_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	injectablesvc "github.com/TetherEducation/doc-assembly/core/internal/core/service/injectable"
	"github.com/TetherEducation/doc-assembly/core/internal/extensions/injectors/datetime"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/registry"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// A request that a service refuses for what the request itself asks answers
// with a 4xx status and the reason, on the routes that reach those services.
func TestCallerErrors_AnswerWithStatusAndReason(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)

	tenantID := testhelper.CreateTestTenant(t, pool, "Caller Error Tenant", "CERR01")
	defer testhelper.CleanupTenant(t, pool, tenantID)
	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Caller Error Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)
	sandboxID := testhelper.CreateTestSandboxWorkspace(t, pool, workspaceID)
	defer testhelper.CleanupWorkspace(t, pool, sandboxID)

	superAdminRole := entity.SystemRoleSuperAdmin
	superAdmin := testhelper.CreateTestUser(t, pool, "caller-errors@test.com", "Super Admin", &superAdminRole)
	defer testhelper.CleanupUser(t, pool, superAdmin.ID)
	client := testhelper.NewHTTPClient(t, ts.URL()).WithAuth(superAdmin.BearerHeader)
	inWorkspace := client.WithWorkspaceID(workspaceID)

	// A scheduled version.
	scheduledTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Scheduled Template", nil)
	defer testhelper.CleanupTemplate(t, pool, scheduledTemplateID)
	scheduledVersionID := testhelper.CreateTestTemplateVersion(t, pool, scheduledTemplateID, 1, "Scheduled", entity.VersionStatusScheduled)

	// Two templates of one document type, the first already on the ENROLLMENT process.
	documentTypeID := testhelper.CreateTestDocumentType(t, pool, tenantID, "CERR_CONTRACT", "Contract")
	defer testhelper.CleanupDocumentType(t, pool, documentTypeID)
	enrollment := dto.SetProcessFieldsRequest{Process: "ENROLLMENT", ProcessType: string(entity.ProcessTypeCanonicalName)}
	firstTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Enrollment Contract", nil)
	defer testhelper.CleanupTemplate(t, pool, firstTemplateID)
	testhelper.SetTemplateDocumentType(t, pool, firstTemplateID, documentTypeID)
	resp, body := inWorkspace.PUT("/api/v1/content/templates/"+firstTemplateID+"/process", enrollment)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	secondTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Other Contract", nil)
	defer testhelper.CleanupTemplate(t, pool, secondTemplateID)
	testhelper.SetTemplateDocumentType(t, pool, secondTemplateID, documentTypeID)

	// A published version in the workspace's sandbox.
	sandboxTemplateID := testhelper.CreateTestTemplate(t, pool, sandboxID, "Sandbox Template", nil)
	defer testhelper.CleanupTemplate(t, pool, sandboxTemplateID)
	sandboxVersionID := testhelper.CreateTestTemplateVersion(t, pool, sandboxTemplateID, 1, "Published", entity.VersionStatusPublished)

	var systemTenantID, systemWorkspaceID string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT id FROM tenancy.tenants WHERE is_system = TRUE LIMIT 1`).Scan(&systemTenantID))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT id FROM tenancy.workspaces WHERE tenant_id = $1 AND type = 'SYSTEM' AND is_sandbox = FALSE`,
		tenantID).Scan(&systemWorkspaceID))

	renamed := "Renamed"
	cases := []struct {
		name    string
		request func() (*http.Response, []byte)
		status  int
		reason  error
	}{
		{
			name: "a tag name of fewer than three characters once normalized",
			request: func() (*http.Response, []byte) {
				return inWorkspace.POST("/api/v1/workspace/tags", dto.CreateTagRequest{Name: "a!!", Color: "#FF0000"})
			},
			status: http.StatusBadRequest,
			reason: entity.ErrFieldTooShort,
		},
		{
			name: "editing a scheduled version",
			request: func() (*http.Response, []byte) {
				return inWorkspace.PUT(
					"/api/v1/content/templates/"+scheduledTemplateID+"/versions/"+scheduledVersionID,
					dto.UpdateVersionRequest{Name: &renamed})
			},
			status: http.StatusBadRequest,
			reason: entity.ErrCannotEditScheduled,
		},
		{
			name: "a process another template of the document type already has",
			request: func() (*http.Response, []byte) {
				return inWorkspace.PUT("/api/v1/content/templates/"+secondTemplateID+"/process", enrollment)
			},
			status: http.StatusConflict,
			reason: entity.ErrProcessSlotConflict,
		},
		{
			name: "promoting a version into a sandbox",
			request: func() (*http.Response, []byte) {
				return inWorkspace.WithHeader("X-Sandbox-Mode", "true").POST(
					"/api/v1/content/templates/"+sandboxTemplateID+"/versions/"+sandboxVersionID+"/promote",
					dto.PromoteVersionRequest{Mode: dto.PromotionModeNewTemplate})
			},
			status: http.StatusBadRequest,
			reason: entity.ErrCannotPromoteToSandbox,
		},
		{
			name: "changing the status of the system tenant",
			request: func() (*http.Response, []byte) {
				return client.PATCH("/api/v1/system/tenants/"+systemTenantID+"/status",
					dto.UpdateTenantStatusRequest{Status: string(entity.TenantStatusActive)})
			},
			status: http.StatusBadRequest,
			reason: entity.ErrCannotModifySystemTenant,
		},
		{
			name: "changing the status of a system workspace",
			request: func() (*http.Response, []byte) {
				return client.WithTenantID(tenantID).PATCH("/api/v1/tenant/workspaces/"+systemWorkspaceID+"/status",
					dto.UpdateWorkspaceStatusRequest{Status: string(entity.WorkspaceStatusActive)})
			},
			status: http.StatusBadRequest,
			reason: entity.ErrCannotModifySystemWorkspace,
		},
		{
			name: "an API key of an unknown type",
			request: func() (*http.Response, []byte) {
				return client.POST("/api/v1/automation-keys/",
					dto.CreateAutomationKeyRequest{Name: "caller-errors", KeyType: "other"})
			},
			status: http.StatusBadRequest,
			reason: entity.ErrInvalidKeyType,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := tc.request()

			require.Equal(t, tc.status, resp.StatusCode, string(body))
			answer := testhelper.ParseJSON[dto.ErrorResponse](t, body)
			assert.Contains(t, answer.Error, tc.reason.Error())
		})
	}
}

// The same for the system injectable routes. The test server has no injector
// registry, so the admin controller's handlers are served here over a
// registry that knows one key, and over the real repository.
func TestCallerErrors_SystemInjectables(t *testing.T) {
	pool := testhelper.GetTestPool(t)

	injectors := registry.NewInjectorRegistry(nil)
	require.NoError(t, injectors.Register(&datetime.YearNowInjector{}))
	admin := controller.NewAdminController(nil, nil,
		injectablesvc.NewSystemInjectableService(systeminjectablerepo.New(pool), injectors))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.PATCH("/system/injectables/:key/activate", admin.ActivateInjectable)
	engine.POST("/system/injectables/:key/assignments", admin.CreateAssignment)
	engine.DELETE("/system/injectables/:key/assignments/:assignmentId", admin.DeleteAssignment)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		reason error
	}{
		{
			name:   "a key that the registry does not know",
			method: http.MethodPatch,
			path:   "/system/injectables/unknown_key/activate",
			status: http.StatusNotFound,
			reason: entity.ErrSystemInjectableNotFound,
		},
		{
			name:   "a tenant assignment that names no tenant",
			method: http.MethodPost,
			path:   "/system/injectables/year_now/assignments",
			body:   `{"scopeType":"TENANT"}`,
			status: http.StatusBadRequest,
			reason: entity.ErrTenantIDRequired,
		},
		{
			name:   "an assignment that does not exist",
			method: http.MethodDelete,
			path:   "/system/injectables/year_now/assignments/" + uuid.NewString(),
			status: http.StatusNotFound,
			reason: entity.ErrAssignmentNotFound,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, request)

			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			answer := testhelper.ParseJSON[dto.ErrorResponse](t, recorder.Body.Bytes())
			assert.Contains(t, answer.Error, tc.reason.Error())
		})
	}
}
