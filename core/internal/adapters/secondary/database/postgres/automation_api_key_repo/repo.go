package automation_api_key_repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const (
	queryCreate = `
INSERT INTO automation.api_keys (name, key_hash, key_prefix, allowed_tenants, is_active, created_by, key_type)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, name, key_hash, key_prefix, key_type, allowed_tenants, is_active, created_by, last_used_at, created_at, revoked_at`

	queryFindByHash = `
SELECT id, name, key_hash, key_prefix, key_type, allowed_tenants, is_active, created_by, last_used_at, created_at, revoked_at
FROM automation.api_keys
WHERE key_hash = $1 AND is_active = true`

	queryFindByID = `
SELECT id, name, key_hash, key_prefix, key_type, allowed_tenants, is_active, created_by, last_used_at, created_at, revoked_at
FROM automation.api_keys
WHERE id = $1`

	queryList = `
SELECT id, name, key_hash, key_prefix, key_type, allowed_tenants, is_active, created_by, last_used_at, created_at, revoked_at
FROM automation.api_keys
ORDER BY created_at DESC`

	queryUpdate = `
UPDATE automation.api_keys
SET name = $2, allowed_tenants = $3
WHERE id = $1
RETURNING id, name, key_hash, key_prefix, key_type, allowed_tenants, is_active, created_by, last_used_at, created_at, revoked_at`

	queryRevoke = `
UPDATE automation.api_keys
SET revoked_at = NOW(), is_active = false
WHERE id = $1`

	queryTouchLastUsed = `
UPDATE automation.api_keys
SET last_used_at = NOW()
WHERE id = $1`
)

// Repository implements port.AutomationAPIKeyRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new automation API key repository.
func New(pool *pgxpool.Pool) port.AutomationAPIKeyRepository {
	return &Repository{pool: pool}
}

// Create persists a new API key and returns the created entity.
func (r *Repository) Create(ctx context.Context, key *entity.AutomationAPIKey) (*entity.AutomationAPIKey, error) {
	row := r.pool.QueryRow(ctx, queryCreate,
		key.Name,
		key.KeyHash,
		key.KeyPrefix,
		key.AllowedTenants,
		key.IsActive,
		key.CreatedBy,
		key.KeyType,
	)
	result, err := scanKey(row)
	if err != nil {
		return nil, fmt.Errorf("creating automation api key: %w", err)
	}
	return result, nil
}

// FindByHash looks up an active API key by its SHA-256 hash.
func (r *Repository) FindByHash(ctx context.Context, keyHash string) (*entity.AutomationAPIKey, error) {
	row := r.pool.QueryRow(ctx, queryFindByHash, keyHash)
	key, err := scanKey(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding automation api key by hash: %w", err)
	}
	return key, nil
}

// FindByID retrieves an API key by its UUID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.AutomationAPIKey, error) {
	row := r.pool.QueryRow(ctx, queryFindByID, id)
	key, err := scanKey(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding automation api key by id %s: %w", id, err)
	}
	return key, nil
}

// List returns all API keys ordered by created_at DESC.
func (r *Repository) List(ctx context.Context) ([]*entity.AutomationAPIKey, error) {
	rows, err := r.pool.Query(ctx, queryList)
	if err != nil {
		return nil, fmt.Errorf("listing automation api keys: %w", err)
	}
	defer rows.Close()

	var keys []*entity.AutomationAPIKey
	for rows.Next() {
		key, err := scanKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning automation api key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating automation api keys: %w", err)
	}
	return keys, nil
}

// Update applies name and allowed_tenants changes to an existing key.
func (r *Repository) Update(ctx context.Context, key *entity.AutomationAPIKey) (*entity.AutomationAPIKey, error) {
	row := r.pool.QueryRow(ctx, queryUpdate,
		key.ID,
		key.Name,
		key.AllowedTenants,
	)
	result, err := scanKey(row)
	if err != nil {
		return nil, fmt.Errorf("updating automation api key %s: %w", key.ID, err)
	}
	return result, nil
}

// Revoke sets revoked_at = NOW() and is_active = false for the given key ID.
func (r *Repository) Revoke(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, queryRevoke, id)
	if err != nil {
		return fmt.Errorf("revoking automation api key %s: %w", id, err)
	}
	return nil
}

// TouchLastUsed updates last_used_at = NOW() for the given key ID.
func (r *Repository) TouchLastUsed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, queryTouchLastUsed, id)
	if err != nil {
		return fmt.Errorf("touching last used for automation api key %s: %w", id, err)
	}
	return nil
}

// scanKey scans a single automation API key from a pgx.Row.
func scanKey(row pgx.Row) (*entity.AutomationAPIKey, error) {
	k := &entity.AutomationAPIKey{}
	err := row.Scan(
		&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.KeyType, &k.AllowedTenants,
		&k.IsActive, &k.CreatedBy, &k.LastUsedAt, &k.CreatedAt, &k.RevokedAt,
	)
	if err != nil {
		return nil, err
	}
	return k, nil
}
