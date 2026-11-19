package entity

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// A gallery asset is stored under the key
//
//	gallery/<workspace ID>/<content hash>-<filename>
//
// The content hash is the first galleryKeyHashLen hexadecimal characters of the
// SHA-256 of the asset's content, and the filename is the name it was uploaded
// under, without path separators. NewGalleryAssetKey builds such a key and
// GalleryAssetKeyWorkspace reads one back. They live together so that what one
// writes is what the other accepts.
const (
	galleryKeyPrefix  = "gallery"
	galleryKeyHashLen = 12
)

// NewGalleryAssetKey returns the storage key of a gallery asset of a workspace.
// sha256Hex is the hexadecimal SHA-256 of the asset's content and filename is
// the name it was uploaded under.
func NewGalleryAssetKey(workspaceID, sha256Hex, filename string) string {
	return fmt.Sprintf("%s/%s/%s-%s",
		galleryKeyPrefix, workspaceID, sha256Hex[:galleryKeyHashLen], galleryAssetFilename(filename))
}

// GalleryAssetKeyWorkspace returns the workspace whose gallery a storage key
// belongs to. ok is false for a key NewGalleryAssetKey cannot have built: the
// key of another kind of stored object, a key with more or fewer than three
// segments (which covers "." and ".." segments), or one whose workspace is not
// a UUID. The workspace is returned parsed, so that two spellings of the same
// ID (upper and lower case, for example) compare equal.
func GalleryAssetKeyWorkspace(key string) (workspaceID uuid.UUID, ok bool) {
	segments := strings.Split(key, "/")
	if len(segments) != 3 || segments[0] != galleryKeyPrefix || !isGalleryAssetName(segments[2]) {
		return uuid.Nil, false
	}

	// uuid.Parse on its own accepts any character before and after a UUID.
	if uuid.Validate(segments[1]) != nil {
		return uuid.Nil, false
	}
	workspaceID, err := uuid.Parse(segments[1])
	if err != nil {
		return uuid.Nil, false
	}

	return workspaceID, true
}

// galleryAssetFilename returns an uploaded name in the form it takes in a key:
// one path segment, never empty.
func galleryAssetFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", "\x00", "")
	sanitized := strings.TrimSpace(replacer.Replace(name))
	if sanitized == "" {
		return "asset"
	}
	return sanitized
}

// isGalleryAssetName reports whether name is the last segment of a gallery
// key: "<content hash>-<filename>", with a filename galleryAssetFilename can
// return.
func isGalleryAssetName(name string) bool {
	if len(name) <= galleryKeyHashLen+1 || name[galleryKeyHashLen] != '-' {
		return false
	}
	for i := range galleryKeyHashLen {
		if !isLowerHexDigit(name[i]) {
			return false
		}
	}
	return !strings.ContainsAny(name, "/\\\x00")
}

func isLowerHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
}
