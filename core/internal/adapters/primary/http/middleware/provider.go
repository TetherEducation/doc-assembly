package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Provider centralizes middleware construction with their required dependencies.
// This avoids passing repositories through multiple layers just to initialize middlewares.
type Provider struct {
	pool                *pgxpool.Pool
	bootstrapEnabled    bool
	userRepo            port.UserRepository
	systemRoleRepo      port.SystemRoleRepository
	workspaceRepo       port.WorkspaceRepository
	workspaceMemberRepo port.WorkspaceMemberRepository
	tenantMemberRepo    port.TenantMemberRepository
}

// NewProvider creates a new middleware provider with all required repositories.
func NewProvider(
	pool *pgxpool.Pool,
	bootstrapEnabled bool,
	userRepo port.UserRepository,
	systemRoleRepo port.SystemRoleRepository,
	workspaceRepo port.WorkspaceRepository,
	workspaceMemberRepo port.WorkspaceMemberRepository,
	tenantMemberRepo port.TenantMemberRepository,
) *Provider {
	return &Provider{
		pool:                pool,
		bootstrapEnabled:    bootstrapEnabled,
		userRepo:            userRepo,
		systemRoleRepo:      systemRoleRepo,
		workspaceRepo:       workspaceRepo,
		workspaceMemberRepo: workspaceMemberRepo,
		tenantMemberRepo:    tenantMemberRepo,
	}
}

// IdentityContext returns a middleware that loads user identity from the database.
// If bootstrap is enabled and no users exist, creates the first user as SUPERADMIN.
func (p *Provider) IdentityContext() gin.HandlerFunc {
	return IdentityContext(p.pool, p.bootstrapEnabled, p.userRepo, p.workspaceMemberRepo)
}

// SystemRoleContext returns a middleware that loads the user's system role if they have one.
func (p *Provider) SystemRoleContext() gin.HandlerFunc {
	return SystemRoleContext(p.systemRoleRepo)
}

// WorkspaceContext returns a middleware that loads workspace context and user's role.
func (p *Provider) WorkspaceContext() gin.HandlerFunc {
	return WorkspaceContext(p.workspaceRepo, p.workspaceMemberRepo, p.tenantMemberRepo)
}

// TenantContext returns a middleware that loads tenant context and user's role.
func (p *Provider) TenantContext() gin.HandlerFunc {
	return TenantContext(p.tenantMemberRepo)
}

// SandboxContext returns a middleware that resolves sandbox workspace when X-Sandbox-Mode header is set.
func (p *Provider) SandboxContext() gin.HandlerFunc {
	return SandboxContext(p.workspaceRepo)
}

// AutomationSandboxContext returns a middleware that resolves sandbox workspace for automation routes.
// It reads workspace ID from URL path params instead of headers.
func (p *Provider) AutomationSandboxContext() gin.HandlerFunc {
	return AutomationSandboxContext(p.workspaceRepo)
}
