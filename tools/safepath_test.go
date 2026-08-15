package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chronosarchive/chronosarchive/tools"
)

// A sibling directory whose name merely starts with the project directory's
// name is a different directory and must be rejected. A raw string-prefix
// containment check accepts these, which is a sandbox escape for every file
// tool, since they all route through SafePath.
func TestSafePath_SiblingPrefixEscape(t *testing.T) {
	for _, target := range []string{
		"../project-evil/secret.txt",
		"../projectX",
		"../project.bak/creds",
		"../project2",
	} {
		if abs, err := tools.SafePath("/project", target); err == nil {
			t.Errorf("SafePath(/project, %q) escaped to %q, want rejection", target, abs)
		}
	}
}

func TestSafePath_AbsoluteOutsideRoot(t *testing.T) {
	for _, target := range []string{"/etc/passwd", "/project-other/x", "/"} {
		if abs, err := tools.SafePath("/project", target); err == nil {
			t.Errorf("SafePath(/project, %q) escaped to %q, want rejection", target, abs)
		}
	}
}

func TestSafePath_AllowsRootAndDescendants(t *testing.T) {
	cases := map[string]string{
		".":              "/project",
		"src/main.go":    "/project/src/main.go",
		"./a/../b":       "/project/b",
		"/project/x.txt": "/project/x.txt",
		"deep/a/b/c.go":  "/project/deep/a/b/c.go",
	}
	for target, want := range cases {
		got, err := tools.SafePath("/project", target)
		if err != nil {
			t.Errorf("SafePath(/project, %q) rejected: %v", target, err)
			continue
		}
		if got != want {
			t.Errorf("SafePath(/project, %q) = %q, want %q", target, got, want)
		}
	}
}

// A trailing separator on the configured project path must not change the
// containment decision.
func TestSafePath_RootWithTrailingSeparator(t *testing.T) {
	if _, err := tools.SafePath("/project/", "src/main.go"); err != nil {
		t.Errorf("descendant rejected: %v", err)
	}
	if abs, err := tools.SafePath("/project/", "../project-evil/x"); err == nil {
		t.Errorf("escaped to %q, want rejection", abs)
	}
}

// End-to-end: the escape must be blocked at the tool boundary, not just in the
// helper, and must not create a file outside the project.
func TestWriteFile_SiblingPrefixEscapeBlocked(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	sibling := filepath.Join(base, "project-evil")
	for _, d := range []string{root, sibling} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	in, _ := json.Marshal(map[string]string{
		"path":    "../project-evil/pwned.txt",
		"content": "pwned",
	})
	if _, err := tools.WriteFile(root, in); err == nil {
		t.Fatal("write_file escaped the project directory")
	}
	if _, err := os.Stat(filepath.Join(sibling, "pwned.txt")); err == nil {
		t.Fatal("file was written outside the project directory")
	}
}

func TestListDir_IncludesDotfilesAndMarksDirs(t *testing.T) {
	root := setupProject(t, map[string]string{
		".gitignore":     "node_modules\n",
		"main.go":        "package main",
		"src/nested.go":  "package src",
		".github/ci.yml": "on: push",
	})

	in, _ := json.Marshal(map[string]any{"path": "."})
	out, err := tools.ListDir(root, in)
	if err != nil {
		t.Fatal(err)
	}
	entries := strings.Split(strings.TrimSpace(out), "\n")
	got := make(map[string]bool, len(entries))
	for _, e := range entries {
		got[e] = true
	}

	// Dotfiles were invisible to the non-recursive listing before.
	for _, want := range []string{".gitignore", "main.go", "src/", ".github/"} {
		if !got[want] {
			t.Errorf("listing missing %q; got %v", want, entries)
		}
	}
	// Plain files must not be marked as directories.
	if got["main.go/"] {
		t.Error("main.go incorrectly marked as a directory")
	}
}
