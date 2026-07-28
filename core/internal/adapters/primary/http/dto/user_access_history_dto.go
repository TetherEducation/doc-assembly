package dto

import "github.com/TetherEducation/doc-assembly/core/internal/core/entity"

// RecordAccessRequest represents a request to record resource access.
type RecordAccessRequest struct {
	EntityType string `json:"entityType" binding:"required"`
	EntityID   string `json:"entityId" binding:"required,uuid"`
}

// Validate validates the RecordAccessRequest.
func (r *RecordAccessRequest) Validate() error {
	if !isValidAccessEntityType(r.EntityType) {
		return ErrInvalidEntityType
	}
	if r.EntityID == "" {
		return ErrIDRequired
	}
	return nil
}

// isValidAccessEntityType checks if the entity type is valid.
func isValidAccessEntityType(entityType string) bool {
	return entity.AccessEntityType(entityType).IsValid()
}
