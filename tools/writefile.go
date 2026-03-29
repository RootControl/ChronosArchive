package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type WriteFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func WriteFile(projectPath string, rawInput json.RawMessage) (string, error) {
	var in WriteFileInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("write_file: bad input: %w", err)
	}
	abs, err := SafePath(projectPath, in.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("write_file: creating directories: %w", err)
	}
	if err := os.WriteFile(abs, []byte(in.Content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
}
