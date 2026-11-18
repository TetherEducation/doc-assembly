package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	galleryTestWorkspace      = "5f0c9a52-7c3e-4c7b-9d55-2f4f6a1f0e11"
	galleryTestOtherWorkspace = "0b54f1c2-6f0e-4a55-9a53-0d0f1f8f4b11"
)

func galleryTestDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// Every key the builder makes is read back as a key of the workspace it was
// built for, and of no other, whatever the uploaded file was called and however
// the workspace ID was spelled.
func TestGalleryAssetKey_RoundTrip(t *testing.T) {
	workspaces := map[string]string{
		"canonical":       galleryTestWorkspace,
		"upper case":      strings.ToUpper(galleryTestWorkspace),
		"without hyphens": strings.ReplaceAll(galleryTestWorkspace, "-", ""),
		"in braces":       "{" + galleryTestWorkspace + "}",
	}
	filenames := []string{
		"logo.png",
		"Logo Colegio (2026).PNG",
		"ñandú.svg",
		"firma.director.jpeg",
		"a/b/c.png",
		`a\b\c.png`,
		"../../etc/passwd",
		`..\..\secret.png`,
		"..",
		".",
		"",
		"   ",
		"\x00",
		"na\x00me.png",
		"  padded.png  ",
		"line\nbreak.png",
		"%2e%2e%2fencoded.png",
		"query?x=1#fragment.png",
		strings.Repeat("long", 60) + ".png",
	}
	want := uuid.MustParse(galleryTestWorkspace)
	other := uuid.MustParse(galleryTestOtherWorkspace)

	for spelling, workspaceID := range workspaces {
		for _, filename := range filenames {
			key := NewGalleryAssetKey(workspaceID, galleryTestDigest(filename), filename)

			got, ok := GalleryAssetKeyWorkspace(key)

			require.Truef(t, ok, "%s workspace ID, filename %q: built key %q is not read as a gallery key", spelling, filename, key)
			assert.Equalf(t, want, got, "%s workspace ID, filename %q: key %q", spelling, filename, key)
			assert.NotEqualf(t, other, got, "%s workspace ID, filename %q: key %q", spelling, filename, key)
		}
	}
}

func TestNewGalleryAssetKey_Layout(t *testing.T) {
	digest := galleryTestDigest("content")

	assert.Equal(t,
		"gallery/"+galleryTestWorkspace+"/"+digest[:12]+"-logo.png",
		NewGalleryAssetKey(galleryTestWorkspace, digest, "logo.png"),
	)
	assert.Equal(t,
		"gallery/"+galleryTestWorkspace+"/"+digest[:12]+"-a_b_c.png",
		NewGalleryAssetKey(galleryTestWorkspace, digest, `a/b\c.png`),
		"path separators in the uploaded name do not add segments to the key",
	)
	assert.Equal(t,
		"gallery/"+galleryTestWorkspace+"/"+digest[:12]+"-asset",
		NewGalleryAssetKey(galleryTestWorkspace, digest, " \x00 "),
		`an uploaded name with nothing left in it is stored as "asset"`,
	)
}

// Anything the builder cannot have made is not a gallery key, so it names no
// workspace.
func TestGalleryAssetKeyWorkspace_RefusesKeysTheBuilderCannotMake(t *testing.T) {
	const name = "0123456789ab-logo.png"
	prefix := "gallery/" + galleryTestWorkspace + "/"

	cases := map[string]string{
		"empty":                              "",
		"prefix only":                        "gallery",
		"no name segment":                    "gallery/" + galleryTestWorkspace,
		"empty name":                         prefix,
		"another top-level folder":           "documents/" + galleryTestWorkspace + "/" + name,
		"a stored document":                  "documents/" + galleryTestWorkspace + "/9d1c7c0e-3a57-4f0a-8a51-6f1f6f0f7a22/attempts/3c1b2a4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d/pre-signed.pdf",
		"prefix in another case":             "Gallery/" + galleryTestWorkspace + "/" + name,
		"leading slash":                      "/" + prefix + name,
		"trailing slash":                     prefix + name + "/",
		"empty segment":                      "gallery//" + galleryTestWorkspace + "/" + name,
		"extra segment":                      prefix + "nested/" + name,
		"parent segment":                     prefix + "../" + galleryTestOtherWorkspace + "/" + name,
		"parent segment as the name":         prefix + "..",
		"current segment as the name":        prefix + ".",
		"backslash parent segment":           prefix + `..\` + galleryTestOtherWorkspace + `\` + name,
		"backslashes instead of slashes":     `gallery\` + galleryTestWorkspace + `\` + name,
		"backslash in the name":              prefix + `0123456789ab-a\b.png`,
		"NUL in the name":                    prefix + "0123456789ab-a\x00b.png",
		"workspace is not a UUID":            "gallery/workspace-1/" + name,
		"workspace is a truncated UUID":      "gallery/" + galleryTestWorkspace[:35] + "/" + name,
		"workspace between spaces":           "gallery/ " + galleryTestWorkspace + " /" + name,
		"workspace between backslashes":      `gallery/\` + galleryTestWorkspace + `\/` + name,
		"workspace with an unclosed brace":   "gallery/{" + galleryTestWorkspace + "]/" + name,
		"name without the content hash":      prefix + "logo.png",
		"content hash too short":             prefix + "0123456789a-logo.png",
		"content hash not hexadecimal":       prefix + "0123456789az-logo.png",
		"content hash in upper case":         prefix + "0123456789AB-logo.png",
		"content hash with nothing after it": prefix + "0123456789ab-",
		"content hash without its separator": prefix + "0123456789ablogo.png",
		"percent-encoded slashes":            "gallery%2F" + galleryTestWorkspace + "%2F" + name,
	}

	for label, key := range cases {
		got, ok := GalleryAssetKeyWorkspace(key)

		assert.Falsef(t, ok, "%s: %q was read as a gallery key", label, key)
		assert.Equalf(t, uuid.Nil, got, "%s: %q", label, key)
	}
}
