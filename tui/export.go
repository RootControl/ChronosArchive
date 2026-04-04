package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chronosarchive/chronosarchive/session"
)

// exportLog writes the session's log to a timestamped file inside the
// project's .chronosarchive directory. Returns the output path on success.
func exportLog(s *session.Session, logs []session.LogEntry) (string, error) {
	dir := filepath.Join(s.Config.ProjectPath, ".chronosarchive")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	ts := time.Now().Format("20060102-150405")
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.log", s.Config.Name, ts))

	var sb strings.Builder
	sb.WriteString("ChronosArchive Session Log\n")
	sb.WriteString(strings.Repeat("=", 60) + "\n")
	sb.WriteString(fmt.Sprintf("Session:  %s\n", s.Config.Name))
	sb.WriteString(fmt.Sprintf("Project:  %s\n", s.Config.ProjectPath))
	sb.WriteString(fmt.Sprintf("Goal:     %s\n", s.Config.Goal))
	sb.WriteString(fmt.Sprintf("Model:    %s\n", s.Config.Model))
	sb.WriteString(fmt.Sprintf("Started:  %s\n", s.StartedAt().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Exported: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(strings.Repeat("=", 60) + "\n\n")

	for _, e := range logs {
		sb.WriteString(formatLogEntryPlain(e))
		sb.WriteString("\n")
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func formatLogEntryPlain(e session.LogEntry) string {
	ts := e.Timestamp.Format("15:04:05")
	switch e.Kind {
	case session.LogToolCall:
		return fmt.Sprintf("[%s] [%s] %s", ts, e.ToolName, e.Text)
	case session.LogToolResult:
		return fmt.Sprintf("[%s]   → %s", ts, e.Text)
	case session.LogPermission:
		return fmt.Sprintf("[%s] [perm] %s", ts, e.Text)
	case session.LogSystem:
		return fmt.Sprintf("[%s] [sys] %s", ts, e.Text)
	default:
		return fmt.Sprintf("[%s] %s", ts, e.Text)
	}
}
