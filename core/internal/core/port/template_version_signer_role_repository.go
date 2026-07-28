package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TemplateVersionSignerRoleRepository defines the interface for template version signer role data access.
type TemplateVersionSignerRoleRepository interface {
	// Create creates a new signer role for a template version.
	Create(ctx context.Context, role *entity.TemplateVersionSignerRole) (string, error)

	// FindByID finds a signer role by ID.
	FindByID(ctx context.Context, id string) (*entity.TemplateVersionSignerRole, error)

	// FindByVersionID lists all signer roles for a template version ordered by signer_order.
	FindByVersionID(ctx context.Context, versionID string) ([]*entity.TemplateVersionSignerRole, error)

	// Update updates a signer role.
	Update(ctx context.Context, role *entity.TemplateVersionSignerRole) error

	// Delete deletes a signer role.
	Delete(ctx context.Context, id string) error

	// DeleteByVersionID deletes all signer roles for a template version.
	DeleteByVersionID(ctx context.Context, versionID string) error

	// ExistsByAnchor checks if an anchor string already exists for the version.
	ExistsByAnchor(ctx context.Context, versionID, anchorString string) (bool, error)

	// ExistsByAnchorExcluding checks if an anchor exists excluding a specific role ID.
	ExistsByAnchorExcluding(ctx context.Context, versionID, anchorString, excludeID string) (bool, error)

	// ExistsByOrder checks if a signer order already exists for the version.
	ExistsByOrder(ctx context.Context, versionID string, order int) (bool, error)

	// ExistsByOrderExcluding checks if an order exists excluding a specific role ID.
	ExistsByOrderExcluding(ctx context.Context, versionID string, order int, excludeID string) (bool, error)

	// CopyFromVersion copies all signer roles from one version to another.
	CopyFromVersion(ctx context.Context, sourceVersionID, targetVersionID string) error
}
