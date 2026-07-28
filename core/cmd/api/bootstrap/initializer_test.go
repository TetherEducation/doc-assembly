package bootstrap

import (
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
)

func TestReadOnlyViewPublicURLFallsBackToServerBasePath(t *testing.T) {
	cfg := config.ServerConfig{BasePath: "doc-assembly"}

	got := readOnlyViewPublicURL(cfg)

	if got != "/doc-assembly" {
		t.Fatalf("unexpected read-only public URL: got %q want %q", got, "/doc-assembly")
	}
}

func TestReadOnlyViewPublicURLPrefersExplicitPublicURL(t *testing.T) {
	cfg := config.ServerConfig{
		BasePath:  "doc-assembly",
		PublicURL: "https://sign.tether.education/doc-assembly",
	}

	got := readOnlyViewPublicURL(cfg)

	if got != "https://sign.tether.education/doc-assembly" {
		t.Fatalf("unexpected read-only public URL: got %q", got)
	}
}
