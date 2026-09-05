package controller

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
	injectableuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/injectable"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// nonAlphanumericRe matches any character that is not uppercase A-Z or 0-9.
var nonAlphanumericRe = regexp.MustCompile(`[^A-Z0-9]+`)

const (
	// automationRequestTimeout is a generous timeout for automation API requests.
	automationRequestTimeout = 60 * time.Second
)

// AutomationController handles all /api/v1/automation/* routes.
// It uses automation API key authentication and audit logging.
type AutomationController struct {
	tenantUC          organizationuc.TenantUseCase
	workspaceUC       organizationuc.WorkspaceUseCase
	injectableUC      injectableuc.InjectableUseCase
	templateUC        templateuc.TemplateUseCase
	templateVersionUC templateuc.TemplateVersionUseCase
	documentTypeUC    cataloguc.DocumentTypeUseCase
	keyRepo           port.AutomationAPIKeyRepository
	auditRepo         port.AutomationAuditLogRepository
	templateMapper    *mapper.TemplateMapper
	versionMapper     *mapper.TemplateVersionMapper
	injectableMapper  *mapper.InjectableMapper
	docTypeMapper     *mapper.DocumentTypeMapper
}

// NewAutomationController creates a new AutomationController.
func NewAutomationController(
	tenantUC organizationuc.TenantUseCase,
	workspaceUC organizationuc.WorkspaceUseCase,
	injectableUC injectableuc.InjectableUseCase,
	templateUC templateuc.TemplateUseCase,
	templateVersionUC templateuc.TemplateVersionUseCase,
	documentTypeUC cataloguc.DocumentTypeUseCase,
	keyRepo port.AutomationAPIKeyRepository,
	auditRepo port.AutomationAuditLogRepository,
	templateMapper *mapper.TemplateMapper,
	versionMapper *mapper.TemplateVersionMapper,
	injectableMapper *mapper.InjectableMapper,
	docTypeMapper *mapper.DocumentTypeMapper,
) *AutomationController {
	return &AutomationController{
		tenantUC:          tenantUC,
		workspaceUC:       workspaceUC,
		injectableUC:      injectableUC,
		templateUC:        templateUC,
		templateVersionUC: templateVersionUC,
		documentTypeUC:    documentTypeUC,
		keyRepo:           keyRepo,
		auditRepo:         auditRepo,
		templateMapper:    templateMapper,
		versionMapper:     versionMapper,
		injectableMapper:  injectableMapper,
		docTypeMapper:     docTypeMapper,
	}
}

// RegisterRoutes sets up the /api/v1/automation route group with its own middleware chain.
//
// Takes gin.IRouter rather than *gin.Engine so the caller can mount this under the
// configured server base path. Mounting on the engine root instead made these routes
// unreachable wherever a base path is set: in production DOC_ENGINE_SERVER_BASE_PATH is
// "/doc-assembly" and the load balancer only forwards /doc-assembly/*, so every
// /api/v1/automation/* request 404'd before reaching the handler. It went unnoticed
// because the test server runs with an empty base path, where engine root and base are
// the same router.
func (ctrl *AutomationController) RegisterRoutes(base gin.IRouter, middlewareProvider *middleware.Provider, maxBodyBytes int64) {
	g := base.Group("/api/v1/automation")
	g.Use(middleware.Operation())
	g.Use(middleware.RequestTimeout(automationRequestTimeout))
	g.Use(middleware.AutomationKeyAuth(ctrl.keyRepo))
	// The body cap must precede the audit logger, which buffers the whole body and answers
	// 413 when the cap is hit.
	g.Use(middleware.RequestBodyLimit(maxBodyBytes))
	g.Use(middleware.AutomationAuditLogger(ctrl.auditRepo))

	// Tenants
	g.GET("/tenants", ctrl.listTenants)

	// Workspaces (tenant-scoped)
	g.GET("/tenants/:tenantId/workspaces", ctrl.listWorkspaces)
	g.POST("/tenants/:tenantId/workspaces", ctrl.createWorkspace)
	g.PATCH("/tenants/:tenantId/workspaces/:workspaceId", ctrl.updateWorkspace)
	g.POST("/tenants/:tenantId/workspaces/:workspaceId/suspend", ctrl.suspendWorkspace)
	g.POST("/tenants/:tenantId/workspaces/:workspaceId/activate", ctrl.activateWorkspace)
	g.POST("/tenants/:tenantId/workspaces/:workspaceId/archive", ctrl.archiveWorkspace)

	// Workspace-scoped routes with sandbox support.
	// AutomationSandboxContext reads :workspaceId from the URL path,
	// and when X-Sandbox-Mode: true is set, resolves/auto-creates the sandbox workspace.
	ws := g.Group("")
	ws.Use(middlewareProvider.AutomationSandboxContext())

	// Injectables (workspace-scoped)
	ws.GET("/workspaces/:workspaceId/injectables", ctrl.listInjectables)

	// Templates (workspace-scoped)
	ws.GET("/workspaces/:workspaceId/templates", ctrl.listTemplates)
	ws.POST("/workspaces/:workspaceId/templates", ctrl.createTemplate)
	ws.GET("/workspaces/:workspaceId/templates/:templateId", ctrl.getTemplate)
	ws.PATCH("/workspaces/:workspaceId/templates/:templateId", ctrl.updateTemplate)

	// Template process fields
	g.PUT("/templates/:templateId/process", ctrl.setProcessFields)

	// Document Types
	g.GET("/document-types", ctrl.listDocumentTypes)
	g.POST("/templates/:templateId/document-type", ctrl.assignDocumentType)

	// Versions
	g.GET("/templates/:templateId/versions", ctrl.listVersions)
	g.POST("/templates/:templateId/versions", ctrl.createVersion)
	g.GET("/templates/:templateId/versions/:versionId", ctrl.getVersion)
	g.PATCH("/templates/:templateId/versions/:versionId", ctrl.updateVersion)
	g.POST("/templates/:templateId/versions/:versionId/publish", ctrl.publishVersion)
	g.POST("/templates/:templateId/versions/:versionId/archive", ctrl.archiveVersion)
	g.GET("/templates/:templateId/versions/:versionId/content", ctrl.getVersionContent)
	g.PUT("/templates/:templateId/versions/:versionId/content", ctrl.updateVersionContent)
}

// checkTenantAccess verifies the API key has access to the given tenant.
// Returns false and writes a 403 response if access is denied.
func (ctrl *AutomationController) checkTenantAccess(c *gin.Context, tenantID string) bool {
	allowed := middleware.GetAutomationAllowedTenants(c)
	if len(allowed) == 0 {
		return true // global access
	}
	for _, t := range allowed {
		if t == tenantID {
			return true
		}
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "API key does not have access to this tenant"})
	return false
}

// --- Tenant Handlers ---

// listTenants lists tenants (filtered by allowedTenants if the key is restricted).
// @Summary List tenants
// @Tags Automation
// @Produce json
// @Success 200 {object} dto.ListResponse[dto.TenantResponse]
// @Failure 401 {object} dto.ErrorResponse
// @Router /api/v1/automation/tenants [get]
// @Security AutomationKey
func (ctrl *AutomationController) listTenants(c *gin.Context) {
	filters := port.TenantFilters{
		Limit:  100,
		Offset: 0,
	}

	tenants, _, err := ctrl.tenantUC.ListTenantsPaginated(c.Request.Context(), filters)
	if err != nil {
		HandleError(c, err)
		return
	}

	// If the key has restricted access, filter to allowed tenants only.
	allowed := middleware.GetAutomationAllowedTenants(c)
	if len(allowed) > 0 {
		allowedSet := make(map[string]struct{}, len(allowed))
		for _, id := range allowed {
			allowedSet[id] = struct{}{}
		}
		filtered := make([]*entity.Tenant, 0, len(allowed))
		for _, t := range tenants {
			if _, ok := allowedSet[t.ID]; ok {
				filtered = append(filtered, t)
			}
		}
		tenants = filtered
	}

	responses := mapper.TenantsToResponses(tenants)
	c.JSON(http.StatusOK, dto.NewListResponse(responses))
}

// --- Workspace Handlers ---

// listWorkspaces lists workspaces for a tenant.
// @Summary List workspaces in tenant
// @Tags Automation
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Success 200 {object} dto.ListResponse[dto.WorkspaceResponse]
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/automation/tenants/{tenantId}/workspaces [get]
// @Security AutomationKey
func (ctrl *AutomationController) listWorkspaces(c *gin.Context) {
	tenantID := c.Param("tenantId")
	if !ctrl.checkTenantAccess(c, tenantID) {
		return
	}

	// automation API returns all workspaces, using a high limit
	filters := port.WorkspaceFilters{
		Limit:  1000,
		Offset: 0,
	}

	workspaces, _, err := ctrl.tenantUC.ListTenantWorkspaces(c.Request.Context(), tenantID, filters)
	if err != nil {
		HandleError(c, err)
		return
	}

	responses := mapper.WorkspacesToResponses(workspaces)
	c.JSON(http.StatusOK, dto.NewListResponse(responses))
}

// createWorkspace creates a new workspace in a tenant.
// @Summary Create workspace in tenant
// @Tags Automation
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Param request body dto.AutomationCreateWorkspaceRequest true "Workspace data"
// @Success 201 {object} dto.WorkspaceResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 413 {object} dto.ErrorResponse
// @Router /api/v1/automation/tenants/{tenantId}/workspaces [post]
// @Security AutomationKey
func (ctrl *AutomationController) createWorkspace(c *gin.Context) {
	tenantID := c.Param("tenantId")
	if !ctrl.checkTenantAccess(c, tenantID) {
		return
	}

	var req dto.AutomationCreateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	// Derive workspace code: use provided code or generate one from the name.
	code := req.Code
	if code == "" {
		code = generateWorkspaceCode(req.Name)
	}

	cmd := organizationuc.CreateWorkspaceCommand{
		TenantID:  &tenantID,
		Name:      req.Name,
		Code:      code,
		Type:      entity.WorkspaceTypeClient,
		CreatedBy: "",
	}

	workspace, err := ctrl.workspaceUC.CreateWorkspace(c.Request.Context(), cmd)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, mapper.WorkspaceToResponse(workspace))
}

// updateWorkspace partially updates a workspace's name and/or code.
// @Summary Update workspace in tenant
// @Tags Automation
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Param workspaceId path string true "Workspace ID"
// @Param request body dto.AutomationUpdateWorkspaceRequest true "Workspace data"
// @Success 200 {object} dto.WorkspaceResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Failure 413 {object} dto.ErrorResponse
// @Router /api/v1/automation/tenants/{tenantId}/workspaces/{workspaceId} [patch]
// @Security AutomationKey
func (ctrl *AutomationController) updateWorkspace(c *gin.Context) {
	tenantID := c.Param("tenantId")
	if !ctrl.checkTenantAccess(c, tenantID) {
		return
	}

	workspaceID := c.Param("workspaceId")

	var req dto.AutomationUpdateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	cmd := organizationuc.UpdateWorkspaceCommand{
		ID:   workspaceID,
		Name: req.Name,
		Code: req.Code,
	}

	updated, err := ctrl.workspaceUC.UpdateWorkspace(c.Request.Context(), cmd)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, mapper.WorkspaceToResponse(updated))
}

// changeWorkspaceStatus is a shared helper for suspend/activate/archive handlers.
func (ctrl *AutomationController) changeWorkspaceStatus(c *gin.Context, status entity.WorkspaceStatus) {
	tenantID := c.Param("tenantId")
	if !ctrl.checkTenantAccess(c, tenantID) {
		return
	}

	workspaceID := c.Param("workspaceId")
	cmd := organizationuc.UpdateWorkspaceStatusCommand{
		ID:     workspaceID,
		Status: status,
	}

	ws, err := ctrl.workspaceUC.UpdateWorkspaceStatus(c.Request.Context(), cmd)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, mapper.WorkspaceToResponse(ws))
}

// suspendWorkspace suspends a workspace.
// @Summary Suspend workspace
// @Tags Automation
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Param workspaceId path string true "Workspace ID"
// @Success 200 {object} dto.WorkspaceResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/automation/tenants/{tenantId}/workspaces/{workspaceId}/suspend [post]
// @Security AutomationKey
func (ctrl *AutomationController) suspendWorkspace(c *gin.Context) {
	ctrl.changeWorkspaceStatus(c, entity.WorkspaceStatusSuspended)
}

// activateWorkspace activates a workspace.
// @Summary Activate workspace
// @Tags Automation
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Param workspaceId path string true "Workspace ID"
// @Success 200 {object} dto.WorkspaceResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/automation/tenants/{tenantId}/workspaces/{workspaceId}/activate [post]
// @Security AutomationKey
func (ctrl *AutomationController) activateWorkspace(c *gin.Context) {
	ctrl.changeWorkspaceStatus(c, entity.WorkspaceStatusActive)
}

// archiveWorkspace archives a workspace.
// @Summary Archive workspace
// @Tags Automation
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Param workspaceId path string true "Workspace ID"
// @Success 200 {object} dto.WorkspaceResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/automation/tenants/{tenantId}/workspaces/{workspaceId}/archive [post]
// @Security AutomationKey
func (ctrl *AutomationController) archiveWorkspace(c *gin.Context) {
	ctrl.changeWorkspaceStatus(c, entity.WorkspaceStatusArchived)
}

// --- Injectable Handlers ---

// listInjectables lists all injectables for a workspace.
// @Summary List injectables in workspace
// @Tags Automation
// @Produce json
// @Param workspaceId path string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode"
// @Success 200 {object} dto.ListInjectablesResponse
// @Failure 401 {object} dto.ErrorResponse
// @Router /api/v1/automation/workspaces/{workspaceId}/injectables [get]
// @Security AutomationKey
func (ctrl *AutomationController) listInjectables(c *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(c)

	result, err := ctrl.injectableUC.ListInjectables(c.Request.Context(), &injectableuc.ListInjectablesRequest{
		WorkspaceID: workspaceID,
	})
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ctrl.injectableMapper.ToListResponse(result.Injectables, result.Groups))
}

// --- Template Handlers ---

// listTemplates lists all templates in a workspace.
// @Summary List templates in workspace
// @Tags Automation
// @Produce json
// @Param workspaceId path string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode"
// @Success 200 {object} dto.ListResponse[dto.TemplateListItemResponse]
// @Failure 401 {object} dto.ErrorResponse
// @Router /api/v1/automation/workspaces/{workspaceId}/templates [get]
// @Security AutomationKey
func (ctrl *AutomationController) listTemplates(c *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(c)

	templates, err := ctrl.templateUC.ListTemplates(c.Request.Context(), workspaceID, port.TemplateFilters{})
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.NewListResponse(ctrl.templateMapper.ToListItemResponseList(templates)))
}

// createTemplate creates a new template in a workspace.
// @Summary Create template in workspace
// @Tags Automation
// @Accept json
// @Produce json
// @Param workspaceId path string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode"
// @Param X-Process header string false "Process identifier"
// @Param X-Process-Type header string false "Process type"
// @Param request body dto.AutomationCreateTemplateRequest true "Template data"
// @Success 201 {object} dto.TemplateCreateResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 413 {object} dto.ErrorResponse
// @Router /api/v1/automation/workspaces/{workspaceId}/templates [post]
// @Security AutomationKey
func (ctrl *AutomationController) createTemplate(c *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(c)

	var req dto.AutomationCreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	cmd := templateuc.CreateTemplateCommand{
		WorkspaceID: workspaceID,
		Title:       req.Name,
		Process:     c.GetHeader(HeaderProcess),
		ProcessType: c.GetHeader(HeaderProcessType),
		CreatedBy:   "", // automation API has no user context — stored as NULL
	}

	template, version, err := ctrl.templateUC.CreateTemplate(c.Request.Context(), cmd)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ctrl.templateMapper.ToCreateResponse(template, version))
}

// getTemplate retrieves a template with its details.
// @Summary Get template
// @Tags Automation
// @Produce json
// @Param workspaceId path string true "Workspace ID"
// @Param templateId path string true "Template ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode"
// @Success 200 {object} dto.TemplateWithDetailsResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/automation/workspaces/{workspaceId}/templates/{templateId} [get]
// @Security AutomationKey
func (ctrl *AutomationController) getTemplate(c *gin.Context) {
	templateID := c.Param("templateId")

	details, err := ctrl.templateUC.GetTemplateWithDetails(c.Request.Context(), templateID)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ctrl.templateMapper.ToDetailsResponse(details))
}

// updateTemplate partially updates a template's metadata.
// @Summary Update template
// @Tags Automation
// @Accept json
// @Produce json
// @Param workspaceId path string true "Workspace ID"
// @Param templateId path string true "Template ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode"
// @Param request body dto.AutomationUpdateTemplateRequest true "Template data"
// @Success 200 {object} dto.TemplateResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 413 {object} dto.ErrorResponse
// @Router /api/v1/automation/workspaces/{workspaceId}/templates/{templateId} [patch]
// @Security AutomationKey
func (ctrl *AutomationController) updateTemplate(c *gin.Context) {
	templateID := c.Param("templateId")

	var req dto.AutomationUpdateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	cmd := templateuc.UpdateTemplateCommand{
		ID:    templateID,
		Title: req.Name,
	}

	template, err := ctrl.templateUC.UpdateTemplate(c.Request.Context(), cmd)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ctrl.templateMapper.ToResponse(template))
}

// --- Document Type Handlers ---

// listDocumentTypes lists all document types.
// Since no tenantId is provided in this route, we return all accessible types.
// For simplicity, we accept an optional tenantId query parameter.
// @Summary List document types
// @Tags Automation
// @Produce json
// @Param tenantId query string true "Tenant ID"
// @Success 200 {object} dto.ListResponse[dto.DocumentTypeResponse]
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/automation/document-types [get]
// @Security AutomationKey
func (ctrl *AutomationController) listDocumentTypes(c *gin.Context) {
	tenantID := c.Query("tenantId")
	if tenantID == "" {
		// Without a tenantId we cannot scope the query; require the caller to pass it.
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenantId query parameter is required"})
		return
	}

	if !ctrl.checkTenantAccess(c, tenantID) {
		return
	}

	docTypes, _, err := ctrl.documentTypeUC.ListDocumentTypes(c.Request.Context(), tenantID, port.DocumentTypeFilters{
		Limit:  200,
		Offset: 0,
	})
	if err != nil {
		HandleError(c, err)
		return
	}

	responses := make([]*dto.DocumentTypeResponse, 0, len(docTypes))
	for _, dt := range docTypes {
		responses = append(responses, ctrl.docTypeMapper.ToResponse(dt))
	}
	c.JSON(http.StatusOK, dto.NewListResponse(responses))
}

// assignDocumentType assigns a document type to a template.
// @Summary Assign document type to template
// @Tags Automation
// @Accept json
// @Produce json
// @Param templateId path string true "Template ID"
// @Param request body dto.AutomationAssignDocumentTypeRequest true "Document type assignment"
// @Success 200 {object} dto.AssignDocumentTypeResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 413 {object} dto.ErrorResponse
// @Router /api/v1/automation/templates/{templateId}/document-type [post]
// @Security AutomationKey
func (ctrl *AutomationController) assignDocumentType(c *gin.Context) {
	templateID := c.Param("templateId")

	var req dto.AutomationAssignDocumentTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	// Retrieve the template to get workspaceID for conflict checking.
	tmpl, err := ctrl.templateUC.GetTemplate(c.Request.Context(), templateID)
	if err != nil {
		HandleError(c, err)
		return
	}

	cmd := templateuc.AssignDocumentTypeCommand{
		TemplateID:     templateID,
		WorkspaceID:    tmpl.WorkspaceID,
		DocumentTypeID: &req.DocumentTypeID,
		Force:          false,
	}

	result, err := ctrl.templateUC.AssignDocumentType(c.Request.Context(), cmd)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, mapper.AssignResultToResponse(result, ctrl.templateMapper))
}

// setProcessFields sets process fields on a template.
// @Summary Set process fields on template
// @Tags Automation
// @Accept json
// @Produce json
// @Param templateId path string true "Template ID"
// @Param request body dto.SetProcessFieldsRequest true "Process fields"
// @Success 200 {object} dto.TemplateResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 413 {object} dto.ErrorResponse
// @Router /api/v1/automation/templates/{templateId}/process [put]
// @Security AutomationKey
func (ctrl *AutomationController) setProcessFields(c *gin.Context) {
	templateID := c.Param("templateId")

	var req dto.SetProcessFieldsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	tmpl, err := ctrl.templateUC.GetTemplate(c.Request.Context(), templateID)
	if err != nil {
		HandleError(c, err)
		return
	}

	cmd := templateuc.SetProcessFieldsCommand{
		TemplateID:  templateID,
		WorkspaceID: tmpl.WorkspaceID,
		Process:     req.Process,
		ProcessType: req.ProcessType,
	}

	updated, err := ctrl.templateUC.SetProcessFields(c.Request.Context(), cmd)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ctrl.templateMapper.ToResponse(updated))
}

// --- Version Handlers ---

// listVersions lists all versions for a template.
// @Summary List template versions
// @Tags Automation
// @Produce json
// @Param templateId path string true "Template ID"
// @Success 200 {object} dto.ListTemplateVersionsResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/automation/templates/{templateId}/versions [get]
// @Security AutomationKey
func (ctrl *AutomationController) listVersions(c *gin.Context) {
	templateID := c.Param("templateId")

	versions, err := ctrl.templateVersionUC.ListVersions(c.Request.Context(), templateID)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ctrl.versionMapper.ToListResponse(versions))
}

// createVersion creates a new version for a template.
// @Summary Create template version
// @Tags Automation
// @Accept json
// @Produce json
// @Param templateId path string true "Template ID"
// @Param request body dto.AutomationCreateVersionRequest true "Version data"
// @Success 201 {object} dto.TemplateVersionResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 413 {object} dto.ErrorResponse
// @Router /api/v1/automation/templates/{templateId}/versions [post]
// @Security AutomationKey
func (ctrl *AutomationController) createVersion(c *gin.Context) {
	templateID := c.Param("templateId")

	var req dto.AutomationCreateVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	cmd := templateuc.CreateVersionCommand{
		TemplateID:  templateID,
		Name:        req.Name,
		Description: &req.Description,
		CreatedBy:   nil, // automation API has no user context — stored as NULL
	}

	version, err := ctrl.templateVersionUC.CreateVersion(c.Request.Context(), cmd)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ctrl.versionMapper.ToResponse(version))
}

// getVersion retrieves a version with details.
// @Summary Get template version
// @Tags Automation
// @Produce json
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Success 200 {object} dto.TemplateVersionDetailResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/automation/templates/{templateId}/versions/{versionId} [get]
// @Security AutomationKey
func (ctrl *AutomationController) getVersion(c *gin.Context) {
	versionID := c.Param("versionId")

	details, err := ctrl.templateVersionUC.GetVersionWithDetails(c.Request.Context(), versionID)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ctrl.versionMapper.ToDetailResponse(details))
}

// updateVersion partially updates a version's name and/or description.
// @Summary Update template version
// @Tags Automation
// @Accept json
// @Produce json
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Param request body dto.AutomationUpdateVersionRequest true "Version data"
// @Success 200 {object} dto.TemplateVersionResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 413 {object} dto.ErrorResponse
// @Router /api/v1/automation/templates/{templateId}/versions/{versionId} [patch]
// @Security AutomationKey
func (ctrl *AutomationController) updateVersion(c *gin.Context) {
	versionID := c.Param("versionId")

	var req dto.AutomationUpdateVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	cmd := templateuc.UpdateVersionCommand{
		ID:          versionID,
		Name:        req.Name,
		Description: req.Description,
	}

	version, err := ctrl.templateVersionUC.UpdateVersion(c.Request.Context(), cmd)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ctrl.versionMapper.ToResponse(version))
}

// publishVersion publishes a version.
// @Summary Publish template version
// @Tags Automation
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/automation/templates/{templateId}/versions/{versionId}/publish [post]
// @Security AutomationKey
func (ctrl *AutomationController) publishVersion(c *gin.Context) {
	versionID := c.Param("versionId")

	if err := ctrl.templateVersionUC.PublishVersion(c.Request.Context(), versionID, ""); err != nil {
		HandleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// archiveVersion archives a published version.
// @Summary Archive template version
// @Tags Automation
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/automation/templates/{templateId}/versions/{versionId}/archive [post]
// @Security AutomationKey
func (ctrl *AutomationController) archiveVersion(c *gin.Context) {
	versionID := c.Param("versionId")

	if err := ctrl.templateVersionUC.ArchiveVersion(c.Request.Context(), versionID, ""); err != nil {
		HandleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// getVersionContent retrieves the full content (including contentStructure) for a version.
// @Summary Get version content
// @Tags Automation
// @Produce json
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Success 200 {object} dto.TemplateVersionDetailResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/automation/templates/{templateId}/versions/{versionId}/content [get]
// @Security AutomationKey
func (ctrl *AutomationController) getVersionContent(c *gin.Context) {
	versionID := c.Param("versionId")

	details, err := ctrl.templateVersionUC.GetVersionWithDetails(c.Request.Context(), versionID)
	if err != nil {
		HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ctrl.versionMapper.ToDetailResponse(details))
}

// generateWorkspaceCode produces a valid workspace code from a human-readable name.
// It uppercases the name, replaces runs of non-alphanumeric characters with underscores,
// and trims leading/trailing underscores. A short UUID suffix ensures uniqueness.
func generateWorkspaceCode(name string) string {
	upper := strings.ToUpper(name)
	clean := nonAlphanumericRe.ReplaceAllString(upper, "_")
	clean = strings.Trim(clean, "_")
	if len(clean) > 30 {
		clean = clean[:30]
	}
	if clean == "" {
		clean = "WS"
	}
	// Append short UUID segment for uniqueness within a tenant.
	suffix := strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
	return clean + "_" + suffix
}

// updateVersionContent replaces the content structure of a DRAFT version.
// @Summary Update version content
// @Tags Automation
// @Accept json
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Param request body dto.AutomationUpdateVersionContentRequest true "Content structure"
// @Success 200 "OK"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 413 {object} dto.ErrorResponse
// @Router /api/v1/automation/templates/{templateId}/versions/{versionId}/content [put]
// @Security AutomationKey
func (ctrl *AutomationController) updateVersionContent(c *gin.Context) {
	versionID := c.Param("versionId")

	var req dto.AutomationUpdateVersionContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if err := ctrl.templateVersionUC.UpdateVersionContent(c.Request.Context(), versionID, req.ContentStructure); err != nil {
		HandleError(c, err)
		return
	}

	c.Status(http.StatusOK)
}
