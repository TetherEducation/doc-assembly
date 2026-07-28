package access

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	accessuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/access"
)

func TestSystemRoleService_AddRole(t *testing.T) {
	t.Run("normalizes email before lookup and create", func(t *testing.T) {
		userRepo := &fakeUserRepo{
			findByEmailFn: func(_ context.Context, email string) (*entity.User, error) {
				assert.Equal(t, "mixedcase@test.com", email)
				return nil, entity.ErrUserNotFound
			},
			createFn: func(_ context.Context, user *entity.User) (string, error) {
				assert.Equal(t, "mixedcase@test.com", user.Email)
				assert.Equal(t, "Full Name", user.FullName)
				return user.ID, nil
			},
		}
		systemRoleRepo := &fakeSystemRoleRepo{}
		svc := &SystemRoleService{
			systemRoleRepo: systemRoleRepo,
			userRepo:       userRepo,
		}

		assignment, err := svc.AddRole(context.Background(), accessuc.AddSystemRoleCommand{
			Email:     "  MixedCase@Test.com ",
			FullName:  "  Full Name  ",
			Role:      entity.SystemRolePlatformAdmin,
			GrantedBy: "admin-id",
		})

		require.NoError(t, err)
		require.NotNil(t, assignment)
		assert.Equal(t, entity.SystemRolePlatformAdmin, assignment.Role)
		assert.Equal(t, 1, userRepo.createCalls)
	})

	t.Run("recovers from duplicate email race by reloading user", func(t *testing.T) {
		existingUser := &entity.User{
			ID:        "user-123",
			Email:     "duplicate@test.com",
			FullName:  "Duplicate User",
			Status:    entity.UserStatusInvited,
			CreatedAt: time.Now().UTC(),
		}

		findCalls := 0
		userRepo := &fakeUserRepo{
			findByEmailFn: func(_ context.Context, email string) (*entity.User, error) {
				findCalls++
				assert.Equal(t, "duplicate@test.com", email)
				if findCalls == 1 {
					return nil, entity.ErrUserNotFound
				}
				return existingUser, nil
			},
			createFn: func(_ context.Context, user *entity.User) (string, error) {
				return "", &pgconn.PgError{
					Code:           "23505",
					ConstraintName: usersEmailUniqueConstraint,
				}
			},
		}
		systemRoleRepo := &fakeSystemRoleRepo{}
		svc := &SystemRoleService{
			systemRoleRepo: systemRoleRepo,
			userRepo:       userRepo,
		}

		assignment, err := svc.AddRole(context.Background(), accessuc.AddSystemRoleCommand{
			Email:     "duplicate@test.com",
			Role:      entity.SystemRoleSuperAdmin,
			GrantedBy: "admin-id",
		})

		require.NoError(t, err)
		require.NotNil(t, assignment)
		assert.Equal(t, existingUser.ID, assignment.UserID)
		assert.Equal(t, 2, findCalls)
		assert.Equal(t, 1, userRepo.createCalls)
	})
}

type fakeUserRepo struct {
	findByEmailFn func(ctx context.Context, email string) (*entity.User, error)
	createFn      func(ctx context.Context, user *entity.User) (string, error)
	updateFn      func(ctx context.Context, user *entity.User) error
	createCalls   int
}

func (f *fakeUserRepo) Create(ctx context.Context, user *entity.User) (string, error) {
	f.createCalls++
	if f.createFn != nil {
		return f.createFn(ctx, user)
	}
	return user.ID, nil
}

func (f *fakeUserRepo) FindByID(_ context.Context, id string) (*entity.User, error) {
	return &entity.User{ID: id, Email: "user@test.com", Status: entity.UserStatusInvited, CreatedAt: time.Now().UTC()}, nil
}

func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	if f.findByEmailFn != nil {
		return f.findByEmailFn(ctx, email)
	}
	return nil, entity.ErrUserNotFound
}

func (f *fakeUserRepo) FindByExternalID(context.Context, string) (*entity.User, error) {
	return nil, entity.ErrUserNotFound
}

func (f *fakeUserRepo) Update(ctx context.Context, user *entity.User) error {
	if f.updateFn != nil {
		return f.updateFn(ctx, user)
	}
	return nil
}

func (f *fakeUserRepo) LinkToIdP(context.Context, string, string) error {
	return nil
}

type fakeSystemRoleRepo struct {
	assignments map[string]*entity.SystemRoleAssignment
}

func (f *fakeSystemRoleRepo) Create(_ context.Context, assignment *entity.SystemRoleAssignment) (string, error) {
	if f.assignments == nil {
		f.assignments = map[string]*entity.SystemRoleAssignment{}
	}
	f.assignments[assignment.UserID] = assignment
	return assignment.ID, nil
}

func (f *fakeSystemRoleRepo) FindByUserID(_ context.Context, userID string) (*entity.SystemRoleAssignment, error) {
	if f.assignments == nil {
		return nil, errors.New("not found")
	}
	assignment, ok := f.assignments[userID]
	if !ok {
		return nil, errors.New("not found")
	}
	return assignment, nil
}

func (f *fakeSystemRoleRepo) FindAll(context.Context) ([]*entity.SystemRoleAssignment, error) {
	return nil, nil
}

func (f *fakeSystemRoleRepo) Delete(context.Context, string) error {
	return nil
}

func (f *fakeSystemRoleRepo) UpdateRole(_ context.Context, userID string, role entity.SystemRole) error {
	if f.assignments == nil {
		f.assignments = map[string]*entity.SystemRoleAssignment{}
	}
	assignment, ok := f.assignments[userID]
	if !ok {
		return errors.New("not found")
	}
	assignment.Role = role
	return nil
}
