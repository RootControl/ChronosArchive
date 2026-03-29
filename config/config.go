package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const DefaultModel = "claude-opus-4-6"
const DefaultMaxTurns = 50

type Config struct {
	Sessions []SessionConfig `yaml:"sessions"`
}

type SessionConfig struct {
	Name            string          `yaml:"name"`
	ProjectPath     string          `yaml:"project_path"`
	Goal            string          `yaml:"goal"`
	Model           string          `yaml:"model"`
	ToolPermissions ToolPermissions `yaml:"tool_permissions"`
	MaxTurns        int             `yaml:"max_turns"`
}

type ToolPermissions struct {
	AutoApproveReads  bool `yaml:"auto_approve_reads"`
	AutoApproveBash   bool `yaml:"auto_approve_bash"`
	AutoApproveWrites bool `yaml:"auto_approve_writes"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if len(cfg.Sessions) == 0 {
		return nil, fmt.Errorf("config has no sessions defined")
	}

	for i := range cfg.Sessions {
		s := &cfg.Sessions[i]
		if s.Name == "" {
			return nil, fmt.Errorf("session %d has no name", i)
		}
		if s.ProjectPath == "" {
			return nil, fmt.Errorf("session %q has no project_path", s.Name)
		}
		if s.Goal == "" {
			return nil, fmt.Errorf("session %q has no goal", s.Name)
		}
		// Resolve to absolute path
		abs, err := filepath.Abs(s.ProjectPath)
		if err != nil {
			return nil, fmt.Errorf("session %q: resolving project_path: %w", s.Name, err)
		}
		s.ProjectPath = abs
		if s.Model == "" {
			s.Model = DefaultModel
		}
		if s.MaxTurns <= 0 {
			s.MaxTurns = DefaultMaxTurns
		}
	}

	return &cfg, nil
}
