package controller

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// handledWith runs HandleError on err and returns what it answered.
func handledWith(t *testing.T, err error) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workspace/tags", nil)

	HandleError(ctx, err)

	return recorder
}

// A domain error that the request itself causes answers with a 4xx status and
// with its own text, whether the service returns it bare or wrapped.
func TestHandleError_CallerErrorStatus(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{entity.ErrSystemInjectableNotFound, http.StatusNotFound},
		{entity.ErrAssignmentNotFound, http.StatusNotFound},

		{entity.ErrProcessSlotConflict, http.StatusConflict},

		{entity.ErrCannotEditScheduled, http.StatusBadRequest},
		{entity.ErrCannotPromoteToSandbox, http.StatusBadRequest},
		{entity.ErrCannotModifySystemTenant, http.StatusBadRequest},
		{entity.ErrCannotModifySystemWorkspace, http.StatusBadRequest},
		{entity.ErrFieldTooShort, http.StatusBadRequest},
		{entity.ErrTenantIDRequired, http.StatusBadRequest},
		{entity.ErrInvalidKeyType, http.StatusBadRequest},

		// The request DTO rejects these values first; the service's own check
		// answers the same.
		{entity.ErrInvalidProcessType, http.StatusBadRequest},
		{entity.ErrInvalidScopeType, http.StatusBadRequest},
		{entity.ErrInvalidTenantStatus, http.StatusBadRequest},
		{entity.ErrInvalidTagColor, http.StatusBadRequest},
		{entity.ErrInvalidEmail, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			for _, err := range []error{tc.err, fmt.Errorf("validating request: %w", tc.err)} {
				recorder := handledWith(t, err)

				assert.Equal(t, tc.status, recorder.Code)
				var body dto.ErrorResponse
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
				assert.Equal(t, err.Error(), body.Error)
			}
		})
	}
}

// Every error in a status list answers with that list's status: the lists are
// checked in a fixed order, so an error in two of them would get the first.
func TestStatusLists_AnswerTheirStatus(t *testing.T) {
	lists := map[int][]error{
		http.StatusNotFound:           notFoundErrors,
		http.StatusConflict:           conflictErrors,
		http.StatusBadRequest:         badRequestErrors,
		http.StatusForbidden:          forbiddenErrors,
		http.StatusUnauthorized:       unauthorizedErrors,
		http.StatusTooManyRequests:    tooManyRequestErrors,
		http.StatusServiceUnavailable: unavailableErrors,
	}
	for status, list := range lists {
		for _, err := range list {
			assert.Equalf(t, status, mapErrorToStatusCode(err), "%q", err)
		}
	}
}

// sentinelsWithoutStatus names the entity sentinels that error_handler.go does
// not name: one that reached HandleError would answer 500. Each group says why
// its members have no status.
var sentinelsWithoutStatus = []string{
	// Not the request's doing: a deployment without a request mapper, a
	// published version without content, a row that another row points to
	// and that is gone.
	"ErrNoMapperRegistered",
	"ErrMissingRequiredContent",
	"ErrRecordNotFound",

	// Checks on values the server sets itself: failing one is a server bug.
	"ErrInvalidDocumentStatus",
	"ErrInvalidDocumentStatusTransition",
	"ErrInvalidInjectableSource",
	"ErrInvalidMembershipStatus",
	"ErrInvalidRecipientStatus",
	"ErrInvalidUserStatus",
	"ErrInvalidWorkspaceStatus",

	// Dealt with before HandleError: by the authentication, tenant and
	// sandbox middlewares, or by the approval, public signing and webhook
	// controllers, which answer for themselves.
	"ErrApprovalAlreadyDecided",
	"ErrApprovalCommentRequired",
	"ErrApprovalContentUnchanged",
	"ErrApprovalDeciderRequired",
	"ErrApprovalNotFound",
	"ErrApprovalNotPending",
	"ErrInvalidRecipientStatusTransition",
	"ErrInvalidTenantID",
	"ErrInvalidToken",
	"ErrInvalidWebhookSignature",
	"ErrMissingToken",
	"ErrSandboxNotFound",
	"ErrSandboxNotSupported",
	"ErrTokenExpired",
	"ErrUnknownIssuer",

	// Returned only by code that no route calls.
	"ErrInvalidAccessEntityType",
	"ErrOptimisticLock",
	"ErrUserSuspended",
	"ErrWorkspaceArchived",
	"ErrWorkspaceSuspended",

	// Returned by no code.
	"ErrContentValidationFailed",
	"ErrDatabaseConnection",
	"ErrDatabaseQuery",
	"ErrDocumentAlreadySent",
	"ErrDocumentCompleted",
	"ErrDocumentVoided",
	"ErrEmailAlreadyInUse",
	"ErrInvalidContentStructure",
	"ErrInvalidUUID",
	"ErrInvalidUserID",
	"ErrInvalidWorkspaceID",
	"ErrMembershipPending",
	"ErrMissingRequiredVariable",
	"ErrMissingUserID",
	"ErrSigningCancelFailed",
	"ErrSigningProviderError",
	"ErrSigningProviderNotConfigured",
	"ErrSigningStatusFailed",
	"ErrSigningURLFailed",
	"ErrSigningUploadFailed",
	"ErrSystemRoleExists",
	"ErrTemplateInjectableNotFound",
	"ErrUserAlreadyExists",
	"ErrUserNotInvited",
	"ErrValidationFailed",
	"ErrWebhookProcessingFailed",

	// Caused by the request, and not given a status here.
	"ErrCannotModifyGlobalType",
}

// Every Err* sentinel of the entity package is either named in
// error_handler.go, which is how it gets a status, or listed in
// sentinelsWithoutStatus. A new sentinel that is in neither fails here, so
// that answering 500 for it is a decision and not an oversight.
func TestEntitySentinels_HaveAStatusOrAreListed(t *testing.T) {
	declared := declaredEntitySentinels(t)
	handled := errorHandlerSentinels(t)

	listed := make(map[string]bool, len(sentinelsWithoutStatus))
	for _, name := range sentinelsWithoutStatus {
		assert.Falsef(t, listed[name], "%s is in sentinelsWithoutStatus twice", name)
		listed[name] = true
		assert.Truef(t, declared[name],
			"%s is in sentinelsWithoutStatus but the entity package does not declare it", name)
	}

	names := make([]string, 0, len(declared))
	for name := range declared {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		assert.Truef(t, handled[name] || listed[name],
			"entity.%s has no status: add it to a status list in error_handler.go, "+
				"or to sentinelsWithoutStatus if it has to answer 500", name)
	}
}

// isSentinelName reports whether name follows the sentinel convention: Err
// followed by an upper-case letter.
func isSentinelName(name string) bool {
	rest, found := strings.CutPrefix(name, "Err")
	return found && rest != "" && unicode.IsUpper(rune(rest[0]))
}

// declaredEntitySentinels returns the names of the package-level Err*
// variables of the entity package, read from its source.
func declaredEntitySentinels(t *testing.T) map[string]bool {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "core", "entity")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	names := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, variable := range packageVariables(file) {
			if isSentinelName(variable) {
				names[variable] = true
			}
		}
	}
	require.NotEmpty(t, names, "no sentinels found in %s", dir)
	return names
}

// packageVariables returns the names of the variables file declares at
// package level.
func packageVariables(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			for _, ident := range spec.(*ast.ValueSpec).Names {
				names = append(names, ident.Name)
			}
		}
	}
	return names
}

// errorHandlerSentinels returns the entity sentinels that error_handler.go
// names, in a status list or in code of its own.
func errorHandlerSentinels(t *testing.T) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "error_handler.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)

	names := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "entity" && isSentinelName(selector.Sel.Name) {
			names[selector.Sel.Name] = true
		}
		return true
	})
	require.NotEmpty(t, names, "no entity sentinels found in error_handler.go")
	return names
}
