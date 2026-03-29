package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafePath resolves target relative to projectPath and returns the absolute
// path, or an error if the result escapes the project directory.
func SafePath(projectPath, target string) (string, error) {
	var abs string
	if filepath.IsAbs(target) {
		abs = filepath.Clean(target)
	} else {
		abs = filepath.Clean(filepath.Join(projectPath, target))
	}

	if !strings.HasPrefix(abs, projectPath) {
		return "", fmt.Errorf("path %q escapes project directory", target)
	}
	return abs, nil
}
