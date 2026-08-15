package session

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/chronosarchive/chronosarchive/config"
)

// gitTimeout bounds each git/gh invocation made during PR creation.
const gitTimeout = 60 * time.Second

// runIn executes a command in dir and returns trimmed stdout. On failure the
// error carries stderr, which is where git and gh write their diagnostics.
func runIn(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	// Detach stdin so a command that would otherwise prompt fails fast
	// instead of blocking the session goroutine forever waiting on a TTY.
	cmd.Stdin = nil

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%s timed out after %s", name, gitTimeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// currentBranch returns the checked-out branch name, or an error when HEAD is
// detached (in which case there is no branch to open a PR from).
func currentBranch(dir string) (string, error) {
	out, err := runIn(dir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("determining current branch: %w", err)
	}
	if out == "" || out == "HEAD" {
		return "", fmt.Errorf("HEAD is detached — check out a branch before creating a PR")
	}
	return out, nil
}

// existingPRURL returns the URL of an open PR already associated with branch,
// or "" when there is none. Used so a retried session reports the existing PR
// instead of failing on a duplicate.
func existingPRURL(dir, branch string) string {
	out, err := runIn(dir, "gh", "pr", "view", branch, "--json", "url", "--jq", ".url")
	if err != nil {
		return "" // no PR, or gh could not tell us; fall through to creation
	}
	return out
}

// createGitHubPR pushes the session's branch and opens a PR for it, returning
// the URL of the resulting pull request.
//
// The branch must be pushed here rather than left to gh: when the branch is
// not fully pushed, `gh pr create` prompts for where to push it, and this runs
// with no TTY, so that prompt cannot be answered. Passing --head makes gh skip
// its own push-and-fork handling entirely, which keeps the behaviour
// deterministic and puts the push under our control.
func createGitHubPR(cfg config.SessionConfig) (string, error) {
	gh := cfg.GitHub

	base := gh.BaseBranch
	if base == "" {
		base = "main"
	}
	remote := gh.Remote
	if remote == "" {
		remote = "origin"
	}

	branch, err := currentBranch(cfg.ProjectPath)
	if err != nil {
		return "", err
	}
	// Opening a PR from a branch onto itself is always an error, and GitHub
	// rejects it with a much less obvious message.
	if branch == base {
		return "", fmt.Errorf("session is on the base branch %q — commit the work to a separate branch to open a PR", base)
	}

	// Refuse to open an empty PR. Without this the failure surfaces as an
	// opaque "No commits between ..." from GitHub after a needless push.
	ahead, err := runIn(cfg.ProjectPath, "git", "rev-list", "--count", remote+"/"+base+".."+branch)
	if err == nil && ahead == "0" {
		return "", fmt.Errorf("branch %q has no commits beyond %s/%s — nothing to open a PR for", branch, remote, base)
	}

	// Push with upstream tracking. This is a plain fast-forward push: if the
	// remote branch has diverged it fails rather than overwriting anything.
	if _, err := runIn(cfg.ProjectPath, "git", "push", "--set-upstream", remote, branch); err != nil {
		return "", fmt.Errorf("pushing branch %q to %s: %w", branch, remote, err)
	}

	// A retried session finds its earlier PR here instead of failing.
	if url := existingPRURL(cfg.ProjectPath, branch); url != "" {
		return url, nil
	}

	title := gh.TitlePrefix + cfg.Goal
	// Truncate title to 120 chars to stay within GitHub limits.
	if len(title) > 120 {
		title = title[:117] + "..."
	}

	body := fmt.Sprintf("Automated by ChronosArchive\n\n**Goal:** %s\n\nCompleted at: %s",
		cfg.Goal, time.Now().Format(time.RFC3339))

	args := []string{
		"pr", "create",
		"--base", base,
		"--head", branch,
		"--title", title,
		"--body", body,
	}
	if gh.Draft {
		args = append(args, "--draft")
	}

	url, err := runIn(cfg.ProjectPath, "gh", args...)
	if err != nil {
		return "", err
	}
	return url, nil
}
