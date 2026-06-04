package sdk

import (
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// SystemWorkspaceCode is the canonical code for the tenant/system administration workspace.
// It is not eligible for template resolution or template-created documents.
const SystemWorkspaceCode = "SYS_WRKSP"

// --- Core Extensibility Interfaces ---

// Injector resolves injectable values during document rendering.
type Injector = port.Injector

// InitFunc is called once before all injectors on each render request.
type InitFunc = port.InitFunc

// ResolveFunc resolves a single injectable value.
type ResolveFunc = port.ResolveFunc

// RequestMapper transforms incoming render requests before processing.
type RequestMapper = port.RequestMapper

// MapperContext provides request context to the mapper.
type MapperContext = port.MapperContext

// TemplateResolver allows custom internal template version selection.
type TemplateResolver = port.TemplateResolver

// TemplateResolverRequest provides context for custom template resolution.
type TemplateResolverRequest = port.TemplateResolverRequest

// InternalTemplateContextSearchAdapter exposes full context search to resolvers.
type InternalTemplateContextSearchAdapter = port.InternalTemplateContextSearchAdapter

// InternalTemplateContextSearchParams are filters for full context search.
type InternalTemplateContextSearchParams = port.InternalTemplateContextSearchParams

// InternalTemplateContext is the full context resolved for internal create.
type InternalTemplateContext = port.InternalTemplateContext

// --- Process Resolution ---

// ProcessResolver provides process discovery and validation.
type ProcessResolver = port.ProcessResolver

// ProcessInfo describes a process available for a tenant.
type ProcessInfo = port.ProcessInfo

// --- Provider Interfaces ---

// WorkspaceInjectableProvider supplies workspace-specific injectable definitions.
type WorkspaceInjectableProvider = port.WorkspaceInjectableProvider

// FilteredWorkspaceInjectableProvider optionally supplies only requested injectable definitions.
type FilteredWorkspaceInjectableProvider = port.FilteredWorkspaceInjectableProvider

// PublicDocumentAccessAuthenticator provides custom auth for /public/doc/:documentId.
type PublicDocumentAccessAuthenticator = port.PublicDocumentAccessAuthenticator

// SigningSessionAuthenticator provides custom auth for
// /api/v1/signing-sessions/:documentId.
type SigningSessionAuthenticator = port.SigningSessionAuthenticator

// ReadOnlyViewLinkAuthenticator provides custom auth for
// /api/v1/documents/:documentId/view-link.
type ReadOnlyViewLinkAuthenticator = port.ReadOnlyViewLinkAuthenticator

// SigningProvider handles document signing via an external provider.
type SigningProvider = port.SigningProvider

// WebhookHandler parses incoming webhook events from a signing provider.
type WebhookHandler = port.WebhookHandler

// StorageAdapter handles file storage (local, S3, etc.).
type StorageAdapter = port.StorageAdapter

// NotificationProvider sends notifications (email, etc.).
type NotificationProvider = port.NotificationProvider

// --- Optional Schema Interfaces ---

// TableSchemaProvider can be implemented by Injector to expose table column schema.
type TableSchemaProvider = port.TableSchemaProvider

// ListSchemaProvider can be implemented by Injector to expose list schema.
type ListSchemaProvider = port.ListSchemaProvider

// --- Provider DTOs ---

// WorkspaceInjectableProvider types
type (
	GetInjectablesResult      = port.GetInjectablesResult
	ProviderInjectable        = port.ProviderInjectable
	ProviderFormat            = port.ProviderFormat
	ProviderGroup             = port.ProviderGroup
	ResolveInjectablesRequest = port.ResolveInjectablesRequest
	ResolveInjectablesResult  = port.ResolveInjectablesResult
)

// PublicDocumentAccessAuthenticator types
type PublicDocumentAccessClaims = port.PublicDocumentAccessClaims

// SigningSessionAuthenticator types
type SigningSessionAuthClaims = port.SigningSessionAuthClaims

// ReadOnlyViewLinkAuthenticator types
type ReadOnlyViewLinkAuthClaims = port.ReadOnlyViewLinkAuthClaims

// SigningProvider types
type (
	SubmitAttemptDocumentRequest          = port.SubmitAttemptDocumentRequest
	SubmitAttemptDocumentResult           = port.SubmitAttemptDocumentResult
	FindProviderDocumentRequest           = port.FindProviderDocumentRequest
	ProviderDocumentResult                = port.ProviderDocumentResult
	GetProviderDocumentStatusRequest      = port.GetProviderDocumentStatusRequest
	ProviderDocumentStatusResult          = port.ProviderDocumentStatusResult
	GetAttemptRecipientEmbeddedURLRequest = port.GetAttemptRecipientEmbeddedURLRequest
	GetAttemptRecipientEmbeddedURLResult  = port.GetAttemptRecipientEmbeddedURLResult
	CleanupProviderDocumentRequest        = port.CleanupProviderDocumentRequest
	CleanupProviderDocumentResult         = port.CleanupProviderDocumentResult
	ProviderCapabilities                  = port.ProviderCapabilities
	ProviderError                         = port.ProviderError
	SigningRecipient                      = port.SigningRecipient
	SignatureFieldPosition                = port.SignatureFieldPosition
	RecipientResult                       = port.RecipientResult
	RecipientStatusResult                 = port.RecipientStatusResult
	WebhookEvent                          = port.WebhookEvent
)

// NotificationProvider types
type (
	NotificationRequest    = port.NotificationRequest
	NotificationAttachment = port.NotificationAttachment
)

// PDF Renderer types
type (
	SignatureField  = port.SignatureField
	SignerRoleValue = port.SignerRoleValue
)

// Registry types
type GroupConfig = port.GroupConfig

// Environment types
type Environment = entity.Environment

// Environment constants and helpers
var (
	EnvironmentProd        = entity.EnvironmentProd
	EnvironmentDev         = entity.EnvironmentDev
	ParseEnvironment       = entity.ParseEnvironment
	EnvironmentFromSandbox = entity.EnvironmentFromSandbox
)

// SigningProvider request types
type ParseWebhookRequest = port.ParseWebhookRequest

// StorageAdapter request types
type (
	StorageUploadRequest = port.StorageUploadRequest
	StorageRequest       = port.StorageRequest
)

// PublicDocumentAccessAuthenticator request type
type AuthenticateRequest = port.AuthenticateRequest

// SigningSessionAuthenticator request type
type SigningSessionAuthenticateRequest = port.SigningSessionAuthenticateRequest

// ReadOnlyViewLinkAuthenticator request type
type ReadOnlyViewLinkAuthenticateRequest = port.ReadOnlyViewLinkAuthenticateRequest
