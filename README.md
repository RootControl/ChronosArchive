# ChronosArchive

A Go terminal dashboard that manages multiple parallel AI coding sessions. Each session runs an Anthropic agent loop against a local project directory, working autonomously toward a defined goal. Sessions pause when they need user approval (file writes, bash commands) and resume after confirmation via the TUI.

## Requirements

- Go 1.21+
- `ANTHROPIC_API_KEY` environment variable

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

### Adding sessions from the TUI

Press `a` while the TUI is running to open the add-session form. Fill in the fields, use `tab` to move between them, `space` to toggle boolean options, and `enter` to launch. The new session starts immediately without restarting.

| Form field | Default |
|---|---|
| Project path | _(required)_ |
| Goal | _(required)_ |
| Name | auto-generated |
| Model | `claude-sonnet-4-6` |
| Auto-approve reads | on |
| Auto-approve bash | on |
| Auto-approve writes | on |

## Config

See `sessions.example.yaml`. Required fields per session: `name`, `project_path`, `goal`.

```yaml
sessions:
  - name: my-session
    project_path: /path/to/project
    goal: "Implement feature X"
    model: claude-opus-4-6       # optional, default: claude-opus-4-6
    max_turns: 50                # optional, default: 50
    tool_permissions:
      auto_approve_reads: true
      auto_approve_bash: false
      auto_approve_writes: false
```

### Tool permissions

| Field | Tools affected |
|---|---|
| `auto_approve_reads` | `read_file`, `list_directory`, `grep` |
| `auto_approve_bash` | `bash` |
| `auto_approve_writes` | `write_file`, `edit_file` |

## TUI Key Bindings

| Key | Action |
|---|---|
| `↑↓` / `jk` | Navigate sessions |
| `tab` | Toggle panel focus |
| `a` | Open add-session form |
| `y` / `n` | Approve / deny permission |
| `pgup` / `pgdn` | Scroll logs |
| `q` | Quit |

**Add-session form keys**

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Next / previous field |
| `space` | Toggle boolean field |
| `enter` | Launch session |
| `esc` | Cancel |

## Architecture

```
main.go                   Entry point: loads config, builds sessions, starts TUI + goroutines
config/config.go          YAML schema (SessionConfig, ToolPermissions) + Load()
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
  model.go                Bubble Tea Model: Init/Update/View, all event handlers
  messages.go             TickMsg (all other msg types live in session/events.go)
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

## Development

```bash
# Test all packages
go test ./...

# Test tools only
go test ./tools/... -v

# Lint
go vet ./...
```
