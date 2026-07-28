package organization

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

func TestTenantMemberService_LastOwnerProtectionFailsClosed(t *testing.T) {
	t.Run("update role propagates tenant lookup failure", func(t *testing.T) {
		svc := &TenantMemberService{
			memberRepo: &fakeTenantMemberRepo{
				findByIDFn: func(context.Context, string) (*entity.TenantMember, error) {
					return &entity.TenantMember{
						ID:       "member-1",
						TenantID: "tenant-1",
						UserID:   "user-1",
						Role:     entity.TenantRoleOwner,
					}, nil
				},
			},
			tenantRepo: &fakeTenantRepo{
				findByIDFn: func(context.Context, string) (*entity.Tenant, error) {
					return nil, errors.New("db down")
				},
			},
		}

		member, err := svc.UpdateMemberRole(context.Background(), organizationuc.UpdateTenantMemberRoleCommand{
			MemberID:  "member-1",
			TenantID:  "tenant-1",
			NewRole:   entity.TenantRoleAdmin,
			UpdatedBy: "actor-1",
		})

		require.Error(t, err)
		assert.Nil(t, member)
		assert.ErrorContains(t, err, "finding tenant")
		assert.ErrorContains(t, err, "db down")
	})

	t.Run("remove member propagates tenant lookup failure", func(t *testing.T) {
		svc := &TenantMemberService{
			memberRepo: &fakeTenantMemberRepo{
				findByIDFn: func(context.Context, string) (*entity.TenantMember, error) {
					return &entity.TenantMember{
						ID:       "member-1",
						TenantID: "tenant-1",
						UserID:   "user-1",
						Role:     entity.TenantRoleOwner,
					}, nil
				},
			},
			tenantRepo: &fakeTenantRepo{
				findByIDFn: func(context.Context, string) (*entity.Tenant, error) {
					return nil, errors.New("db down")
				},
			},
		}

		err := svc.RemoveMember(context.Background(), organizationuc.RemoveTenantMemberCommand{
			MemberID:  "member-1",
			TenantID:  "tenant-1",
			RemovedBy: "actor-1",
		})

		require.Error(t, err)
		assert.ErrorContains(t, err, "finding tenant")
		assert.ErrorContains(t, err, "db down")
	})
}

type fakeTenantMemberRepo struct {
	findByIDFn    func(ctx context.Context, id string) (*entity.TenantMember, error)
	countByRoleFn func(ctx context.Context, tenantID string, role entity.TenantRole) (int, error)
	updateRoleFn  func(ctx context.Context, id string, role entity.TenantRole) error
	deleteFn      func(ctx context.Context, id string) error
}

func (f *fakeTenantMemberRepo) Create(context.Context, *entity.TenantMember) (string, error) {
	return "", nil
}

func (f *fakeTenantMemberRepo) FindByID(ctx context.Context, id string) (*entity.TenantMember, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, entity.ErrTenantMemberNotFound
}

func (f *fakeTenantMemberRepo) FindByUserAndTenant(context.Context, string, string) (*entity.TenantMember, error) {
	return nil, entity.ErrTenantMemberNotFound
}

func (f *fakeTenantMemberRepo) FindByTenant(context.Context, string) ([]*entity.TenantMemberWithUser, error) {
	return nil, nil
}

func (f *fakeTenantMemberRepo) FindByUser(context.Context, string) ([]*entity.TenantMember, error) {
	return nil, nil
}

func (f *fakeTenantMemberRepo) FindTenantsWithRoleByUser(context.Context, string) ([]*entity.TenantWithRole, error) {
	return nil, nil
}

func (f *fakeTenantMemberRepo) FindActiveByUserAndTenant(context.Context, string, string) (*entity.TenantMember, error) {
	return nil, entity.ErrTenantMemberNotFound
}

func (f *fakeTenantMemberRepo) Delete(ctx context.Context, id string) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return nil
}

func (f *fakeTenantMemberRepo) UpdateRole(ctx context.Context, id string, role entity.TenantRole) error {
	if f.updateRoleFn != nil {
		return f.updateRoleFn(ctx, id, role)
	}
	return nil
}

func (f *fakeTenantMemberRepo) CountByRole(ctx context.Context, tenantID string, role entity.TenantRole) (int, error) {
	if f.countByRoleFn != nil {
		return f.countByRoleFn(ctx, tenantID, role)
	}
	return 0, nil
}

func (f *fakeTenantMemberRepo) FindTenantsWithRoleByUserAndIDs(context.Context, string, []string) ([]*entity.TenantWithRole, error) {
	return nil, nil
}

func (f *fakeTenantMemberRepo) FindTenantsWithRoleByUserPaginated(context.Context, string, port.TenantMemberFilters) ([]*entity.TenantWithRole, int64, error) {
	return nil, 0, nil
}

type fakeTenantRepo struct {
	findByIDFn func(ctx context.Context, id string) (*entity.Tenant, error)
}

func (f *fakeTenantRepo) Create(context.Context, *entity.Tenant) (string, error) { return "", nil }

func (f *fakeTenantRepo) FindByID(ctx context.Context, id string) (*entity.Tenant, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, entity.ErrTenantNotFound
}

func (f *fakeTenantRepo) FindByCode(context.Context, string) (*entity.Tenant, error) {
	return nil, entity.ErrTenantNotFound
}
func (f *fakeTenantRepo) FindAll(context.Context) ([]*entity.Tenant, error) { return nil, nil }
func (f *fakeTenantRepo) FindAllPaginated(context.Context, port.TenantFilters) ([]*entity.Tenant, int64, error) {
	return nil, 0, nil
}
func (f *fakeTenantRepo) SearchByNameOrCode(context.Context, string, int) ([]*entity.Tenant, error) {
	return nil, nil
}
func (f *fakeTenantRepo) Update(context.Context, *entity.Tenant) error { return nil }
func (f *fakeTenantRepo) UpdateStatus(context.Context, string, entity.TenantStatus, *time.Time) error {
	return nil
}
func (f *fakeTenantRepo) Delete(context.Context, string) error               { return nil }
func (f *fakeTenantRepo) ExistsByCode(context.Context, string) (bool, error) { return false, nil }
func (f *fakeTenantRepo) FindSystemTenant(context.Context) (*entity.Tenant, error) {
	return nil, entity.ErrTenantNotFound
}
