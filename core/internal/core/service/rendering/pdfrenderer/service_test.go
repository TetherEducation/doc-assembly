package pdfrenderer

import (
	"context"
	"fmt"
	"math"
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

func TestRequireExtractedSignaturePositionsRejectsFallbackCoordinates(t *testing.T) {
	err := requireExtractedSignaturePositions([]port.SignatureField{{
		RoleID:       "role_1",
		AnchorString: "signature_anchor_role_1",
		Page:         1,
		PositionX:    35,
		PositionY:    55,
		Width:        30,
		Height:       8,
	}})
	if err == nil {
		t.Fatal("expected fallback-only signature field to fail")
	}
}

func TestRequireExtractedSignaturePositionsAcceptsPDFCoordinates(t *testing.T) {
	err := requireExtractedSignaturePositions([]port.SignatureField{{
		RoleID:       "role_1",
		AnchorString: "signature_anchor_role_1",
		Page:         1,
		PositionX:    35,
		PositionY:    55,
		Width:        30,
		Height:       8,
		PDFPointY:    500,
		PDFPageW:     612,
		PDFPageH:     792,
	}})
	if err != nil {
		t.Fatalf("requireExtractedSignaturePositions() error = %v", err)
	}
}

func TestRenderPreview_SignatureFieldsTrackRenderedLineWithVariableContent(t *testing.T) {
	service, err := NewService(DefaultTypstOptions(), nil, NewTypstConverterFactory(DefaultDesignTokens()), DefaultDesignTokens(), nil)
	if err != nil {
		t.Skipf("Typst not available, skipping test: %v", err)
	}
	defer func() { _ = service.Close() }()

	cases := []struct {
		name             string
		paragraphRepeats int
		layout           string
		signers          []signatureFieldTestSigner
	}{
		{
			name:             "short content single signer",
			paragraphRepeats: 1,
			layout:           portabledoc.LayoutSingleCenter,
			signers:          testSigners(1, false),
		},
		{
			name:             "medium content single signer",
			paragraphRepeats: 8,
			layout:           portabledoc.LayoutSingleCenter,
			signers:          testSigners(1, false),
		},
		{
			name:             "long content pushes single signer",
			paragraphRepeats: 18,
			layout:           portabledoc.LayoutSingleCenter,
			signers:          testSigners(1, false),
		},
		{
			name:             "long content dual signer with long labels",
			paragraphRepeats: 18,
			layout:           portabledoc.LayoutDualSides,
			signers:          testSigners(2, true),
		},
		{
			name:             "near bottom quad signer with long labels",
			paragraphRepeats: 14,
			layout:           portabledoc.LayoutQuadGrid,
			signers:          testSigners(4, true),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &port.RenderPreviewRequest{
				Document:    buildSignatureFieldVariableContentDoc(tc.paragraphRepeats, tc.layout, tc.signers),
				Injectables: map[string]any{},
			}

			result, err := service.RenderPreview(context.Background(), req)
			if err != nil {
				t.Fatalf("RenderPreview failed: %v", err)
			}
			if len(result.SignatureFields) != len(tc.signers) {
				t.Fatalf("signature fields = %d, want %d: %+v", len(result.SignatureFields), len(tc.signers), result.SignatureFields)
			}

			for _, field := range result.SignatureFields {
				if field.PDFPointY == 0 || field.PDFPageH == 0 || field.PDFPageW == 0 {
					t.Fatalf("field %s used fallback/no extracted PDF coordinates: %+v", field.RoleID, field)
				}
				if field.Page < 1 {
					t.Fatalf("field %s page = %d, want a 1-indexed PDF page", field.RoleID, field.Page)
				}

				_, posY := port.ConvertFieldToDocumensoPosition(field)
				lineTopPct := 100 - ((field.PDFPointY / field.PDFPageH) * 100)
				bottomPct := posY + field.Height
				if math.Abs(bottomPct-lineTopPct) > 0.01 {
					t.Fatalf("field %s bottom %.4f does not align with line %.4f: %+v", field.RoleID, bottomPct, lineTopPct, field)
				}
			}
		})
	}
}

type signatureFieldTestSigner struct {
	roleID   string
	label    string
	subtitle string
}

func testSigners(count int, longText bool) []signatureFieldTestSigner {
	signers := make([]signatureFieldTestSigner, count)
	for i := range count {
		roleID := fmt.Sprintf("role_%d", i+1)
		label := fmt.Sprintf("Firmante %d", i+1)
		subtitle := fmt.Sprintf("Rol %d", i+1)
		if longText {
			label = fmt.Sprintf("Firma del firmante responsable número %d con representación legal extendida", i+1)
			subtitle = fmt.Sprintf("Nombre completo y cargo institucional del firmante número %d", i+1)
		}
		signers[i] = signatureFieldTestSigner{roleID: roleID, label: label, subtitle: subtitle}
	}
	return signers
}

func buildSignatureFieldVariableContentDoc(paragraphRepeats int, layout string, signers []signatureFieldTestSigner) *portabledoc.Document {
	nodes := []portabledoc.Node{
		{
			Type:  portabledoc.NodeTypeHeading,
			Attrs: map[string]any{"level": float64(1)},
			Content: []portabledoc.Node{
				{Type: portabledoc.NodeTypeText, Text: strPtr("Contrato de prueba para ubicación de firma")},
			},
		},
	}
	for i := range paragraphRepeats {
		nodes = append(nodes, portabledoc.Node{
			Type: portabledoc.NodeTypeParagraph,
			Content: []portabledoc.Node{
				{Type: portabledoc.NodeTypeText, Text: strPtr(fmt.Sprintf(
					"Cláusula %02d: Este párrafo contractual de prueba contiene texto suficiente para modificar el flujo vertical del documento sin cambiar la composición del bloque de firmas. La línea de firma debe seguir siendo el único punto de referencia para el campo del proveedor.",
					i+1,
				))},
			},
		})
	}
	nodes = append(nodes, portabledoc.Node{
		Type: portabledoc.NodeTypeParagraph,
		Content: []portabledoc.Node{
			{Type: portabledoc.NodeTypeText, Text: strPtr("Marque su decisión:")},
		},
	})
	nodes = append(nodes, portabledoc.Node{
		Type:  portabledoc.NodeTypeSignature,
		Attrs: buildSignatureFieldAttrs(layout, signers),
	})

	roles := make([]portabledoc.SignerRole, len(signers))
	for i, signer := range signers {
		roles[i] = portabledoc.SignerRole{
			ID:    signer.roleID,
			Label: signer.label,
			Name:  portabledoc.FieldValue{Type: "text", Value: signer.label},
			Email: portabledoc.FieldValue{Type: "text", Value: fmt.Sprintf("%s@example.test", signer.roleID)},
			Order: i + 1,
		}
	}

	return &portabledoc.Document{
		Version: portabledoc.CurrentVersion,
		Meta:    portabledoc.Meta{Title: "Signature Field Variable Content Test", Language: "es"},
		PageConfig: portabledoc.PageConfig{
			FormatID: portabledoc.PageFormatLetter,
			Width:    816,
			Height:   1056,
			Margins:  portabledoc.Margins{Top: 96, Bottom: 96, Left: 72, Right: 72},
		},
		SignerRoles: roles,
		Content:     &portabledoc.ProseMirrorDoc{Type: "doc", Content: nodes},
	}
}

func buildSignatureFieldAttrs(layout string, signers []signatureFieldTestSigner) map[string]any {
	signatures := make([]any, len(signers))
	for i, signer := range signers {
		signatures[i] = map[string]any{
			"id":       fmt.Sprintf("sig_%d", i+1),
			"roleId":   signer.roleID,
			"label":    signer.label,
			"subtitle": signer.subtitle,
		}
	}
	return map[string]any{
		"count":      float64(len(signers)),
		"layout":     layout,
		"lineWidth":  "md",
		"signatures": signatures,
	}
}

func strPtr(s string) *string {
	return &s
}
