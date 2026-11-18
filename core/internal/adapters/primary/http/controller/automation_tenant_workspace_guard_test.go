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
)

const (
	guardTenantID      = "11111111-1111-4111-8111-111111111111"
	guardOtherTenantID = "22222222-2222-4222-8222-222222222222"
	guardWorkspaceID   = "33333333-3333-4333-8333-333333333333"
)

// guardWorkspaceUC answers every lookup with one workspace, or with the
// not-found error when it has none.
type guardWorkspaceUC struct {
	organizationuc.WorkspaceUseCase
	workspace *entity.Workspace
}

func (s guardWorkspaceUC) GetWorkspace(_ context.Context, id string) (*entity.Workspace, error) {
	if s.workspace == nil {
		return nil, fmt.Errorf("finding workspace %s: %w", id, entity.ErrWorkspaceNotFound)
	}
	return s.workspace, nil
}

func guardWorkspaceIn(tenantID string) *entity.Workspace {
	return &entity.Workspace{ID: guardWorkspaceID, TenantID: &tenantID}
}

func TestAutomationWorkspaceInTenantBeforeMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		keyTenants  []string
		pathTenant  string
		workspaceID string
		workspace   *entity.Workspace
		want        int // 0: the handler runs
	}{
		{"workspace of the tenant", nil, guardTenantID, guardWorkspaceID, guardWorkspaceIn(guardTenantID), 0},
		{"workspace of the tenant, scoped key", []string{guardTenantID}, guardTenantID, guardWorkspaceID, guardWorkspaceIn(guardTenantID), 0},
		{"tenant ID in upper case", nil, strings.ToUpper(guardTenantID), guardWorkspaceID, guardWorkspaceIn(guardTenantID), 0},
		{"workspace of another tenant", nil, guardTenantID, guardWorkspaceID, guardWorkspaceIn(guardOtherTenantID), http.StatusNotFound},
		{"workspace of another tenant, scoped key", []string{guardTenantID}, guardTenantID, guardWorkspaceID, guardWorkspaceIn(guardOtherTenantID), http.StatusNotFound},
		{"workspace of another tenant, key scoped to both", []string{guardTenantID, guardOtherTenantID}, guardTenantID, guardWorkspaceID, guardWorkspaceIn(guardOtherTenantID), http.StatusNotFound},
		{"workspace without a tenant", nil, guardTenantID, guardWorkspaceID, &entity.Workspace{ID: guardWorkspaceID}, http.StatusNotFound},
		{"missing workspace", nil, guardTenantID, guardWorkspaceID, nil, http.StatusNotFound},
		{"workspace ID that is not a UUID", nil, guardTenantID, "not-a-uuid", guardWorkspaceIn(guardTenantID), http.StatusNotFound},
		{"tenant the key may not use", []string{guardOtherTenantID}, guardTenantID, guardWorkspaceID, guardWorkspaceIn(guardTenantID), http.StatusForbidden},
		{"tenant the key may not use, missing workspace", []string{guardOtherTenantID}, guardTenantID, guardWorkspaceID, nil, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := &AutomationController{workspaceUC: guardWorkspaceUC{workspace: tc.workspace}}
			mutated := false
			r := gin.New()
			r.Use(middleware.AutomationKeyAuth(scopeKeyRepo{tenants: tc.keyTenants}))
			r.POST("/tenants/:tenantId/workspaces/:workspaceId/suspend", ctrl.requireWorkspaceInTenant, func(c *gin.Context) {
				mutated = true
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodPost, "/tenants/"+tc.pathTenant+"/workspaces/"+tc.workspaceID+"/suspend", nil)
			req.Header.Set("X-Automation-Key", "test-key")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			want := tc.want
			if want == 0 {
				want = http.StatusNoContent
			}
			if w.Code != want || mutated != (tc.want == 0) {
				t.Fatalf("status=%d mutated=%v, want status=%d mutated=%v", w.Code, mutated, want, tc.want == 0)
			}
		})
	}
}

// TestAutomationWorkspaceInTenantBeforeMutation proves requireWorkspaceInTenant
// is correct. This proves it is wired: it drives the real route table, so a
// route that names a workspace under a tenant but is registered outside the
// check fails here instead of acting on another tenant's workspace.
func TestEveryTenantWorkspaceRouteChecksTheTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := &AutomationController{
		workspaceUC: guardWorkspaceUC{workspace: guardWorkspaceIn(guardOtherTenantID)},
		keyRepo:     scopeKeyRepo{},
		auditRepo:   noopAuditRepo{},
	}
	r := gin.New()
	ctrl.RegisterRoutes(r, &middleware.Provider{}, 1<<20)

	const prefix = "/api/v1/automation/tenants/:tenantId/workspaces/:workspaceId"
	params := strings.NewReplacer(":tenantId", guardTenantID, ":workspaceId", guardWorkspaceID)
	checked := 0
	for _, route := range r.Routes() {
		if !strings.HasPrefix(route.Path, prefix) {
			continue
		}
		checked++
		code, panicked := serveWithKey(r, route.Method, params.Replace(route.Path))
		if panicked != nil || code != http.StatusNotFound {
			t.Errorf("%s %s: status=%d panic=%v — not behind requireWorkspaceInTenant",
				route.Method, route.Path, code, panicked)
		}
	}
	// Guards against the loop silently matching nothing if the prefix moves.
	if checked < 4 {
		t.Fatalf("found %d automation routes that name a tenant's workspace, want at least 4", checked)
	}
}
