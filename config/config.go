package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const DefaultModel = "claude-opus-4-6"
const DefaultMaxTurns = 50
const DefaultMaxOutputTokens = 8192

type Config struct {
	Sessions []SessionConfig `yaml:"sessions"`
}

type SessionConfig struct {
	Name            string          `yaml:"name"`
	ProjectPath     string          `yaml:"project_path"`
	ProjectPaths    []string        `yaml:"project_paths"` // expanded into per-project sessions by Resolve()
	Goal            string          `yaml:"goal"`
	Model           string          `yaml:"model"`
	ToolPermissions ToolPermissions `yaml:"tool_permissions"`
	MaxTurns        *int            `yaml:"max_turns"` // nil (omitted) = DefaultMaxTurns; 0 = unlimited
	Thinking        bool            `yaml:"thinking"`
	ThinkingBudget  int             `yaml:"thinking_budget"` // tokens; default 10000 when thinking enabled
	Batch           bool            `yaml:"batch"`           // submit via Anthropic Message Batches API (50% cost, async, single-turn)
	ContextWindow   int             `yaml:"context_window"`  // keep first + last N messages when history exceeds 2×N (0 = disabled)
	DependsOn       []string        `yaml:"depends_on"`      // session names that must complete (StateDone) before this one starts
	GitHub          GitHubConfig    `yaml:"github"`          // optional: auto-create a PR on completion
	SpinThreshold   int             `yaml:"spin_threshold"`  // pause when same tool+input repeats N times; 0=default(3), -1=disabled
	MaxRetries      int             `yaml:"max_retries"`     // API retry attempts on retryable errors; 0=default(3), -1=disabled
	RetryBaseMs     int             `yaml:"retry_base_ms"`   // base backoff in ms for retries (default 1000)
	SystemPrompt    string          `yaml:"system_prompt"`   // optional extra instructions appended to the built-in system prompt

	MaxOutputTokens    int    `yaml:"max_output_tokens"`    // max_tokens per API call (default 8192)
	Effort             string `yaml:"effort"`               // low|medium|high|max — thinking/spend control on adaptive-thinking models
	DisablePromptCache bool   `yaml:"disable_prompt_cache"` // turn off prompt caching (on by default; cuts cost on multi-turn runs)
}

// GitHubConfig controls auto-PR creation after a session completes.
type GitHubConfig struct {
	CreatePR    bool   `yaml:"create_pr"`    // if true, run gh pr create on StateDone
	BaseBranch  string `yaml:"base_branch"`  // target branch (default: main)
	TitlePrefix string `yaml:"title_prefix"` // prepended to auto-generated PR title
	Draft       bool   `yaml:"draft"`        // open as draft PR
}

type ToolPermissions struct {
	AutoApproveReads     bool `yaml:"auto_approve_reads"`
	AutoApproveBash      bool `yaml:"auto_approve_bash"`
	AutoApproveWrites    bool `yaml:"auto_approve_writes"`
	AutoApproveWebFetch  bool `yaml:"auto_approve_web_fetch"`
	AutoApproveHTTP      bool `yaml:"auto_approve_http"`
	AutoApproveFileOps   bool `yaml:"auto_approve_file_ops"` // create_directory, move_file, delete_file
	AutoApproveWebSearch bool `yaml:"auto_approve_web_search"` // web_search
	AutoApproveGitReads  bool `yaml:"auto_approve_git_reads"`  // git status/log/diff/branch/show/blame
	AutoApproveGitWrites bool `yaml:"auto_approve_git_writes"` // git add/commit/checkout/…
	AutoApproveRunTests  bool `yaml:"auto_approve_run_tests"`  // run_tests
}

// intPtr returns a pointer to v, for setting optional int config fields.
func intPtr(v int) *int { return &v }

// MaxTurnsOrDefault returns the effective turn cap: DefaultMaxTurns when
// max_turns was omitted, otherwise the configured value. A result of 0 means
// unlimited turns. Safe to call on a config that has not been through Resolve.
func (c SessionConfig) MaxTurnsOrDefault() int {
	if c.MaxTurns == nil {
		return DefaultMaxTurns
	}
	if *c.MaxTurns < 0 {
		return DefaultMaxTurns
	}
	return *c.MaxTurns
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
// Entries with project_paths are expanded into one session per path.
func Resolve(cfg *Config) error {
	var expanded []SessionConfig
	for i, s := range cfg.Sessions {
		if s.Name == "" {
			return fmt.Errorf("session %d has no name", i)
		}
		if s.Goal == "" {
			return fmt.Errorf("session %q has no goal", s.Name)
		}

		// Expand project_paths into individual sessions.
		paths := s.ProjectPaths
		if s.ProjectPath != "" {
			paths = append([]string{s.ProjectPath}, paths...)
		}
		if len(paths) == 0 {
			return fmt.Errorf("session %q has no project_path or project_paths", s.Name)
		}

		for j, p := range paths {
			sc := s // copy
			abs, err := filepath.Abs(p)
			if err != nil {
				return fmt.Errorf("session %q: resolving path %q: %w", s.Name, p, err)
			}
			sc.ProjectPath = abs
			sc.ProjectPaths = nil
			if len(paths) > 1 {
				sc.Name = fmt.Sprintf("%s-%d", s.Name, j+1)
			}
			if sc.Model == "" {
				sc.Model = DefaultModel
			}
			// Omitting max_turns applies the default cap; an explicit 0 opts
			// into unlimited turns. A pointer is what keeps those two cases
			// distinguishable — with a plain int both are the zero value.
			// A negative value is treated as "use the default".
			if sc.MaxTurns == nil || *sc.MaxTurns < 0 {
				sc.MaxTurns = intPtr(DefaultMaxTurns)
			}
			if sc.Thinking && sc.ThinkingBudget <= 0 {
				sc.ThinkingBudget = 10000
			}
			if sc.SpinThreshold == 0 {
				sc.SpinThreshold = 3
			}
			if sc.MaxRetries == 0 {
				sc.MaxRetries = 3
			}
			if sc.RetryBaseMs == 0 {
				sc.RetryBaseMs = 1000
			}
			if sc.MaxOutputTokens <= 0 {
				sc.MaxOutputTokens = DefaultMaxOutputTokens
			}
			switch sc.Effort {
			case "", "low", "medium", "high", "max":
			default:
				return fmt.Errorf("session %q: invalid effort %q (want low, medium, high, or max)", s.Name, sc.Effort)
			}
			expanded = append(expanded, sc)
		}
	}
	cfg.Sessions = expanded
	return nil
}
