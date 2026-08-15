package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chronosarchive/chronosarchive/session"
)

// pricing holds per-million-token list rates in USD.
type pricing struct{ in, out float64 }

// modelRates maps model ID to its published $/MTok input and output rates.
var modelRates = map[string]pricing{
	"claude-fable-5":            {10.0, 50.0},
	"claude-mythos-5":           {10.0, 50.0},
	"claude-opus-5":             {5.0, 25.0},
	"claude-opus-4-8":           {5.0, 25.0},
	"claude-opus-4-7":           {5.0, 25.0},
	"claude-opus-4-6":           {5.0, 25.0},
	"claude-opus-4-5":           {5.0, 25.0},
	"claude-opus-4-1":           {15.0, 75.0},
	"claude-opus-4-0":           {15.0, 75.0},
	"claude-sonnet-5":           {3.0, 15.0},
	"claude-sonnet-4-6":         {3.0, 15.0},
	"claude-sonnet-4-5":         {3.0, 15.0},
	"claude-sonnet-4-0":         {3.0, 15.0},
	"claude-haiku-4-5":          {1.0, 5.0},
	"claude-haiku-4-5-20251001": {1.0, 5.0},
}

// Prompt-cache multipliers applied to the model's input rate.
const (
	cacheWriteMultiplier = 1.25 // 5-minute TTL write premium
	cacheReadMultiplier  = 0.10 // cache hit
	batchDiscount        = 0.50 // Message Batches API
)

// usage is the token breakdown for one session, as reported by the API.
// Input counts only the uncached remainder; cache reads and writes are billed
// at different rates and so are tracked separately.
type usage struct {
	input      int64
	output     int64
	cacheWrite int64
	cacheRead  int64
}

// estimateCost returns the approximate USD cost for the given model and token
// counts, accounting for prompt-cache read/write rates and the Batch API
// discount. Unknown models fall back to Sonnet rates.
func estimateCost(model string, u usage, batch bool) float64 {
	p, ok := modelRates[model]
	if !ok {
		p = modelRates["claude-sonnet-4-6"] // fallback
	}
	cost := float64(u.input)/1e6*p.in +
		float64(u.output)/1e6*p.out +
		float64(u.cacheWrite)/1e6*p.in*cacheWriteMultiplier +
		float64(u.cacheRead)/1e6*p.in*cacheReadMultiplier
	if batch {
		cost *= batchDiscount
	}
	return cost
}

// sessionUsage collects the full token breakdown for a session.
func sessionUsage(s *session.Session) usage {
	in, out := s.TokenUsage()
	cw, cr := s.CacheUsage()
	return usage{input: in, output: out, cacheWrite: cw, cacheRead: cr}
}

// sessionCost returns the estimated USD cost for a session.
func sessionCost(s *session.Session) float64 {
	return estimateCost(s.Config.Model, sessionUsage(s), s.Config.Batch)
}

// total returns the sum of all token counts, for display purposes.
func (u usage) total() int64 { return u.input + u.output + u.cacheWrite + u.cacheRead }

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
	if u.total() > 0 {
		fmt.Fprintf(&sb, "Tokens:   %d in / %d out  ($%.4f est.)\n", u.input, u.output, sessionCost(s))
		if u.cacheWrite+u.cacheRead > 0 {
			fmt.Fprintf(&sb, "Cache:    %d written / %d read\n", u.cacheWrite, u.cacheRead)
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
