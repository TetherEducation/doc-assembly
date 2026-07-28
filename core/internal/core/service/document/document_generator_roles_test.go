package document

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	portable_doc "github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
)

func TestUnresolvedInjectableRefs_EmptyStringIsUnresolved(t *testing.T) {
	field := portable_doc.FieldValue{
		Type:   portable_doc.FieldTypeInjectable,
		Values: []string{"adm_legalguardian_first_name", "adm_legalguardian_first_last_name"},
	}

	resolvedValues := map[string]any{
		"adm_legalguardian_first_name":      "",
		"adm_legalguardian_first_last_name": "  ",
	}

	unresolved := unresolvedInjectableRefs(field, resolvedValues, nil)
	if len(unresolved) != 2 {
		t.Fatalf("expected 2 unresolved refs, got %d (%v)", len(unresolved), unresolved)
	}
}

func TestBuildAndValidateRecipient_ResolverErrorIsReportedAsUnresolved(t *testing.T) {
	g := &DocumentGenerator{}

	signerRole := portable_doc.SignerRole{
		Label: "Apoderado/a",
		Name: portable_doc.FieldValue{
			Type:   portable_doc.FieldTypeInjectable,
			Values: []string{"adm_legalguardian_first_name"},
		},
		Email: portable_doc.FieldValue{
			Type:  portable_doc.FieldTypeText,
			Value: "guardian@example.com",
		},
	}

	anchor := portable_doc.GenerateAnchorString(signerRole.Label)
	roleByAnchor := map[string]*entity.TemplateVersionSignerRole{
		anchor: {
			ID:           "role-id-1",
			AnchorString: anchor,
			RoleName:     signerRole.Label,
		},
	}

	resolvedValues := map[string]any{
		"adm_legalguardian_first_name": "María",
	}
	resolveErrors := map[string]error{
		"adm_legalguardian_first_name": errors.New("repository missing for env dev"),
	}

	_, err := g.buildAndValidateRecipient(
		context.Background(),
		signerRole,
		roleByAnchor,
		resolvedValues,
		resolveErrors,
	)
	if err == nil {
		t.Fatal("expected unresolved injectable error, got nil")
	}

	got := err.Error()
	if !strings.Contains(got, "name has unresolved injectables") {
		t.Fatalf("expected unresolved injectables error, got: %s", got)
	}
	if !strings.Contains(got, "adm_legalguardian_first_name") {
		t.Fatalf("expected missing code in error, got: %s", got)
	}
}
