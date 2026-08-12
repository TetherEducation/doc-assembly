package pdfrenderer

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSanitizeTransparentRaster_BleedsTransparentEdgesWithoutChangingAlpha(t *testing.T) {
	input := makeTransparentEdgePNG(t)

	got, ext, err := sanitizeTransparentRaster(input, ".png")
	if err != nil {
		t.Fatalf("sanitizeTransparentRaster returned error: %v", err)
	}
	if ext != ".png" {
		t.Fatalf("expected sanitized extension .png, got %q", ext)
	}

	img, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("decode sanitized png: %v", err)
	}

	corner := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	if corner.A != 0 {
		t.Fatalf("expected transparent corner alpha to remain 0, got %d", corner.A)
	}
	if corner.R == 0 && corner.G == 0 && corner.B == 0 {
		t.Fatalf("expected transparent corner RGB to be bled from opaque neighbor, got %#v", corner)
	}
}

func TestDownloadImages_SanitizesTransparentRasterIntoPNG(t *testing.T) {
	input := makeTransparentEdgePNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(input)
	}))
	defer server.Close()

	dir := t.TempDir()
	renames, err := downloadImages(context.Background(), map[string]string{
		server.URL + "/logo.png": "img_1.png",
	}, dir, server.Client())
	if err != nil {
		t.Fatalf("downloadImages returned error: %v", err)
	}
	if len(renames) != 0 {
		t.Fatalf("expected no rename for already-png input, got %#v", renames)
	}

	data, err := os.ReadFile(filepath.Join(dir, "img_1.png"))
	if err != nil {
		t.Fatalf("read sanitized image: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode sanitized cached png: %v", err)
	}

	corner := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	if corner.A != 0 {
		t.Fatalf("expected cached transparent corner alpha to remain 0, got %d", corner.A)
	}
	if corner.R == 0 && corner.G == 0 && corner.B == 0 {
		t.Fatalf("expected cached transparent corner RGB to be bled from opaque neighbor, got %#v", corner)
	}
}

func makeTransparentEdgePNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	transparentBlack := color.NRGBA{R: 0, G: 0, B: 0, A: 0}
	opaqueRed := color.NRGBA{R: 220, G: 40, B: 40, A: 255}

	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			img.SetNRGBA(x, y, transparentBlack)
		}
	}
	img.SetNRGBA(1, 1, opaqueRed)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

// A failed download must not be recorded under the image's own cache key: doing
// so served the placeholder for every later render until the entry aged out, so
// one blip from the image host degraded a school's letterhead for the whole
// cache window.
func TestImageCache_FailedDownloadIsNotCachedUnderImageKey(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(makeTransparentEdgePNG(t))
	}))
	defer server.Close()

	cache, err := NewImageCache(ImageCacheOptions{Dir: t.TempDir(), MaxAge: time.Hour})
	if err != nil {
		t.Fatalf("NewImageCache returned error: %v", err)
	}
	defer cache.Close()

	url := server.URL + "/logo.png"

	// Deliberately asserted through observable behaviour rather than the
	// placeholder's filename, so this still fails against the old
	// store-under-the-url-key implementation instead of failing to compile.
	first := cache.resolveOne(context.Background(), url, "img_1.png", server.Client())
	if first == "" {
		t.Fatalf("failed download should still resolve to some renderable file")
	}
	if path, found := cache.Lookup(url); found {
		t.Fatalf("failed download must leave no cache entry for the url, found %q", path)
	}

	cache.resolveOne(context.Background(), url, "img_1.png", server.Client())
	if hits != 2 {
		t.Fatalf("expected the download to be retried, got %d request(s)", hits)
	}
	if _, found := cache.Lookup(url); !found {
		t.Fatalf("the successful retry should have been cached")
	}
}

// The placeholder is scaled by the layout, so an opaque one renders as a visible
// block where a school's letterhead belongs. It must be fully transparent.
func TestPlaceholderPNGIsFullyTransparent(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(getPlaceholderPNG()))
	if err != nil {
		t.Fatalf("placeholder is not a decodable png: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 1 || bounds.Dy() != 1 {
		t.Fatalf("expected a 1x1 placeholder, got %dx%d", bounds.Dx(), bounds.Dy())
	}
	if _, _, _, alpha := img.At(bounds.Min.X, bounds.Min.Y).RGBA(); alpha != 0 {
		t.Fatalf("expected a fully transparent placeholder, got alpha %d", alpha)
	}
}
