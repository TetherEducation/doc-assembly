package dto

import (
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// SystemInjectableResponse represents a system injectable in API responses.
type SystemInjectableResponse struct {
	Key         string            `json:"key"`
	Label       map[string]string `json:"label"`
	Description map[string]string `json:"description"`
	DataType    string            `json:"dataType"`
	Group       *string           `json:"group,omitempty"`
	IsActive    bool              `json:"isActive"`
	IsPublic    bool              `json:"isPublic"`
}

// ListSystemInjectablesResponse is the response for listing system injectables.
type ListSystemInjectablesResponse struct {
	Injectables []SystemInjectableResponse `json:"injectables"`
}

// SystemInjectableAssignmentResponse represents an assignment in API responses.
type SystemInjectableAssignmentResponse struct {
	ID            string  `json:"id"`
	InjectableKey string  `json:"injectableKey"`
	ScopeType     string  `json:"scopeType"`
	TenantID      *string `json:"tenantId,omitempty"`
	TenantName    *string `json:"tenantName,omitempty"`
	WorkspaceID   *string `json:"workspaceId,omitempty"`
	WorkspaceName *string `json:"workspaceName,omitempty"`
	IsActive      bool    `json:"isActive"`
	CreatedAt     string  `json:"createdAt"`
}

// ListAssignmentsResponse is the response for listing assignments.
type ListAssignmentsResponse struct {
	Assignments []SystemInjectableAssignmentResponse `json:"assignments"`
}

// CreateAssignmentRequest is the request body for creating an assignment.
type CreateAssignmentRequest struct {
	ScopeType   string  `json:"scopeType" binding:"required,oneof=PUBLIC TENANT WORKSPACE"`
	TenantID    *string `json:"tenantId"`
	WorkspaceID *string `json:"workspaceId"`
}

// ToSystemInjectableResponse converts an entity to a DTO response.
func ToSystemInjectableResponse(info *entity.SystemInjectableInfo) SystemInjectableResponse {
	return SystemInjectableResponse{
		Key:         info.Key,
		Label:       info.Label,
		Description: info.Description,
		DataType:    string(info.DataType),
		Group:       info.Group,
		IsActive:    info.IsActive,
		IsPublic:    info.IsPublic,
	}
}

// ToListSystemInjectablesResponse converts a slice of entities to a list response.
func ToListSystemInjectablesResponse(infos []*entity.SystemInjectableInfo) ListSystemInjectablesResponse {
	items := make([]SystemInjectableResponse, len(infos))
	for i, info := range infos {
		items[i] = ToSystemInjectableResponse(info)
	}
	return ListSystemInjectablesResponse{Injectables: items}
}

// ToAssignmentResponse converts an entity to a DTO response.
func ToAssignmentResponse(a *entity.SystemInjectableAssignment) SystemInjectableAssignmentResponse {
	return SystemInjectableAssignmentResponse{
		ID:            a.ID,
		InjectableKey: a.InjectableKey,
		ScopeType:     string(a.ScopeType),
		TenantID:      a.TenantID,
		TenantName:    a.TenantName,
		WorkspaceID:   a.WorkspaceID,
		WorkspaceName: a.WorkspaceName,
		IsActive:      a.IsActive,
		CreatedAt:     a.CreatedAt.Format(time.RFC3339),
	}
}

// ToListAssignmentsResponse converts a slice of entities to a list response.
func ToListAssignmentsResponse(assignments []*entity.SystemInjectableAssignment) ListAssignmentsResponse {
	items := make([]SystemInjectableAssignmentResponse, len(assignments))
	for i, a := range assignments {
		items[i] = ToAssignmentResponse(a)
	}
	return ListAssignmentsResponse{Assignments: items}
}

// BulkPublicAssignmentsRequest is the request body for bulk PUBLIC assignment operations.
type BulkPublicAssignmentsRequest struct {
	Keys []string `json:"keys" binding:"required,min=1"`
}

// BulkOperationError represents an error for a specific key in a bulk operation.
type BulkOperationError struct {
	Key   string `json:"key"`
	Error string `json:"error"`
}

// BulkPublicAssignmentsResponse is the response for bulk PUBLIC assignment operations.
type BulkPublicAssignmentsResponse struct {
	Succeeded []string             `json:"succeeded"`
	Failed    []BulkOperationError `json:"failed,omitempty"`
}
