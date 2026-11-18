package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

const (
	routeScopeTenantID         = "44444444-4444-4444-8444-444444444444"
	routeScopeOtherTenantID    = "55555555-5555-4555-8555-555555555555"
	routeScopeWorkspaceID      = "66666666-6666-4666-8666-666666666666"
	routeScopeOtherWorkspaceID = "77777777-7777-4777-8777-777777777777"
	routeScopeTemplateID       = "88888888-8888-4888-8888-888888888888"
)

// routeScopeWorkspaceUC answers every lookup with one workspace, or with the
// not-found error when it has none.
type routeScopeWorkspaceUC struct {
	organizationuc.WorkspaceUseCase
	workspace *entity.Workspace
}

func (s routeScopeWorkspaceUC) GetWorkspace(_ context.Context, id string) (*entity.Workspace, error) {
	if s.workspace == nil {
		return nil, fmt.Errorf("finding workspace %s: %w", id, entity.ErrWorkspaceNotFound)
	}
	return s.workspace, nil
}

// routeScopeTemplateUC answers every lookup with one template, or with the
// not-found error when it has none.
type routeScopeTemplateUC struct {
	templateuc.TemplateUseCase
	template *entity.Template
}

func (s routeScopeTemplateUC) GetTemplate(_ context.Context, id string) (*entity.Template, error) {
	if s.template == nil {
		return nil, fmt.Errorf("finding template %s: %w", id, entity.ErrTemplateNotFound)
	}
	return s.template, nil
}

func routeScopeWorkspaceIn(tenantID *string) *entity.Workspace {
	return &entity.Workspace{ID: routeScopeWorkspaceID, TenantID: tenantID}
}

func TestAutomationWorkspaceScopeBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant := routeScopeTenantID
	for _, tc := range []struct {
		name        string
		keyTenants  []string
		workspaceID string
		workspace   *entity.Workspace
		want        int // 0: the handler runs
	}{
		{"matching tenant", []string{routeScopeTenantID}, routeScopeWorkspaceID, routeScopeWorkspaceIn(&tenant), 0},
		{"foreign tenant", []string{routeScopeOtherTenantID}, routeScopeWorkspaceID, routeScopeWorkspaceIn(&tenant), http.StatusForbidden},
		{"scoped key on a workspace without a tenant", []string{routeScopeTenantID}, routeScopeWorkspaceID, routeScopeWorkspaceIn(nil), http.StatusForbidden},
		{"global key", nil, routeScopeWorkspaceID, routeScopeWorkspaceIn(&tenant), 0},
		{"global key on a workspace without a tenant", nil, routeScopeWorkspaceID, routeScopeWorkspaceIn(nil), 0},
		{"missing workspace", nil, routeScopeWorkspaceID, nil, http.StatusNotFound},
		{"missing workspace, scoped key", []string{routeScopeTenantID}, routeScopeWorkspaceID, nil, http.StatusNotFound},
		{"workspace ID that is not a UUID", nil, "not-a-uuid", routeScopeWorkspaceIn(&tenant), http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := &AutomationController{workspaceUC: routeScopeWorkspaceUC{workspace: tc.workspace}}
			reached := false
			r := gin.New()
			r.Use(middleware.AutomationKeyAuth(scopeKeyRepo{tenants: tc.keyTenants}))
			r.GET("/workspaces/:workspaceId/templates", ctrl.requireWorkspaceAccess, func(c *gin.Context) {
				reached = true
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/workspaces/"+tc.workspaceID+"/templates", nil)
			req.Header.Set("X-Automation-Key", "test-key")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			want := tc.want
			if want == 0 {
				want = http.StatusNoContent
			}
			if w.Code != want || reached != (tc.want == 0) {
				t.Fatalf("status=%d reached=%v, want status=%d reached=%v", w.Code, reached, want, tc.want == 0)
			}
		})
	}
}

func TestAutomationTemplateInWorkspaceBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name          string
		pathWorkspace string
		templateID    string
		template      *entity.Template
		allowed       bool
	}{
		{"template of the workspace", routeScopeWorkspaceID, routeScopeTemplateID, &entity.Template{ID: routeScopeTemplateID, WorkspaceID: routeScopeWorkspaceID}, true},
		{"workspace ID in upper case", strings.ToUpper(routeScopeWorkspaceID), routeScopeTemplateID, &entity.Template{ID: routeScopeTemplateID, WorkspaceID: routeScopeWorkspaceID}, true},
		{"template of another workspace", routeScopeWorkspaceID, routeScopeTemplateID, &entity.Template{ID: routeScopeTemplateID, WorkspaceID: routeScopeOtherWorkspaceID}, false},
		{"missing template", routeScopeWorkspaceID, routeScopeTemplateID, nil, false},
		{"template ID that is not a UUID", routeScopeWorkspaceID, "not-a-uuid", &entity.Template{ID: routeScopeTemplateID, WorkspaceID: routeScopeWorkspaceID}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := &AutomationController{templateUC: routeScopeTemplateUC{template: tc.template}}
			reached := false
			r := gin.New()
			// Without X-Sandbox-Mode the sandbox middleware only records the workspace
			// of the URL, which is all this check reads.
			r.GET("/workspaces/:workspaceId/templates/:templateId", middleware.AutomationSandboxContext(nil), ctrl.requireTemplateInWorkspace, func(c *gin.Context) {
				reached = true
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/workspaces/"+tc.pathWorkspace+"/templates/"+tc.templateID, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			want := http.StatusNotFound
			if tc.allowed {
				want = http.StatusNoContent
			}
			if w.Code != want || reached != tc.allowed {
				t.Fatalf("status=%d reached=%v, want status=%d reached=%v", w.Code, reached, want, tc.allowed)
			}
			if !tc.allowed && w.Body.String() != `{"error":"template not found"}` {
				t.Fatalf("body=%s, want the body of a missing template", w.Body.String())
			}
		})
	}
}

// The two tests above prove the checks are correct. These prove they are wired:
// they drive the real route table, so a route addressed by workspace that is
// registered outside a check fails here instead of serving another tenant's
// workspace, or another workspace's template.
func TestEveryWorkspaceRouteRefusesForeignTenantKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant := routeScopeTenantID
	ctrl := &AutomationController{
		workspaceUC: routeScopeWorkspaceUC{workspace: routeScopeWorkspaceIn(&tenant)},
		templateUC:  routeScopeTemplateUC{template: &entity.Template{ID: routeScopeTemplateID, WorkspaceID: routeScopeWorkspaceID}},
		keyRepo:     scopeKeyRepo{tenants: []string{routeScopeOtherTenantID}},
		auditRepo:   noopAuditRepo{},
	}
	r := gin.New()
	ctrl.RegisterRoutes(r, &middleware.Provider{}, 1<<20)

	checked := routeScopeCheckRoutes(t, r, "/api/v1/automation/workspaces/:workspaceId", http.StatusForbidden, "requireWorkspaceAccess")
	// Guards against the loop silently matching nothing if the prefix moves.
	if checked < 5 {
		t.Fatalf("found %d automation routes addressed by workspace, want at least 5", checked)
	}
}

func TestEveryWorkspaceTemplateRouteChecksTheWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant := routeScopeTenantID
	ctrl := &AutomationController{
		workspaceUC: routeScopeWorkspaceUC{workspace: routeScopeWorkspaceIn(&tenant)},
		templateUC:  routeScopeTemplateUC{template: &entity.Template{ID: routeScopeTemplateID, WorkspaceID: routeScopeOtherWorkspaceID}},
		keyRepo:     scopeKeyRepo{},
		auditRepo:   noopAuditRepo{},
	}
	r := gin.New()
	ctrl.RegisterRoutes(r, &middleware.Provider{}, 1<<20)

	checked := routeScopeCheckRoutes(t, r, "/api/v1/automation/workspaces/:workspaceId/templates/:templateId", http.StatusNotFound, "requireTemplateInWorkspace")
	if checked < 2 {
		t.Fatalf("found %d automation routes that name a workspace's template, want at least 2", checked)
	}
}

// routeScopeCheckRoutes calls every registered route under prefix and reports
// the ones that do not answer want. It returns how many routes it called.
func routeScopeCheckRoutes(t *testing.T, r *gin.Engine, prefix string, want int, check string) int {
	t.Helper()
	params := strings.NewReplacer(":workspaceId", routeScopeWorkspaceID, ":templateId", routeScopeTemplateID)
	checked := 0
	for _, route := range r.Routes() {
		if !strings.HasPrefix(route.Path, prefix) {
			continue
		}
		checked++
		code, panicked := serveWithKey(r, route.Method, params.Replace(route.Path))
		if panicked != nil || code != want {
			t.Errorf("%s %s: status=%d panic=%v — not behind %s", route.Method, route.Path, code, panicked, check)
		}
	}
	return checked
}
