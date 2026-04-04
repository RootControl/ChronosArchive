package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"os/exec"
)

// gitReadSubcommands are safe, non-mutating git operations auto-approvable
// without the write permission.
var gitReadSubcommands = map[string]bool{
	"status": true,
	"log":    true,
	"diff":   true,
	"branch": true,
	"show":   true,
	"blame":  true,
}

// gitWriteSubcommands require the write permission gate.
var gitWriteSubcommands = map[string]bool{
	"add":      true,
	"commit":   true,
	"checkout": true,
	"switch":   true,
	"stash":    true,
	"restore":  true,
	"reset":    true,
	"merge":    true,
	"rebase":   true,
	"tag":      true,
}

type gitInput struct {
	Subcommand string `json:"subcommand"`
	Args       string `json:"args"`
}

// GitIsReadSubcommand reports whether the given subcommand is a read-only git op.
// Used by the permission layer to decide which auto-approve flag applies.
func GitIsReadSubcommand(sub string) bool {
	return gitReadSubcommands[strings.ToLower(sub)]
}

// Git runs a git subcommand with optional extra args inside the project directory.
func Git(projectPath string, rawInput json.RawMessage) (string, error) {
	var in gitInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return "", fmt.Errorf("git: bad input: %w", err)
	}
	sub := strings.ToLower(strings.TrimSpace(in.Subcommand))
	if sub == "" {
		return "", fmt.Errorf("git: subcommand is required")
	}
	if !gitReadSubcommands[sub] && !gitWriteSubcommands[sub] {
		return "", fmt.Errorf("git: subcommand %q is not allowed (allowed: status, log, diff, branch, show, blame, add, commit, checkout, switch, stash, restore, reset, merge, rebase, tag)", sub)
	}

	args := []string{sub}
	if in.Args != "" {
		// Split args naively on spaces (no shell expansion — prevents injection).
		for _, a := range strings.Fields(in.Args) {
			args = append(args, a)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
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
		out += stderr.String()
	}
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("git: timed out")
	}
	if err != nil && out == "" {
		return "", fmt.Errorf("git %s: %w", sub, err)
	}
	if out == "" {
		return "(no output)", nil
	}
	return out, nil
}
