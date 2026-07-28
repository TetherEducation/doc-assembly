package tenantmemberrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Ensure Repository implements the port interface.
var _ port.TenantMemberRepository = (*Repository)(nil)

// New creates a new tenant member repository.
func New(pool *pgxpool.Pool) port.TenantMemberRepository {
	return &Repository{pool: pool}
}

// Repository implements the tenant member repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new tenant membership.
func (r *Repository) Create(ctx context.Context, member *entity.TenantMember) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		member.ID,
		member.TenantID,
		member.UserID,
		member.Role,
		member.MembershipStatus,
		member.GrantedBy,
		member.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting tenant member: %w", err)
	}

	return id, nil
}

// FindByID finds a tenant membership by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.TenantMember, error) {
	var member entity.TenantMember
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&member.ID,
		&member.TenantID,
		&member.UserID,
		&member.Role,
		&member.MembershipStatus,
		&member.GrantedBy,
		&member.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrTenantMemberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying tenant member: %w", err)
	}

	return &member, nil
}

// FindByUserAndTenant finds a membership for a specific user and tenant.
func (r *Repository) FindByUserAndTenant(ctx context.Context, userID, tenantID string) (*entity.TenantMember, error) {
	var member entity.TenantMember
	err := r.pool.QueryRow(ctx, queryFindByUserAndTenant, userID, tenantID).Scan(
		&member.ID,
		&member.TenantID,
		&member.UserID,
		&member.Role,
		&member.MembershipStatus,
		&member.GrantedBy,
		&member.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrTenantMemberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying tenant member: %w", err)
	}

	return &member, nil
}

// FindByTenant lists all members of a tenant.
func (r *Repository) FindByTenant(ctx context.Context, tenantID string) ([]*entity.TenantMemberWithUser, error) {
	rows, err := r.pool.Query(ctx, queryFindByTenant, tenantID)
	if err != nil {
		return nil, fmt.Errorf("querying tenant members: %w", err)
	}
	defer rows.Close()

	var result []*entity.TenantMemberWithUser
	for rows.Next() {
		var member entity.TenantMember
		var user entity.User
		err := rows.Scan(
			&member.ID,
			&member.TenantID,
			&member.UserID,
			&member.Role,
			&member.MembershipStatus,
			&member.GrantedBy,
			&member.CreatedAt,
			&user.ID,
			&user.Email,
			&user.FullName,
			&user.ExternalIdentityID,
			&user.Status,
			&user.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning tenant member with user: %w", err)
		}
		result = append(result, &entity.TenantMemberWithUser{
			TenantMember: member,
			User:         &user,
		})
	}

	return result, rows.Err()
}

// FindByUser lists all tenant memberships for a user.
func (r *Repository) FindByUser(ctx context.Context, userID string) ([]*entity.TenantMember, error) {
	rows, err := r.pool.Query(ctx, queryFindByUser, userID)
	if err != nil {
		return nil, fmt.Errorf("querying user tenant memberships: %w", err)
	}
	defer rows.Close()

	var result []*entity.TenantMember
	for rows.Next() {
		var member entity.TenantMember
		err := rows.Scan(
			&member.ID,
			&member.TenantID,
			&member.UserID,
			&member.Role,
			&member.MembershipStatus,
			&member.GrantedBy,
			&member.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning tenant member: %w", err)
		}
		result = append(result, &member)
	}

	return result, rows.Err()
}

// FindTenantsWithRoleByUser lists all tenants a user belongs to with their roles.
func (r *Repository) FindTenantsWithRoleByUser(ctx context.Context, userID string) ([]*entity.TenantWithRole, error) {
	rows, err := r.pool.Query(ctx, queryFindTenantsWithRoleByUser, userID)
	if err != nil {
		return nil, fmt.Errorf("querying user tenants: %w", err)
	}
	defer rows.Close()

	var result []*entity.TenantWithRole
	for rows.Next() {
		var tenant entity.Tenant
		var role entity.TenantRole
		err := rows.Scan(
			&tenant.ID,
			&tenant.Name,
			&tenant.Code,
			&tenant.Settings,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
			&role,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning tenant with role: %w", err)
		}
		result = append(result, &entity.TenantWithRole{
			Tenant: &tenant,
			Role:   role,
		})
	}

	return result, rows.Err()
}

// FindActiveByUserAndTenant finds an active membership.
func (r *Repository) FindActiveByUserAndTenant(ctx context.Context, userID, tenantID string) (*entity.TenantMember, error) {
	var member entity.TenantMember
	err := r.pool.QueryRow(ctx, queryFindActiveByUserAndTenant, userID, tenantID).Scan(
		&member.ID,
		&member.TenantID,
		&member.UserID,
		&member.Role,
		&member.MembershipStatus,
		&member.GrantedBy,
		&member.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrTenantMemberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying active tenant membership: %w", err)
	}

	return &member, nil
}

// Delete removes a tenant membership.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting tenant member: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrTenantMemberNotFound
	}

	return nil
}

// UpdateRole updates a member's tenant role.
func (r *Repository) UpdateRole(ctx context.Context, id string, role entity.TenantRole) error {
	result, err := r.pool.Exec(ctx, queryUpdateRole, id, role)
	if err != nil {
		return fmt.Errorf("updating tenant member role: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrTenantMemberNotFound
	}

	return nil
}

// CountByRole counts members with a specific role in a tenant.
func (r *Repository) CountByRole(ctx context.Context, tenantID string, role entity.TenantRole) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryCountByRole, tenantID, role).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting tenant members by role: %w", err)
	}

	return count, nil
}

// FindTenantsWithRoleByUserAndIDs lists tenants by specific IDs that the user belongs to.
// Returns tenants in the same order as the provided IDs.
func (r *Repository) FindTenantsWithRoleByUserAndIDs(ctx context.Context, userID string, tenantIDs []string) ([]*entity.TenantWithRole, error) {
	if len(tenantIDs) == 0 {
		return []*entity.TenantWithRole{}, nil
	}

	rows, err := r.pool.Query(ctx, queryFindTenantsWithRoleByUserAndIDs, userID, tenantIDs)
	if err != nil {
		return nil, fmt.Errorf("querying tenants by IDs: %w", err)
	}
	defer rows.Close()

	var result []*entity.TenantWithRole
	for rows.Next() {
		var tenant entity.Tenant
		var role entity.TenantRole
		err := rows.Scan(
			&tenant.ID,
			&tenant.Name,
			&tenant.Code,
			&tenant.Settings,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
			&role,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning tenant with role: %w", err)
		}
		result = append(result, &entity.TenantWithRole{
			Tenant: &tenant,
			Role:   role,
		})
	}

	return result, rows.Err()
}

// FindTenantsWithRoleByUserPaginated lists tenants a user belongs to with pagination and optional search.
// When filters.Query is provided, orders by similarity. Otherwise, orders by access history.
func (r *Repository) FindTenantsWithRoleByUserPaginated(ctx context.Context, userID string, filters port.TenantMemberFilters) ([]*entity.TenantWithRole, int64, error) {
	// Get total count with search filter
	var total int64
	err := r.pool.QueryRow(ctx, queryCountTenantsWithRoleByUser, userID, filters.Query).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting user tenants: %w", err)
	}

	// Get paginated results with unified ordering
	rows, err := r.pool.Query(ctx, queryFindTenantsWithRoleByUserPaginated, userID, filters.Query, filters.Limit, filters.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying user tenants paginated: %w", err)
	}
	defer rows.Close()

	var result []*entity.TenantWithRole
	for rows.Next() {
		var tenant entity.Tenant
		var role entity.TenantRole
		err := rows.Scan(
			&tenant.ID,
			&tenant.Name,
			&tenant.Code,
			&tenant.Settings,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
			&role,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning tenant with role: %w", err)
		}
		result = append(result, &entity.TenantWithRole{
			Tenant: &tenant,
			Role:   role,
		})
	}

	return result, total, rows.Err()
}
