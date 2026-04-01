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
main.go                   Entry point: loads config/CLI flags, builds sessions, starts TUI + goroutines
config/config.go          YAML schema (SessionConfig, ToolPermissions) + Load() + Resolve()
session/
  session.go              Session struct, state machine, thread-safe accessors
  events.go               tea.Msg types sent to the TUI (StateMsg, LogMsg, PermissionMsg, DoneMsg)
  run.go                  Agent loop: streaming Anthropic API → tool dispatch → history management
  tools.go                buildToolDefinitions() + executeTool() dispatcher
  permission.go           checkPermission(): auto-approve or block goroutine on RespCh
tools/
  safepath.go             Path traversal check — all tools use SafePath()
  readfile.go / writefile.go / editfile.go / listdir.go / bash.go / grep.go
tui/
  model.go                Bubble Tea Model: Init/Update/View, all event handlers + add-session form
  messages.go             TickMsg, NewSessionMsg, LaunchFunc (other msg types in session/events.go)
  styles.go               Lipgloss color/style constants
cmd/testloop/main.go      Standalone agent runner without TUI (for debugging)
sessions.example.yaml     Example config
```

### Key data flow

1. `main.go` creates one goroutine per session via `go s.Run(ctx, client, prog.Send)`
2. Session goroutines send `session.*Msg` events to the TUI via `prog.Send()` (non-blocking)
3. When a tool needs approval, the session sends `session.PermissionMsg` then **blocks** on `s.RespCh`
4. The TUI shows the permission prompt; user presses `y`/`n`
5. TUI writes `session.PermissionResponse` to `s.RespCh` → session goroutine unblocks
6. `session` package never imports `tui` — circular import avoided by defining msg types in `session/events.go`

### Tool permissions (per session in config)

| Field | Tools affected |
|---|---|
| `auto_approve_reads` | `read_file`, `list_directory`, `grep` |
| `auto_approve_bash` | `bash` |
| `auto_approve_writes` | `write_file`, `edit_file` |

### TUI key bindings

`↑↓`/`jk` — navigate sessions · `tab` — toggle panel focus · `a` — add session · `y`/`n` — approve/deny · `pgup`/`pgdn` — scroll logs · `q` — quit

**Add-session form:** `tab`/`shift+tab` — next/prev field · `space` — toggle bool · `enter` — launch · `esc` — cancel

## Config format

See `sessions.example.yaml`. Required fields per session: `name`, `project_path`, `goal`. Alternatively, pass `-project` and `-goal` CLI flags to skip the config file entirely.

## Anthropic Go SDK patterns

- Batch API: `client.Messages.Batches.New/Get/ResultsStreaming` — not `client.Batches`
- Result union types use `AsAny()` type switch, not constants: `switch v := result.AsAny().(type) { case anthropic.MessageBatchSucceededResult: ... }`
- Batch error message path: `variant.Error.Error.Message` (ErrorResponse → ErrorObjectUnion → Message)
- Content block text: `block.AsAny().(anthropic.TextBlock)` — no `ContentBlockTypeText` constant exists
