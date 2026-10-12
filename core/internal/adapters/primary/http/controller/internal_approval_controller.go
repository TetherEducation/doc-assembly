package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	templatesvc "github.com/TetherEducation/doc-assembly/core/internal/core/service/template"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// InternalApprovalController exposes the school-approval lifecycle to
// service-to-service callers.
//
// Separate from InternalTemplateController rather than added to it: that one holds a
// deliberately read-only slice of the template use case so its endpoints cannot
// mutate a template even by mistake. These endpoints do write, and keeping them
// apart preserves that guarantee as structure rather than as a promise.
//
// The panel is not an alternative home. doc-assembly's identity store holds Tether
// staff alone, so no school user can reach a panel route - which is why Phase 1's
// read endpoints exist here too, and why crm-admission is the only caller that can
// act on a school's behalf.
type InternalApprovalController struct {
	approvalUC templateuc.TemplateApprovalUseCase
}

// NewInternalApprovalController creates the controller.
func NewInternalApprovalController(
	approvalUC templateuc.TemplateApprovalUseCase,
) *InternalApprovalController {
	return &InternalApprovalController{approvalUC: approvalUC}
}

// RegisterRoutes registers the internal approval routes.
//
// Two groups rather than one: gin refuses a static segment and a wildcard at the
// same position, and version-scoped reads sit naturally under the version while a
// decision belongs to the approval it decides.
func (c *InternalApprovalController) RegisterRoutes(
	api *gin.RouterGroup,
	authMiddleware gin.HandlerFunc,
) {
	versions := api.Group("/internal/template-versions")
	versions.Use(authMiddleware)
	{
		versions.POST("/:versionId/approvals", c.ProposeApproval)
		versions.GET("/:versionId/approvals", c.ListApprovals)
		versions.GET("/:versionId/approval", c.GetApprovalState)
	}

	approvals := api.Group("/internal/approvals")
	approvals.Use(authMiddleware)
	{
		approvals.POST("/:approvalId/decision", c.DecideApproval)
		approvals.DELETE("/:approvalId", c.WithdrawApproval)
	}
}

// ProposeApproval puts a version's current content to the school.
// @Summary Propose a template version for school approval
// @Description Records a PENDING approval pinned to the version's content as it stands now. Refuses when a proposal is already outstanding.
// @Tags Internal
// @Accept json
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param versionId path string true "Template version ID"
// @Param request body dto.InternalProposeApprovalRequest true "Proposal"
// @Success 201 {object} dto.InternalApprovalResponse
// @Failure 400 {object} dto.InternalErrorResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Failure 409 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/template-versions/{versionId}/approvals [post]
func (c *InternalApprovalController) ProposeApproval(ctx *gin.Context) {
	var req dto.InternalProposeApprovalRequest
	if !readInternalJSON(ctx, &req) {
		return
	}
	if strings.TrimSpace(req.ProposedBy) == "" {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: "proposedBy is required",
			Code:  "INVALID_BODY",
		})
		return
	}

	workspace, ok := requireWorkspace(ctx)
	if !ok {
		return
	}

	approval, err := c.approvalUC.Propose(ctx.Request.Context(), templateuc.ProposeApprovalCommand{
		TemplateVersionID: ctx.Param("versionId"),
		ProposedBy:        strings.TrimSpace(req.ProposedBy),
		WorkspaceCode:     workspace,
	})
	if err != nil {
		writeApprovalError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, approvalToResponse(approval))
}

// DecideApproval records the school's decision.
// @Summary Record a school's decision on a proposed version
// @Description Approves or requests changes. A change request requires a comment. A proposal already decided is refused rather than overwritten.
// @Tags Internal
// @Accept json
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param approvalId path string true "Approval ID"
// @Param request body dto.InternalDecideApprovalRequest true "Decision"
// @Success 200 {object} dto.InternalApprovalResponse
// @Failure 400 {object} dto.InternalErrorResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Failure 409 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/approvals/{approvalId}/decision [post]
func (c *InternalApprovalController) DecideApproval(ctx *gin.Context) {
	var req dto.InternalDecideApprovalRequest
	if !readInternalJSON(ctx, &req) {
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		// Without a decider the row records that someone agreed but not who, which
		// is the one thing this table exists to answer.
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: "email is required: an approval must name who made it",
			Code:  "INVALID_BODY",
		})
		return
	}

	workspace, ok := requireWorkspace(ctx)
	if !ok {
		return
	}

	approval, err := c.approvalUC.Decide(ctx.Request.Context(), templateuc.DecideApprovalCommand{
		ApprovalID:    ctx.Param("approvalId"),
		WorkspaceCode: workspace,
		Approved:      req.Approved,
		Email:         strings.TrimSpace(req.Email),
		Name:          strings.TrimSpace(req.Name),
		Campus:        strings.TrimSpace(req.Campus),
		Comment:       req.Comment,
	})
	if err != nil {
		writeApprovalError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, approvalToResponse(approval))
}

// GetApprovalState answers whether a version may be published right now.
// @Summary Current approval state of a template version
// @Description Reports whether publishing is authorized, and separately whether an approval went stale because the content was edited after sign-off.
// @Tags Internal
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param versionId path string true "Template version ID"
// @Success 200 {object} dto.InternalApprovalStateResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/template-versions/{versionId}/approval [get]
func (c *InternalApprovalController) GetApprovalState(ctx *gin.Context) {
	state, err := c.approvalUC.GetState(ctx.Request.Context(), ctx.Param("versionId"))
	if err != nil {
		writeApprovalError(ctx, err)
		return
	}

	resp := dto.InternalApprovalStateResponse{
		Authorized:      state.Authorized,
		Stale:           state.Stale,
		CurrentChecksum: state.CurrentChecksum,
	}
	if state.Latest != nil {
		latest := approvalToResponse(state.Latest)
		resp.Latest = &latest
	}
	ctx.JSON(http.StatusOK, resp)
}

// ListApprovals returns a version's approval history, newest first.
// @Summary Approval history for a template version
// @Tags Internal
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param versionId path string true "Template version ID"
// @Success 200 {object} dto.InternalApprovalHistoryResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/template-versions/{versionId}/approvals [get]
func (c *InternalApprovalController) ListApprovals(ctx *gin.Context) {
	approvals, err := c.approvalUC.ListHistory(ctx.Request.Context(), ctx.Param("versionId"))
	if err != nil {
		writeApprovalError(ctx, err)
		return
	}

	// An empty history is an empty list, never null: a caller iterating the response
	// should not have to special-case "never proposed".
	items := make([]dto.InternalApprovalResponse, 0, len(approvals))
	for _, a := range approvals {
		items = append(items, approvalToResponse(a))
	}
	ctx.JSON(http.StatusOK, dto.InternalApprovalHistoryResponse{Items: items})
}

// WithdrawApproval retracts a proposal that has not been decided.
// @Summary Withdraw an undecided proposal
// @Description Removes a PENDING proposal so one sent by mistake need not be answered by the school. A decided approval is never retracted.
// @Tags Internal
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param X-Workspace-Code header string true "Workspace business code"
// @Param approvalId path string true "Approval ID"
// @Success 204
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Failure 409 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/approvals/{approvalId} [delete]
func (c *InternalApprovalController) WithdrawApproval(ctx *gin.Context) {
	workspace, ok := requireWorkspace(ctx)
	if !ok {
		return
	}
	if err := c.approvalUC.Withdraw(ctx.Request.Context(), ctx.Param("approvalId"), workspace); err != nil {
		writeApprovalError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

// requireWorkspace reads the workspace the caller claims to be acting for.
//
// The internal API key alone only proves "a Tether service is calling". Which
// school it is calling for comes from this header and is verified against the
// template downstream, matching how every other internal endpoint scopes itself.
func requireWorkspace(ctx *gin.Context) (string, bool) {
	workspace := strings.TrimSpace(ctx.GetHeader(HeaderWorkspaceCode))
	if workspace == "" {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error:   "missing required header",
			Code:    "MISSING_HEADERS",
			Details: []string{HeaderWorkspaceCode},
		})
		return "", false
	}
	return workspace, true
}

// readInternalJSON decodes a required JSON body, answering 400 on malformed input.
func readInternalJSON(ctx *gin.Context, target any) bool {
	if ctx.Request.Body == nil {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: "a request body is required", Code: "INVALID_BODY",
		})
		return false
	}
	raw, err := io.ReadAll(ctx.Request.Body)
	if err != nil || len(raw) == 0 {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: "a request body is required", Code: "INVALID_BODY",
		})
		return false
	}
	if err := json.Unmarshal(raw, target); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: "invalid request body", Code: "INVALID_BODY",
		})
		return false
	}
	return true
}

// writeApprovalError maps domain errors onto status codes.
//
// The distinctions matter to the caller: 404 means propose it first, 409 means
// someone got there before you, 400 means the request itself was wrong. Collapsing
// them into 500 would make the CRM unable to say anything useful to a school.
func writeApprovalError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, entity.ErrApprovalNotFound):
		ctx.JSON(http.StatusNotFound, dto.InternalErrorResponse{
			Error: err.Error(), Code: "APPROVAL_NOT_FOUND",
		})
	case errors.Is(err, entity.ErrApprovalAlreadyDecided):
		ctx.JSON(http.StatusConflict, dto.InternalErrorResponse{
			Error: err.Error(), Code: "APPROVAL_ALREADY_DECIDED",
		})
	case errors.Is(err, entity.ErrApprovalContentUnchanged):
		ctx.JSON(http.StatusConflict, dto.InternalErrorResponse{
			Error: err.Error(), Code: "APPROVAL_CONTENT_UNCHANGED",
		})
	case errors.Is(err, entity.ErrApprovalNotPending):
		ctx.JSON(http.StatusConflict, dto.InternalErrorResponse{
			Error: err.Error(), Code: "APPROVAL_NOT_PENDING",
		})
	case errors.Is(err, templatesvc.ErrWorkspaceMismatch),
		errors.Is(err, templatesvc.ErrWorkspaceScopeRequired):
		// 403, not 404: the caller asked about something real and was refused.
		ctx.JSON(http.StatusForbidden, dto.InternalErrorResponse{
			Error: err.Error(), Code: "WORKSPACE_SCOPE",
		})
	case errors.Is(err, entity.ErrApprovalDeciderRequired),
		errors.Is(err, entity.ErrApprovalCommentRequired):
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: err.Error(), Code: "APPROVAL_COMMENT_REQUIRED",
		})
	default:
		HandleError(ctx, err)
	}
}

func approvalToResponse(a *entity.TemplateVersionApproval) dto.InternalApprovalResponse {
	return dto.InternalApprovalResponse{
		ID:                a.ID,
		TemplateVersionID: a.TemplateVersionID,
		Status:            string(a.Status),
		ContentChecksum:   a.ContentChecksum,
		ProposedBy:        a.ProposedBy,
		ProposedAt:        a.ProposedAt,
		DecidedByEmail:    a.DecidedByEmail,
		DecidedByName:     a.DecidedByName,
		DecidedAtCampus:   a.DecidedAtCampus,
		DecidedAt:         a.DecidedAt,
		Comment:           a.Comment,
	}
}
