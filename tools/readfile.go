package tools

import (
	"encoding/json"
	"fmt"
	"os"
)

type ReadFileInput struct {
	Path string `json:"path"`
}

func ReadFile(projectPath string, rawInput json.RawMessage) (string, error) {
	var in ReadFileInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("read_file: bad input: %w", err)
	}
	abs, err := SafePath(projectPath, in.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	return string(data), nil
}
