package dto

import "github.com/rendis/doc-assembly/core/internal/core/entity"

// CreateDocumentRequest is the request body for creating a document.
type CreateDocumentRequest struct {
	TemplateVersionID         string                   `json:"templateVersionId" binding:"required"`
	Title                     string                   `json:"title" binding:"required"`
	ClientExternalReferenceID *string                  `json:"clientExternalReferenceId,omitempty"`
	InjectedValues            map[string]any           `json:"injectedValues,omitempty"`
	Recipients                []CreateRecipientRequest `json:"recipients" binding:"required,min=1,dive"`
	OperationType             *string                  `json:"operationType,omitempty"`     // CREATE, RENEW, or AMEND (defaults to CREATE)
	RelatedDocumentID         *string                  `json:"relatedDocumentId,omitempty"` // Required for RENEW/AMEND
}

// CreateRecipientRequest represents a recipient in the create document request.
type CreateRecipientRequest struct {
	RoleID string `json:"roleId" binding:"required"`
	Name   string `json:"name" binding:"required"`
	Email  string `json:"email" binding:"required,email"`
}

// DocumentResponse represents a document in API responses.
type DocumentResponse struct {
	ID                        string              `json:"id"`
	WorkspaceID               string              `json:"workspaceId"`
	TemplateVersionID         string              `json:"templateVersionId"`
	Title                     *string             `json:"title,omitempty"`
	ClientExternalReferenceID *string             `json:"clientExternalReferenceId,omitempty"`
	SignerProvider            *string             `json:"signerProvider,omitempty"`
	Status                    string              `json:"status"`
	CompletedPDFURL           *string             `json:"completedPdfUrl,omitempty"`
	CreatedAt                 string              `json:"createdAt"`
	UpdatedAt                 *string             `json:"updatedAt,omitempty"`
	Recipients                []RecipientResponse `json:"recipients,omitempty"`
}

// DocumentListResponse represents a document in list responses.
type DocumentListResponse struct {
	ID                        string                          `json:"id"`
	WorkspaceID               string                          `json:"workspaceId"`
	TemplateVersionID         string                          `json:"templateVersionId"`
	DocumentTypeID            string                          `json:"documentTypeId"`
	DocumentTypeName          *string                         `json:"documentTypeName,omitempty"`
	TemplateName              string                          `json:"templateName"`
	Title                     *string                         `json:"title,omitempty"`
	ClientExternalReferenceID *string                         `json:"clientExternalReferenceId,omitempty"`
	SignerProvider            *string                         `json:"signerProvider,omitempty"`
	Recipients                []DocumentListRecipientResponse `json:"recipients,omitempty"`
	Status                    string                          `json:"status"`
	CreatedAt                 string                          `json:"createdAt"`
	UpdatedAt                 *string                         `json:"updatedAt,omitempty"`
}

// DocumentListRecipientResponse represents recipient data in list responses.
type DocumentListRecipientResponse struct {
	ID                    string `json:"id"`
	DocumentID            string `json:"documentId"`
	TemplateVersionRoleID string `json:"templateVersionRoleId"`
	Name                  string `json:"name"`
	Email                 string `json:"email"`
	Status                string `json:"status"`
	RoleName              string `json:"roleName,omitempty"`
	SignerOrder           int    `json:"signerOrder,omitempty"`
}

// RecipientResponse represents a document recipient in API responses.
type RecipientResponse struct {
	ID          string  `json:"id"`
	RoleID      string  `json:"roleId"`
	RoleName    string  `json:"roleName,omitempty"`
	Name        string  `json:"name"`
	Email       string  `json:"email"`
	Status      string  `json:"status"`
	SignerOrder int     `json:"signerOrder,omitempty"`
	SignedAt    *string `json:"signedAt,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   *string `json:"updatedAt,omitempty"`
}

// SigningURLResponse represents the response for a signing URL request.
type SigningURLResponse struct {
	SigningURL string  `json:"signingUrl"`
	ExpiresAt  *string `json:"expiresAt,omitempty"`
}

// DocumentStatisticsResponse represents document statistics.
type DocumentStatisticsResponse struct {
	Total      int            `json:"total"`
	ByStatus   map[string]int `json:"byStatus"`
	Pending    int            `json:"pending"`
	InProgress int            `json:"inProgress"`
	Completed  int            `json:"completed"`
	Declined   int            `json:"declined"`
}

// BatchCreateDocumentRequest is the request body for creating multiple documents in a batch.
type BatchCreateDocumentRequest struct {
	Documents []CreateDocumentRequest `json:"documents" binding:"required,min=1,max=50,dive"`
}

// BatchCreateDocumentResponse is the response for batch document creation.
type BatchCreateDocumentResponse struct {
	Results []BatchDocumentResultResponse `json:"results"`
}

// BatchDocumentResultResponse represents the result of a single document in a batch.
type BatchDocumentResultResponse struct {
	Index    int                            `json:"index"`
	Success  bool                           `json:"success"`
	Document *entity.DocumentWithRecipients `json:"document,omitempty"`
	Error    string                         `json:"error,omitempty"`
}

// DocumentEventResponse represents a document event in API responses.
type DocumentEventResponse struct {
	ID          string `json:"id"`
	DocumentID  string `json:"documentId"`
	EventType   string `json:"eventType"`
	ActorType   string `json:"actorType"`
	ActorID     string `json:"actorId,omitempty"`
	OldStatus   string `json:"oldStatus,omitempty"`
	NewStatus   string `json:"newStatus,omitempty"`
	RecipientID string `json:"recipientId,omitempty"`
	Metadata    any    `json:"metadata,omitempty"`
	CreatedAt   string `json:"createdAt"`
}
