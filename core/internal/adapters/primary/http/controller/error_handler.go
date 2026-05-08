package controller

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	galleryuc "github.com/rendis/doc-assembly/core/internal/core/usecase/gallery"
)

var notFoundErrors = []error{
	entity.ErrGalleryAssetNotFound,
	entity.ErrInjectableNotFound,
	entity.ErrTemplateNotFound,
	entity.ErrTagNotFound,
	entity.ErrVersionNotFound,
	entity.ErrSignerRoleNotFound,
	entity.ErrVersionInjectableNotFound,
	entity.ErrWorkspaceNotFound,
	entity.ErrFolderNotFound,
	entity.ErrUserNotFound,
	entity.ErrMemberNotFound,
	entity.ErrTenantNotFound,
	entity.ErrTenantMemberNotFound,
	entity.ErrSystemRoleNotFound,
	entity.ErrDocumentTypeNotFound,
	entity.ErrDocumentNotFound,
	entity.ErrSigningAttemptNotFound,
	entity.ErrProcessNotFound,
	entity.ErrAPIKeyNotFound,
	entity.ErrInternalTemplateResolutionNotFound,
}

var conflictErrors = []error{
	entity.ErrInjectableAlreadyExists,
	entity.ErrTemplateAlreadyExists,
	entity.ErrVersionAlreadyExists,
	entity.ErrVersionNameExists,
	entity.ErrDuplicateSignerAnchor,
	entity.ErrDuplicateSignerOrder,
	entity.ErrWorkspaceAlreadyExists,
	entity.ErrWorkspaceCodeExists,
	entity.ErrFolderAlreadyExists,
	entity.ErrTagAlreadyExists,
	entity.ErrSystemWorkspaceExists,
	entity.ErrMemberAlreadyExists,
	entity.ErrTenantAlreadyExists,
	entity.ErrGlobalWorkspaceExists,
	entity.ErrTenantMemberExists,
	entity.ErrScheduledTimeConflict,
	entity.ErrDocumentTypeCodeExists,
	entity.ErrDocumentTypeAlreadyAssigned,
	entity.ErrProcessCodeExists,
	entity.ErrInvalidDocumentState,
}

var badRequestErrors = []error{
	galleryuc.ErrQueryRequired,
	galleryuc.ErrAssetKeyRequired,
	galleryuc.ErrUploadEmpty,
	entity.ErrGalleryInvalidContentType,
	entity.ErrGalleryFileTooLarge,
	entity.ErrInjectableInUse,
	entity.ErrNoPublishedVersion,
	entity.ErrInvalidInjectableKey,
	entity.ErrRequiredField,
	entity.ErrFieldTooLong,
	entity.ErrInvalidDataType,
	entity.ErrCannotEditPublished,
	entity.ErrCannotEditArchived,
	entity.ErrVersionNotPublished,
	entity.ErrVersionAlreadyPublished,
	entity.ErrCannotArchiveWithoutReplacement,
	entity.ErrInvalidVersionStatus,
	entity.ErrInvalidVersionNumber,
	entity.ErrScheduledTimeInPast,
	entity.ErrInvalidSignerRole,
	entity.ErrFolderHasChildren,
	entity.ErrFolderHasTemplates,
	entity.ErrTagInUse,
	entity.ErrCircularReference,
	entity.ErrCannotArchiveSystem,
	entity.ErrInvalidParentFolder,
	entity.ErrCannotRemoveOwner,
	entity.ErrCannotRemoveLastSystemMember,
	entity.ErrInvalidRole,
	entity.ErrInvalidTenantCode,
	entity.ErrInvalidWorkspaceType,
	entity.ErrInvalidWorkspaceCode,
	entity.ErrInvalidSystemRole,
	entity.ErrMissingTenantID,
	entity.ErrMissingWorkspaceID,
	entity.ErrCannotRemoveTenantOwner,
	entity.ErrInvalidTenantRole,
	entity.ErrVersionDoesNotBelongToTemplate,
	entity.ErrTargetTemplateRequired,
	entity.ErrTargetTemplateNotInWorkspace,
	entity.ErrOnlyTextTypeAllowed,
	entity.ErrWorkspaceIDRequired,
	entity.ErrCannotModifyGlobal,
	entity.ErrDocumentTypeCodeImmutable,
	entity.ErrDocumentTypeHasTemplates,
	entity.ErrProcessHasTemplates,
	entity.ErrCannotModifyGlobalProcess,
	entity.ErrCannotDeleteDefaultProcess,
	entity.ErrInvalidOperationType,
	entity.ErrDocumentNotCompleted,
	entity.ErrDocumentNotTerminal,
	entity.ErrRelatedDocumentRequired,
	entity.ErrRelatedDocumentSameWorkspace,
}

var forbiddenErrors = []error{
	entity.ErrWorkspaceAccessDenied,
	entity.ErrForbidden,
	entity.ErrInsufficientRole,
	entity.ErrTenantAccessDenied,
}

var unauthorizedErrors = []error{
	entity.ErrUnauthorized,
	entity.ErrMissingAPIKey,
	entity.ErrInvalidAPIKey,
}

var tooManyRequestErrors = []error{
	entity.ErrTooManyRequests,
}

var unavailableErrors = []error{
	entity.ErrLLMServiceUnavailable,
	entity.ErrRendererBusy,
}

// respondError sends an error response.
func respondError(ctx *gin.Context, statusCode int, err error) {
	ctx.JSON(statusCode, dto.NewErrorResponse(err))
}

// HandleError maps domain errors to HTTP status codes.
// This is a centralized error handler that consolidates all error handling logic
// from the various controller-specific error handlers.
func HandleError(ctx *gin.Context, err error) {
	// Check for ContentValidationError first (special handling)
	var validationErr *entity.ContentValidationError
	if errors.As(err, &validationErr) {
		ctx.JSON(http.StatusUnprocessableEntity, dto.NewContentValidationErrorResponse(validationErr))
		return
	}

	// Check for MissingInjectablesError (special handling)
	var missingInjectablesErr *entity.MissingInjectablesError
	if errors.As(err, &missingInjectablesErr) {
		ctx.JSON(http.StatusBadRequest, dto.NewMissingInjectablesErrorResponse(missingInjectablesErr))
		return
	}

	// Check for RecipientValidationError (special handling)
	var recipientErr *entity.RecipientValidationError
	if errors.As(err, &recipientErr) {
		ctx.JSON(http.StatusUnprocessableEntity, dto.NewRecipientValidationErrorResponse(recipientErr))
		return
	}

	statusCode := mapErrorToStatusCode(err)
	//nolint:staticcheck
	switch {
	case statusCode == http.StatusInternalServerError:
		slog.ErrorContext(ctx.Request.Context(), "unhandled error", slog.Any("error", err))
	case statusCode == http.StatusNotFound:
		slog.WarnContext(ctx.Request.Context(), "not found",
			slog.String("error", err.Error()),
			slog.String("path", ctx.Request.URL.Path),
		)
	}
	respondError(ctx, statusCode, err)
}

// mapErrorToStatusCode determines the appropriate HTTP status code for an error.
func mapErrorToStatusCode(err error) int {
	switch {
	case is404Error(err):
		return http.StatusNotFound
	case is409Error(err):
		return http.StatusConflict
	case is400Error(err):
		return http.StatusBadRequest
	case is403Error(err):
		return http.StatusForbidden
	case is401Error(err):
		return http.StatusUnauthorized
	case is429Error(err):
		return http.StatusTooManyRequests
	case is503Error(err):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// is404Error returns true if the error should result in a 404 Not Found response.
//
//nolint:gocyclo // Simple list of domain error checks by status code.
func is404Error(err error) bool {
	return isAnyError(err, notFoundErrors...)
}

// is409Error returns true if the error should result in a 409 Conflict response.
//
//nolint:gocyclo // Simple list of error checks that grows with features
func is409Error(err error) bool {
	return isAnyError(err, conflictErrors...)
}

// is400Error returns true if the error should result in a 400 Bad Request response.
func is400Error(err error) bool {
	return isAnyError(err, badRequestErrors...)
}

// is403Error returns true if the error should result in a 403 Forbidden response.
func is403Error(err error) bool {
	return isAnyError(err, forbiddenErrors...)
}

// is401Error returns true if the error should result in a 401 Unauthorized response.
func is401Error(err error) bool {
	return isAnyError(err, unauthorizedErrors...)
}

// is429Error returns true if the error should result in a 429 Too Many Requests response.
func is429Error(err error) bool {
	return isAnyError(err, tooManyRequestErrors...)
}

// is503Error returns true if the error should result in a 503 Service Unavailable response.
func is503Error(err error) bool {
	return isAnyError(err, unavailableErrors...)
}

func isAnyError(err error, candidates ...error) bool {
	for _, candidate := range candidates {
		if errors.Is(err, candidate) {
			return true
		}
	}
	return false
}
