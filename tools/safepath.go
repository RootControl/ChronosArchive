package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafePath resolves target relative to projectPath and returns the absolute
// path, or an error if the result escapes the project directory.
//
// The containment check compares path segments, not raw string prefixes. A
// bare strings.HasPrefix is not sufficient: with a project at /project it
// would also accept /project-evil and /project.bak, since those share the
// prefix but are entirely different directories.
func SafePath(projectPath, target string) (string, error) {
	root := filepath.Clean(projectPath)

	var abs string
	if filepath.IsAbs(target) {
		abs = filepath.Clean(target)
	} else {
		abs = filepath.Clean(filepath.Join(root, target))
	}

	if !withinRoot(root, abs) {
		return "", fmt.Errorf("path %q escapes project directory", target)
	}
	return abs, nil
}

// withinRoot reports whether abs is root itself or lies beneath it, comparing
// whole path segments so sibling directories with a shared name prefix are
// rejected. Both arguments must already be cleaned.
func withinRoot(root, abs string) bool {
	if abs == root {
		return true
	}
	sep := string(filepath.Separator)
	// A trailing separator on the root turns the prefix test into a segment
	// boundary test: "/project/" matches "/project/src" but not "/project-evil".
	if !strings.HasSuffix(root, sep) {
		root += sep
	}
	return strings.HasPrefix(abs, root)
}
