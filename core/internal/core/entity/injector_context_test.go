package entity

import "testing"

func TestInjectorContextDocumentType(t *testing.T) {
	ctx := NewInjectorContextWithCodes(
		"external-001",
		"template-001",
		"transaction-001",
		"create",
		"CL",
		"210900001",
		EnvironmentDev,
		nil,
		nil,
	)

	if got := ctx.DocumentType(); got != "" {
		t.Fatalf("expected empty document type before set, got %q", got)
	}

	ctx.SetDocumentType("ENROLLMENT_COMMITMENT")

	if got := ctx.DocumentType(); got != "ENROLLMENT_COMMITMENT" {
		t.Fatalf("expected document type ENROLLMENT_COMMITMENT, got %q", got)
	}
}
