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

// splitArgs tokenises an argument string the way a shell would group words,
// honouring single and double quotes and backslash escapes, but performing no
// expansion of any kind — no globbing, no variable or command substitution.
// The result is passed directly to exec as an argv slice, so there is no shell
// to inject into.
//
// Splitting on whitespace alone is not adequate: `-m "my commit message"`
// would become four arguments, and git would read the trailing words as
// pathspecs rather than as part of the message.
func splitArgs(s string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		inWord  bool
		quote   rune // 0, '\'' or '"'
		escaped bool
	)

	flush := func() {
		if inWord {
			args = append(args, cur.String())
			cur.Reset()
			inWord = false
		}
	}

	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			inWord = true
			escaped = false
		case r == '\\' && quote != '\'':
			// Backslash escapes everywhere except inside single quotes.
			escaped = true
			inWord = true
		case quote != 0:
			if r == quote {
				quote = 0 // closing quote; the word continues
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inWord = true // "" and '' are valid empty arguments
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}

	if quote != 0 {
		return nil, fmt.Errorf("unbalanced %q quote in args", string(quote))
	}
	if escaped {
		return nil, fmt.Errorf("trailing backslash in args")
	}
	flush()
	return args, nil
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
		parsed, err := splitArgs(in.Args)
		if err != nil {
			return "", fmt.Errorf("git: %w", err)
		}
		args = append(args, parsed...)
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
	// Report every non-zero exit. Previously a failure was only surfaced when
	// git produced no output at all, so the common case — git writing a
	// diagnostic to stderr and exiting non-zero — was reported as success and
	// the agent carried on believing the command had worked.
	if err != nil {
		if out == "" {
			return "", fmt.Errorf("git %s: %w", sub, err)
		}
		return out, fmt.Errorf("git %s: %w", sub, err)
	}
	if out == "" {
		return "(no output)", nil
	}
	return out, nil
}
