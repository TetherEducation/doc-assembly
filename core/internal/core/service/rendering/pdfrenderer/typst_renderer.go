package pdfrenderer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// TypstRenderer handles PDF generation using the Typst CLI.
type TypstRenderer struct {
	opts TypstOptions
}

// TypstOptions configures the Typst renderer.
type TypstOptions struct {
	// BinPath is the path to the typst binary (default: "typst").
	BinPath string

	// Timeout is the maximum time to wait for PDF compilation.
	Timeout time.Duration

	// FontDirs are additional directories to search for fonts.
	FontDirs []string

	// MaxConcurrent limits simultaneous typst processes (0 = unlimited).
	MaxConcurrent int

	// AcquireTimeout is the max wait time to acquire a render slot.
	AcquireTimeout time.Duration
}

// DefaultTypstOptions returns sensible default options.
func DefaultTypstOptions() TypstOptions {
	return TypstOptions{
		BinPath: "typst",
		Timeout: 10 * time.Second,
	}
}

// NewTypstRenderer creates a new Typst-based PDF renderer.
func NewTypstRenderer(opts TypstOptions) (*TypstRenderer, error) {
	if opts.BinPath == "" {
		opts.BinPath = "typst"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 10 * time.Second
	}

	// Verify typst binary exists
	if _, err := exec.LookPath(opts.BinPath); err != nil {
		return nil, fmt.Errorf("typst binary not found at %q: %w", opts.BinPath, err)
	}

	return &TypstRenderer{opts: opts}, nil
}

// GeneratePDF compiles Typst source to PDF bytes.
// rootDir is optional; if set, it is passed as --root to typst for resolving local file paths.
func (r *TypstRenderer) GeneratePDF(ctx context.Context, typstSource string, rootDir string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.opts.Timeout)
	defer cancel()

	args := r.buildArgs(rootDir)
	cmd := exec.CommandContext(ctx, r.opts.BinPath, args...) //nolint:gosec // BinPath is validated at init
	cmd.Stdin = bytes.NewReader([]byte(typstSource))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("typst compile failed: %w\nstderr: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// buildArgs constructs the CLI arguments for typst compile.
func (r *TypstRenderer) buildArgs(rootDir string) []string {
	args := make([]string, 0, 3+2*len(r.opts.FontDirs)+4)
	args = append(args, "compile", "--format", "pdf")

	if rootDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			if abs, absErr := filepath.Abs(cwd); absErr == nil {
				rootDir = abs
			} else {
				rootDir = cwd
			}
		}
	}
	if rootDir != "" {
		args = append(args, "--root", rootDir)
	}

	for _, dir := range r.opts.FontDirs {
		args = append(args, "--font-path", dir)
	}

	// Read from stdin, write to stdout
	args = append(args, "-", "-")
	return args
}

// Close is a no-op for Typst (no persistent processes to clean up).
func (r *TypstRenderer) Close() error {
	return nil
}
