package session

import (
	"encoding/json"
	"fmt"
	"strings"
)

// autoApprove returns true if the tool call should be approved without asking
// the user, based on the session's ToolPermissions config.
func (s *Session) autoApprove(toolName string) bool {
	p := s.Config.ToolPermissions
	switch toolName {
	case "read_file", "list_directory", "grep":
		return p.AutoApproveReads
	case "bash":
		return p.AutoApproveBash
	case "write_file", "edit_file":
		return p.AutoApproveWrites
	case "web_fetch":
		return p.AutoApproveWebFetch
	case "http_request":
		return p.AutoApproveHTTP
	case "create_directory", "move_file", "delete_file":
		return p.AutoApproveFileOps
	}
	return false
}

// checkPermission decides whether to approve a tool call. It either
// auto-approves (returning true immediately) or blocks the session goroutine
// on RespCh until the user responds via the TUI.
//
// tuiSend must be tea.Program.Send — it is safe to call from any goroutine.
func (s *Session) checkPermission(toolName string, rawInput json.RawMessage, tuiSend func(any)) (bool, error) {
	if s.autoApprove(toolName) {
		s.appendLog(LogEntry{
			Kind: LogPermission,
			Text: fmt.Sprintf("auto-approved %s", toolName),
		})
		return true, nil
	}

	desc := formatPermDesc(toolName, rawInput)

	// Transition to WaitingPermission and notify the TUI.
	s.setState(StateWaitingPermission)
	tuiSend(PermissionMsg{
		SessionID:   s.ID,
		ToolName:    toolName,
		RawInput:    []byte(rawInput),
		Description: desc,
	})

	// Block the goroutine until the user responds.
	resp := <-s.RespCh

	// Resume Running state.
	s.setState(StateRunning)
	tuiSend(StateMsg{SessionID: s.ID, NewState: StateRunning})

	action := "approved"
	if !resp.Approved {
		action = "denied"
	}
	s.appendLog(LogEntry{
		Kind: LogPermission,
		Text: fmt.Sprintf("%s %s: %s", action, toolName, desc),
	})
	return resp.Approved, nil
}

// formatPermDesc produces a one-line human-readable description of a tool call.
func formatPermDesc(toolName string, rawInput json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(rawInput, &m); err != nil {
		return string(rawInput)
	}
	switch toolName {
	case "write_file":
		path, _ := m["path"].(string)
		content, _ := m["content"].(string)
		lines := len(strings.Split(content, "\n"))
		return fmt.Sprintf("%s (%d lines)", path, lines)
	case "edit_file":
		path, _ := m["path"].(string)
		return fmt.Sprintf("%s (str_replace)", path)
	case "bash":
		cmd, _ := m["command"].(string)
		if len(cmd) > 80 {
			cmd = cmd[:77] + "..."
		}
		return cmd
	case "read_file":
		path, _ := m["path"].(string)
		return path
	case "list_directory":
		path, _ := m["path"].(string)
		if path == "" {
			path = "."
		}
		return path
	case "grep":
		pattern, _ := m["pattern"].(string)
		path, _ := m["path"].(string)
		if path == "" {
			path = "."
		}
		return fmt.Sprintf("pattern=%q in %s", pattern, path)
	case "web_fetch":
		url, _ := m["url"].(string)
		return url
	case "http_request":
		method, _ := m["method"].(string)
		url, _ := m["url"].(string)
		return fmt.Sprintf("%s %s", method, url)
	case "create_directory":
		path, _ := m["path"].(string)
		return path
	case "move_file":
		src, _ := m["source"].(string)
		dst, _ := m["destination"].(string)
		return fmt.Sprintf("%s → %s", src, dst)
	case "delete_file":
		path, _ := m["path"].(string)
		recursive, _ := m["recursive"].(bool)
		if recursive {
			return fmt.Sprintf("%s (recursive)", path)
		}
		return path
	}
	return fmt.Sprintf("%v", m)
}
