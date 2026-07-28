package contentvalidator

import (
	"testing"

	"github.com/google/uuid"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

func ptrTo[T any](value T) *T {
	return &value
}

func TestExtractInjectables_IncludesImageRefsFromContentAndHeader(t *testing.T) {
	bodyDefID := uuid.NewString()
	headerDefID := uuid.NewString()
	headerURL := "data:image/svg+xml;base64,placeholder"

	vctx := &validationContext{
		versionID: "version-1",
		doc: &portabledoc.Document{
			Content: &portabledoc.ProseMirrorDoc{
				Type: "doc",
				Content: []portabledoc.Node{
					{
						Type: portabledoc.NodeTypeCustomImage,
						Attrs: map[string]any{
							"injectableId": "img_body",
						},
					},
				},
			},
			Header: &portabledoc.DocumentHeader{
				Enabled:           true,
				ImageURL:          &headerURL,
				ImageInjectableID: ptrTo("img_header"),
			},
		},
		accessibleInjectableList: []*entity.InjectableDefinition{
			{ID: bodyDefID, Key: "img_body", SourceType: entity.InjectableSourceTypeInternal},
			{ID: headerDefID, Key: "img_header", SourceType: entity.InjectableSourceTypeInternal},
		},
	}

	got := extractInjectables(vctx)
	if len(got) != 2 {
		t.Fatalf("expected 2 extracted injectables, got %d", len(got))
	}

	seen := map[string]bool{}
	for _, injectable := range got {
		if injectable.InjectableDefinitionID != nil {
			seen[*injectable.InjectableDefinitionID] = true
		}
	}

	if !seen[bodyDefID] {
		t.Fatalf("expected extracted injectables to include body image binding")
	}
	if !seen[headerDefID] {
		t.Fatalf("expected extracted injectables to include header image binding")
	}
}

func TestValidateVariables_FlagsImageRefsMissingFromVariableIDs(t *testing.T) {
	service := &Service{}
	result := port.NewValidationResult()

	vctx := &validationContext{
		doc: &portabledoc.Document{
			VariableIDs: nil,
			Content: &portabledoc.ProseMirrorDoc{
				Type: "doc",
				Content: []portabledoc.Node{
					{
						Type: portabledoc.NodeTypeImage,
						Attrs: map[string]any{
							"injectableId": "img_body",
						},
					},
				},
			},
			Header: &portabledoc.DocumentHeader{
				Enabled:           true,
				ImageInjectableID: ptrTo("img_header"),
			},
		},
		result:      result,
		variableSet: make(portabledoc.Set[string]),
		accessibleInjectables: portabledoc.NewSet([]string{
			"img_body",
			"img_header",
		}),
	}

	service.validateVariables(vctx)

	if result.ErrorCount() != 2 {
		t.Fatalf("expected 2 validation errors, got %d", result.ErrorCount())
	}

	errors := result.Errors
	if errors[0].Path != "content.image[0].attrs.injectableId" && errors[1].Path != "content.image[0].attrs.injectableId" {
		t.Fatalf("expected body image binding error path, got %#v", errors)
	}
	if errors[0].Path != "header.imageInjectableId" && errors[1].Path != "header.imageInjectableId" {
		t.Fatalf("expected header image binding error path, got %#v", errors)
	}
}
