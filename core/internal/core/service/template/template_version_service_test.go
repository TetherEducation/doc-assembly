package template

import (
	"context"
	"testing"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

func TestReplaceInjectables_EnsuresSystemDefinitionBeforeInsert(t *testing.T) {
	ctx := context.Background()
	key := "substitute_legalguardian_first_name"
	systemRepo := &recordingSystemInjectableRepo{}
	injectableRepo := &recordingTemplateVersionInjectableRepo{
		onCreate: func(tvi *entity.TemplateVersionInjectable) {
			if !systemRepo.ensured[key] {
				t.Fatalf("expected system injectable definition to be ensured before creating version injectable")
			}
		},
	}
	service := &TemplateVersionService{
		injectableRepo:       injectableRepo,
		systemInjectableRepo: systemRepo,
	}

	err := service.replaceInjectables(ctx, "version-1", []*entity.TemplateVersionInjectable{
		entity.NewTemplateVersionInjectableFromSystemKey("version-1", key),
	})
	if err != nil {
		t.Fatalf("replaceInjectables returned error: %v", err)
	}

	if len(injectableRepo.created) != 1 {
		t.Fatalf("expected 1 version injectable created, got %d", len(injectableRepo.created))
	}
	if !systemRepo.ensured[key] {
		t.Fatalf("expected system injectable definition %q to be ensured", key)
	}
}

type recordingTemplateVersionInjectableRepo struct {
	created  []*entity.TemplateVersionInjectable
	onCreate func(*entity.TemplateVersionInjectable)
}

func (r *recordingTemplateVersionInjectableRepo) Create(
	_ context.Context,
	injectable *entity.TemplateVersionInjectable,
) (string, error) {
	if r.onCreate != nil {
		r.onCreate(injectable)
	}
	r.created = append(r.created, injectable)
	return "version-injectable-1", nil
}

func (r *recordingTemplateVersionInjectableRepo) FindByID(
	context.Context,
	string,
) (*entity.TemplateVersionInjectable, error) {
	panic("not implemented")
}

func (r *recordingTemplateVersionInjectableRepo) FindByVersionID(
	context.Context,
	string,
) ([]*entity.VersionInjectableWithDefinition, error) {
	panic("not implemented")
}

func (r *recordingTemplateVersionInjectableRepo) Update(context.Context, *entity.TemplateVersionInjectable) error {
	panic("not implemented")
}

func (r *recordingTemplateVersionInjectableRepo) Delete(context.Context, string) error {
	panic("not implemented")
}

func (r *recordingTemplateVersionInjectableRepo) DeleteByVersionID(context.Context, string) error {
	return nil
}

func (r *recordingTemplateVersionInjectableRepo) Exists(context.Context, string, string) (bool, error) {
	panic("not implemented")
}

func (r *recordingTemplateVersionInjectableRepo) ExistsSystemKey(context.Context, string, string) (bool, error) {
	panic("not implemented")
}

func (r *recordingTemplateVersionInjectableRepo) CopyFromVersion(context.Context, string, string) error {
	panic("not implemented")
}

type recordingSystemInjectableRepo struct {
	ensured map[string]bool
}

func (r *recordingSystemInjectableRepo) FindActiveKeysForWorkspace(context.Context, string) ([]string, error) {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) FindActiveKeysForWorkspaceAndKeys(
	context.Context,
	string,
	[]string,
) ([]string, error) {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) FindAllDefinitions(context.Context) (map[string]bool, error) {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) UpsertDefinition(_ context.Context, key string, isActive bool) error {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) EnsureDefinition(_ context.Context, key string) error {
	if r.ensured == nil {
		r.ensured = make(map[string]bool)
	}
	r.ensured[key] = true
	return nil
}

func (r *recordingSystemInjectableRepo) FindAssignmentsByKey(
	context.Context,
	string,
) ([]*entity.SystemInjectableAssignment, error) {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) CreateAssignment(
	context.Context,
	*entity.SystemInjectableAssignment,
) error {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) DeleteAssignment(context.Context, string) error {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) SetAssignmentActive(context.Context, string, bool) error {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) FindPublicActiveKeys(context.Context) (map[string]bool, error) {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) CreatePublicAssignments(context.Context, []string) (int, error) {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) DeletePublicAssignments(context.Context, []string) (int, error) {
	panic("not implemented")
}

func (r *recordingSystemInjectableRepo) FindPublicAssignmentsByKeys(
	context.Context,
	[]string,
) (map[string]string, error) {
	panic("not implemented")
}

func TestBuildPromotedTemplate_HasValidDefaultProcessFields(t *testing.T) {
	service := &TemplateVersionService{}

	template := service.buildPromotedTemplate("workspace-1", nil, "Promoted Template")

	if err := template.Validate(); err != nil {
		t.Fatalf("expected promoted template to be valid, got %v", err)
	}
	if template.Process != entity.DefaultProcess {
		t.Fatalf("expected default process %q, got %q", entity.DefaultProcess, template.Process)
	}
	if template.ProcessType != entity.DefaultProcessType {
		t.Fatalf("expected default process type %q, got %q", entity.DefaultProcessType, template.ProcessType)
	}
}
