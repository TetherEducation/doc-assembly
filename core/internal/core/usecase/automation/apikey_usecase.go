package automation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// CreateAPIKeyResult holds the result of creating a new API key.
// RawKey is shown exactly once and never stored.
type CreateAPIKeyResult struct {
	Key    *entity.AutomationAPIKey
	RawKey string // shown exactly once, never stored
}

// APIKeyUseCase defines the input port for API key management operations.
type APIKeyUseCase interface {
	// CreateKey generates a new API key, hashes it, and persists it.
	// The raw key is returned once and never stored.
	// keyType must be entity.KeyTypeAutomation or entity.KeyTypeInternal.
	CreateKey(ctx context.Context, name string, allowedTenants []string, createdBy string, keyType string) (*CreateAPIKeyResult, error)

	// ListKeys returns all API keys (metadata only, no hash).
	ListKeys(ctx context.Context) ([]*entity.AutomationAPIKey, error)

	// GetKey retrieves a single API key by ID.
	GetKey(ctx context.Context, id string) (*entity.AutomationAPIKey, error)

	// UpdateKey updates the name and/or allowed tenants of a key.
	UpdateKey(ctx context.Context, id string, name string, allowedTenants []string) (*entity.AutomationAPIKey, error)

	// RevokeKey revokes an API key, preventing future authentication.
	RevokeKey(ctx context.Context, id string) error

	// ListAuditLog returns paginated audit log entries for a specific API key.
	ListAuditLog(ctx context.Context, apiKeyID string, limit, offset int) ([]*entity.AutomationAuditLog, error)
}

// apiKeyService implements APIKeyUseCase.
type apiKeyService struct {
	keyRepo   port.AutomationAPIKeyRepository
	auditRepo port.AutomationAuditLogRepository
}

// NewAPIKeyUseCase creates a new APIKeyUseCase.
func NewAPIKeyUseCase(
	keyRepo port.AutomationAPIKeyRepository,
	auditRepo port.AutomationAuditLogRepository,
) APIKeyUseCase {
	return &apiKeyService{keyRepo: keyRepo, auditRepo: auditRepo}
}

// generateKey creates a new API key.
// Returns (rawKey, keyHash, keyPrefix, error).
// rawKey = "doca_" + 64 random hex chars
// keyHash = SHA-256 of rawKey, as lowercase hex string
// keyPrefix = first 12 chars of rawKey (e.g. "doca_XXXXXXX")
func generateKey() (rawKey, keyHash, keyPrefix string, err error) {
	b := make([]byte, 32) // 32 bytes = 64 hex chars
	if _, err = rand.Read(b); err != nil {
		return
	}
	rawKey = "doca_" + hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(rawKey))
	keyHash = hex.EncodeToString(sum[:])
	keyPrefix = rawKey[:12]
	return
}

// CreateKey generates a new API key, hashes it, and persists it.
func (s *apiKeyService) CreateKey(ctx context.Context, name string, allowedTenants []string, createdBy string, keyType string) (*CreateAPIKeyResult, error) {
	if keyType != entity.KeyTypeAutomation && keyType != entity.KeyTypeInternal {
		return nil, entity.ErrInvalidKeyType
	}

	rawKey, keyHash, keyPrefix, err := generateKey()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	// Internal keys always have global tenant access.
	if keyType == entity.KeyTypeInternal {
		allowedTenants = nil
	}

	k := &entity.AutomationAPIKey{
		Name:           name,
		KeyHash:        keyHash,
		KeyPrefix:      keyPrefix,
		KeyType:        keyType,
		AllowedTenants: allowedTenants,
		IsActive:       true,
		CreatedBy:      createdBy,
	}
	created, err := s.keyRepo.Create(ctx, k)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	return &CreateAPIKeyResult{Key: created, RawKey: rawKey}, nil
}

// ListKeys returns all API keys (metadata only, no hash).
func (s *apiKeyService) ListKeys(ctx context.Context) ([]*entity.AutomationAPIKey, error) {
	return s.keyRepo.List(ctx)
}

// GetKey retrieves a single API key by ID.
func (s *apiKeyService) GetKey(ctx context.Context, id string) (*entity.AutomationAPIKey, error) {
	key, err := s.keyRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find api key by id: %w", err)
	}
	if key == nil {
		return nil, entity.ErrAPIKeyNotFound
	}
	return key, nil
}

// UpdateKey updates the name and/or allowed tenants of a key.
func (s *apiKeyService) UpdateKey(ctx context.Context, id string, name string, allowedTenants []string) (*entity.AutomationAPIKey, error) {
	key, err := s.keyRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find api key by id: %w", err)
	}
	if key == nil {
		return nil, entity.ErrAPIKeyNotFound
	}
	key.Name = name
	// Internal keys always have global tenant access.
	if key.KeyType == entity.KeyTypeInternal {
		allowedTenants = nil
	}
	key.AllowedTenants = allowedTenants
	return s.keyRepo.Update(ctx, key)
}

// RevokeKey revokes an API key, preventing future authentication.
func (s *apiKeyService) RevokeKey(ctx context.Context, id string) error {
	return s.keyRepo.Revoke(ctx, id)
}

// ListAuditLog returns paginated audit log entries for a specific API key.
func (s *apiKeyService) ListAuditLog(ctx context.Context, apiKeyID string, limit, offset int) ([]*entity.AutomationAuditLog, error) {
	return s.auditRepo.ListByKeyID(ctx, apiKeyID, limit, offset)
}
