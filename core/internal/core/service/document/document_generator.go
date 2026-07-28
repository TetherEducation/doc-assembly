package document

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	portable_doc "github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	injectable_svc "github.com/TetherEducation/doc-assembly/core/internal/core/service/injectable"
	injectable_uc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/injectable"
	"github.com/TetherEducation/doc-assembly/core/internal/core/validation"
)

// DocumentGenerationResult contains the result of document generation.
type DocumentGenerationResult struct {
	Document       *entity.Document
	Recipients     []*entity.DocumentRecipient
	Version        *entity.TemplateVersionWithDetails
	PortableDoc    *portable_doc.Document
	ResolvedValues map[string]any
}

// PreparedDocumentData contains generation output before persistence.
type PreparedDocumentData struct {
	WorkspaceID    string
	Version        *entity.TemplateVersionWithDetails
	PortableDoc    *portable_doc.Document
	ResolvedValues map[string]any
	Recipients     []*entity.DocumentRecipient
}

// DocumentGenerator is the centralized service for document generation.
// It orchestrates the entire flow: validation, mapping, injection, and creation.
// This service is reusable by CREATE, RENEW, AMEND operations.
type DocumentGenerator struct {
	templateRepo   port.TemplateRepository
	versionRepo    port.TemplateVersionRepository
	documentRepo   port.DocumentRepository
	recipientRepo  port.DocumentRecipientRepository
	injectableUC   injectable_uc.InjectableUseCase
	mapperRegistry port.MapperRegistry
	resolver       *injectable_svc.InjectableResolverService
}

// NewDocumentGenerator creates a new DocumentGenerator instance.
func NewDocumentGenerator(
	templateRepo port.TemplateRepository,
	versionRepo port.TemplateVersionRepository,
	documentRepo port.DocumentRepository,
	recipientRepo port.DocumentRecipientRepository,
	injectableUC injectable_uc.InjectableUseCase,
	mapperRegistry port.MapperRegistry,
	resolver *injectable_svc.InjectableResolverService,
) *DocumentGenerator {
	return &DocumentGenerator{
		templateRepo:   templateRepo,
		versionRepo:    versionRepo,
		documentRepo:   documentRepo,
		recipientRepo:  recipientRepo,
		injectableUC:   injectableUC,
		mapperRegistry: mapperRegistry,
		resolver:       resolver,
	}
}

// generationContext holds intermediate state during document generation.
type generationContext struct {
	workspaceID      string
	injectables      []*entity.InjectableDefinition
	dbInjectables    []*entity.InjectableDefinition
	activeSystemKeys []string
	version          *entity.TemplateVersionWithDetails
	portableDoc      *portable_doc.Document
	referencedCodes  []string
	payload          any
	resolvedValues   map[string]any
	resolveErrors    map[string]error
	recipients       []*entity.DocumentRecipient
}

// GenerateDocument is the centralized method for document generation.
// It handles the complete flow from template lookup through document creation.
// Note: PDF rendering and signing provider upload are NOT handled here.
// The caller (InternalDocumentService) handles those steps after generation.
func (g *DocumentGenerator) GenerateDocument(
	ctx context.Context,
	mapCtx *port.MapperContext,
) (*DocumentGenerationResult, error) {
	prepared, err := g.PrepareDocument(ctx, mapCtx)
	if err != nil {
		return nil, err
	}

	doc, err := g.persistDocumentAndRecipients(
		ctx,
		prepared.WorkspaceID,
		prepared.Version.ID,
		mapCtx,
		prepared.ResolvedValues,
		prepared.Recipients,
	)
	if err != nil {
		return nil, err
	}

	return &DocumentGenerationResult{
		Document:       doc,
		Recipients:     prepared.Recipients,
		Version:        prepared.Version,
		PortableDoc:    prepared.PortableDoc,
		ResolvedValues: prepared.ResolvedValues,
	}, nil
}

// PrepareDocument resolves template data, injectables and recipients without persistence.
func (g *DocumentGenerator) PrepareDocument(
	ctx context.Context,
	mapCtx *port.MapperContext,
) (*PreparedDocumentData, error) {
	genCtx, err := g.prepareGenerationContext(ctx, mapCtx)
	if err != nil {
		return nil, err
	}

	return &PreparedDocumentData{
		WorkspaceID:    genCtx.workspaceID,
		Version:        genCtx.version,
		PortableDoc:    genCtx.portableDoc,
		ResolvedValues: genCtx.resolvedValues,
		Recipients:     genCtx.recipients,
	}, nil
}

// prepareDocumentWithResolvedTemplate resolves injectables and recipients using template data
// already loaded by the caller. It avoids reloading template/version during internal create.
func (g *DocumentGenerator) prepareDocumentWithResolvedTemplate(
	ctx context.Context,
	mapCtx *port.MapperContext,
	workspaceID string,
	version *entity.TemplateVersionWithDetails,
	dbInjectables []*entity.InjectableDefinition,
	activeSystemKeys []string,
) (*PreparedDocumentData, error) {
	genCtx, err := g.prepareGenerationContextWithResolvedTemplate(
		ctx,
		mapCtx,
		workspaceID,
		version,
		dbInjectables,
		activeSystemKeys,
	)
	if err != nil {
		return nil, err
	}

	return &PreparedDocumentData{
		WorkspaceID:    genCtx.workspaceID,
		Version:        genCtx.version,
		PortableDoc:    genCtx.portableDoc,
		ResolvedValues: genCtx.resolvedValues,
		Recipients:     genCtx.recipients,
	}, nil
}

// prepareGenerationContext fetches all required data and resolves injectables.
func (g *DocumentGenerator) prepareGenerationContext(
	ctx context.Context,
	mapCtx *port.MapperContext,
) (*generationContext, error) {
	genCtx, err := g.fetchTemplateData(ctx, mapCtx)
	if err != nil {
		return nil, err
	}

	if err := g.resolveAndBuildRecipients(ctx, mapCtx, genCtx); err != nil {
		return nil, err
	}

	return genCtx, nil
}

// prepareGenerationContextWithResolvedTemplate resolves injectables and recipients from already loaded template data.
func (g *DocumentGenerator) prepareGenerationContextWithResolvedTemplate(
	ctx context.Context,
	mapCtx *port.MapperContext,
	workspaceID string,
	version *entity.TemplateVersionWithDetails,
	dbInjectables []*entity.InjectableDefinition,
	activeSystemKeys []string,
) (*generationContext, error) {
	genCtx, err := g.fetchTemplateDataFromResolved(
		ctx,
		mapCtx,
		workspaceID,
		version,
		dbInjectables,
		activeSystemKeys,
	)
	if err != nil {
		return nil, err
	}

	if err := g.resolveAndBuildRecipients(ctx, mapCtx, genCtx); err != nil {
		return nil, err
	}

	return genCtx, nil
}

// fetchTemplateData fetches workspace, injectables, version, and parses the portable document.
//
//nolint:funlen // Extended diagnostics keeps flow linear for troubleshooting.
func (g *DocumentGenerator) fetchTemplateData(ctx context.Context, mapCtx *port.MapperContext) (*generationContext, error) {
	genCtx := &generationContext{}
	var err error

	templateID := mapCtx.TemplateID
	if mapCtx.TemplateVersionID != "" {
		genCtx.version, err = g.findVersionByID(ctx, mapCtx.TemplateVersionID)
		if err != nil {
			return nil, err
		}
		if !genCtx.version.IsPublished() {
			return nil, entity.ErrInternalTemplateResolutionNotFound
		}
		templateID = genCtx.version.TemplateID
		mapCtx.TemplateID = templateID
	}

	genCtx.workspaceID, err = g.findWorkspaceID(ctx, templateID)
	if err != nil {
		return nil, err
	}

	if genCtx.version == nil {
		genCtx.version, err = g.findPublishedVersion(ctx, templateID)
		if err != nil {
			return nil, err
		}
	}

	if err := g.finishTemplateData(ctx, mapCtx, genCtx); err != nil {
		return nil, err
	}

	return genCtx, nil
}

func (g *DocumentGenerator) fetchTemplateDataFromResolved(
	ctx context.Context,
	mapCtx *port.MapperContext,
	workspaceID string,
	version *entity.TemplateVersionWithDetails,
	dbInjectables []*entity.InjectableDefinition,
	activeSystemKeys []string,
) (*generationContext, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace id is required to prepare resolved template")
	}
	if version == nil {
		return nil, fmt.Errorf("template version is required to prepare resolved template")
	}
	if !version.IsPublished() {
		return nil, entity.ErrInternalTemplateResolutionNotFound
	}
	mapCtx.TemplateID = version.TemplateID

	genCtx := &generationContext{
		workspaceID:      workspaceID,
		version:          version,
		dbInjectables:    dbInjectables,
		activeSystemKeys: activeSystemKeys,
	}
	if err := g.finishTemplateData(ctx, mapCtx, genCtx); err != nil {
		return nil, err
	}
	return genCtx, nil
}

func (g *DocumentGenerator) finishTemplateData(
	ctx context.Context,
	mapCtx *port.MapperContext,
	genCtx *generationContext,
) error {
	var err error
	genCtx.portableDoc, err = g.parseContentStructure(genCtx.version.ContentStructure)
	if err != nil {
		return fmt.Errorf("parsing content structure: %w", err)
	}

	versionCodes := collectVersionInjectableCodes(genCtx.version.Injectables)
	roleCodes := collectSignerRoleInjectableCodes(genCtx.portableDoc.SignerRoles)
	genCtx.referencedCodes = mergeUniqueCodes(versionCodes, roleCodes)
	slog.InfoContext(ctx, "collected referenced injectables",
		"template_id", genCtx.version.TemplateID,
		"version_id", genCtx.version.ID,
		"referenced_codes_count", len(genCtx.referencedCodes),
		"version_codes_count", len(versionCodes),
		"role_codes_count", len(roleCodes),
		"referenced_codes", genCtx.referencedCodes,
		"version_codes", versionCodes,
		"role_codes", roleCodes,
	)

	// Build InjectorContext for provider integration during injectable listing
	injCtx := entity.NewInjectorContext(
		mapCtx.ExternalID,
		mapCtx.TemplateID,
		mapCtx.TransactionalID,
		string(mapCtx.Operation),
		mapCtx.Environment,
		mapCtx.Headers,
		nil,
	)
	injCtx.SetDocumentType(mapCtx.DocumentTypeCode)
	usedPreloadedInjectables := genCtx.preloadedInjectablesCoverReferencedCodes()
	if usedPreloadedInjectables {
		genCtx.injectables, err = g.completePreloadedInjectables(ctx, genCtx, injCtx)
	} else {
		genCtx.injectables, err = g.fetchAvailableInjectables(ctx, genCtx.workspaceID, injCtx, genCtx.referencedCodes)
	}
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "available injectables for generation",
		"workspace_id", genCtx.workspaceID,
		"available_injectables_count", len(genCtx.injectables),
		"preloaded", usedPreloadedInjectables,
	)
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "available injectable keys",
			"workspace_id", genCtx.workspaceID,
			"keys", extractInjectableKeys(genCtx.injectables),
		)
	}

	if err := g.validateRequiredInjectables(ctx, genCtx.referencedCodes, genCtx.injectables); err != nil {
		return err
	}

	return nil
}

func (g *DocumentGenerator) findVersionByID(
	ctx context.Context,
	versionID string,
) (*entity.TemplateVersionWithDetails, error) {
	version, err := g.versionRepo.FindByIDWithDetails(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("finding template version by id: %w", err)
	}
	return version, nil
}

// resolveAndBuildRecipients executes the mapper, resolves injectables, and builds recipients.
func (g *DocumentGenerator) resolveAndBuildRecipients(
	ctx context.Context,
	mapCtx *port.MapperContext,
	genCtx *generationContext,
) error {
	var err error

	genCtx.payload, err = g.executeMapper(ctx, mapCtx)
	if err != nil {
		return err
	}

	genCtx.resolvedValues, genCtx.resolveErrors, err = g.resolveInjectables(
		ctx,
		mapCtx,
		genCtx.payload,
		genCtx.referencedCodes,
	)
	if err != nil {
		return err
	}

	genCtx.recipients, err = g.buildRecipientsFromSignerRoles(
		ctx,
		genCtx.portableDoc.SignerRoles,
		genCtx.version.SignerRoles,
		genCtx.resolvedValues,
		genCtx.resolveErrors,
	)
	if err != nil {
		slog.ErrorContext(ctx, "recipient validation failed", "error", err, slog.String("versionId", genCtx.version.ID))
		return err
	}

	return nil
}

// findWorkspaceID retrieves the workspace ID from the template.
func (g *DocumentGenerator) findWorkspaceID(ctx context.Context, templateID string) (string, error) {
	if templateID == "" {
		return "", fmt.Errorf("template id is required to resolve workspace")
	}

	template, err := g.templateRepo.FindByID(ctx, templateID)
	if err != nil {
		return "", fmt.Errorf("finding template: %w", err)
	}

	slog.InfoContext(ctx, "resolved template workspace", "template_id", templateID, "workspace_id", template.WorkspaceID)
	return template.WorkspaceID, nil
}

func (g *generationContext) preloadedInjectablesCoverReferencedCodes() bool {
	if len(g.dbInjectables) == 0 && len(g.activeSystemKeys) == 0 {
		return false
	}

	available := make(map[string]struct{}, len(g.dbInjectables)+len(g.activeSystemKeys))
	for _, injectable := range g.dbInjectables {
		if injectable == nil || injectable.Key == "" {
			continue
		}
		available[injectable.Key] = struct{}{}
	}
	for _, key := range g.activeSystemKeys {
		if key == "" {
			continue
		}
		available[key] = struct{}{}
	}
	for _, code := range g.referencedCodes {
		if _, ok := available[code]; !ok {
			return false
		}
	}
	return true
}

func (g *DocumentGenerator) completePreloadedInjectables(
	ctx context.Context,
	genCtx *generationContext,
	injCtx *entity.InjectorContext,
) ([]*entity.InjectableDefinition, error) {
	result, err := g.injectableUC.CompleteGenerationInjectables(ctx, &injectable_uc.CompleteGenerationInjectablesRequest{
		WorkspaceID:      genCtx.workspaceID,
		Environment:      injCtx.Environment(),
		Codes:            genCtx.referencedCodes,
		DBInjectables:    genCtx.dbInjectables,
		ActiveSystemKeys: genCtx.activeSystemKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("completing preloaded injectables: %w", err)
	}

	slog.InfoContext(ctx, "completed preloaded injectables",
		"workspace_id", genCtx.workspaceID,
		"requested_codes_count", len(genCtx.referencedCodes),
		"db_injectables_count", len(genCtx.dbInjectables),
		"active_system_keys_count", len(genCtx.activeSystemKeys),
		"injectables_count", len(result.Injectables),
		"groups_count", len(result.Groups),
	)
	return result.Injectables, nil
}

// fetchAvailableInjectables retrieves all injectables available for the workspace.
func (g *DocumentGenerator) fetchAvailableInjectables(
	ctx context.Context,
	workspaceID string,
	injCtx *entity.InjectorContext,
	codes []string,
) ([]*entity.InjectableDefinition, error) {
	result, err := g.injectableUC.ListInjectables(ctx, &injectable_uc.ListInjectablesRequest{
		WorkspaceID: workspaceID,
		Environment: injCtx.Environment(),
		Codes:       codes,
	})
	if err != nil {
		return nil, fmt.Errorf("listing available injectables: %w", err)
	}

	slog.InfoContext(ctx, "listed available injectables",
		"workspace_id", workspaceID,
		"requested_codes_count", len(codes),
		"injectables_count", len(result.Injectables),
		"groups_count", len(result.Groups),
	)
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "listed available injectable keys",
			"workspace_id", workspaceID,
			"keys", extractInjectableKeys(result.Injectables),
		)
	}
	return result.Injectables, nil
}

// findPublishedVersion retrieves the published version for the template.
func (g *DocumentGenerator) findPublishedVersion(
	ctx context.Context,
	templateID string,
) (*entity.TemplateVersionWithDetails, error) {
	version, err := g.versionRepo.FindPublishedByTemplateIDWithDetails(ctx, templateID)
	if err != nil {
		return nil, entity.ErrNoPublishedVersion
	}

	slog.DebugContext(ctx, "found published version",
		"versionID", version.ID,
		"versionNumber", version.VersionNumber,
	)
	return version, nil
}

// parseContentStructure parses the JSON ContentStructure into portable_doc.Document.
func (g *DocumentGenerator) parseContentStructure(content json.RawMessage) (*portable_doc.Document, error) {
	if content == nil {
		return nil, entity.ErrMissingRequiredContent
	}

	var doc portable_doc.Document
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("unmarshaling content structure: %w", err)
	}

	return &doc, nil
}

func collectVersionInjectableCodes(versionInjectables []*entity.VersionInjectableWithDefinition) []string {
	codeSet := make(map[string]struct{})
	for _, vi := range versionInjectables {
		addVersionInjectableCode(codeSet, vi)
	}
	return setKeys(codeSet)
}

func collectSignerRoleInjectableCodes(signerRoles []portable_doc.SignerRole) []string {
	codeSet := make(map[string]struct{})
	for _, signerRole := range signerRoles {
		addFieldInjectableRefs(codeSet, signerRole.Name)
		addFieldInjectableRefs(codeSet, signerRole.Email)
	}
	return setKeys(codeSet)
}

func mergeUniqueCodes(codeSets ...[]string) []string {
	merged := make(map[string]struct{})
	for _, codes := range codeSets {
		for _, code := range codes {
			addCodeIfNotEmpty(merged, code)
		}
	}
	return setKeys(merged)
}

func addVersionInjectableCode(
	codeSet map[string]struct{},
	vi *entity.VersionInjectableWithDefinition,
) {
	switch {
	case vi.Definition != nil:
		addCodeIfNotEmpty(codeSet, vi.Definition.Key)
	case vi.SystemInjectableKey != nil:
		addCodeIfNotEmpty(codeSet, *vi.SystemInjectableKey)
	}
}

func addFieldInjectableRefs(codeSet map[string]struct{}, field portable_doc.FieldValue) {
	if !field.IsInjectable() {
		return
	}
	for _, ref := range field.InjectableRefs() {
		addCodeIfNotEmpty(codeSet, ref)
	}
}

func addCodeIfNotEmpty(codeSet map[string]struct{}, code string) {
	if code == "" {
		return
	}
	codeSet[code] = struct{}{}
}

func setKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func extractInjectableKeys(injectables []*entity.InjectableDefinition) []string {
	keys := make([]string, 0, len(injectables))
	for _, injectable := range injectables {
		if injectable != nil && injectable.Key != "" {
			keys = append(keys, injectable.Key)
		}
	}
	return keys
}

// validateRequiredInjectables validates that all required injectable codes are available.
func (g *DocumentGenerator) validateRequiredInjectables(
	ctx context.Context,
	requiredCodes []string,
	availableInjectables []*entity.InjectableDefinition,
) error {
	availableKeys := make(map[string]bool, len(availableInjectables))
	for _, inj := range availableInjectables {
		availableKeys[inj.Key] = true
	}

	var missingCodes []string
	for _, code := range requiredCodes {
		if !availableKeys[code] {
			missingCodes = append(missingCodes, code)
		}
	}

	if len(missingCodes) == 0 {
		return nil
	}

	slog.WarnContext(ctx, "missing required injectables",
		"missingCodes", missingCodes,
		"requiredCount", len(requiredCodes),
		"availableCount", len(availableInjectables),
	)
	return &entity.MissingInjectablesError{MissingCodes: missingCodes}
}

// executeMapper runs the registered mapper to transform the request.
func (g *DocumentGenerator) executeMapper(
	ctx context.Context,
	mapCtx *port.MapperContext,
) (any, error) {
	mapper, ok := g.mapperRegistry.Get()
	if !ok {
		return nil, entity.ErrNoMapperRegistered
	}

	payload, err := mapper.Map(ctx, mapCtx)
	if err != nil {
		return nil, fmt.Errorf("mapper failed: %w", err)
	}

	slog.DebugContext(ctx, "mapper executed successfully")
	return payload, nil
}

// resolveInjectables executes injectors and returns resolved values.
//
//nolint:funlen
func (g *DocumentGenerator) resolveInjectables(
	ctx context.Context,
	mapCtx *port.MapperContext,
	payload any,
	referencedCodes []string,
) (map[string]any, map[string]error, error) {
	var injCtx *entity.InjectorContext
	if mapCtx.TenantCode != "" || mapCtx.WorkspaceCode != "" {
		injCtx = entity.NewInjectorContextWithCodes(
			mapCtx.ExternalID,
			mapCtx.TemplateID,
			mapCtx.TransactionalID,
			string(mapCtx.Operation),
			mapCtx.TenantCode,
			mapCtx.WorkspaceCode,
			mapCtx.Environment,
			mapCtx.Headers,
			payload,
		)
	} else {
		injCtx = entity.NewInjectorContext(
			mapCtx.ExternalID,
			mapCtx.TemplateID,
			mapCtx.TransactionalID,
			string(mapCtx.Operation),
			mapCtx.Environment,
			mapCtx.Headers,
			payload,
		)
	}
	injCtx.SetDocumentType(mapCtx.DocumentTypeCode)
	slog.InfoContext(ctx, "resolving injectables for generation",
		"tenant_code", mapCtx.TenantCode,
		"workspace_code", mapCtx.WorkspaceCode,
		"template_id", mapCtx.TemplateID,
		"referenced_codes_count", len(referencedCodes),
	)
	slog.DebugContext(ctx, "resolver requested codes", "referenced_codes", referencedCodes)

	resolveResult, err := g.resolver.Resolve(ctx, injCtx, referencedCodes)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving injectors: %w", err)
	}

	slog.InfoContext(ctx, "injectables resolved for generation",
		"resolved_values_count", len(resolveResult.Values),
		"non_critical_errors_count", len(resolveResult.Errors),
	)
	if len(resolveResult.Errors) > 0 {
		slog.WarnContext(ctx, "injectable resolver returned non-critical errors",
			"error_codes", mapKeysError(resolveResult.Errors),
		)
	}

	resolvedValues := make(map[string]any, len(resolveResult.Values))
	for code, val := range resolveResult.Values {
		resolvedValues[code] = val.AsAny()
	}

	resolveErrors := make(map[string]error, len(resolveResult.Errors))
	for code, resolveErr := range resolveResult.Errors {
		resolveErrors[code] = resolveErr
	}

	return resolvedValues, resolveErrors, nil
}

// buildRecipientsFromSignerRoles builds and validates DocumentRecipient entities from portable_doc SignerRoles.
func (g *DocumentGenerator) buildRecipientsFromSignerRoles(
	ctx context.Context,
	portableSignerRoles []portable_doc.SignerRole,
	dbSignerRoles []*entity.TemplateVersionSignerRole,
	resolvedValues map[string]any,
	resolveErrors map[string]error,
) ([]*entity.DocumentRecipient, error) {
	roleByAnchor := make(map[string]*entity.TemplateVersionSignerRole, len(dbSignerRoles))
	for _, r := range dbSignerRoles {
		roleByAnchor[r.AnchorString] = r
	}

	var validationErrors []string
	recipients := make([]*entity.DocumentRecipient, 0, len(portableSignerRoles))

	for _, sr := range portableSignerRoles {
		recipient, err := g.buildAndValidateRecipient(ctx, sr, roleByAnchor, resolvedValues, resolveErrors)
		if err != nil {
			validationErrors = append(validationErrors, err.Error())
			continue
		}
		recipients = append(recipients, recipient)
	}

	if len(validationErrors) > 0 {
		return nil, &entity.RecipientValidationError{Errors: validationErrors}
	}

	return recipients, nil
}

// buildAndValidateRecipient creates and validates a single DocumentRecipient from a SignerRole.
//
//nolint:funlen // Field-level diagnostics require explicit per-branch logging.
func (g *DocumentGenerator) buildAndValidateRecipient(
	ctx context.Context,
	sr portable_doc.SignerRole,
	roleByAnchor map[string]*entity.TemplateVersionSignerRole,
	resolvedValues map[string]any,
	resolveErrors map[string]error,
) (*entity.DocumentRecipient, error) {
	anchor := portable_doc.GenerateAnchorString(sr.Label)
	dbRole, found := roleByAnchor[anchor]

	// Validate with descriptive error messages
	if !found {
		return nil, fmt.Errorf("role '%s': no matching signature anchor found", sr.Label)
	}
	if missingNameRefs := unresolvedInjectableRefs(sr.Name, resolvedValues, resolveErrors); len(missingNameRefs) > 0 {
		return nil, fmt.Errorf(
			"role '%s': name has unresolved injectables [%s]",
			sr.Label,
			strings.Join(missingNameRefs, ", "),
		)
	}
	if missingEmailRefs := unresolvedInjectableRefs(sr.Email, resolvedValues, resolveErrors); len(missingEmailRefs) > 0 {
		return nil, fmt.Errorf(
			"role '%s': email has unresolved injectables [%s]",
			sr.Label,
			strings.Join(missingEmailRefs, ", "),
		)
	}

	nameResolved, nameDiagnostics := resolveFieldValueDiagnostics(sr.Name, resolvedValues)
	emailResolved, emailDiagnostics := resolveFieldValueDiagnostics(sr.Email, resolvedValues)
	name := validation.NormalizeName(nameResolved)
	email := strings.TrimSpace(emailResolved)

	if name == "" {
		slog.WarnContext(ctx, "role field resolved to empty value",
			"role_id", dbRole.ID,
			"role_label", sr.Label,
			"field", "name",
			"refs", sr.Name.InjectableRefs(),
			"refs_diagnostic", nameDiagnostics,
			"resolved_parts_count", countResolvedParts(nameDiagnostics),
			"final_resolved_length", len(nameResolved),
			"normalized_name_length", len(name),
		)
		return nil, fmt.Errorf("role '%s': name is empty after resolution", sr.Label)
	}
	if email == "" {
		slog.WarnContext(ctx, "role field resolved to empty value",
			"role_id", dbRole.ID,
			"role_label", sr.Label,
			"field", "email",
			"refs", sr.Email.InjectableRefs(),
			"refs_diagnostic", emailDiagnostics,
			"resolved_parts_count", countResolvedParts(emailDiagnostics),
			"final_resolved_length", len(emailResolved),
			"email_length", len(email),
		)
		return nil, fmt.Errorf("role '%s': email is empty after resolution", sr.Label)
	}
	if !validation.IsValidEmail(email) {
		return nil, fmt.Errorf("role '%s': invalid email format '%s'", sr.Label, email)
	}

	return &entity.DocumentRecipient{
		ID:                    uuid.NewString(),
		TemplateVersionRoleID: dbRole.ID,
		Name:                  name,
		Email:                 email,
		Status:                entity.RecipientStatusPending,
	}, nil
}

//nolint:funlen // Diagnostics capture each ref state in a single pass.
func resolveFieldValueDiagnostics(
	field portable_doc.FieldValue,
	resolvedValues map[string]any,
) (string, []map[string]any) {
	if field.IsText() {
		return field.Value, nil
	}

	if !field.IsInjectable() {
		return "", nil
	}

	refs := field.InjectableRefs()
	if len(refs) == 0 {
		return "", nil
	}

	resolved := make([]string, 0, len(refs))
	diagnostics := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		val, ok := resolvedValues[ref]
		if !ok {
			diagnostics = append(diagnostics, map[string]any{
				"ref":    ref,
				"status": "missing_key",
			})
			continue
		}
		if val == nil {
			diagnostics = append(diagnostics, map[string]any{
				"ref":    ref,
				"status": "nil_value",
			})
			continue
		}
		if strVal, ok := val.(string); ok {
			status := "ok"
			if strings.TrimSpace(strVal) == "" {
				status = "empty_string"
			}
			diagnostics = append(diagnostics, map[string]any{
				"ref":           ref,
				"status":        status,
				"string_length": len(strVal),
			})
			resolved = append(resolved, strVal)
			continue
		}
		strVal := fmt.Sprintf("%v", val)
		diagnostics = append(diagnostics, map[string]any{
			"ref":           ref,
			"status":        "non_string_type",
			"type":          reflect.TypeOf(val).String(),
			"string_length": len(strVal),
		})
		resolved = append(resolved, strVal)
	}

	if len(resolved) == 0 {
		return "", diagnostics
	}
	return strings.Join(resolved, field.ResolveSeparator()), diagnostics
}

func unresolvedInjectableRefs(
	field portable_doc.FieldValue,
	resolvedValues map[string]any,
	resolveErrors map[string]error,
) []string {
	if !field.IsInjectable() {
		return nil
	}

	refs := field.InjectableRefs()
	if len(refs) == 0 {
		return nil
	}

	unresolved := make([]string, 0, len(refs))
	for _, ref := range refs {
		if resolveErr, hasResolveErr := resolveErrors[ref]; hasResolveErr && resolveErr != nil {
			unresolved = append(unresolved, ref)
			continue
		}
		val, ok := resolvedValues[ref]
		if !ok || val == nil {
			unresolved = append(unresolved, ref)
			continue
		}
		if strVal, isString := val.(string); isString && strings.TrimSpace(strVal) == "" {
			unresolved = append(unresolved, ref)
		}
	}

	return unresolved
}

func countResolvedParts(refDiagnostics []map[string]any) int {
	if len(refDiagnostics) == 0 {
		return 0
	}
	count := 0
	for _, diagnostic := range refDiagnostics {
		status, _ := diagnostic["status"].(string)
		switch status {
		case "ok", "empty_string", "non_string_type":
			count++
		}
	}
	return count
}

func mapKeysError(input map[string]error) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	return keys
}

// createDocument creates and persists the document entity.
func (g *DocumentGenerator) createDocument(
	ctx context.Context,
	workspaceID string,
	versionID string,
	mapCtx *port.MapperContext,
	resolvedValues map[string]any,
) (*entity.Document, error) {
	doc := entity.NewDocument(workspaceID, versionID)
	doc.DocumentTypeID = mapCtx.DocumentTypeID
	doc.SetOperationType(mapCtx.Operation)
	doc.SetExternalReference(mapCtx.ExternalID)
	doc.SetTransactionalID(mapCtx.TransactionalID)

	if err := doc.SetInjectedValuesSnapshot(resolvedValues); err != nil {
		return nil, fmt.Errorf("setting injected values snapshot: %w", err)
	}

	docID, err := g.documentRepo.Create(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("saving document: %w", err)
	}

	doc.ID = docID
	slog.DebugContext(ctx, "document saved", "documentID", docID)

	return doc, nil
}

// persistDocumentAndRecipients creates and saves the document with its recipients.
func (g *DocumentGenerator) persistDocumentAndRecipients(
	ctx context.Context,
	workspaceID, versionID string,
	mapCtx *port.MapperContext,
	resolvedValues map[string]any,
	recipients []*entity.DocumentRecipient,
) (*entity.Document, error) {
	doc, err := g.createDocument(ctx, workspaceID, versionID, mapCtx, resolvedValues)
	if err != nil {
		return nil, err
	}

	if err := g.saveRecipients(ctx, doc.ID, recipients); err != nil {
		return nil, err
	}

	return doc, nil
}

// saveRecipients persists the document recipients.
func (g *DocumentGenerator) saveRecipients(
	ctx context.Context,
	documentID string,
	recipients []*entity.DocumentRecipient,
) error {
	if len(recipients) == 0 {
		return nil
	}

	for _, r := range recipients {
		r.DocumentID = documentID
	}

	if err := g.recipientRepo.CreateBatch(ctx, recipients); err != nil {
		return fmt.Errorf("saving recipients: %w", err)
	}

	slog.DebugContext(ctx, "recipients saved", "count", len(recipients))
	return nil
}
