package pdfrenderer

import (
	"context"
	"testing"

	"github.com/rendis/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

func TestRenderPreview_Basic(t *testing.T) {
	// Skip if Typst is not available (CI environments)
	service, err := NewService(DefaultTypstOptions(), nil, NewTypstConverterFactory(DefaultDesignTokens()), DefaultDesignTokens(), nil)
	if err != nil {
		t.Skipf("Typst not available, skipping test: %v", err)
	}
	defer func() { _ = service.Close() }()

	// Create a simple document
	doc := &portabledoc.Document{
		Version: portabledoc.CurrentVersion,
		Meta: portabledoc.Meta{
			Title:    "Test Document",
			Language: "es",
		},
		PageConfig: portabledoc.PageConfig{
			FormatID: portabledoc.PageFormatA4,
			Width:    794,
			Height:   1123,
			Margins: portabledoc.Margins{
				Top:    96,
				Bottom: 96,
				Left:   72,
				Right:  72,
			},
			ShowPageNumbers: true,
		},
		VariableIDs: []string{"client_name", "contract_date"},
		SignerRoles: []portabledoc.SignerRole{
			{
				ID:    "role_1",
				Label: "Cliente",
				Name:  portabledoc.FieldValue{Type: "injectable", Value: "client_name"},
				Email: portabledoc.FieldValue{Type: "text", Value: "cliente@example.com"},
				Order: 1,
			},
		},
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				{
					Type:  portabledoc.NodeTypeHeading,
					Attrs: map[string]any{"level": float64(1)},
					Content: []portabledoc.Node{
						{Type: portabledoc.NodeTypeText, Text: strPtr("CONTRATO DE SERVICIOS")},
					},
				},
				{
					Type: portabledoc.NodeTypeParagraph,
					Content: []portabledoc.Node{
						{Type: portabledoc.NodeTypeText, Text: strPtr("Entre ")},
						{
							Type: portabledoc.NodeTypeInjector,
							Attrs: map[string]any{
								"type":       "TEXT",
								"label":      "Nombre del cliente",
								"variableId": "client_name",
							},
						},
						{Type: portabledoc.NodeTypeText, Text: strPtr(" y la empresa.")},
					},
				},
				{
					Type:  portabledoc.NodeTypeHeading,
					Attrs: map[string]any{"level": float64(2)},
					Content: []portabledoc.Node{
						{Type: portabledoc.NodeTypeText, Text: strPtr("Firmas")},
					},
				},
				{
					Type: portabledoc.NodeTypeSignature,
					Attrs: map[string]any{
						"count":     float64(1),
						"layout":    "single-center",
						"lineWidth": "md",
						"signatures": []any{
							map[string]any{
								"id":     "sig_1",
								"roleId": "role_1",
								"label":  "El Cliente",
							},
						},
					},
				},
			},
		},
	}

	// Render with injectables
	req := &port.RenderPreviewRequest{
		Document: doc,
		Injectables: map[string]any{
			"client_name":   "Juan Perez Garcia",
			"contract_date": "2025-01-15",
		},
	}

	ctx := context.Background()
	result, err := service.RenderPreview(ctx, req)
	if err != nil {
		t.Fatalf("RenderPreview failed: %v", err)
	}

	// Verify result
	if result == nil {
		t.Fatal("result is nil")
	}

	if len(result.PDF) == 0 {
		t.Fatal("PDF is empty")
	}

	// Check PDF magic bytes
	if len(result.PDF) < 4 || string(result.PDF[:4]) != "%PDF" {
		t.Fatal("result is not a valid PDF (missing %PDF header)")
	}

	if result.Filename == "" {
		t.Error("filename is empty")
	}

	t.Logf("Generated PDF: %d bytes, filename: %s", len(result.PDF), result.Filename)
}

func TestRenderPreview_EmptyInjectables(t *testing.T) {
	service, err := NewService(DefaultTypstOptions(), nil, NewTypstConverterFactory(DefaultDesignTokens()), DefaultDesignTokens(), nil)
	if err != nil {
		t.Skipf("Typst not available, skipping test: %v", err)
	}
	defer func() { _ = service.Close() }()

	doc := &portabledoc.Document{
		Version: portabledoc.CurrentVersion,
		Meta: portabledoc.Meta{
			Title:    "Test Document",
			Language: "en",
		},
		PageConfig: portabledoc.PageConfig{
			FormatID: portabledoc.PageFormatA4,
			Width:    794,
			Height:   1123,
			Margins:  portabledoc.Margins{Top: 96, Bottom: 96, Left: 72, Right: 72},
		},
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				{
					Type: portabledoc.NodeTypeParagraph,
					Content: []portabledoc.Node{
						{Type: portabledoc.NodeTypeText, Text: strPtr("Simple document without variables.")},
					},
				},
			},
		},
	}

	req := &port.RenderPreviewRequest{
		Document:    doc,
		Injectables: nil,
	}

	ctx := context.Background()
	result, err := service.RenderPreview(ctx, req)
	if err != nil {
		t.Fatalf("RenderPreview failed: %v", err)
	}

	if len(result.PDF) == 0 {
		t.Fatal("PDF is empty")
	}

	t.Logf("Generated PDF: %d bytes", len(result.PDF))
}

func TestRenderPreview_RoleVariableFromInjectables(t *testing.T) {
	service, err := NewService(DefaultTypstOptions(), nil, NewTypstConverterFactory(DefaultDesignTokens()), DefaultDesignTokens(), nil)
	if err != nil {
		t.Skipf("Typst not available, skipping test: %v", err)
	}
	defer func() { _ = service.Close() }()

	doc := &portabledoc.Document{
		Version: portabledoc.CurrentVersion,
		Meta: portabledoc.Meta{
			Title:    "Test ROLE Variables",
			Language: "es",
		},
		PageConfig: portabledoc.PageConfig{
			FormatID: portabledoc.PageFormatA4,
			Width:    794,
			Height:   1123,
			Margins:  portabledoc.Margins{Top: 96, Bottom: 96, Left: 72, Right: 72},
		},
		VariableIDs: []string{"ROLE.Rol_1.email", "ROLE.Rol_1.name"},
		SignerRoles: []portabledoc.SignerRole{
			{
				ID:    "role_1",
				Label: "Rol_1",
				Name:  portabledoc.FieldValue{Type: "text", Value: ""},
				Email: portabledoc.FieldValue{Type: "text", Value: ""},
				Order: 1,
			},
		},
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				{
					Type: portabledoc.NodeTypeParagraph,
					Content: []portabledoc.Node{
						{Type: portabledoc.NodeTypeText, Text: strPtr("Email: ")},
						{
							Type: portabledoc.NodeTypeInjector,
							Attrs: map[string]any{
								"type":           "ROLE_TEXT",
								"label":          "Rol 1.email",
								"variableId":     "ROLE.Rol_1.email",
								"isRoleVariable": true,
								"roleId":         "role_1",
								"roleLabel":      "Rol_1",
								"propertyKey":    "email",
							},
						},
					},
				},
				{
					Type: portabledoc.NodeTypeParagraph,
					Content: []portabledoc.Node{
						{Type: portabledoc.NodeTypeText, Text: strPtr("Name: ")},
						{
							Type: portabledoc.NodeTypeInjector,
							Attrs: map[string]any{
								"type":           "ROLE_TEXT",
								"label":          "Rol 1.name",
								"variableId":     "ROLE.Rol_1.name",
								"isRoleVariable": true,
								"roleId":         "role_1",
								"roleLabel":      "Rol_1",
								"propertyKey":    "name",
							},
						},
					},
				},
			},
		},
	}

	req := &port.RenderPreviewRequest{
		Document: doc,
		Injectables: map[string]any{
			"ROLE.Rol_1.email": "test@example.com",
			"ROLE.Rol_1.name":  "Test User",
		},
	}

	ctx := context.Background()
	result, err := service.RenderPreview(ctx, req)
	if err != nil {
		t.Fatalf("RenderPreview failed: %v", err)
	}

	if len(result.PDF) == 0 {
		t.Fatal("PDF is empty")
	}

	// Check PDF magic bytes
	if len(result.PDF) < 4 || string(result.PDF[:4]) != "%PDF" {
		t.Fatal("result is not a valid PDF (missing %PDF header)")
	}

	t.Logf("Generated PDF with ROLE variables from injectables: %d bytes", len(result.PDF))
}

func strPtr(s string) *string {
	return &s
}
