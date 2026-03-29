package tools

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type GrepInput struct {
	Pattern   string `json:"pattern"`
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

func Grep(projectPath string, rawInput json.RawMessage) (string, error) {
	var in GrepInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("grep: bad input: %w", err)
	}
	if in.Pattern == "" {
		return "", fmt.Errorf("grep: pattern is empty")
	}

	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return "", fmt.Errorf("grep: invalid pattern %q: %w", in.Pattern, err)
	}

	searchPath := in.Path
	if searchPath == "" {
		searchPath = "."
	}
	abs, err := SafePath(projectPath, searchPath)
	if err != nil {
		return "", err
	}

	var results []string
	const maxResults = 200

	searchFile := func(path string) error {
		if len(results) >= maxResults {
			return fs.SkipAll
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(projectPath, path)
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				results = append(results, fmt.Sprintf("%s:%d: %s", rel, i+1, line))
				if len(results) >= maxResults {
					return fs.SkipAll
				}
			}
		}
		return nil
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("grep: %w", err)
	}

	if info.IsDir() {
		if in.Recursive {
			filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				return searchFile(path)
			})
		} else {
			entries, _ := os.ReadDir(abs)
			for _, e := range entries {
				if !e.IsDir() {
					searchFile(filepath.Join(abs, e.Name()))
				}
			}
		}
	} else {
		searchFile(abs)
	}

	if len(results) == 0 {
		return "(no matches)", nil
	}
	if len(results) >= maxResults {
		results = append(results, fmt.Sprintf("... (truncated at %d results)", maxResults))
	}
	return strings.Join(results, "\n"), nil
}
