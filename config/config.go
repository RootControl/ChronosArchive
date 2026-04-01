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
	Thinking        bool            `yaml:"thinking"`
	ThinkingBudget  int             `yaml:"thinking_budget"` // tokens; default 10000 when thinking enabled
	Batch           bool            `yaml:"batch"`           // submit via Anthropic Message Batches API (50% cost, async, single-turn)
}

type ToolPermissions struct {
	AutoApproveReads     bool `yaml:"auto_approve_reads"`
	AutoApproveBash      bool `yaml:"auto_approve_bash"`
	AutoApproveWrites    bool `yaml:"auto_approve_writes"`
	AutoApproveWebFetch  bool `yaml:"auto_approve_web_fetch"`
	AutoApproveHTTP      bool `yaml:"auto_approve_http"`
	AutoApproveFileOps   bool `yaml:"auto_approve_file_ops"` // create_directory, move_file, delete_file
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

	return &cfg, Resolve(&cfg)
}

// Resolve validates and normalises all sessions in cfg (absolute paths, defaults).
func Resolve(cfg *Config) error {
	for i := range cfg.Sessions {
		s := &cfg.Sessions[i]
		if s.Name == "" {
			return fmt.Errorf("session %d has no name", i)
		}
		if s.ProjectPath == "" {
			return fmt.Errorf("session %q has no project_path", s.Name)
		}
		if s.Goal == "" {
			return fmt.Errorf("session %q has no goal", s.Name)
		}
		abs, err := filepath.Abs(s.ProjectPath)
		if err != nil {
			return fmt.Errorf("session %q: resolving project_path: %w", s.Name, err)
		}
		s.ProjectPath = abs
		if s.Model == "" {
			s.Model = DefaultModel
		}
		if s.MaxTurns <= 0 {
			s.MaxTurns = DefaultMaxTurns
		}
		if s.Thinking && s.ThinkingBudget <= 0 {
			s.ThinkingBudget = 10000
		}
	}
	return nil
}
