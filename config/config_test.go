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
	// An omitted max_turns applies the default cap, so a config that simply
	// leaves the key out can never run uncapped.
	if s.MaxTurnsOrDefault() != DefaultMaxTurns {
		t.Errorf("max_turns: got %d, want %d", s.MaxTurnsOrDefault(), DefaultMaxTurns)
	}
}

func TestResolve_NegativeMaxTurnsGetsDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig(dir)
	cfg.Sessions[0].MaxTurns = intPtr(-1)
	if err := Resolve(cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Sessions[0].MaxTurnsOrDefault(); got != DefaultMaxTurns {
		t.Errorf("max_turns: got %d, want %d", got, DefaultMaxTurns)
	}
}

// An explicit 0 is the documented opt-in to unlimited turns and must survive
// Resolve — this is the case a plain int could not distinguish from "omitted".
func TestResolve_ExplicitZeroMeansUnlimited(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig(dir)
	cfg.Sessions[0].MaxTurns = intPtr(0)
	if err := Resolve(cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Sessions[0].MaxTurnsOrDefault(); got != 0 {
		t.Errorf("explicit max_turns: 0 should stay unlimited, got %d", got)
	}
}

// Omitting the key in YAML must behave the same as never setting the field.
func TestLoad_OmittedMaxTurnsGetsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.yaml")
	yaml := "sessions:\n  - name: s\n    project_path: " + dir + "\n    goal: g\n"
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Sessions[0].MaxTurnsOrDefault(); got != DefaultMaxTurns {
		t.Errorf("omitted max_turns: got %d, want %d", got, DefaultMaxTurns)
	}
}

// An explicit 0 in YAML must be distinguishable from an omitted key.
func TestLoad_ExplicitZeroMaxTurnsIsUnlimited(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.yaml")
	yaml := "sessions:\n  - name: s\n    project_path: " + dir + "\n    goal: g\n    max_turns: 0\n"
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Sessions[0].MaxTurnsOrDefault(); got != 0 {
		t.Errorf("max_turns: 0 should mean unlimited, got %d", got)
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
	cfg.Sessions[0].MaxTurns = intPtr(10)
	if err := Resolve(cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Sessions[0].MaxTurnsOrDefault(); got != 10 {
		t.Errorf("explicit max_turns should not be overwritten, got %d", got)
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
