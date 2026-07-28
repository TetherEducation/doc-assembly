package dto

import (
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TenantMemberResponse represents a tenant member in API responses.
type TenantMemberResponse struct {
	ID               string                    `json:"id"`
	TenantID         string                    `json:"tenantId"`
	Role             string                    `json:"role"`
	MembershipStatus string                    `json:"membershipStatus"`
	CreatedAt        time.Time                 `json:"createdAt"`
	User             *TenantMemberUserResponse `json:"user"`
}

// TenantMemberUserResponse represents the user data within a tenant member response.
type TenantMemberUserResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"fullName"`
	Status   string `json:"status"`
}

// AddTenantMemberRequest represents a request to add a user to a tenant.
type AddTenantMemberRequest struct {
	Email    string `json:"email" binding:"required,email"`
	FullName string `json:"fullName,omitempty" binding:"max=255"`
	Role     string `json:"role" binding:"required"`
}

// UpdateTenantMemberRoleRequest represents a request to update a tenant member's role.
type UpdateTenantMemberRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// Validate validates the AddTenantMemberRequest.
func (r *AddTenantMemberRequest) Validate() error {
	if r.Email == "" {
		return ErrEmailRequired
	}
	if !isValidTenantRole(r.Role) {
		return ErrInvalidTenantRole
	}
	return nil
}

// Validate validates the UpdateTenantMemberRoleRequest.
func (r *UpdateTenantMemberRoleRequest) Validate() error {
	if !isValidTenantRole(r.Role) {
		return ErrInvalidTenantRole
	}
	return nil
}

// isValidTenantRole checks if the role is a valid tenant role.
func isValidTenantRole(role string) bool {
	switch entity.TenantRole(role) {
	case entity.TenantRoleOwner, entity.TenantRoleAdmin:
		return true
	}
	return false
}
