# ChronosArchive

A Go terminal dashboard that manages multiple parallel AI coding sessions. Each session runs an Anthropic agent loop against a local project directory, working autonomously toward a defined goal. Sessions pause when they need user approval (file writes, bash commands) and resume after confirmation via the TUI.

## Requirements

- Go 1.21+
- `ANTHROPIC_API_KEY` environment variable
- `gh` CLI (optional — required only for `github.create_pr`)

## Usage

```bash
# Build
go build ./...

# Run with a config file (multiple sessions)
export ANTHROPIC_API_KEY=sk-ant-...
go run main.go -config sessions.yaml

# Run a single session directly via CLI flags (no config file needed)
go run main.go -project /path/to/project -goal "Refactor auth to use JWT"

# All CLI flags for single-session mode:
go run main.go \
  -project /path/to/project \
  -goal "Refactor auth to use JWT" \
  -name my-session \           # default: "session"
  -model claude-sonnet-4-6 \   # default: claude-opus-4-6
  -max-turns 30 \              # default: 50
  -approve-reads \             # default: true
  -approve-bash \              # default: false
  -approve-writes              # default: false

# Single-session test without TUI (useful for development)
go run ./cmd/testloop -project /tmp/test -goal "Create hello.go that prints Hello World"
go run ./cmd/testloop -project /tmp/test -goal "..." -auto-approve   # skip permission prompts
```

## TUI Key Bindings

| Key | Action |
|---|---|
| `↑↓` / `jk` | Navigate sessions |
| `tab` | Toggle panel focus (list ↔ detail) |
| `y` / `n` | Approve / deny permission prompt |
| `p` | Pause / resume selected session |
| `r` | Retry selected failed or completed session |
| `e` | Export session log to file |
| `/` | Search / filter logs in detail panel |
| `T` | Save selected session as a reusable template |
| `a` | Open add-session form |
| `pgup` / `pgdn` | Scroll logs |
| `q` | Quit |

**Add-session form keys**

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Next / previous field |
| `space` | Toggle boolean field |
| `ctrl+t` | Cycle through saved templates (pre-fills all fields) |
| `enter` | Launch session |
| `esc` | Cancel |

## Config

See `sessions.example.yaml`. Required fields per session: `name`, `project_path` (or `project_paths`), `goal`.

```yaml
sessions:
  - name: my-session
    project_path: /path/to/project
    goal: "Implement feature X"
    model: claude-opus-4-6       # optional, default: claude-opus-4-6
    max_turns: 50                # optional, default: 50
    thinking: false              # enable extended thinking
    thinking_budget: 10000       # thinking token budget
    context_window: 0            # sliding-window compression (0 = off; keep first + last N messages)
    batch: false                 # submit via Anthropic Batch API (50% cost, single-turn, async)
    depends_on: []               # session names that must complete before this one starts
    tool_permissions:
      auto_approve_reads: true
      auto_approve_bash: false
      auto_approve_writes: false
      auto_approve_web_fetch: false
      auto_approve_http: false
      auto_approve_file_ops: false   # create_directory, move_file, delete_file
      auto_approve_web_search: false
      auto_approve_git_reads: false  # git status/log/diff/branch/show/blame
      auto_approve_git_writes: false # git add/commit/checkout/…
      auto_approve_run_tests: false
    github:
      create_pr: false           # run gh pr create when session completes
      base_branch: main
      title_prefix: "feat: "
      draft: false
```

### Session groups

Run the same goal across multiple project directories — each path becomes its own session named `<name>-1`, `<name>-2`, etc.:

```yaml
sessions:
  - name: add-linting
    project_paths:
      - /path/to/project-a
      - /path/to/project-b
    goal: "Add ESLint with the airbnb config and fix all warnings."
```

### Pipeline mode (session dependencies)

```yaml
sessions:
  - name: generate-tests
    project_path: /path/to/project
    goal: "Write unit tests for all exported functions."

  - name: fix-failures
    project_path: /path/to/project
    goal: "Run the tests and fix any failures."
    depends_on: [generate-tests]   # waits for generate-tests to complete
```

### Batch mode

```yaml
sessions:
  - name: summarise-readme
    project_path: /path/to/project
    goal: "Summarise the README in one paragraph."
    batch: true    # 50% cost, async, single-turn — no tool loop or permission prompts
```

### Tool permissions

| Field | Tools |
|---|---|
| `auto_approve_reads` | `read_file`, `list_directory`, `grep` |
| `auto_approve_bash` | `bash` |
| `auto_approve_writes` | `write_file`, `edit_file` |
| `auto_approve_web_fetch` | `web_fetch` |
| `auto_approve_http` | `http_request` |
| `auto_approve_file_ops` | `create_directory`, `move_file`, `delete_file` |
| `auto_approve_web_search` | `web_search` |
| `auto_approve_git_reads` | `git status/log/diff/branch/show/blame` |
| `auto_approve_git_writes` | `git add/commit/checkout/switch/stash/…` |
| `auto_approve_run_tests` | `run_tests` |

## Session Features

### Persistence
Sessions automatically snapshot their full message history after each completed tool turn to `<project_path>/.chronosarchive/<name>.json`. On restart, the session resumes from where it left off. Snapshots are deleted on clean completion.

### Pause / Resume
Press `p` on a running session to pause it before its next API call. Press `p` again to resume. Quitting while paused cleanly cancels the session.

### Retry
Press `r` on a failed or completed session to re-run it in place (same list slot). Failed sessions resume from their last snapshot; completed sessions start fresh.

### Templates
Press `T` on any session to save its config as a named template to `~/.chronosarchive/templates.json`. In the add-session form, press `ctrl+t` to cycle through templates and pre-fill all fields.

### Log Export
Press `e` to export the selected session's full log to `<project_path>/.chronosarchive/<name>-YYYYMMDD-HHMMSS.log` as plain text with a metadata header.

### Log Search
Press `/` to filter the log viewport to entries matching a search string. Press `esc` or `enter` to exit search mode (filter stays active). Navigate to another session to clear it.

### Token Usage & Cost
The detail panel heading shows cumulative token usage (in thousands) and estimated USD cost next to the session state icon. Estimates use per-model pricing rates.

## Architecture

```
main.go                     Entry point: loads config/CLI flags, builds sessions, starts TUI + goroutines
config/
  config.go                 YAML schema (SessionConfig, ToolPermissions, GitHubConfig) + Load() + Resolve()
  templates.go              Template save/load from ~/.chronosarchive/templates.json
session/
  session.go                Session struct, state machine, thread-safe accessors, Pause/Resume
  events.go                 tea.Msg types sent to the TUI (StateMsg, LogMsg, PermissionMsg, DoneMsg)
  run.go                    Agent loop: streaming API → tool dispatch → history management
  tools.go                  buildToolDefinitions() + executeTool() dispatcher
  permission.go             checkPermission(): auto-approve or block goroutine on RespCh
  persist.go                Snapshot save/load (resume across restarts)
  compress.go               Sliding-window context compression
  batch.go                  Anthropic Message Batches API: submit, poll, deliver results
  github.go                 gh pr create on session completion
tools/
  safepath.go               Path traversal check — all file tools use SafePath()
  readfile.go               read_file
  writefile.go              write_file
  editfile.go               edit_file (str_replace)
  listdir.go                list_directory
  bash.go                   bash (with timeout)
  grep.go                   grep (regex search)
  webfetch.go               web_fetch (HTTP GET)
  httprequest.go            http_request (any method)
  filesystem.go             create_directory, move_file, delete_file
  websearch.go              web_search (DuckDuckGo Instant Answer API)
  git.go                    git (allowlisted subcommands)
  runtests.go               run_tests (auto-detects Go/Node/Python/Rust/Ruby)
tui/
  model.go                  Bubble Tea Model: Init/Update/View, all event handlers + forms
  messages.go               TUI-specific message types and callback types
  export.go                 Log export and cost estimation
  styles.go                 Lipgloss color/style constants
cmd/testloop/main.go        Standalone agent runner without TUI (for debugging)
sessions.example.yaml       Annotated example config
```

### Key data flow

1. `main.go` creates one goroutine per session via `go s.Run(ctx, client, prog.Send)`
2. Session goroutines send `session.*Msg` events to the TUI via `prog.Send()` (non-blocking)
3. When a tool needs approval, the session sends `session.PermissionMsg` then **blocks** on `s.RespCh`
4. The TUI shows the permission prompt; user presses `y`/`n`
5. TUI writes `session.PermissionResponse` to `s.RespCh` → session goroutine unblocks
6. `session` package never imports `tui` — circular import avoided by defining msg types in `session/events.go`

## Development

```bash
# Test all packages
go test ./...

# Test specific packages
go test ./config/... -v
go test ./tools/... -v

# Lint
go vet ./...
```
