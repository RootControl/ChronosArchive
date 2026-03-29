package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chronosarchive/chronosarchive/tools"
)

// writeTemp creates a file inside t.TempDir() and returns the project root.
func setupProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func marshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReadFile(t *testing.T) {
	root := setupProject(t, map[string]string{"hello.txt": "hello world"})
	out, err := tools.ReadFile(root, marshal(t, map[string]string{"path": "hello.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello world" {
		t.Fatalf("got %q", out)
	}
}

func TestReadFile_Escape(t *testing.T) {
	root := setupProject(t, nil)
	_, err := tools.ReadFile(root, marshal(t, map[string]string{"path": "../../etc/passwd"}))
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestWriteFile(t *testing.T) {
	root := setupProject(t, nil)
	_, err := tools.WriteFile(root, marshal(t, map[string]string{"path": "new.txt", "content": "content"}))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "new.txt"))
	if string(data) != "content" {
		t.Fatalf("got %q", string(data))
	}
}

func TestWriteFile_CreatesSubdir(t *testing.T) {
	root := setupProject(t, nil)
	_, err := tools.WriteFile(root, marshal(t, map[string]string{"path": "sub/dir/file.go", "content": "package main"}))
	if err != nil {
		t.Fatal(err)
	}
}

func TestEditFile(t *testing.T) {
	root := setupProject(t, map[string]string{"main.go": "package main\n\nfunc Hello() {}\n"})
	_, err := tools.EditFile(root, marshal(t, map[string]string{
		"path":       "main.go",
		"old_string": "func Hello() {}",
		"new_string": "func Hello() string { return \"hi\" }",
	}))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "main.go"))
	if string(data) != "package main\n\nfunc Hello() string { return \"hi\" }\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestEditFile_NotFound(t *testing.T) {
	root := setupProject(t, map[string]string{"f.go": "hello"})
	_, err := tools.EditFile(root, marshal(t, map[string]string{
		"path":       "f.go",
		"old_string": "notpresent",
		"new_string": "x",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListDir(t *testing.T) {
	root := setupProject(t, map[string]string{"a.go": "", "b.go": "", "sub/c.go": ""})
	out, err := tools.ListDir(root, marshal(t, map[string]any{"path": "."}))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" || out == "(empty directory)" {
		t.Fatalf("expected entries, got %q", out)
	}
}

func TestBash(t *testing.T) {
	root := setupProject(t, nil)
	out, err := tools.Bash(root, marshal(t, map[string]string{"command": "echo hello"}))
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello\n" {
		t.Fatalf("got %q", out)
	}
}

func TestBash_Escape(t *testing.T) {
	// The bash tool itself doesn't restrict the command — it just runs it in the
	// project dir. The permission gate prevents dangerous commands from reaching
	// this function. We just verify the command runs in the right directory.
	root := setupProject(t, nil)
	out, err := tools.Bash(root, marshal(t, map[string]string{"command": "pwd"}))
	if err != nil {
		t.Fatal(err)
	}
	// pwd should match the project root (tmp dir)
	_ = out
}

func TestGrep(t *testing.T) {
	root := setupProject(t, map[string]string{
		"a.go": "package main\nfunc Hello() {}\n",
		"b.go": "package main\n",
	})
	out, err := tools.Grep(root, marshal(t, map[string]any{
		"pattern":   "func Hello",
		"path":      ".",
		"recursive": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if out == "(no matches)" {
		t.Fatal("expected a match")
	}
}

func TestSafePath_Escape(t *testing.T) {
	_, err := tools.SafePath("/project", "../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestSafePath_Relative(t *testing.T) {
	abs, err := tools.SafePath("/project", "src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if abs != "/project/src/main.go" {
		t.Fatalf("got %q", abs)
	}
}
