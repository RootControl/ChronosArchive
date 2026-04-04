# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

**ChronosArchive** is a Go terminal dashboard that manages multiple parallel AI coding sessions. Each session runs an Anthropic agent loop against a local project directory, working autonomously toward a defined goal. Sessions pause when they need user approval (file writes, bash commands) and resume after confirmation via the TUI.

## Commands

```bash
# Build
go build ./...

# Run with config file (requires ANTHROPIC_API_KEY env var)
export ANTHROPIC_API_KEY=sk-ant-...
go run main.go -config sessions.yaml

# Run single session via CLI flags (no config file needed)
go run main.go -project /path/to/project -goal "Refactor auth to use JWT"
go run main.go -project /path/to/project -goal "..." -name my-session -model claude-sonnet-4-6 -approve-bash -approve-writes

# Test all packages
go test ./...

# Test specific packages
go test ./config/... -v
go test ./tui/... -v
go test ./tools/... -v

# Single-session test without TUI (useful for development)
go run ./cmd/testloop -project /tmp/test -goal "Create hello.go that prints Hello World"
go run ./cmd/testloop -project /tmp/test -goal "..." -auto-approve   # skip permission prompts

# Lint
go vet ./...
```

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
7. Session dependencies: `main.go` wraps each goroutine to `<-dep.DoneCh` for each name in `DependsOn`

### Tool permissions (per session in config)

| Field | Tools affected |
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

### TUI key bindings

`↑↓`/`jk` — navigate sessions · `tab` — toggle panel focus · `a` — add session · `y`/`n` — approve/deny · `p` — pause/resume · `r` — retry · `e` — export log · `/` — search logs · `T` — save template · `pgup`/`pgdn` — scroll logs · `q` — quit

**Add-session form:** `tab`/`shift+tab` — next/prev field · `space` — toggle bool · `ctrl+t` — cycle templates · `enter` — launch · `esc` — cancel

## Config format

See `sessions.example.yaml`. Required fields per session: `name`, `project_path` (or `project_paths`), `goal`. Alternatively, pass `-project` and `-goal` CLI flags to skip the config file entirely.

Key optional fields:
- `model` — default `claude-opus-4-6`
- `max_turns` — default `50`
- `thinking` / `thinking_budget` — extended thinking
- `context_window` — sliding-window compression (keep first + last N messages; `0` = off)
- `batch` — submit via Anthropic Batch API (50% cost, async, single-turn)
- `depends_on` — list of session names that must complete before this one starts
- `project_paths` — list of paths; each becomes its own session named `<name>-1`, `<name>-2`, etc.
- `github.create_pr` — run `gh pr create` on completion; also `base_branch`, `title_prefix`, `draft`

## Anthropic Go SDK patterns

- Batch API: `client.Messages.Batches.New/Get/ResultsStreaming` — not `client.Batches`
- Result union types use `AsAny()` type switch, not constants: `switch v := result.AsAny().(type) { case anthropic.MessageBatchSucceededResult: ... }`
- Batch error message path: `variant.Error.Error.Message` (ErrorResponse → ErrorObjectUnion → Message)
- Content block text: `block.AsAny().(anthropic.TextBlock)` — no `ContentBlockTypeText` constant exists
- `MessageParam` and `ContentBlockParamUnion` both implement `MarshalJSON`/`UnmarshalJSON` — safe to JSON-persist `[]anthropic.MessageParam` directly
