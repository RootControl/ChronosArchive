package tools

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type ListDirInput struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

func ListDir(projectPath string, rawInput json.RawMessage) (string, error) {
	var in ListDirInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("list_directory: bad input: %w", err)
	}
	if in.Path == "" {
		in.Path = "."
	}
	abs, err := SafePath(projectPath, in.Path)
	if err != nil {
		return "", err
	}

	var entries []string
	if in.Recursive {
		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable entries
			}
			rel, _ := filepath.Rel(abs, path)
			if rel == "." {
				return nil
			}
			if d.IsDir() {
				entries = append(entries, rel+"/")
			} else {
				entries = append(entries, rel)
			}
			return nil
		})
	} else {
		// os.ReadDir rather than filepath.Glob: Glob's "*" skips dotfiles, so
		// .gitignore, .env and .github/ were invisible to a non-recursive
		// listing even though the recursive branch reported them.
		dirEntries, readErr := os.ReadDir(abs)
		err = readErr
		for _, e := range dirEntries {
			// Mark directories with a trailing slash, matching the recursive
			// branch, so the model can tell files and directories apart.
			if e.IsDir() {
				entries = append(entries, e.Name()+"/")
			} else {
				entries = append(entries, e.Name())
			}
		}
	}
	if err != nil {
		return "", fmt.Errorf("list_directory: %w", err)
	}
	if len(entries) == 0 {
		return "(empty directory)", nil
	}
	return strings.Join(entries, "\n"), nil
}
