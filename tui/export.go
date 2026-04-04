package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chronosarchive/chronosarchive/session"
)

// estimateCost returns the approximate USD cost for the given model and token counts.
// Prices are per-million-token input/output rates.
func estimateCost(model string, inputTokens, outputTokens int64) float64 {
	type pricing struct{ in, out float64 } // $/MTok
	rates := map[string]pricing{
		"claude-opus-4-6":           {15.0, 75.0},
		"claude-sonnet-4-6":         {3.0, 15.0},
		"claude-haiku-4-5":          {0.25, 1.25},
		"claude-haiku-4-5-20251001": {0.25, 1.25},
	}
	p, ok := rates[model]
	if !ok {
		p = rates["claude-sonnet-4-6"] // fallback
	}
	return float64(inputTokens)/1e6*p.in + float64(outputTokens)/1e6*p.out
}

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
	in, out := s.TokenUsage()
	if in+out > 0 {
		fmt.Fprintf(&sb, "Tokens:   %d in / %d out  ($%.4f est.)\n", in, out, estimateCost(s.Config.Model, in, out))
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
