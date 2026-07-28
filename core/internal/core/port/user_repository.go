package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// UserRepository defines the interface for user data access.
type UserRepository interface {
	// Create creates a new user.
	Create(ctx context.Context, user *entity.User) (string, error)

	// FindByID finds a user by their internal ID.
	FindByID(ctx context.Context, id string) (*entity.User, error)

	// FindByEmail finds a user by email.
	FindByEmail(ctx context.Context, email string) (*entity.User, error)

	// FindByExternalID finds a user by their external IdP ID.
	FindByExternalID(ctx context.Context, externalID string) (*entity.User, error)

	// Update updates a user's information.
	Update(ctx context.Context, user *entity.User) error

	// LinkToIdP links a user to their external IdP account.
	LinkToIdP(ctx context.Context, id, externalID string) error
}
