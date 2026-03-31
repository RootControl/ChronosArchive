package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type CreateDirectoryInput struct {
	Path string `json:"path"`
}

func CreateDirectory(projectPath string, rawInput json.RawMessage) (string, error) {
	var in CreateDirectoryInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("create_directory: bad input: %w", err)
	}
	abs, err := SafePath(projectPath, in.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create_directory: %w", err)
	}
	return fmt.Sprintf("created directory: %s", in.Path), nil
}

type MoveFileInput struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func MoveFile(projectPath string, rawInput json.RawMessage) (string, error) {
	var in MoveFileInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("move_file: bad input: %w", err)
	}
	src, err := SafePath(projectPath, in.Source)
	if err != nil {
		return "", fmt.Errorf("move_file: source: %w", err)
	}
	dst, err := SafePath(projectPath, in.Destination)
	if err != nil {
		return "", fmt.Errorf("move_file: destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("move_file: creating destination directory: %w", err)
	}
	if err := os.Rename(src, dst); err != nil {
		return "", fmt.Errorf("move_file: %w", err)
	}
	return fmt.Sprintf("moved %s → %s", in.Source, in.Destination), nil
}

type DeleteFileInput struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

func DeleteFile(projectPath string, rawInput json.RawMessage) (string, error) {
	var in DeleteFileInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("delete_file: bad input: %w", err)
	}
	abs, err := SafePath(projectPath, in.Path)
	if err != nil {
		return "", err
	}
	if in.Recursive {
		if err := os.RemoveAll(abs); err != nil {
			return "", fmt.Errorf("delete_file: %w", err)
		}
		return fmt.Sprintf("deleted (recursive): %s", in.Path), nil
	}
	if err := os.Remove(abs); err != nil {
		return "", fmt.Errorf("delete_file: %w", err)
	}
	return fmt.Sprintf("deleted: %s", in.Path), nil
}
