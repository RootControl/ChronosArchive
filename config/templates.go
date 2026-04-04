package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Template is a reusable session configuration, stored globally across projects.
// It holds everything from SessionConfig except project_path and batch.
type Template struct {
	Name            string          `json:"name"`
	Goal            string          `json:"goal,omitempty"`
	Model           string          `json:"model,omitempty"`
	MaxTurns        int             `json:"max_turns,omitempty"`
	Thinking        bool            `json:"thinking"`
	ThinkingBudget  int             `json:"thinking_budget,omitempty"`
	ToolPermissions ToolPermissions `json:"tool_permissions"`
}

// TemplateFromSession constructs a Template from a SessionConfig.
func TemplateFromSession(cfg SessionConfig) Template {
	return Template{
		Name:            cfg.Name,
		Goal:            cfg.Goal,
		Model:           cfg.Model,
		MaxTurns:        cfg.MaxTurns,
		Thinking:        cfg.Thinking,
		ThinkingBudget:  cfg.ThinkingBudget,
		ToolPermissions: cfg.ToolPermissions,
	}
}

func templatesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".chronosarchive", "templates.json"), nil
}

// LoadTemplates reads all saved templates. Returns nil (no error) when the
// file does not exist yet.
func LoadTemplates() ([]Template, error) {
	path, err := templatesPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Template
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveTemplate upserts a template by name, then persists the list to disk.
func SaveTemplate(t Template) error {
	existing, err := LoadTemplates()
	if err != nil {
		return err
	}
	found := false
	for i, e := range existing {
		if e.Name == t.Name {
			existing[i] = t
			found = true
			break
		}
	}
	if !found {
		existing = append(existing, t)
	}
	return writeTemplates(existing)
}

func writeTemplates(templates []Template) error {
	path, err := templatesPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(templates, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
