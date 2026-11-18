package catalog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

const (
	scopeTenant       = "tenant-caller"
	scopeOtherTenant  = "tenant-other"
	scopeSystemTenant = "tenant-system"

	scopeOwnID     = "6f1c2a9e-3b4d-4e8f-9a01-2b3c4d5e6f70"
	scopeSpareID   = "7a2d3b0f-4c5e-4f90-8b12-3c4d5e6f7a81"
	scopeOtherID   = "8b3e4c1a-5d6f-4a01-9c23-4d5e6f7a8b92"
	scopeGlobalID  = "9c4f5d2b-6e7a-4b12-8d34-5e6f7a8b9ca3"
	scopeMissingID = "ad5a6e3c-7f8b-4c23-9e45-6f7a8b9cadb4"
)

// docTypeScopeRepoFake finds document types by ID and records what is written.
type docTypeScopeRepoFake struct {
	port.DocumentTypeRepository
	types     map[string]*entity.DocumentType
	templates []*entity.DocumentTypeTemplateInfo // templates of the type being deleted
	writes    []string
}

func (f *docTypeScopeRepoFake) FindByID(_ context.Context, id string) (*entity.DocumentType, error) {
	if docType, ok := f.types[id]; ok {
		copied := *docType
		return &copied, nil
	}
	return nil, entity.ErrDocumentTypeNotFound
}

func (f *docTypeScopeRepoFake) IsSysTenant(_ context.Context, tenantID string) (bool, error) {
	return tenantID == scopeSystemTenant, nil
}

func (f *docTypeScopeRepoFake) FindTemplatesByType(context.Context, string) ([]*entity.DocumentTypeTemplateInfo, error) {
	return f.templates, nil
}

func (f *docTypeScopeRepoFake) Update(_ context.Context, docType *entity.DocumentType) error {
	f.writes = append(f.writes, "update "+docType.ID)
	return nil
}

func (f *docTypeScopeRepoFake) Delete(_ context.Context, id string) error {
	f.writes = append(f.writes, "delete "+id)
	return nil
}

// processScopeRepoFake finds processes by ID and records what is written.
type processScopeRepoFake struct {
	port.ProcessRepository
	processes map[string]*entity.Process
	writes    []string
}

func (f *processScopeRepoFake) FindByID(_ context.Context, id string) (*entity.Process, error) {
	if process, ok := f.processes[id]; ok {
		copied := *process
		return &copied, nil
	}
	return nil, entity.ErrProcessNotFound
}

func (f *processScopeRepoFake) IsSysTenant(_ context.Context, tenantID string) (bool, error) {
	return tenantID == scopeSystemTenant, nil
}

func (f *processScopeRepoFake) FindTemplatesByProcess(context.Context, string, string) ([]*entity.ProcessTemplateInfo, error) {
	return nil, nil
}

func (f *processScopeRepoFake) Update(_ context.Context, process *entity.Process) error {
	f.writes = append(f.writes, "update "+process.ID)
	return nil
}

func (f *processScopeRepoFake) Delete(_ context.Context, id string) error {
	f.writes = append(f.writes, "delete "+id)
	return nil
}

// templateTypeRepoFake records the document types set on templates.
type templateTypeRepoFake struct {
	port.TemplateRepository
	writes []string
}

func (f *templateTypeRepoFake) UpdateDocumentType(_ context.Context, templateID string, documentTypeID *string) error {
	f.writes = append(f.writes, "retype "+templateID)
	return nil
}

func newDocumentTypeScopeService() (*DocumentTypeService, *docTypeScopeRepoFake, *templateTypeRepoFake) {
	name := entity.I18nText{"en": "Type"}
	repo := &docTypeScopeRepoFake{types: map[string]*entity.DocumentType{
		scopeOwnID:    {ID: scopeOwnID, TenantID: scopeTenant, Code: "OWN", Name: name},
		scopeSpareID:  {ID: scopeSpareID, TenantID: scopeTenant, Code: "SPARE", Name: name},
		scopeOtherID:  {ID: scopeOtherID, TenantID: scopeOtherTenant, Code: "OTHER", Name: name},
		scopeGlobalID: {ID: scopeGlobalID, TenantID: scopeSystemTenant, Code: "GLOBAL", Name: name},
	}}
	templates := &templateTypeRepoFake{}
	return &DocumentTypeService{docTypeRepo: repo, templateRepo: templates}, repo, templates
}

// A tenant sees its own document types and the global ones. A type of another
// tenant, like an ID that is not a UUID, answers exactly as a missing type
// does, to a read, an update and a delete, and nothing is written.
func TestDocumentTypeService_TypeOfAnotherTenant(t *testing.T) {
	ctx := context.Background()
	name := entity.I18nText{"en": "Renamed"}
	actions := map[string]func(*DocumentTypeService, string) error{
		"get": func(svc *DocumentTypeService, id string) error {
			_, err := svc.GetDocumentType(ctx, id, scopeTenant)
			return err
		},
		"update": func(svc *DocumentTypeService, id string) error {
			_, err := svc.UpdateDocumentType(ctx, cataloguc.UpdateDocumentTypeCommand{ID: id, TenantID: scopeTenant, Name: name})
			return err
		},
		"delete": func(svc *DocumentTypeService, id string) error {
			_, err := svc.DeleteDocumentType(ctx, cataloguc.DeleteDocumentTypeCommand{ID: id, TenantID: scopeTenant, Force: true})
			return err
		},
	}

	for action, act := range actions {
		t.Run(action, func(t *testing.T) {
			own, _, _ := newDocumentTypeScopeService()
			require.NoError(t, act(own, scopeOwnID), "the tenant's own type")

			svc, repo, _ := newDocumentTypeScopeService()
			missingErr := act(svc, scopeMissingID)
			require.ErrorIs(t, missingErr, entity.ErrDocumentTypeNotFound)
			for _, id := range []string{scopeOtherID, "not-a-uuid"} {
				err := act(svc, id)
				require.ErrorIs(t, err, entity.ErrDocumentTypeNotFound)
				assert.EqualError(t, err, missingErr.Error(), "the answer must be the one a missing type gets")
			}
			assert.Empty(t, repo.writes, "nothing may be written")
		})
	}

	t.Run("a global type is read but not changed", func(t *testing.T) {
		svc, repo, _ := newDocumentTypeScopeService()
		require.NoError(t, actions["get"](svc, scopeGlobalID))
		require.ErrorIs(t, actions["update"](svc, scopeGlobalID), entity.ErrCannotModifyGlobalType)
		require.ErrorIs(t, actions["delete"](svc, scopeGlobalID), entity.ErrCannotModifyGlobalType)
		assert.Empty(t, repo.writes, "nothing may be written")
	})
}

// The type that replaces a deleted one on its templates must be one the tenant
// can see. A type of another tenant answers exactly as a missing replacement
// does, and neither the templates nor the type are touched.
func TestDocumentTypeService_ReplacementOfAnotherTenant(t *testing.T) {
	ctx := context.Background()
	remove := func(replaceWithID string) (typeWrites, templateWrites []string, err error) {
		svc, repo, templates := newDocumentTypeScopeService()
		repo.templates = []*entity.DocumentTypeTemplateInfo{{ID: "template-1"}}
		_, err = svc.DeleteDocumentType(ctx, cataloguc.DeleteDocumentTypeCommand{
			ID: scopeOwnID, TenantID: scopeTenant, ReplaceWithID: &replaceWithID,
		})
		return repo.writes, templates.writes, err
	}

	for name, replacement := range map[string]string{"own type": scopeSpareID, "global type": scopeGlobalID} {
		t.Run(name, func(t *testing.T) {
			typeWrites, templateWrites, err := remove(replacement)
			require.NoError(t, err)
			assert.Equal(t, []string{"retype template-1"}, templateWrites)
			assert.Equal(t, []string{"delete " + scopeOwnID}, typeWrites)
		})
	}

	_, _, missingErr := remove(scopeMissingID)
	require.ErrorIs(t, missingErr, entity.ErrDocumentTypeNotFound)
	for name, replacement := range map[string]string{"type of another tenant": scopeOtherID, "not a UUID": "not-a-uuid"} {
		t.Run(name, func(t *testing.T) {
			typeWrites, templateWrites, err := remove(replacement)
			require.ErrorIs(t, err, entity.ErrDocumentTypeNotFound)
			assert.EqualError(t, err, missingErr.Error(), "the answer must be the one a missing replacement gets")
			assert.Empty(t, templateWrites, "no template may be retyped")
			assert.Empty(t, typeWrites, "the type may not be deleted")
		})
	}
}

// A tenant sees its own processes and the global ones. A process of another
// tenant, like an ID that is not a UUID, answers exactly as a missing process
// does, to a read, an update and a delete, and nothing is written.
func TestProcessService_ProcessOfAnotherTenant(t *testing.T) {
	ctx := context.Background()
	name := entity.I18nText{"en": "Renamed"}
	newService := func() (*ProcessService, *processScopeRepoFake) {
		repo := &processScopeRepoFake{processes: map[string]*entity.Process{
			scopeOwnID:    {ID: scopeOwnID, TenantID: scopeTenant, Code: "OWN", ProcessType: entity.DefaultProcessType, Name: name},
			scopeOtherID:  {ID: scopeOtherID, TenantID: scopeOtherTenant, Code: "OTHER", ProcessType: entity.DefaultProcessType, Name: name},
			scopeGlobalID: {ID: scopeGlobalID, TenantID: scopeSystemTenant, Code: "GLOBAL", ProcessType: entity.DefaultProcessType, Name: name},
		}}
		return &ProcessService{processRepo: repo}, repo
	}
	actions := map[string]func(*ProcessService, string) error{
		"get": func(svc *ProcessService, id string) error {
			_, err := svc.GetProcess(ctx, id, scopeTenant)
			return err
		},
		"update": func(svc *ProcessService, id string) error {
			_, err := svc.UpdateProcess(ctx, cataloguc.UpdateProcessCommand{ID: id, TenantID: scopeTenant, Name: name})
			return err
		},
		"delete": func(svc *ProcessService, id string) error {
			_, err := svc.DeleteProcess(ctx, cataloguc.DeleteProcessCommand{ID: id, TenantID: scopeTenant, Force: true})
			return err
		},
	}

	for action, act := range actions {
		t.Run(action, func(t *testing.T) {
			own, _ := newService()
			require.NoError(t, act(own, scopeOwnID), "the tenant's own process")

			svc, repo := newService()
			missingErr := act(svc, scopeMissingID)
			require.ErrorIs(t, missingErr, entity.ErrProcessNotFound)
			for _, id := range []string{scopeOtherID, "not-a-uuid"} {
				err := act(svc, id)
				require.ErrorIs(t, err, entity.ErrProcessNotFound)
				assert.EqualError(t, err, missingErr.Error(), "the answer must be the one a missing process gets")
			}
			assert.Empty(t, repo.writes, "nothing may be written")
		})
	}

	t.Run("a global process is read but not changed", func(t *testing.T) {
		svc, repo := newService()
		require.NoError(t, actions["get"](svc, scopeGlobalID))
		require.ErrorIs(t, actions["update"](svc, scopeGlobalID), entity.ErrCannotModifyGlobalProcess)
		require.ErrorIs(t, actions["delete"](svc, scopeGlobalID), entity.ErrCannotModifyGlobalProcess)
		assert.Empty(t, repo.writes, "nothing may be written")
	})
}
