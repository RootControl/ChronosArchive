package tools

import (
	"encoding/json"
	"fmt"
	"io/fs"
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
		dirEntries, readErr := filepath.Glob(filepath.Join(abs, "*"))
		err = readErr
		if err == nil {
			for _, e := range dirEntries {
				rel, _ := filepath.Rel(abs, e)
				info, statErr := filepath.EvalSymlinks(e)
				if statErr == nil {
					_ = info
				}
				// Check if directory
				fi, fiErr := filepath.Abs(e)
				if fiErr == nil {
					_ = fi
				}
				entries = append(entries, rel)
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
