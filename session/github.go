package session

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/chronosarchive/chronosarchive/config"
)

// createGitHubPR runs `gh pr create` in the session's project directory and
// returns the URL of the newly created PR.
func createGitHubPR(cfg config.SessionConfig) (string, error) {
	gh := cfg.GitHub
	base := gh.BaseBranch
	if base == "" {
		base = "main"
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
		"--title", title,
		"--body", body,
	}
	if gh.Draft {
		args = append(args, "--draft")
	}

	cmd := exec.Command("gh", args...)
	cmd.Dir = cfg.ProjectPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("%s", errMsg)
	}

	url := strings.TrimSpace(stdout.String())
	return url, nil
}
