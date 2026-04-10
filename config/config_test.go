package config

import (
	"os"
	"path/filepath"
	"testing"
)

func validConfig(projectPath string) *Config {
	return &Config{Sessions: []SessionConfig{{
		Name:        "test",
		ProjectPath: projectPath,
		Goal:        "do something",
	}}}
}

func TestResolve_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig(dir)
	if err := Resolve(cfg); err != nil {
		t.Fatal(err)
	}
	s := cfg.Sessions[0]
	if s.Model != DefaultModel {
		t.Errorf("model: got %q, want %q", s.Model, DefaultModel)
	}
	// MaxTurns == 0 (unset) means unlimited — default is NOT applied.
	if s.MaxTurns != 0 {
		t.Errorf("max_turns: got %d, want 0 (unlimited)", s.MaxTurns)
	}
}

func TestResolve_NegativeMaxTurnsGetsDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig(dir)
	cfg.Sessions[0].MaxTurns = -1
	if err := Resolve(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Sessions[0].MaxTurns != DefaultMaxTurns {
		t.Errorf("max_turns: got %d, want %d", cfg.Sessions[0].MaxTurns, DefaultMaxTurns)
	}
}

func TestResolve_AbsoluteProjectPath(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig(".")
	_ = os.Chdir(dir) // make "." resolve to dir
	if err := Resolve(cfg); err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cfg.Sessions[0].ProjectPath) {
		t.Errorf("project_path should be absolute, got %q", cfg.Sessions[0].ProjectPath)
	}
}

func TestResolve_ModelNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig(dir)
	cfg.Sessions[0].Model = "claude-haiku-4-5-20251001"
	if err := Resolve(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Sessions[0].Model != "claude-haiku-4-5-20251001" {
		t.Errorf("explicit model should not be overwritten, got %q", cfg.Sessions[0].Model)
	}
}

func TestResolve_MaxTurnsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig(dir)
	cfg.Sessions[0].MaxTurns = 10
	if err := Resolve(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Sessions[0].MaxTurns != 10 {
		t.Errorf("explicit max_turns should not be overwritten, got %d", cfg.Sessions[0].MaxTurns)
	}
}

func TestResolve_MissingName(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig(dir)
	cfg.Sessions[0].Name = ""
	if err := Resolve(cfg); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestResolve_MissingProjectPath(t *testing.T) {
	cfg := &Config{Sessions: []SessionConfig{{Name: "test", Goal: "do something"}}}
	if err := Resolve(cfg); err == nil {
		t.Fatal("expected error for missing project_path")
	}
}

func TestResolve_MissingGoal(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Sessions: []SessionConfig{{Name: "test", ProjectPath: dir}}}
	if err := Resolve(cfg); err == nil {
		t.Fatal("expected error for missing goal")
	}
}

func TestLoad_NoSessions(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("sessions: []\n")
	f.Close()
	if _, err := Load(f.Name()); err == nil {
		t.Fatal("expected error for empty sessions list")
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("sessions:\n  - name: s\n    project_path: " + dir + "\n    goal: g\n")
	f.Close()
	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(cfg.Sessions))
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/path.yaml"); err == nil {
		t.Fatal("expected error for missing file")
	}
}
