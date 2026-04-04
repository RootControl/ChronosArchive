package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type runTestsInput struct {
	Pattern string `json:"pattern"` // optional test filter (passed as -run for Go, -k for pytest, etc.)
	Timeout int    `json:"timeout"` // seconds; default 120
}

// RunTests detects the project type and runs its test suite.
func RunTests(projectPath string, rawInput json.RawMessage) (string, error) {
	var in runTestsInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("run_tests: bad input: %w", err)
	}

	timeout := 120 * time.Second
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Second
	}

	cmd, err := buildTestCommand(projectPath, in.Pattern)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd.WithContext(ctx)
	c := exec.CommandContext(ctx, cmd.name, cmd.args...)
	c.Dir = projectPath

	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out

	runErr := c.Run()
	result := out.String()

	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("run_tests: timed out after %s", timeout)
	}
	if result == "" {
		result = "(no output)"
	}
	if runErr != nil {
		return result, fmt.Errorf("run_tests: tests failed")
	}
	return result, nil
}

type testCmd struct {
	name string
	args []string
}

func (t *testCmd) WithContext(_ context.Context) {} // placeholder for clarity

func buildTestCommand(projectPath, pattern string) (*testCmd, error) {
	// Go
	if exists(projectPath, "go.mod") {
		args := []string{"test", "./..."}
		if pattern != "" {
			args = append(args, "-run", pattern)
		}
		return &testCmd{"go", args}, nil
	}
	// Node (npm / yarn / pnpm)
	if exists(projectPath, "package.json") {
		// Prefer npm test; pattern passed via -- for most frameworks
		args := []string{"test"}
		if pattern != "" {
			args = append(args, "--", "--testNamePattern="+pattern)
		}
		mgr := "npm"
		if exists(projectPath, "yarn.lock") {
			mgr = "yarn"
		} else if exists(projectPath, "pnpm-lock.yaml") {
			mgr = "pnpm"
		}
		return &testCmd{mgr, args}, nil
	}
	// Python (pytest preferred, fallback unittest)
	if exists(projectPath, "pyproject.toml") || exists(projectPath, "setup.py") ||
		exists(projectPath, "setup.cfg") || exists(projectPath, "pytest.ini") {
		args := []string{}
		if pattern != "" {
			args = append(args, "-k", pattern)
		}
		return &testCmd{"pytest", args}, nil
	}
	// Rust
	if exists(projectPath, "Cargo.toml") {
		args := []string{"test"}
		if pattern != "" {
			args = append(args, pattern)
		}
		return &testCmd{"cargo", args}, nil
	}
	// Ruby
	if exists(projectPath, "Gemfile") {
		return &testCmd{"bundle", []string{"exec", "rspec"}}, nil
	}
	return nil, fmt.Errorf("run_tests: could not detect project type (no go.mod, package.json, pyproject.toml, Cargo.toml, or Gemfile found)")
}

func exists(dir, file string) bool {
	_, err := os.Stat(filepath.Join(dir, file))
	return err == nil
}
