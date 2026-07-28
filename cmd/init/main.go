package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/*
var templatesFS embed.FS

type projectData struct {
	ProjectName string
	ModulePath  string
}

func main() {
	force := false
	args := os.Args[1:]

	// Parse flags
	var positional []string
	modulePath := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--force":
			force = true
		case "--module":
			if i+1 < len(args) {
				i++
				modulePath = args[i]
			} else {
				fatalf("--module requires a value")
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				fatalf("unknown flag: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: go run github.com/TetherEducation/doc-assembly/cmd/init@latest <project-name> --module <module-path> [--force]")
		os.Exit(1)
	}

	projectName := positional[0]
	displayName := filepath.Base(projectName)
	if modulePath == "" {
		modulePath = "github.com/myorg/" + displayName
	}

	data := projectData{
		ProjectName: displayName,
		ModulePath:  modulePath,
	}

	if err := scaffold(projectName, data, force); err != nil {
		fatalf("scaffold error: %v", err)
	}

	absPath, _ := filepath.Abs(projectName)
	fmt.Printf("\nProject %q created at %s\n\n", displayName, absPath)
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", absPath)
	fmt.Println("  go mod tidy")
	fmt.Println("  # Edit settings/app.yaml with your database config")
	fmt.Println("  go run . migrate")
	fmt.Println("  go run .")
}

func scaffold(projectName string, data projectData, force bool) error {
	return fs.WalkDir(templatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Strip "templates/" prefix
		relPath := strings.TrimPrefix(path, "templates/")
		if relPath == "" {
			return nil
		}

		// Convert .tmpl extension
		outPath := filepath.Join(projectName, strings.TrimSuffix(relPath, ".tmpl"))

		if d.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}

		// Skip existing files unless --force
		if !force {
			if _, statErr := os.Stat(outPath); statErr == nil {
				fmt.Printf("  skip: %s (exists)\n", outPath)
				return nil
			}
		}

		content, readErr := fs.ReadFile(templatesFS, path)
		if readErr != nil {
			return readErr
		}

		// If it's a .tmpl file, execute as template
		var output []byte
		if strings.HasSuffix(path, ".tmpl") {
			tmpl, tmplErr := template.New(filepath.Base(path)).Parse(string(content))
			if tmplErr != nil {
				return fmt.Errorf("parsing template %s: %w", path, tmplErr)
			}
			var buf strings.Builder
			if execErr := tmpl.Execute(&buf, data); execErr != nil {
				return fmt.Errorf("executing template %s: %w", path, execErr)
			}
			output = []byte(buf.String())
		} else {
			output = content
		}

		if mkErr := os.MkdirAll(filepath.Dir(outPath), 0o755); mkErr != nil {
			return mkErr
		}

		fmt.Printf("  create: %s\n", outPath)
		return os.WriteFile(outPath, output, 0o644)
	})
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
