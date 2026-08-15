package session

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chronosarchive/chronosarchive/config"
)

// repoWithRemote builds a working repo whose "origin" is a local bare repo, so
// pushes are exercised for real without touching the network. Returns the
// working tree path.
func repoWithRemote(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	bare := filepath.Join(base, "remote.git")
	work := filepath.Join(base, "work")

	mustGit(t, base, "init", "--bare", "-q", "-b", "main", bare)
	mustGit(t, base, "clone", "-q", bare, work)
	mustGit(t, work, "config", "user.email", "test@example.com")
	mustGit(t, work, "config", "user.name", "Test")

	writeFile(t, filepath.Join(work, "README.md"), "hello\n")
	mustGit(t, work, "add", "README.md")
	mustGit(t, work, "commit", "-q", "-m", "initial")
	mustGit(t, work, "push", "-q", "-u", "origin", "main")
	return work
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := exec.Command("sh", "-c", "printf %s "+shellQuote(content)+" > "+shellQuote(path)).Run(); err != nil {
		t.Fatal(err)
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func TestCurrentBranch(t *testing.T) {
	work := repoWithRemote(t)
	got, err := currentBranch(work)
	if err != nil {
		t.Fatal(err)
	}
	if got != "main" {
		t.Errorf("got %q, want main", got)
	}

	mustGit(t, work, "checkout", "-q", "-b", "feature/x")
	got, err = currentBranch(work)
	if err != nil {
		t.Fatal(err)
	}
	if got != "feature/x" {
		t.Errorf("got %q, want feature/x", got)
	}
}

// A detached HEAD has no branch to open a PR from and must be reported
// clearly rather than pushing something unexpected.
func TestCurrentBranch_DetachedHEAD(t *testing.T) {
	work := repoWithRemote(t)
	mustGit(t, work, "checkout", "-q", "--detach", "HEAD")

	if _, err := currentBranch(work); err == nil {
		t.Fatal("expected an error for detached HEAD")
	} else if !strings.Contains(err.Error(), "detached") {
		t.Errorf("error should mention the detached HEAD, got: %v", err)
	}
}

// Opening a PR from the base branch onto itself can never succeed.
func TestCreateGitHubPR_RejectsBaseBranch(t *testing.T) {
	work := repoWithRemote(t)
	cfg := config.SessionConfig{
		ProjectPath: work,
		Goal:        "g",
		GitHub:      config.GitHubConfig{CreatePR: true, BaseBranch: "main"},
	}
	_, err := createGitHubPR(cfg)
	if err == nil {
		t.Fatal("expected an error when the session is on the base branch")
	}
	if !strings.Contains(err.Error(), "base branch") {
		t.Errorf("error should explain the base-branch clash, got: %v", err)
	}
}

// A branch with no commits beyond base would produce an empty PR; that is
// caught before pushing so the failure is understandable.
func TestCreateGitHubPR_RejectsEmptyBranch(t *testing.T) {
	work := repoWithRemote(t)
	mustGit(t, work, "checkout", "-q", "-b", "feature/empty")

	cfg := config.SessionConfig{
		ProjectPath: work,
		Goal:        "g",
		GitHub:      config.GitHubConfig{CreatePR: true, BaseBranch: "main"},
	}
	_, err := createGitHubPR(cfg)
	if err == nil {
		t.Fatal("expected an error for a branch with no new commits")
	}
	if !strings.Contains(err.Error(), "no commits") {
		t.Errorf("error should explain the branch is empty, got: %v", err)
	}
	// Nothing should have been pushed.
	c := exec.Command("git", "ls-remote", "--heads", "origin", "feature/empty")
	c.Dir = work
	out, _ := c.Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Error("empty branch was pushed despite the guard")
	}
}

// The push itself must work and set upstream tracking. gh is not involved up
// to this point, so this is verified end-to-end against the local bare remote.
func TestCreateGitHubPR_PushesBranch(t *testing.T) {
	work := repoWithRemote(t)
	mustGit(t, work, "checkout", "-q", "-b", "feature/work")
	writeFile(t, filepath.Join(work, "new.txt"), "content\n")
	mustGit(t, work, "add", "new.txt")
	mustGit(t, work, "commit", "-q", "-m", "add new.txt")

	cfg := config.SessionConfig{
		ProjectPath: work,
		Goal:        "g",
		GitHub:      config.GitHubConfig{CreatePR: true, BaseBranch: "main"},
	}
	// gh will fail here (the local bare remote is not a GitHub repo), but the
	// push must already have happened by then — that is the regression under
	// test: previously nothing was ever pushed.
	_, _ = createGitHubPR(cfg)

	c := exec.Command("git", "ls-remote", "--heads", "origin", "feature/work")
	c.Dir = work
	out, err := c.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "feature/work") {
		t.Fatal("branch was not pushed to the remote before PR creation")
	}

	// Upstream tracking must be configured so later pushes work unattended.
	c = exec.Command("git", "rev-parse", "--abbrev-ref", "feature/work@{upstream}")
	c.Dir = work
	up, err := c.Output()
	if err != nil {
		t.Fatalf("no upstream tracking configured: %v", err)
	}
	if got := strings.TrimSpace(string(up)); got != "origin/feature/work" {
		t.Errorf("upstream = %q, want origin/feature/work", got)
	}
}

// A non-default remote name must be honoured.
func TestCreateGitHubPR_CustomRemote(t *testing.T) {
	work := repoWithRemote(t)
	base := filepath.Dir(work)
	upstream := filepath.Join(base, "upstream.git")
	mustGit(t, base, "init", "--bare", "-q", "-b", "main", upstream)
	mustGit(t, work, "remote", "add", "upstream", upstream)
	mustGit(t, work, "push", "-q", "upstream", "main")

	mustGit(t, work, "checkout", "-q", "-b", "feature/alt")
	writeFile(t, filepath.Join(work, "alt.txt"), "x\n")
	mustGit(t, work, "add", "alt.txt")
	mustGit(t, work, "commit", "-q", "-m", "alt")

	cfg := config.SessionConfig{
		ProjectPath: work,
		Goal:        "g",
		GitHub:      config.GitHubConfig{CreatePR: true, BaseBranch: "main", Remote: "upstream"},
	}
	_, _ = createGitHubPR(cfg)

	c := exec.Command("git", "ls-remote", "--heads", "upstream", "feature/alt")
	c.Dir = work
	out, _ := c.Output()
	if !strings.Contains(string(out), "feature/alt") {
		t.Error("branch was not pushed to the configured remote")
	}
}
