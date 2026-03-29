package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type EditFileInput struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func EditFile(projectPath string, rawInput json.RawMessage) (string, error) {
	var in EditFileInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("edit_file: bad input: %w", err)
	}
	abs, err := SafePath(projectPath, in.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("edit_file: reading file: %w", err)
	}
	content := string(data)
	count := strings.Count(content, in.OldString)
	if count == 0 {
		return "", fmt.Errorf("edit_file: old_string not found in %s", in.Path)
	}
	if count > 1 {
		return "", fmt.Errorf("edit_file: old_string found %d times in %s (must be unique)", count, in.Path)
	}
	newContent := strings.Replace(content, in.OldString, in.NewString, 1)
	if err := os.WriteFile(abs, []byte(newContent), 0o644); err != nil {
		return "", fmt.Errorf("edit_file: writing file: %w", err)
	}
	return fmt.Sprintf("edited %s", in.Path), nil
}
