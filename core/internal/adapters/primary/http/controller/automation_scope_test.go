package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
	"github.com/gin-gonic/gin"
)

type scopeTemplateUC struct{ templateuc.TemplateUseCase }

func (scopeTemplateUC) GetTemplate(context.Context, string) (*entity.Template, error) {
	return &entity.Template{ID: "template", WorkspaceID: "workspace"}, nil
}

type scopeWorkspaceUC struct {
	organizationuc.WorkspaceUseCase
	tenantID *string
}

func (s scopeWorkspaceUC) GetWorkspace(context.Context, string) (*entity.Workspace, error) {
	return &entity.Workspace{ID: "workspace", TenantID: s.tenantID}, nil
}

type scopeVersionUC struct {
	templateuc.TemplateVersionUseCase
	templateID string
}

func (s scopeVersionUC) GetVersion(context.Context, string) (*entity.TemplateVersion, error) {
	return &entity.TemplateVersion{ID: "version", TemplateID: s.templateID}, nil
}

type scopeKeyRepo struct {
	port.AutomationAPIKeyRepository
	tenants []string
}

func (s scopeKeyRepo) FindByHash(context.Context, string) (*entity.AutomationAPIKey, error) {
	return &entity.AutomationAPIKey{ID: "key", IsActive: true, KeyType: entity.KeyTypeAutomation, AllowedTenants: s.tenants}, nil
}

func TestAutomationTemplateScopeBeforeMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant := "tenant"
	for _, tc := range []struct {
		name            string
		tenants         []string
		tenant          *string
		versionTemplate string
		allowed         bool
	}{
		{"matching tenant", []string{tenant}, &tenant, "template", true},
		{"foreign tenant", []string{"other"}, &tenant, "template", false},
		{"foreign version", []string{tenant}, &tenant, "other-template", false},
		{"scoped key on system template", []string{tenant}, nil, "template", false},
		{"global key", nil, &tenant, "template", true},
		{"global key on system template", nil, nil, "template", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := &AutomationController{templateUC: scopeTemplateUC{}, workspaceUC: scopeWorkspaceUC{tenantID: tc.tenant}, templateVersionUC: scopeVersionUC{templateID: tc.versionTemplate}}
			for _, path := range []string{"/source-document", "/placements", "/publish", "/content"} {
				mutated := false
				r := gin.New()
				r.Use(middleware.AutomationKeyAuth(scopeKeyRepo{tenants: tc.tenants}))
				r.POST("/templates/:templateId/versions/:versionId"+path, ctrl.requireTemplateAccess, func(c *gin.Context) { mutated = true; c.Status(http.StatusNoContent) })
				req := httptest.NewRequest(http.MethodPost, "/templates/template/versions/version"+path, nil)
				req.Header.Set("X-Automation-Key", "test-key")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if mutated != tc.allowed || (tc.allowed && w.Code != http.StatusNoContent) || (!tc.allowed && w.Code < 400) {
					t.Fatalf("%s status=%d mutated=%v expected=%v", path, w.Code, mutated, tc.allowed)
				}
			}
		})
	}
}

type noopAuditRepo struct {
	port.AutomationAuditLogRepository
}

// The audit middleware writes from a goroutine, so an unimplemented Create would
// crash the test binary instead of failing a test.
func (noopAuditRepo) Create(context.Context, *entity.AutomationAuditLog) error { return nil }

// TestAutomationTemplateScopeBeforeMutation proves requireTemplateAccess is
// correct. This proves it is wired: it drives the real route table, so a template
// route registered on the bare group — outside the middleware — fails here instead
// of silently accepting another tenant's key.
func TestEveryTemplateRouteRefusesForeignTenantKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant := "tenant"
	ctrl := &AutomationController{
		templateUC:        scopeTemplateUC{},
		workspaceUC:       scopeWorkspaceUC{tenantID: &tenant},
		templateVersionUC: scopeVersionUC{templateID: "template"},
		keyRepo:           scopeKeyRepo{tenants: []string{"other"}},
		auditRepo:         noopAuditRepo{},
	}
	r := gin.New()
	ctrl.RegisterRoutes(r, &middleware.Provider{}, 1<<20)

	const prefix = "/api/v1/automation/templates/:templateId"
	params := strings.NewReplacer(":templateId", "template", ":versionId", "version")
	checked := 0
	for _, route := range r.Routes() {
		if !strings.HasPrefix(route.Path, prefix) {
			continue
		}
		checked++
		code, panicked := serveWithKey(r, route.Method, params.Replace(route.Path))
		if panicked != nil || code != http.StatusForbidden {
			t.Errorf("%s %s: status=%d panic=%v — not behind requireTemplateAccess",
				route.Method, route.Path, code, panicked)
		}
	}
	// Guards against the loop silently matching nothing if the prefix moves.
	if checked < 12 {
		t.Fatalf("found %d template-scoped automation routes, want at least 12", checked)
	}
}

// serveWithKey reports a panic instead of propagating it: a route that skipped the
// scope check runs its handler against stub use cases, which panic.
func serveWithKey(r *gin.Engine, method, path string) (code int, panicked any) {
	defer func() { panicked = recover() }()
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("X-Automation-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, nil
}
