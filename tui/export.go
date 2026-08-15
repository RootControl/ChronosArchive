package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chronosarchive/chronosarchive/pricing"
	"github.com/chronosarchive/chronosarchive/session"
)

// sessionUsage collects the full token breakdown for a session.
func sessionUsage(s *session.Session) pricing.Usage { return s.Usage() }

// sessionCost returns the estimated USD cost for a session.
func sessionCost(s *session.Session) float64 { return s.CostUSD() }

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
	fmt.Fprintf(&sb, "Session:  %s\n", s.Config.Name)
	fmt.Fprintf(&sb, "Project:  %s\n", s.Config.ProjectPath)
	fmt.Fprintf(&sb, "Goal:     %s\n", s.Config.Goal)
	fmt.Fprintf(&sb, "Model:    %s\n", s.Config.Model)
	fmt.Fprintf(&sb, "Started:  %s\n", s.StartedAt().Format(time.RFC3339))
	fmt.Fprintf(&sb, "Exported: %s\n", time.Now().Format(time.RFC3339))
	u := sessionUsage(s)
	if u.Total() > 0 {
		fmt.Fprintf(&sb, "Tokens:   %d in / %d out  ($%.4f est.)\n", u.Input, u.Output, sessionCost(s))
		if u.CacheWrite+u.CacheRead > 0 {
			fmt.Fprintf(&sb, "Cache:    %d written / %d read\n", u.CacheWrite, u.CacheRead)
		}
	}
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
