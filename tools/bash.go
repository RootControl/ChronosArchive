package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type BashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // seconds; 0 = default 30
}

const defaultBashTimeout = 30 * time.Second
const maxBashTimeout = 120 * time.Second

func Bash(projectPath string, rawInput json.RawMessage) (string, error) {
	var in BashInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("bash: bad input: %w", err)
	}
	if in.Command == "" {
		return "", fmt.Errorf("bash: command is empty")
	}

	timeout := defaultBashTimeout
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Second
		if timeout > maxBashTimeout {
			timeout = maxBashTimeout
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", in.Command)
	cmd.Dir = projectPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	out := stdout.String()
	if stderr.Len() > 0 {
		if out != "" {
			out += "\n"
		}
		out += "stderr: " + stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("bash: command timed out after %s", timeout)
	}
	if err != nil {
		if out == "" {
			return "", fmt.Errorf("bash: %w", err)
		}
		return out, fmt.Errorf("bash: exit error: %w", err)
	}
	if out == "" {
		return "(no output)", nil
	}
	return out, nil
}
