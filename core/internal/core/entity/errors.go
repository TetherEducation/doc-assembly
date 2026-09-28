package entity

import (
	"errors"
	"fmt"
	"strings"
)

// Authentication and Authorization errors.
var (
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("access denied")
	ErrTooManyRequests  = errors.New("too many requests")
	ErrInvalidToken     = errors.New("invalid token")
	ErrTokenExpired     = errors.New("token expired")
	ErrMissingToken     = errors.New("missing authorization token")
	ErrInsufficientRole = errors.New("insufficient role permissions")
	ErrUnknownIssuer    = errors.New("unknown token issuer")
)

// API Key errors (for internal service-to-service authentication).
var (
	ErrMissingAPIKey = errors.New("missing API key")
	ErrInvalidAPIKey = errors.New("invalid API key")
)

// Context errors.
var (
	ErrMissingWorkspaceID = errors.New("missing workspace ID")
	ErrMissingTenantID    = errors.New("missing tenant ID")
	ErrMissingUserID      = errors.New("missing user ID")
	ErrInvalidWorkspaceID = errors.New("invalid workspace ID format")
	ErrInvalidTenantID    = errors.New("invalid tenant ID format")
	ErrInvalidUserID      = errors.New("invalid user ID format")
)

// System Role errors.
var (
	ErrSystemRoleNotFound = errors.New("system role not found")
	ErrSystemRoleExists   = errors.New("user already has a system role")
	ErrInvalidSystemRole  = errors.New("invalid system role")
)

// Tenant Member errors.
var (
	ErrTenantMemberNotFound    = errors.New("tenant member not found")
	ErrTenantMemberExists      = errors.New("user is already a member of this tenant")
	ErrTenantAccessDenied      = errors.New("tenant access denied")
	ErrInvalidTenantRole       = errors.New("invalid tenant role")
	ErrCannotRemoveTenantOwner = errors.New("cannot remove tenant owner")
)

// Tenant errors.
var (
	ErrTenantNotFound           = errors.New("tenant not found")
	ErrTenantAlreadyExists      = errors.New("tenant already exists")
	ErrInvalidTenantCode        = errors.New("invalid tenant code")
	ErrInvalidTenantStatus      = errors.New("invalid tenant status")
	ErrCannotModifySystemTenant = errors.New("cannot modify system tenant")
)

// Workspace errors.
var (
	ErrWorkspaceNotFound           = errors.New("workspace not found")
	ErrWorkspaceAlreadyExists      = errors.New("workspace already exists")
	ErrWorkspaceAccessDenied       = errors.New("workspace access denied")
	ErrWorkspaceSuspended          = errors.New("workspace is suspended")
	ErrWorkspaceArchived           = errors.New("workspace is archived")
	ErrSystemWorkspaceExists       = errors.New("system workspace already exists for this tenant")
	ErrGlobalWorkspaceExists       = errors.New("global system workspace already exists")
	ErrInvalidWorkspaceType        = errors.New("invalid workspace type")
	ErrInvalidWorkspaceStatus      = errors.New("invalid workspace status")
	ErrCannotArchiveSystem         = errors.New("cannot archive system workspace")
	ErrCannotModifySystemWorkspace = errors.New("cannot modify system workspace status")
	ErrWorkspaceCodeExists         = errors.New("workspace with this code already exists in tenant")
	ErrInvalidWorkspaceCode        = errors.New("invalid workspace code")
	ErrSandboxNotFound             = errors.New("sandbox workspace not found")
	ErrSandboxNotSupported         = errors.New("this workspace type does not support sandbox mode")
	ErrCannotPromoteToSandbox      = errors.New("cannot promote to a sandbox workspace")
)

// User errors.
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrUserSuspended     = errors.New("user is suspended")
	ErrUserNotInvited    = errors.New("user has not been invited to the system")
	ErrInvalidUserStatus = errors.New("invalid user status")
	ErrEmailAlreadyInUse = errors.New("email already in use")
	ErrInvalidEmail      = errors.New("invalid email format")
)

// Workspace Member errors.
var (
	ErrMemberNotFound               = errors.New("workspace member not found")
	ErrMemberAlreadyExists          = errors.New("user is already a member of this workspace")
	ErrMembershipPending            = errors.New("membership is pending")
	ErrCannotRemoveOwner            = errors.New("cannot remove workspace owner")
	ErrCannotRemoveLastSystemMember = errors.New("cannot remove the last direct member from the global system workspace")
	ErrInvalidRole                  = errors.New("invalid workspace role")
	ErrInvalidMembershipStatus      = errors.New("invalid membership status")
)

// Folder errors.
var (
	ErrFolderNotFound      = errors.New("folder not found")
	ErrFolderAlreadyExists = errors.New("folder with this name already exists")
	ErrFolderHasChildren   = errors.New("folder has child folders")
	ErrFolderHasTemplates  = errors.New("folder contains templates")
	ErrInvalidParentFolder = errors.New("invalid parent folder")
	ErrCircularReference   = errors.New("circular folder reference detected")
)

// Tag errors.
var (
	ErrTagNotFound      = errors.New("tag not found")
	ErrTagAlreadyExists = errors.New("tag with this name already exists")
	ErrTagInUse         = errors.New("tag is in use by templates")
	ErrInvalidTagColor  = errors.New("invalid tag color format")
)

// Injectable Definition errors.
var (
	ErrInjectableNotFound         = errors.New("injectable definition not found")
	ErrInjectableAlreadyExists    = errors.New("injectable with this key already exists")
	ErrInjectableInUse            = errors.New("injectable is in use by templates")
	ErrInvalidInjectableKey       = errors.New("invalid injectable key")
	ErrInvalidDataType            = errors.New("invalid injectable data type")
	ErrInvalidInjectableSource    = errors.New("must specify either injectable definition ID or system key, not both")
	ErrTemplateInjectableNotFound = errors.New("template injectable not found")
	ErrOnlyTextTypeAllowed        = errors.New("only TEXT type injectables can be created by workspaces")
	ErrWorkspaceIDRequired        = errors.New("workspace ID is required for this injectable")
	ErrCannotModifyGlobal         = errors.New("cannot modify global injectable definitions")
)

// System Injectable errors.
var (
	ErrSystemInjectableNotFound = errors.New("system injectable not found in registry")
	ErrInvalidScopeType         = errors.New("invalid scope type")
	ErrTenantIDRequired         = errors.New("tenant ID is required for TENANT scope")
	ErrAssignmentNotFound       = errors.New("system injectable assignment not found")
)

// Document Generation errors.
var (
	ErrNoMapperRegistered = errors.New("no mapper registered in registry")
	// ErrInternalTemplateResolutionNotFound indicates no published template version
	// could be resolved for the internal create request.
	ErrInternalTemplateResolutionNotFound = errors.New("no published template version resolved")
)

// MissingInjectablesError indicates that required injectables are not available.
type MissingInjectablesError struct {
	MissingCodes []string
}

// Error implements the error interface.
func (e *MissingInjectablesError) Error() string {
	return fmt.Sprintf("missing required injectables: %v", e.MissingCodes)
}

// RecipientValidationError indicates that one or more recipients failed validation.
type RecipientValidationError struct {
	Errors []string
}

// Error implements the error interface.
func (e *RecipientValidationError) Error() string {
	return fmt.Sprintf("recipient validation failed: %s", strings.Join(e.Errors, "; "))
}

// Document Type errors.
var (
	ErrDocumentTypeNotFound        = errors.New("document type not found")
	ErrDocumentTypeCodeExists      = errors.New("document type with this code already exists")
	ErrDocumentTypeCodeImmutable   = errors.New("document type code cannot be modified")
	ErrDocumentTypeAlreadyAssigned = errors.New("workspace already has a template for this document type")
	ErrDocumentTypeHasTemplates    = errors.New("document type is assigned to templates")
	ErrCannotModifyGlobalType      = errors.New("cannot modify global document type")
)

// Process errors.
var (
	ErrProcessNotFound            = errors.New("process not found")
	ErrProcessCodeExists          = errors.New("process with this code already exists")
	ErrProcessHasTemplates        = errors.New("process is assigned to templates")
	ErrCannotModifyGlobalProcess  = errors.New("cannot modify global process")
	ErrCannotDeleteDefaultProcess = errors.New("cannot delete the default process")
)

// Template errors.
var (
	ErrTemplateNotFound      = errors.New("template not found")
	ErrTemplateAlreadyExists = errors.New("template with this title already exists")
	ErrInvalidProcessType    = errors.New("processType must be ID or CANONICAL_NAME")
	ErrProcessSlotConflict   = errors.New("a template with this document type and process already exists in the workspace")
)

// Template Version errors.
var (
	ErrVersionNotFound                 = errors.New("template version not found")
	ErrVersionAlreadyExists            = errors.New("version number already exists for this template")
	ErrVersionNameExists               = errors.New("version name already exists for this template")
	ErrVersionNotPublished             = errors.New("version must be published to promote")
	ErrVersionAlreadyPublished         = errors.New("version is already published")
	ErrCannotEditPublished             = errors.New("cannot edit published version")
	ErrCannotEditArchived              = errors.New("cannot edit archived version")
	ErrCannotEditScheduled             = errors.New("cannot edit scheduled version")
	ErrNoPublishedVersion              = errors.New("template has no published version")
	ErrCannotArchiveWithoutReplacement = errors.New("cannot schedule archive without scheduled replacement")
	ErrInvalidVersionStatus            = errors.New("invalid version status")
	ErrInvalidVersionNumber            = errors.New("invalid version number")
	ErrScheduledTimeInPast             = errors.New("scheduled time must be in the future")
	ErrInvalidContentStructure         = errors.New("invalid template content structure")
	ErrMissingRequiredVariable         = errors.New("missing required template variable")
	ErrSignerRoleNotFound              = errors.New("signer role not found")
	ErrInvalidSignerRole               = errors.New("invalid signer role configuration")
	ErrDuplicateSignerAnchor           = errors.New("duplicate signer anchor")
	ErrDuplicateSignerOrder            = errors.New("duplicate signer order")
	ErrVersionInjectableNotFound       = errors.New("version injectable not found")
	ErrContentValidationFailed         = errors.New("content validation failed")
	ErrMissingRequiredContent          = errors.New("content structure is required for publishing")
	ErrVersionDoesNotBelongToTemplate  = errors.New("version does not belong to the specified template")
	ErrUploadedSourceNotFound          = errors.New("uploaded source not found")
	ErrInvalidPDF                      = errors.New("invalid pdf")
	ErrInvalidPlacedField              = errors.New("invalid placed field")
	ErrTargetTemplateRequired          = errors.New("target template ID is required for NEW_VERSION mode")
	ErrTargetTemplateNotInWorkspace    = errors.New("target template does not belong to the destination workspace")
	ErrScheduledTimeConflict           = errors.New("another version is already scheduled at this time")
)

// Document errors.
var (
	ErrDocumentNotFound                 = errors.New("document not found")
	ErrDocumentAlreadySent              = errors.New("document already sent for signing")
	ErrDocumentCompleted                = errors.New("document signing already completed")
	ErrDocumentVoided                   = errors.New("document has been voided")
	ErrInvalidDocumentState             = errors.New("invalid document state for this operation")
	ErrInvalidDocumentStatus            = errors.New("invalid document status")
	ErrInvalidDocumentStatusTransition  = errors.New("invalid document status transition")
	ErrDocumentRecipientNotFound        = errors.New("document recipient not found")
	ErrInvalidRecipientStatus           = errors.New("invalid recipient status")
	ErrInvalidRecipientStatusTransition = errors.New("invalid recipient status transition")
	ErrDuplicateRecipientRole           = errors.New("duplicate recipient role assignment")
	ErrInvalidOperationType             = errors.New("invalid operation type")
	ErrDocumentNotCompleted             = errors.New("related document must be completed for RENEW")
	ErrDocumentNotTerminal              = errors.New("related document must be in a terminal state for AMEND")
	ErrRelatedDocumentRequired          = errors.New("related document ID is required for RENEW/AMEND operations")
	ErrRelatedDocumentSameWorkspace     = errors.New("related document must belong to the same workspace")
)

// Signing Provider errors.
var (
	ErrSigningProviderNotConfigured = errors.New("signing provider not configured")
	ErrSigningAttemptNotFound       = errors.New("signing attempt not found")
	ErrSigningProviderError         = errors.New("signing provider error")
	ErrSigningUploadFailed          = errors.New("failed to upload document to signing provider")
	ErrSigningURLFailed             = errors.New("failed to get signing URL")
	ErrSigningStatusFailed          = errors.New("failed to get document status from provider")
	ErrSigningCancelFailed          = errors.New("failed to cancel document")
	ErrInvalidWebhookSignature      = errors.New("invalid webhook signature")
	ErrWebhookProcessingFailed      = errors.New("webhook processing failed")
)

// Validation errors.
var (
	ErrValidationFailed = errors.New("validation failed")
	ErrInvalidUUID      = errors.New("invalid UUID format")
	ErrRequiredField    = errors.New("required field is missing")
	ErrFieldTooLong     = errors.New("field exceeds maximum length")
	ErrFieldTooShort    = errors.New("field is below minimum length")
)

// Database errors.
var (
	ErrDatabaseConnection = errors.New("database connection error")
	ErrDatabaseQuery      = errors.New("database query error")
	ErrOptimisticLock     = errors.New("optimistic lock conflict - record was modified")
	ErrRecordNotFound     = errors.New("record not found")
)

// Access History errors.
var (
	ErrInvalidAccessEntityType = errors.New("invalid access entity type")
)

// LLM Service errors.
var (
	ErrLLMServiceUnavailable = errors.New("AI generation service is temporarily unavailable")
)

// Renderer capacity errors.
var (
	ErrRendererBusy = errors.New("PDF renderer is at capacity, try again shortly")
)

// Automation API key errors.
var (
	ErrAPIKeyNotFound = errors.New("api key not found")
	ErrInvalidKeyType = errors.New("keyType must be 'automation' or 'internal'")
)

// ContentValidationError wraps multiple validation errors from content validation.
type ContentValidationError struct {
	Errors   []ContentValidationItem
	Warnings []ContentValidationItem
}

// ContentValidationItem represents a single validation error or warning.
type ContentValidationItem struct {
	Code    string
	Path    string
	Message string
}

// Error implements the error interface.
func (e *ContentValidationError) Error() string {
	if len(e.Errors) == 0 {
		return "content validation failed"
	}
	return "content validation failed: " + e.Errors[0].Message
}

// HasErrors returns true if there are validation errors.
func (e *ContentValidationError) HasErrors() bool {
	return len(e.Errors) > 0
}

// HasWarnings returns true if there are validation warnings.
func (e *ContentValidationError) HasWarnings() bool {
	return len(e.Warnings) > 0
}
