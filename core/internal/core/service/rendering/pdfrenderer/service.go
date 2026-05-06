package pdfrenderer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// ConverterFactory creates a TypstConverter for a given render request.
// This allows each render call to have its own converter state (page tracking, images, etc.).
type ConverterFactory func(
	injectables map[string]any,
	injectableDefaults map[string]string,
	signerRoleValues map[string]port.SignerRoleValue,
	signerRoles []portabledoc.SignerRole,
	fieldResponses map[string]json.RawMessage,
) TypstConverter

// Service implements the PDFRenderer interface using Typst.
type Service struct {
	typst            *TypstRenderer
	httpClient       *http.Client
	sem              chan struct{}
	acquireTimeout   time.Duration
	imageCache       *ImageCache
	converterFactory ConverterFactory
	tokens           TypstDesignTokens
	storageAdapter   port.StorageAdapter
}

// NewService creates a new Typst-based PDF renderer service.
// storageAdapter is optional (may be nil); when provided, storage:// image URLs in documents
// are resolved directly from storage rather than failing during PDF rendering.
func NewService(opts TypstOptions, imageCache *ImageCache, factory ConverterFactory, tokens TypstDesignTokens, storageAdapter port.StorageAdapter) (*Service, error) {
	typst, err := NewTypstRenderer(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create typst renderer: %w", err)
	}

	s := &Service{
		typst: typst,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		acquireTimeout:   opts.AcquireTimeout,
		imageCache:       imageCache,
		converterFactory: factory,
		tokens:           tokens,
		storageAdapter:   storageAdapter,
	}

	if opts.MaxConcurrent > 0 {
		s.sem = make(chan struct{}, opts.MaxConcurrent)
	}
	if s.acquireTimeout == 0 {
		s.acquireTimeout = 5 * time.Second
	}

	return s, nil
}

// RenderPreview generates a preview PDF with injected values.
func (s *Service) RenderPreview(ctx context.Context, req *port.RenderPreviewRequest) (*port.RenderPreviewResult, error) {
	if err := s.acquireSlot(ctx); err != nil {
		return nil, err
	}
	defer s.releaseSlot()

	if req.Document == nil {
		return nil, fmt.Errorf("document is required")
	}

	// Resolve signer role values if not provided
	signerRoleValues := req.SignerRoleValues
	if signerRoleValues == nil {
		signerRoleValues = s.resolveSignerRoleValues(req.Document.SignerRoles, req.Injectables)
	}

	// Ensure defaults map is not nil
	injectableDefaults := req.InjectableDefaults
	if injectableDefaults == nil {
		injectableDefaults = make(map[string]string)
	}

	// Ensure field responses map is not nil
	fieldResponses := req.FieldResponses
	if fieldResponses == nil {
		fieldResponses = make(map[string]json.RawMessage)
	}

	// Create converter for this request
	converter := s.converterFactory(req.Injectables, injectableDefaults, signerRoleValues, req.Document.SignerRoles, fieldResponses)

	// Build Typst document
	builder := NewTypstBuilder(converter, s.tokens)
	typstSource, pageCount, signatureFields := builder.Build(req.Document)

	// Resolve remote images
	remoteImages := builder.RemoteImages()
	remoteImages = s.resolveStorageEntries(ctx, remoteImages)
	rootDir, renames, cleanup, err := s.resolveRemoteImages(ctx, remoteImages)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	for oldName, newName := range renames {
		typstSource = strings.ReplaceAll(typstSource, oldName, newName)
	}

	// Generate PDF using Typst
	pdfBytes, err := s.typst.GeneratePDF(ctx, typstSource, rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	// Extract actual anchor positions from generated PDF. Signature fields must never
	// fall back to approximate/static coordinates because provider fields are
	// absolute and stale/wrong positions are hard to repair once submitted.
	if len(signatureFields) > 0 {
		signatureFields, err = s.extractAndUpdatePositions(ctx, pdfBytes, signatureFields)
		if err != nil {
			return nil, fmt.Errorf("extracting signature positions: %w", err)
		}
	}

	// Generate filename from document title
	filename := s.generateFilename(req.Document.Meta.Title)

	return &port.RenderPreviewResult{
		PDF:             pdfBytes,
		Filename:        filename,
		PageCount:       pageCount,
		SignatureFields: signatureFields,
	}, nil
}

// resolveStorageEntries converts storage:// entries in the images map to data: URIs
// by downloading bytes directly from the storage adapter. Non-storage entries pass through.
// images is url -> localFilename (same layout as typstConverter.remoteImages).
func (s *Service) resolveStorageEntries(ctx context.Context, images map[string]string) map[string]string {
	if s.storageAdapter == nil || len(images) == 0 {
		return images
	}
	out := make(map[string]string, len(images))
	for rawURL, filename := range images {
		if !strings.HasPrefix(rawURL, "storage://") {
			out[rawURL] = filename
			continue
		}
		key := strings.TrimPrefix(rawURL, "storage://")
		data, err := s.storageAdapter.Download(ctx, &port.StorageRequest{
			Key:         key,
			Environment: entity.EnvironmentProd,
		})
		if err != nil {
			slog.WarnContext(ctx, "gallery image not found for PDF render",
				slog.String("key", key),
				slog.String("error", err.Error()),
			)
			continue
		}
		ct := http.DetectContentType(data)
		out["data:"+ct+";base64,"+base64.StdEncoding.EncodeToString(data)] = filename
	}
	return out
}

// resolveRemoteImages handles image resolution via cache or direct download.
// Returns rootDir, renames map, optional cleanup func, and error.
func (s *Service) resolveRemoteImages(ctx context.Context, images map[string]string) (string, map[string]string, func(), error) {
	if len(images) == 0 {
		return "", nil, nil, nil
	}

	if s.imageCache != nil {
		renames := s.imageCache.ResolveImages(ctx, images, s.httpClient)
		return s.imageCache.Dir(), renames, nil, nil
	}

	tmpDir, err := os.MkdirTemp("", "typst-images-*")
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	renames, dlErr := downloadImages(ctx, images, tmpDir, s.httpClient)
	if dlErr != nil {
		slog.WarnContext(ctx, "some images failed to download", slog.Any("error", dlErr))
	}

	return tmpDir, renames, func() { _ = os.RemoveAll(tmpDir) }, nil
}

// resolveSignerRoleValues resolves signer role values from the document and injectables.
func (s *Service) resolveSignerRoleValues(
	signerRoles []portabledoc.SignerRole,
	injectables map[string]any,
) map[string]port.SignerRoleValue {
	result := make(map[string]port.SignerRoleValue)

	for _, role := range signerRoles {
		result[role.ID] = port.SignerRoleValue{
			Name:  resolveSignerRoleFieldValue(role.Name, injectables),
			Email: resolveSignerRoleFieldValue(role.Email, injectables),
		}
	}

	return result
}

func resolveSignerRoleFieldValue(
	field portabledoc.FieldValue,
	injectables map[string]any,
) string {
	if field.IsText() {
		return field.Value
	}
	if !field.IsInjectable() {
		return ""
	}

	refs := field.InjectableRefs()
	if len(refs) == 0 {
		return ""
	}

	resolved := make([]string, 0, len(refs))
	for _, ref := range refs {
		if v, ok := injectables[ref]; ok {
			resolved = append(resolved, fmt.Sprintf("%v", v))
		}
	}
	if len(resolved) == 0 {
		return ""
	}
	return strings.Join(resolved, field.ResolveSeparator())
}

// generateFilename creates a safe filename from the document title.
func (s *Service) generateFilename(title string) string {
	if title == "" {
		return "document.pdf"
	}

	// Simple sanitization - remove problematic characters
	safe := make([]rune, 0, len(title))
	for _, r := range title {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == ' ' {
			safe = append(safe, r)
		}
	}

	filename := string(safe)
	if filename == "" {
		filename = "document"
	}

	return filename + ".pdf"
}

// extractAndUpdatePositions extracts anchor positions from PDF and updates signature fields.
func (s *Service) extractAndUpdatePositions(
	ctx context.Context,
	pdfBytes []byte,
	fields []port.SignatureField,
) ([]port.SignatureField, error) {
	tmpPath, cleanup, err := s.writeTempPDF(ctx, pdfBytes)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	positions, err := s.extractPositions(ctx, tmpPath, fields)
	if err != nil {
		return nil, err
	}

	updated := s.updateFieldPositions(ctx, fields, positions)
	if err := requireExtractedSignaturePositions(updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// writeTempPDF writes PDF bytes to a temp file and returns path + cleanup function.
func (s *Service) writeTempPDF(ctx context.Context, pdfBytes []byte) (string, func(), error) {
	tmpFile, err := os.CreateTemp("", "sig-extract-*.pdf")
	if err != nil {
		slog.WarnContext(ctx, "failed to create temp file", "error", err)
		return "", nil, err
	}

	tmpPath := tmpFile.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmpFile.Write(pdfBytes); err != nil {
		_ = tmpFile.Close()
		cleanup()
		slog.WarnContext(ctx, "failed to write temp PDF", "error", err)
		return "", nil, err
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		slog.WarnContext(ctx, "failed to close temp PDF", "error", err)
		return "", nil, err
	}

	return tmpPath, cleanup, nil
}

// extractPositions extracts anchor positions from the PDF file.
func (s *Service) extractPositions(
	ctx context.Context,
	tmpPath string,
	fields []port.SignatureField,
) (map[string]AnchorPosition, error) {
	anchors := s.collectAnchors(fields)
	slog.DebugContext(ctx, "extracting anchors", "count", len(anchors))

	positions, err := ExtractAnchorPositions(ctx, tmpPath, anchors)
	if err != nil {
		slog.WarnContext(ctx, "anchor extraction failed", "error", err)
		return nil, err
	}

	slog.DebugContext(ctx, "anchors found", "count", len(positions))
	return positions, nil
}

// collectAnchors collects non-empty anchor strings from fields.
func (s *Service) collectAnchors(fields []port.SignatureField) []string {
	anchors := make([]string, 0, len(fields))
	for _, f := range fields {
		if f.AnchorString != "" {
			anchors = append(anchors, f.AnchorString)
		}
	}
	return anchors
}

// updateFieldPositions updates field positions based on extracted anchor positions.
func (s *Service) updateFieldPositions(
	ctx context.Context,
	fields []port.SignatureField,
	positions map[string]AnchorPosition,
) []port.SignatureField {
	updated := make([]port.SignatureField, len(fields))
	copy(updated, fields)

	for i := range updated {
		pos, ok := positions[updated[i].AnchorString]
		if !ok {
			continue
		}
		s.setRawPosition(ctx, &updated[i], pos)
	}
	return updated
}

// setRawPosition stores raw PDF coordinates on the field for later provider-specific conversion.

func requireExtractedSignaturePositions(fields []port.SignatureField) error {
	missing := make([]string, 0)
	for _, field := range fields {
		if field.PDFPageW > 0 && field.PDFPageH > 0 && field.PDFPointY > 0 && field.Page > 0 {
			continue
		}
		identifier := field.AnchorString
		if identifier == "" {
			identifier = field.RoleID
		}
		missing = append(missing, identifier)
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing extracted PDF coordinates for signature anchors: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (s *Service) setRawPosition(ctx context.Context, field *port.SignatureField, pos AnchorPosition) {
	field.Page = pos.Page
	field.PDFPointX = pos.X
	field.PDFPointY = pos.Y
	field.PDFPageW = pos.PageWidth
	field.PDFPageH = pos.PageHeight
	field.PDFAnchorW = pos.Width
	slog.DebugContext(ctx, "raw position set", "anchor", field.AnchorString,
		"x", pos.X, "y", pos.Y, "anchorW", pos.Width, "page", pos.Page)
}

// acquireSlot blocks until a render slot is available or the timeout expires.
func (s *Service) acquireSlot(ctx context.Context) error {
	if s.sem == nil {
		return nil
	}
	timer := time.NewTimer(s.acquireTimeout)
	defer timer.Stop()
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return entity.ErrRendererBusy
	}
}

// releaseSlot returns a render slot to the pool.
func (s *Service) releaseSlot() {
	if s.sem == nil {
		return
	}
	<-s.sem
}

// Close releases resources held by the service.
func (s *Service) Close() error {
	if s.imageCache != nil {
		s.imageCache.Close()
	}
	if s.typst != nil {
		return s.typst.Close()
	}
	return nil
}

// Ensure Service implements port.PDFRenderer
var _ port.PDFRenderer = (*Service)(nil)
