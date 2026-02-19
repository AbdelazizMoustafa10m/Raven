package config

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/charmbracelet/log"
)

//go:embed all:templates
var templateFS embed.FS

// templatesRoot is the top-level directory in the embedded FS that contains
// all project templates.
const templatesRoot = "templates"

// TemplateVars holds variables available for text/template substitution when
// rendering .tmpl files. Non-template files are copied as-is.
type TemplateVars struct {
	// ProjectName is the name of the new project (e.g., "my-service").
	ProjectName string
	// Language is the primary programming language (e.g., "go").
	Language string
	// ModulePath is the Go module path (e.g., "github.com/org/my-service").
	ModulePath string
}

// ListTemplates returns the names of all available project templates by reading
// the top-level directories from the embedded filesystem.
func ListTemplates() ([]string, error) {
	entries, err := templateFS.ReadDir(templatesRoot)
	if err != nil {
		return nil, fmt.Errorf("reading templates directory: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// TemplateExists reports whether a template with the given name exists in the
// embedded filesystem.
func TemplateExists(name string) bool {
	path := templatesRoot + "/" + name
	info, err := fs.Stat(templateFS, path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// RenderTemplate writes the named template's files into destDir. Files whose
// names end in ".tmpl" are processed with text/template using vars; all other
// files are copied byte-for-byte. The ".tmpl" extension is stripped from the
// output filename. When force is false, existing files in destDir are silently
// skipped. When force is true, existing files are overwritten.
//
// Returns the list of file paths created (relative to destDir). Returns an
// error if the template does not exist or if any I/O operation fails.
func RenderTemplate(name string, destDir string, vars TemplateVars, force bool) ([]string, error) {
	if !TemplateExists(name) {
		return nil, fmt.Errorf("template %q not found", name)
	}

	templateDir := templatesRoot + "/" + name
	var created []string

	walkErr := fs.WalkDir(templateFS, templateDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking template %s: %w", path, err)
		}

		// Skip directories -- they are created implicitly when writing files.
		if d.IsDir() {
			return nil
		}

		// Compute the relative path within the template (strip the "templates/name/" prefix).
		relPath, err := filepath.Rel(filepath.FromSlash(templateDir), filepath.FromSlash(path))
		if err != nil {
			return fmt.Errorf("computing relative path for %s: %w", path, err)
		}

		// Determine the destination filename (strip .tmpl extension if present).
		destRel := relPath
		isTmpl := strings.HasSuffix(relPath, ".tmpl")
		if isTmpl {
			destRel = strings.TrimSuffix(relPath, ".tmpl")
		}

		destFile := filepath.Join(destDir, destRel)

		// Skip existing files unless force is set.
		if _, statErr := os.Stat(destFile); statErr == nil {
			if !force {
				log.Debug("skipping existing file", "path", destFile)
				return nil
			}
			log.Debug("overwriting existing file", "path", destFile)
		}

		// Ensure the parent directory exists.
		if mkdirErr := os.MkdirAll(filepath.Dir(destFile), 0o755); mkdirErr != nil {
			return fmt.Errorf("creating directory for %s: %w", destFile, mkdirErr)
		}

		// Read the source file from the embedded FS.
		// Use forward-slash paths for embed.FS (it always uses /).
		embedPath := filepath.ToSlash(path)
		content, readErr := templateFS.ReadFile(embedPath)
		if readErr != nil {
			return fmt.Errorf("reading embedded file %s: %w", embedPath, readErr)
		}

		// Process .tmpl files with text/template; copy others as-is.
		var output []byte
		if isTmpl {
			tmpl, parseErr := template.New(d.Name()).Parse(string(content))
			if parseErr != nil {
				return fmt.Errorf("parsing template %s: %w", embedPath, parseErr)
			}
			var buf bytes.Buffer
			if execErr := tmpl.Execute(&buf, vars); execErr != nil {
				return fmt.Errorf("executing template %s: %w", embedPath, execErr)
			}
			output = buf.Bytes()
		} else {
			output = content
		}

		// Write the file.
		if writeErr := os.WriteFile(destFile, output, 0o600); writeErr != nil {
			return fmt.Errorf("writing file %s: %w", destFile, writeErr)
		}

		log.Debug("created template file", "path", destFile)
		created = append(created, destFile)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return created, nil
}
