package pdfrenderer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/webp"
)

// ImageCache provides a shared disk-based cache for downloaded images.
// Files are keyed by SHA-256 of the URL and cleaned up periodically by age.
type ImageCache struct {
	dir     string
	maxAge  time.Duration
	mu      sync.RWMutex
	stopCh  chan struct{}
	stopped chan struct{}
}

// ImageCacheOptions configures the image cache.
type ImageCacheOptions struct {
	Dir             string
	MaxAge          time.Duration
	CleanupInterval time.Duration
}

// NewImageCache creates and starts an image cache with periodic cleanup.
// If dir is empty, a temp directory is created.
func NewImageCache(opts ImageCacheOptions) (*ImageCache, error) {
	if opts.Dir == "" {
		dir, err := os.MkdirTemp("", "typst-image-cache-*")
		if err != nil {
			return nil, err
		}
		opts.Dir = dir
	}

	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, err
	}

	if opts.MaxAge <= 0 {
		opts.MaxAge = 5 * time.Minute
	}
	if opts.CleanupInterval <= 0 {
		opts.CleanupInterval = time.Minute
	}

	ic := &ImageCache{
		dir:     opts.Dir,
		maxAge:  opts.MaxAge,
		stopCh:  make(chan struct{}),
		stopped: make(chan struct{}),
	}

	go ic.cleanupLoop(opts.CleanupInterval)
	return ic, nil
}

// cacheKeyForURL returns a hex-encoded SHA-256 hash of the URL.
func cacheKeyForURL(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:])
}

// Lookup checks if an image for the given URL exists in cache.
// Returns the file path and true if found, or empty string and false if not.
func (ic *ImageCache) Lookup(url string) (string, bool) {
	ic.mu.RLock()
	defer ic.mu.RUnlock()

	prefix := cacheKeyForURL(url)
	matches, err := filepath.Glob(filepath.Join(ic.dir, prefix+".*"))
	if err != nil || len(matches) == 0 {
		return "", false
	}

	// Touch the file to keep it alive in cache
	now := time.Now()
	_ = os.Chtimes(matches[0], now, now)

	return matches[0], true
}

// Store saves image data to the cache, returning the stored file path.
func (ic *ImageCache) Store(url string, ext string, data []byte) (string, error) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	filename := cacheKeyForURL(url) + ext
	path := filepath.Join(ic.dir, filename)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// Dir returns the cache directory path for use as Typst --root.
func (ic *ImageCache) Dir() string {
	return ic.dir
}

// cleanupLoop periodically removes files older than maxAge.
func (ic *ImageCache) cleanupLoop(interval time.Duration) {
	defer close(ic.stopped)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ic.stopCh:
			return
		case <-ticker.C:
			ic.cleanup()
		}
	}
}

func (ic *ImageCache) cleanup() {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	cutoff := time.Now().Add(-ic.maxAge)
	entries, err := os.ReadDir(ic.dir)
	if err != nil {
		return
	}

	var removed int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(ic.dir, entry.Name()))
			removed++
		}
	}
	if removed > 0 {
		slog.Info("image cache cleanup",
			slog.Int("removed", removed),
			slog.String("dir", ic.dir),
		)
	}
}

// Close stops the cleanup goroutine.
func (ic *ImageCache) Close() {
	close(ic.stopCh)
	<-ic.stopped
}

// ResolveImages downloads images that are not cached, stores them, and returns
// a map of typst placeholder filenames to actual filenames in the cache dir.
func (ic *ImageCache) ResolveImages(ctx context.Context, images map[string]string, httpClient *http.Client) map[string]string {
	renames := make(map[string]string)
	for url, typstFilename := range images {
		if cachedName := ic.resolveOne(ctx, url, typstFilename, httpClient); cachedName != typstFilename {
			renames[typstFilename] = cachedName
		}
	}
	return renames
}

// resolveOne resolves a single image, returning the actual filename in the cache dir.
func (ic *ImageCache) resolveOne(ctx context.Context, url, typstFilename string, httpClient *http.Client) string {
	if cachedPath, found := ic.Lookup(url); found {
		return filepath.Base(cachedPath)
	}

	storedName, err := ic.downloadAndStore(ctx, url, httpClient)
	if err != nil {
		slog.WarnContext(ctx, "failed to download image, using placeholder",
			slog.String("url", url), slog.Any("error", err),
		)
		if name := ic.writePlaceholder(); name != "" {
			return name
		}
		// The placeholder could not be written; keep the original name rather
		// than renaming the reference to nothing.
		return typstFilename
	}
	return storedName
}

// downloadAndStore downloads an image and stores it in the cache.
func (ic *ImageCache) downloadAndStore(ctx context.Context, url string, httpClient *http.Client) (string, error) {
	data, ext, err := downloadImage(ctx, url, httpClient)
	if err != nil {
		return "", err
	}
	originalData, originalExt := data, ext

	data, ext, err = sanitizeTransparentRaster(data, ext)
	if err != nil {
		slog.WarnContext(ctx, "failed to sanitize transparent raster image; using original bytes",
			slog.String("url", url),
			slog.Any("error", err),
		)
		data, ext = originalData, originalExt
	}

	storedPath, err := ic.Store(url, ext, data)
	if err != nil {
		return "", err
	}
	return filepath.Base(storedPath), nil
}

// placeholderFilename names the shared placeholder inside the cache directory.
// A cache key is a 64-character hex digest, so this cannot collide with one.
const placeholderFilename = "image-download-failed.png"

// writePlaceholder writes the shared placeholder into the cache directory and
// returns its filename.
//
// It is deliberately not keyed by URL. Lookup treats any file named after the
// URL's digest as a hit, so storing the placeholder under that key made a single
// failed download serve the placeholder for every later render until the entry
// aged out — one transient blip from the image host degraded a school's
// letterhead for the whole cache window. Writing to a shared name instead leaves
// no entry for the URL, so the next render retries the download.
func (ic *ImageCache) writePlaceholder() string {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	path := filepath.Join(ic.dir, placeholderFilename)
	if err := os.WriteFile(path, getPlaceholderPNG(), 0o600); err != nil {
		return ""
	}
	return placeholderFilename
}

var (
	placeholderPNG     []byte
	placeholderPNGOnce sync.Once
)

// getPlaceholderPNG returns a valid 1x1 fully transparent PNG image.
//
// The placeholder exists so a failed download does not leave Typst pointing at a
// missing file, which fails the whole compile. It is transparent rather than
// light gray because the documents rendered here are signed contracts: a header
// image is sized by the layout (96px tall, fit: "contain"), so an opaque
// placeholder scales up into a visible block where the school's letterhead
// should be. Rendering nothing is the same graceful degradation an unset image
// injectable already produces, and the download failure is still logged at WARN.
func getPlaceholderPNG() []byte {
	placeholderPNGOnce.Do(func() {
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		img.Set(0, 0, color.RGBA{R: 0, G: 0, B: 0, A: 0})
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		placeholderPNG = buf.Bytes()
	})
	return placeholderPNG
}

// downloadImage fetches an image URL, validates the content, and returns the bytes with correct extension.
//
//nolint:nestif
func downloadImage(ctx context.Context, url string, httpClient *http.Client) ([]byte, string, error) {
	// Handle data URLs (e.g. "data:image/png;base64,iVBORw0KGg...")
	if strings.HasPrefix(url, "data:") {
		commaIdx := strings.Index(url, ",")
		if commaIdx < 0 {
			return nil, "", fmt.Errorf("malformed data URL: no comma")
		}
		data, err := base64.StdEncoding.DecodeString(url[commaIdx+1:])
		if err != nil {
			// Some encoders omit padding — try raw encoding
			data, err = base64.RawStdEncoding.DecodeString(url[commaIdx+1:])
			if err != nil {
				return nil, "", fmt.Errorf("decoding data URL base64: %w", err)
			}
		}
		ext := detectImageExt(data)
		if ext == "" {
			return nil, "", fmt.Errorf("not a valid image in data URL")
		}
		return data, ext, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := httpClient.Do(req) //nolint:gosec // Template images may reference tenant-managed HTTP URLs; caller controls allowed document content.
	if err != nil {
		return nil, "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("downloading %s: status %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading response: %w", err)
	}

	ext := detectImageExt(data)
	if ext == "" {
		return nil, "", fmt.Errorf("not a valid image: %s", url)
	}

	return data, ext, nil
}

func sanitizeTransparentRaster(data []byte, ext string) ([]byte, string, error) {
	if !isRasterImageExt(ext) || strings.EqualFold(ext, ".jpg") || strings.EqualFold(ext, ".jpeg") {
		return data, ext, nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode raster image: %w", err)
	}

	rgba, hasAlpha, hasFullyTransparent := toNRGBAWithAlphaInfo(img)
	if !hasAlpha {
		return data, ext, nil
	}

	if hasFullyTransparent {
		alphaBleedTransparentPixels(rgba)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		return nil, "", fmt.Errorf("encode sanitized png: %w", err)
	}

	return buf.Bytes(), ".png", nil
}

func toNRGBAWithAlphaInfo(src image.Image) (*image.NRGBA, bool, bool) {
	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)
	hasAlpha := false
	hasFullyTransparent := false

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			dst.SetNRGBA(x, y, c)
			if c.A < 255 {
				hasAlpha = true
			}
			if c.A == 0 {
				hasFullyTransparent = true
			}
		}
	}

	return dst, hasAlpha, hasFullyTransparent
}

//nolint:funlen,gocognit
func alphaBleedTransparentPixels(img *image.NRGBA) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width == 0 || height == 0 {
		return
	}

	type point struct{ x, y int }
	directions := [8]point{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	visited := make([]bool, width*height)
	queue := make([]point, 0, width*height)

	index := func(x, y int) int {
		return (y-bounds.Min.Y)*width + (x - bounds.Min.X)
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				continue
			}
			i := index(x, y)
			visited[i] = true
			queue = append(queue, point{x: x, y: y})
		}
	}

	for head := 0; head < len(queue); head++ {
		p := queue[head]
		source := img.NRGBAAt(p.x, p.y)

		for _, dir := range directions {
			nx := p.x + dir.x
			ny := p.y + dir.y
			if nx < bounds.Min.X || nx >= bounds.Max.X || ny < bounds.Min.Y || ny >= bounds.Max.Y {
				continue
			}

			i := index(nx, ny)
			if visited[i] {
				continue
			}
			visited[i] = true

			neighbor := img.NRGBAAt(nx, ny)
			if neighbor.A == 0 {
				img.SetNRGBA(nx, ny, color.NRGBA{
					R: source.R,
					G: source.G,
					B: source.B,
					A: 0,
				})
			}

			queue = append(queue, point{x: nx, y: ny})
		}
	}
}

// detectImageExt returns the file extension for the detected image type, or "" if not a valid image.
func detectImageExt(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	switch {
	case bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}):
		return ".png"
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return ".jpg"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return ".gif"
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return ".webp"
	case isSVG(data):
		return ".svg"
	default:
		return ""
	}
}

// isSVG checks if data looks like an SVG by searching for "<svg" in the first 256 bytes.
func isSVG(data []byte) bool {
	limit := len(data)
	if limit > 256 {
		limit = 256
	}
	return bytes.Contains(bytes.ToLower(data[:limit]), []byte("<svg"))
}

// downloadImages downloads remote images to the given directory (fallback when no cache is available).
// Returns a map of old filename to new filename for cases where the extension was corrected.
// For failed downloads, creates a 1x1 PNG placeholder so Typst does not crash.
func downloadImages(ctx context.Context, images map[string]string, dir string, httpClient *http.Client) (map[string]string, error) {
	renames := make(map[string]string)
	var lastErr error
	for url, filename := range images {
		data, ext, err := downloadImage(ctx, url, httpClient)
		if err != nil {
			slog.WarnContext(ctx, "failed to download image, using placeholder",
				slog.String("url", url),
				slog.Any("error", err),
			)
			lastErr = err
			// Use .png for placeholder since it's a real PNG
			placeholderName := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".png"
			placeholderDest := filepath.Join(dir, placeholderName)
			_ = os.WriteFile(placeholderDest, getPlaceholderPNG(), 0o600)
			if placeholderName != filename {
				renames[filename] = placeholderName
			}
			continue
		}
		originalData, originalExt := data, ext

		data, ext, err = sanitizeTransparentRaster(data, ext)
		if err != nil {
			slog.WarnContext(ctx, "failed to sanitize transparent raster image; using original bytes",
				slog.String("url", url),
				slog.Any("error", err),
			)
			data, ext = originalData, originalExt
		}

		// Fix extension to match actual content
		base := strings.TrimSuffix(filename, filepath.Ext(filename))
		actualName := base + ext
		actualPath := filepath.Join(dir, actualName)

		if err := os.WriteFile(actualPath, data, 0o600); err != nil {
			lastErr = err
			continue
		}
		if actualName != filename {
			renames[filename] = actualName
		}
	}
	return renames, lastErr
}
