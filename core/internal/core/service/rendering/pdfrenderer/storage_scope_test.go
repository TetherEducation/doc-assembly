package pdfrenderer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"reflect"
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// The workspaces of these tests: a production workspace and its sandbox, and
// another production workspace with a sandbox of its own.
const (
	scopeWorkspace      = "5f0c9a52-7c3e-4c7b-9d55-2f4f6a1f0e11"
	scopeSandbox        = "7a1d3e90-2b4c-4d6e-8f10-3c5e7a9b1d22"
	scopeOtherWorkspace = "0b54f1c2-6f0e-4a55-9a53-0d0f1f8f4b11"
	scopeOtherSandbox   = "c4e8a1b6-9d2f-4e37-b5a0-6f1d8c3e7a44"
)

// recordingStorage serves the objects it holds and records every key it is
// asked to download.
type recordingStorage struct {
	port.StorageAdapter
	objects    map[string][]byte
	downloaded []string
}

// newRecordingStorage returns a storage that holds a different image under
// each key.
func newRecordingStorage(keys ...string) *recordingStorage {
	storage := &recordingStorage{objects: make(map[string][]byte, len(keys))}
	for i, key := range keys {
		storage.objects[key] = onePixelPNG(uint8(i))
	}
	return storage
}

// onePixelPNG returns an opaque 1x1 PNG; each shade gives different bytes.
func onePixelPNG(shade uint8) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 40 + shade, G: 90, B: 160, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (s *recordingStorage) Download(_ context.Context, req *port.StorageRequest) ([]byte, error) {
	s.downloaded = append(s.downloaded, req.Key)
	data, ok := s.objects[req.Key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return data, nil
}

// recordingWorkspaces is a WorkspaceFinder over a fixed set of workspaces that
// records the IDs it is asked for. With err set it fails, and still returns
// the workspace it has for the ID.
type recordingWorkspaces struct {
	byID  map[string]*entity.Workspace
	err   error
	asked []string
}

func (w *recordingWorkspaces) FindByID(_ context.Context, id string) (*entity.Workspace, error) {
	w.asked = append(w.asked, id)
	if w.err != nil {
		return w.byID[id], w.err
	}
	workspace, ok := w.byID[id]
	if !ok {
		return nil, entity.ErrWorkspaceNotFound
	}
	return workspace, nil
}

func scopeWorkspaces() *recordingWorkspaces {
	return &recordingWorkspaces{byID: map[string]*entity.Workspace{
		scopeWorkspace:      {ID: scopeWorkspace},
		scopeSandbox:        sandboxOf(scopeSandbox, scopeWorkspace),
		scopeOtherWorkspace: {ID: scopeOtherWorkspace},
		scopeOtherSandbox:   sandboxOf(scopeOtherSandbox, scopeOtherWorkspace),
	}}
}

func sandboxOf(id, parentID string) *entity.Workspace {
	return &entity.Workspace{ID: id, IsSandbox: true, SandboxOfID: &parentID}
}

// galleryKey is the key the gallery gives an uploaded file in a workspace.
func galleryKey(workspaceID, filename string) string {
	sum := sha256.Sum256([]byte(filename))
	return entity.NewGalleryAssetKey(workspaceID, hex.EncodeToString(sum[:]), filename)
}

func assertDownloaded(t *testing.T, storage *recordingStorage, want ...string) {
	t.Helper()
	if len(want) == 0 && len(storage.downloaded) == 0 {
		return
	}
	if !reflect.DeepEqual(storage.downloaded, want) {
		t.Errorf("storage was asked for %q, want %q", storage.downloaded, want)
	}
}

// A stored image is resolved only when its key is a gallery key of the
// workspace the render is for or, for a sandbox, of the workspace the sandbox
// belongs to. Every case holds an object under the key, so a key that is not
// resolved is one storage could have served.
func TestResolveStorageEntries_ResolvesOnlyGalleryKeysOfTheRenderWorkspace(t *testing.T) {
	const name = "0123456789ab-letterhead.png"
	// What an unset workspace ID parses to must not count as a workspace.
	const zeroWorkspace = "00000000-0000-0000-0000-000000000000"
	ownKey := galleryKey(scopeWorkspace, "letterhead.png")
	otherKey := galleryKey(scopeOtherWorkspace, "letterhead.png")
	noParent := ""

	cases := []struct {
		name        string
		workspaceID string
		workspaces  WorkspaceFinder
		key         string
		resolved    bool
	}{
		{
			name:        "key of the render's workspace",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: ownKey, resolved: true,
		},
		{
			name:        "key of the render's workspace, without a workspace finder",
			workspaceID: scopeWorkspace,
			key:         ownKey, resolved: true,
		},
		{
			name:        "key that spells the render's workspace in upper case",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: galleryKey(strings.ToUpper(scopeWorkspace), "letterhead.png"), resolved: true,
		},
		{
			name:        "render that spells its workspace in upper case",
			workspaceID: strings.ToUpper(scopeWorkspace), workspaces: scopeWorkspaces(),
			key: ownKey, resolved: true,
		},
		{
			name:        "key of another workspace",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: otherKey,
		},
		{
			name:        "key of another workspace, without a workspace finder",
			workspaceID: scopeWorkspace,
			key:         otherKey,
		},
		{
			name:        "sandbox render, key of the workspace the sandbox belongs to",
			workspaceID: scopeSandbox, workspaces: scopeWorkspaces(),
			key: ownKey, resolved: true,
		},
		{
			name:        "sandbox render, key of another workspace",
			workspaceID: scopeSandbox, workspaces: scopeWorkspaces(),
			key: otherKey,
		},
		{
			name:        "sandbox render, key of another workspace's sandbox",
			workspaceID: scopeSandbox, workspaces: scopeWorkspaces(),
			key: galleryKey(scopeOtherSandbox, "letterhead.png"),
		},
		{
			name:        "sandbox of another workspace, key of this one",
			workspaceID: scopeOtherSandbox, workspaces: scopeWorkspaces(),
			key: ownKey,
		},
		{
			name:        "production render, key under its sandbox",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: galleryKey(scopeSandbox, "letterhead.png"),
		},
		{
			name:        "sandbox render, without a workspace finder",
			workspaceID: scopeSandbox,
			key:         ownKey,
		},
		{
			name:        "sandbox render, the workspace finder fails",
			workspaceID: scopeSandbox, workspaces: &recordingWorkspaces{err: errors.New("connection refused")},
			key: ownKey,
		},
		{
			name:        "sandbox render, the workspace finder fails and returns the sandbox all the same",
			workspaceID: scopeSandbox,
			workspaces: &recordingWorkspaces{
				byID: map[string]*entity.Workspace{scopeSandbox: sandboxOf(scopeSandbox, scopeWorkspace)},
				err:  errors.New("connection reset"),
			},
			key: ownKey,
		},
		{
			name:        "sandbox render, the workspace finder does not know the sandbox",
			workspaceID: scopeSandbox, workspaces: &recordingWorkspaces{},
			key: ownKey,
		},
		{
			name:        "sandbox render, the workspace finder returns no workspace",
			workspaceID: scopeSandbox,
			workspaces:  &recordingWorkspaces{byID: map[string]*entity.Workspace{scopeSandbox: nil}},
			key:         ownKey,
		},
		{
			name:        "sandbox that names no parent",
			workspaceID: scopeSandbox,
			workspaces: &recordingWorkspaces{byID: map[string]*entity.Workspace{
				scopeSandbox: {ID: scopeSandbox, IsSandbox: true},
			}},
			key: ownKey,
		},
		{
			name:        "sandbox whose parent is not a workspace ID",
			workspaceID: scopeSandbox,
			workspaces: &recordingWorkspaces{byID: map[string]*entity.Workspace{
				scopeSandbox: {ID: scopeSandbox, IsSandbox: true, SandboxOfID: &noParent},
			}},
			key: ownKey,
		},
		{
			name:        "sandbox whose parent is not a workspace ID, key under the zero workspace ID",
			workspaceID: scopeSandbox,
			workspaces: &recordingWorkspaces{byID: map[string]*entity.Workspace{
				scopeSandbox: {ID: scopeSandbox, IsSandbox: true, SandboxOfID: &noParent},
			}},
			key: "gallery/" + zeroWorkspace + "/" + name,
		},
		{
			name:        "workspace that names a parent but is not a sandbox",
			workspaceID: scopeOtherWorkspace,
			workspaces: &recordingWorkspaces{byID: map[string]*entity.Workspace{
				scopeOtherWorkspace: {ID: scopeOtherWorkspace, SandboxOfID: ptrTo(scopeWorkspace)},
			}},
			key: ownKey,
		},
		{
			name:       "render that names no workspace",
			workspaces: scopeWorkspaces(),
			key:        ownKey,
		},
		{
			name:       "render that names no workspace, key under the zero workspace ID",
			workspaces: scopeWorkspaces(),
			key:        "gallery/" + zeroWorkspace + "/" + name,
		},
		{
			name:        "render whose workspace is not a workspace ID",
			workspaceID: "workspace-1", workspaces: scopeWorkspaces(),
			key: "gallery/workspace-1/" + name,
		},
		{
			name:        "a rendered document of the render's workspace",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: "documents/" + scopeWorkspace + "/9d1c7c0e-3a57-4f0a-8a51-6f1f6f0f7a22/attempts/3c1b2a4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d/pre-signed.pdf",
		},
		{
			name:        "an uploaded source document",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: "templates/3c1b2a4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d/source.pdf",
		},
		{
			name:        "parent segment out of the render's workspace",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: "gallery/" + scopeWorkspace + "/../" + scopeOtherWorkspace + "/" + name,
		},
		{
			name:        "backslash parent segment out of the render's workspace",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: "gallery/" + scopeWorkspace + `/..\` + scopeOtherWorkspace + `\` + name,
		},
		{
			name:        "parent segments out of the storage root",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: "../../etc/" + name,
		},
		{
			name:        "extra segment under the render's workspace",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: "gallery/" + scopeWorkspace + "/nested/" + name,
		},
		{
			name:        "the render's workspace folder itself",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: "gallery/" + scopeWorkspace,
		},
		{
			name:        "empty key",
			workspaceID: scopeWorkspace, workspaces: scopeWorkspaces(),
			key: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storage := newRecordingStorage(tc.key)
			service := &Service{storageAdapter: storage, workspaces: tc.workspaces}

			out := service.resolveStorageEntries(context.Background(), map[string]string{
				"storage://" + tc.key: "img_1.png",
			}, tc.workspaceID)

			if !tc.resolved {
				assertDownloaded(t, storage)
				if len(out) != 0 {
					t.Errorf("resolved %d image(s), want the key left out", len(out))
				}
				return
			}

			assertDownloaded(t, storage, tc.key)
			if len(out) != 1 {
				t.Fatalf("resolved %d image(s), want 1", len(out))
			}
			for source, filename := range out {
				if !strings.HasPrefix(source, "data:image/png;base64,") {
					t.Errorf("resolved source = %.40q, want a PNG data URI", source)
				}
				if filename != "img_1.png" {
					t.Errorf("resolved filename = %q, want %q", filename, "img_1.png")
				}
			}
		})
	}
}

// A key outside the render's workspace ends up exactly where a key storage
// does not have ends up: left out of the images, with the other images
// untouched. The only difference is that storage is never asked for it.
func TestResolveStorageEntries_LeavesOutAKeyOutsideTheWorkspaceLikeAMissingOne(t *testing.T) {
	const remote = "https://example.test/seal.png"
	outside := galleryKey(scopeOtherWorkspace, "letterhead.png")
	missing := galleryKey(scopeWorkspace, "letterhead.png")

	resolve := func(storage *recordingStorage, key string) map[string]string {
		service := &Service{storageAdapter: storage, workspaces: scopeWorkspaces()}
		return service.resolveStorageEntries(context.Background(), map[string]string{
			"storage://" + key: "img_1.png",
			remote:             "img_2.png",
		}, scopeWorkspace)
	}

	outsideStorage := newRecordingStorage(outside)
	outsideOut := resolve(outsideStorage, outside)
	missingStorage := newRecordingStorage()
	missingOut := resolve(missingStorage, missing)

	want := map[string]string{remote: "img_2.png"}
	if !reflect.DeepEqual(missingOut, want) {
		t.Errorf("with a missing key: images = %v, want %v", missingOut, want)
	}
	if !reflect.DeepEqual(outsideOut, missingOut) {
		t.Errorf("with a key outside the workspace: images = %v, want what a missing key leaves: %v", outsideOut, missingOut)
	}
	assertDownloaded(t, missingStorage, missing)
	assertDownloaded(t, outsideStorage)
}

// The workspace finder is asked only when a key belongs to another workspace
// than the render's, and then once for the whole render.
func TestResolveStorageEntries_AsksForTheWorkspaceOnceAndOnlyWhenNeeded(t *testing.T) {
	t.Run("keys of the render's workspace", func(t *testing.T) {
		first, second := galleryKey(scopeWorkspace, "letterhead.png"), galleryKey(scopeWorkspace, "seal.png")
		workspaces := scopeWorkspaces()
		service := &Service{storageAdapter: newRecordingStorage(first, second), workspaces: workspaces}

		out := service.resolveStorageEntries(context.Background(), map[string]string{
			"storage://" + first:  "img_1.png",
			"storage://" + second: "img_2.png",
		}, scopeWorkspace)

		if len(out) != 2 {
			t.Errorf("resolved %d image(s), want 2", len(out))
		}
		if len(workspaces.asked) != 0 {
			t.Errorf("the workspace finder was asked for %q, want no lookup", workspaces.asked)
		}
	})

	t.Run("sandbox render with keys of its parent and of another workspace", func(t *testing.T) {
		first, second := galleryKey(scopeWorkspace, "letterhead.png"), galleryKey(scopeWorkspace, "seal.png")
		outside := galleryKey(scopeOtherWorkspace, "letterhead.png")
		workspaces := scopeWorkspaces()
		service := &Service{storageAdapter: newRecordingStorage(first, second, outside), workspaces: workspaces}

		out := service.resolveStorageEntries(context.Background(), map[string]string{
			"storage://" + first:   "img_1.png",
			"storage://" + second:  "img_2.png",
			"storage://" + outside: "img_3.png",
		}, strings.ToUpper(scopeSandbox))

		if len(out) != 2 {
			t.Errorf("resolved %d image(s), want 2", len(out))
		}
		if want := []string{scopeSandbox}; !reflect.DeepEqual(workspaces.asked, want) {
			t.Errorf("the workspace finder was asked for %q, want %q", workspaces.asked, want)
		}
	})
}

// A stored image can be named by an image node, by the document header, or by
// the value or the default of an injectable either of them is bound to. Each
// way goes through the same check, with the workspace of the render request.
func TestBuildTypst_ChecksEveryWayAStoredImageIsNamed(t *testing.T) {
	const injectable = "campus_logo"
	imageNode := func(src string) portabledoc.Node {
		return portabledoc.Node{Type: portabledoc.NodeTypeImage, Attrs: map[string]any{"src": src, "width": float64(120)}}
	}
	boundImageNode := portabledoc.Node{
		Type:  portabledoc.NodeTypeCustomImage,
		Attrs: map[string]any{"injectableId": injectable, "width": float64(120)},
	}
	header := func(src string) *portabledoc.DocumentHeader {
		return &portabledoc.DocumentHeader{Enabled: true, Layout: portabledoc.HeaderLayoutImageLeft, ImageURL: &src}
	}
	boundHeader := &portabledoc.DocumentHeader{
		Enabled: true, Layout: portabledoc.HeaderLayoutImageLeft, ImageInjectableID: ptrTo(injectable),
	}

	sources := []struct {
		name    string
		request func(src string) *port.RenderPreviewRequest
	}{
		{"image node", func(src string) *port.RenderPreviewRequest {
			return &port.RenderPreviewRequest{Document: scopeDocument(nil, imageNode(src))}
		}},
		{"header image", func(src string) *port.RenderPreviewRequest {
			return &port.RenderPreviewRequest{Document: scopeDocument(header(src))}
		}},
		{"value of an image node's injectable", func(src string) *port.RenderPreviewRequest {
			return &port.RenderPreviewRequest{
				Document:    scopeDocument(nil, boundImageNode),
				Injectables: map[string]any{injectable: src},
			}
		}},
		{"default of an image node's injectable", func(src string) *port.RenderPreviewRequest {
			return &port.RenderPreviewRequest{
				Document:           scopeDocument(nil, boundImageNode),
				InjectableDefaults: map[string]string{injectable: src},
			}
		}},
		{"value of the header image's injectable", func(src string) *port.RenderPreviewRequest {
			return &port.RenderPreviewRequest{
				Document:    scopeDocument(boundHeader),
				Injectables: map[string]any{injectable: src},
			}
		}},
		{"default of the header image's injectable", func(src string) *port.RenderPreviewRequest {
			return &port.RenderPreviewRequest{
				Document:           scopeDocument(boundHeader),
				InjectableDefaults: map[string]string{injectable: src},
			}
		}},
	}

	ownKey := galleryKey(scopeWorkspace, "letterhead.png")
	otherKey := galleryKey(scopeOtherWorkspace, "letterhead.png")
	renders := []struct {
		name        string
		workspaceID string
		key         string
		resolved    bool
	}{
		{"key of the render's workspace", scopeWorkspace, ownKey, true},
		{"key of another workspace", scopeWorkspace, otherKey, false},
		{"sandbox render, key of its parent", scopeSandbox, ownKey, true},
		{"sandbox render, key of another workspace", scopeSandbox, otherKey, false},
		{"no workspace", "", ownKey, false},
	}

	for _, source := range sources {
		for _, render := range renders {
			t.Run(source.name+"/"+render.name, func(t *testing.T) {
				storage := newRecordingStorage(ownKey, otherKey)
				tokens := DefaultDesignTokens()
				service := &Service{
					converterFactory: NewTypstConverterFactory(tokens),
					tokens:           tokens,
					storageAdapter:   storage,
					workspaces:       scopeWorkspaces(),
				}
				req := source.request("storage://" + render.key)
				req.WorkspaceID = render.workspaceID

				typstSource, _, _, images := service.buildTypst(context.Background(), req)

				if !strings.Contains(typstSource, "#image(") {
					t.Fatalf("the Typst source has no image:\n%s", typstSource)
				}
				if !render.resolved {
					assertDownloaded(t, storage)
					if len(images) != 0 {
						t.Errorf("resolved %d image(s), want the key left out", len(images))
					}
					return
				}
				assertDownloaded(t, storage, render.key)
				if len(images) != 1 {
					t.Errorf("resolved %d image(s), want 1", len(images))
				}
			})
		}
	}
}

// End to end, with the real Typst binary: a stored image of the render's
// workspace is rendered, and one outside it fails the render exactly as a
// stored image that does not exist does, without storage being asked for it.
func TestRenderPreview_AStoredImageOutsideTheWorkspaceFailsLikeAMissingOne(t *testing.T) {
	own := galleryKey(scopeWorkspace, "letterhead.png")
	outside := galleryKey(scopeOtherWorkspace, "letterhead.png")
	missing := galleryKey(scopeWorkspace, "missing.png")
	storage := newRecordingStorage(own, outside)

	service, err := NewService(DefaultTypstOptions(), nil, NewTypstConverterFactory(DefaultDesignTokens()),
		DefaultDesignTokens(), storage, scopeWorkspaces())
	if err != nil {
		t.Skipf("Typst not available, skipping test: %v", err)
	}
	defer func() { _ = service.Close() }()

	render := func(key string) error {
		storage.downloaded = nil
		_, err := service.RenderPreview(context.Background(), &port.RenderPreviewRequest{
			Document: scopeDocument(nil, portabledoc.Node{
				Type:  portabledoc.NodeTypeImage,
				Attrs: map[string]any{"src": "storage://" + key, "width": float64(120)},
			}),
			WorkspaceID: scopeWorkspace,
		})
		return err
	}

	if err := render(own); err != nil {
		t.Fatalf("a stored image of the render's workspace did not render: %v", err)
	}
	assertDownloaded(t, storage, own)

	missingErr := render(missing)
	if missingErr == nil {
		t.Fatal("a stored image that does not exist rendered; this test assumes it fails the render")
	}
	assertDownloaded(t, storage, missing)

	outsideErr := render(outside)
	assertDownloaded(t, storage)
	if outsideErr == nil {
		t.Fatal("a stored image outside the render's workspace rendered")
	}
	if outsideErr.Error() != missingErr.Error() {
		t.Errorf("outside the workspace: %v\nmissing: %v\nwant the same failure", outsideErr, missingErr)
	}
}

// scopeDocument is a one-paragraph document with an optional header and extra
// nodes after the paragraph.
func scopeDocument(header *portabledoc.DocumentHeader, nodes ...portabledoc.Node) *portabledoc.Document {
	content := append([]portabledoc.Node{{
		Type:    portabledoc.NodeTypeParagraph,
		Content: []portabledoc.Node{{Type: portabledoc.NodeTypeText, Text: ptrTo("Stored image scope")}},
	}}, nodes...)

	return &portabledoc.Document{
		Version: portabledoc.CurrentVersion,
		Meta:    portabledoc.Meta{Title: "Stored image scope", Language: "en"},
		PageConfig: portabledoc.PageConfig{
			FormatID: portabledoc.PageFormatA4,
			Width:    794,
			Height:   1123,
			Margins:  portabledoc.Margins{Top: 96, Bottom: 96, Left: 72, Right: 72},
		},
		Header:  header,
		Content: &portabledoc.ProseMirrorDoc{Type: "doc", Content: content},
	}
}
