package organization

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// AddTenantMemberCommand contains data for adding a user to a tenant.
type AddTenantMemberCommand struct {
	TenantID  string
	Email     string
	FullName  string
	Role      entity.TenantRole
	GrantedBy string
}

// UpdateTenantMemberRoleCommand contains data for updating a tenant member's role.
type UpdateTenantMemberRoleCommand struct {
	MemberID  string
	TenantID  string
	NewRole   entity.TenantRole
	UpdatedBy string // The user performing the update
}

// RemoveTenantMemberCommand contains data for removing a tenant member.
type RemoveTenantMemberCommand struct {
	MemberID  string
	TenantID  string
	RemovedBy string // The user performing the removal
}

// TenantMemberUseCase defines the interface for tenant member operations.
type TenantMemberUseCase interface {
	// ListMembers lists all members of a tenant.
	ListMembers(ctx context.Context, tenantID string) ([]*entity.TenantMemberWithUser, error)

	// GetMember retrieves a specific tenant member by ID.
	GetMember(ctx context.Context, memberID string) (*entity.TenantMemberWithUser, error)

	// AddMember adds a user to a tenant.
	// Creates a shadow user if the email doesn't exist.
	AddMember(ctx context.Context, cmd AddTenantMemberCommand) (*entity.TenantMemberWithUser, error)

	// UpdateMemberRole updates a tenant member's role.
	UpdateMemberRole(ctx context.Context, cmd UpdateTenantMemberRoleCommand) (*entity.TenantMemberWithUser, error)

	// RemoveMember removes a member from the tenant.
	RemoveMember(ctx context.Context, cmd RemoveTenantMemberCommand) error

	// CountOwners counts the number of TENANT_OWNER members in a tenant.
	CountOwners(ctx context.Context, tenantID string) (int, error)
}
