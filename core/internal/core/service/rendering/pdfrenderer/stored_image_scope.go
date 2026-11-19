package pdfrenderer

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// storedImageScope decides which stored images one render may resolve: the
// images of the gallery of the workspace the render is for and, when that
// workspace is a sandbox, those of the gallery of the workspace it belongs to.
// A sandbox has no gallery of its own, because the gallery routes carry no
// sandbox context, so a sandbox template names the keys of its parent.
//
// Nothing is in scope when the render names no workspace.
type storedImageScope struct {
	workspaces WorkspaceFinder

	workspace    uuid.UUID
	hasWorkspace bool

	parent         uuid.UUID
	hasParent      bool
	parentLookedUp bool
}

// newStoredImageScope returns the scope of a render for workspaceID. A
// workspaceID that is empty, or is not a workspace ID at all, gives a scope
// with nothing in it.
func newStoredImageScope(workspaces WorkspaceFinder, workspaceID string) *storedImageScope {
	scope := &storedImageScope{workspaces: workspaces}
	if id, err := uuid.Parse(workspaceID); err == nil {
		scope.workspace, scope.hasWorkspace = id, true
	}
	return scope
}

// allows reports whether key is a gallery key the render may ask storage for.
func (s *storedImageScope) allows(ctx context.Context, key string) bool {
	keyWorkspace, ok := entity.GalleryAssetKeyWorkspace(key)
	if !ok || !s.hasWorkspace {
		return false
	}
	if keyWorkspace == s.workspace {
		return true
	}
	parent, ok := s.sandboxParent(ctx)
	return ok && keyWorkspace == parent
}

// sandboxParent returns the workspace the render's workspace is a sandbox of.
// The workspace is looked up at most once per render, and only for a key of
// another workspace than the render's own, so a render that uses its own
// gallery makes no lookup. Without a finder, or when the lookup fails, there is
// no parent: an error never widens the scope.
func (s *storedImageScope) sandboxParent(ctx context.Context) (uuid.UUID, bool) {
	if s.parentLookedUp {
		return s.parent, s.hasParent
	}
	s.parentLookedUp = true

	if s.workspaces == nil {
		return uuid.Nil, false
	}
	workspace, err := s.workspaces.FindByID(ctx, s.workspace.String())
	if err != nil {
		slog.WarnContext(ctx, "could not load the workspace of a PDF render to resolve its stored images",
			slog.String("workspace_id", s.workspace.String()),
			slog.String("error", err.Error()),
		)
		return uuid.Nil, false
	}
	if workspace == nil || !workspace.IsSandbox || workspace.SandboxOfID == nil {
		return uuid.Nil, false
	}
	parent, err := uuid.Parse(*workspace.SandboxOfID)
	if err != nil {
		return uuid.Nil, false
	}

	s.parent, s.hasParent = parent, true
	return s.parent, true
}
