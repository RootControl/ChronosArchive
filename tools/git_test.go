package tools_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/chronosarchive/chronosarchive/tools"
)

// initRepo creates a git repo with one staged file.
func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		c := exec.Command("git", args...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root
}

func gitCall(t *testing.T, root, sub, args string) (string, error) {
	t.Helper()
	in, err := json.Marshal(map[string]string{"subcommand": sub, "args": args})
	if err != nil {
		t.Fatal(err)
	}
	return tools.Git(root, in)
}

// A quoted commit message must survive as a single argument. Splitting on
// whitespace made git read the trailing words as pathspecs, so the commit was
// never created — and the tool reported success anyway.
func TestGit_CommitWithQuotedMessage(t *testing.T) {
	root := initRepo(t)
	if err := exec.Command("sh", "-c", "echo hi > "+root+"/a.txt").Run(); err != nil {
		t.Fatal(err)
	}
	if out, err := gitCall(t, root, "add", "a.txt"); err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	out, err := gitCall(t, root, "commit", `-m "my commit message"`)
	if err != nil {
		t.Fatalf("commit failed: %v\n%s", err, out)
	}

	log, err := gitCall(t, root, "log", "--oneline")
	if err != nil {
		t.Fatalf("log failed: %v\n%s", err, log)
	}
	if !strings.Contains(log, "my commit message") {
		t.Fatalf("commit message not recorded; log = %q", log)
	}
}

// A failing git command must be reported as an error. It previously returned
// nil whenever git wrote anything to stderr, which is the usual case, so the
// agent was told a failed command had succeeded.
func TestGit_NonZeroExitIsReported(t *testing.T) {
	root := initRepo(t)
	out, err := gitCall(t, root, "commit", "-m nothing-to-commit")
	if err == nil {
		t.Fatalf("expected an error for a failing commit, got success with output %q", out)
	}
}

func TestGit_RejectsDisallowedSubcommand(t *testing.T) {
	root := initRepo(t)
	for _, sub := range []string{"push", "clone", "remote", "config"} {
		if _, err := gitCall(t, root, sub, ""); err == nil {
			t.Errorf("subcommand %q should be rejected", sub)
		}
	}
}

func TestGit_SplitArgsQuoting(t *testing.T) {
	root := initRepo(t)
	// An unbalanced quote is a user error and must not reach git.
	if _, err := gitCall(t, root, "log", `--grep "unterminated`); err == nil {
		t.Error("expected an error for an unbalanced quote")
	}
	// A trailing backslash is likewise malformed.
	if _, err := gitCall(t, root, "log", `--grep x\`); err == nil {
		t.Error("expected an error for a trailing backslash")
	}
}

// Arguments are passed as an argv slice with no shell involved, so shell
// metacharacters must be inert rather than interpreted.
func TestGit_NoShellInjection(t *testing.T) {
	root := initRepo(t)
	if err := exec.Command("sh", "-c", "echo hi > "+root+"/a.txt").Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := gitCall(t, root, "add", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitCall(t, root, "commit", `-m "touched; rm -rf ."`); err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	// The repo still exists and the message was stored verbatim.
	log, err := gitCall(t, root, "log", "--oneline")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log, "touched; rm -rf .") {
		t.Errorf("message not stored verbatim; log = %q", log)
	}
}
