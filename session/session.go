package session

import (
	"sync"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/chronosarchive/chronosarchive/config"
	"github.com/chronosarchive/chronosarchive/pricing"
)

// State represents the lifecycle state of a session.
type State int

const (
	StateStarting          State = iota
	StateRunning
	StateWaitingPermission
	StatePaused
	StateDone
	StateFailed
)

func (s State) String() string {
	switch s {
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateWaitingPermission:
		return "waiting"
	case StatePaused:
		return "paused"
	case StateDone:
		return "done"
	case StateFailed:
		return "failed"
	}
	return "unknown"
}

// LogKind classifies a log entry for display purposes.
type LogKind int

const (
	LogText       LogKind = iota // assistant text output
	LogToolCall                  // tool invocation (before execution)
	LogToolResult                // tool execution result
	LogPermission                // permission decision
	LogSystem                    // system-level messages
)

// LogEntry is a single line of session output.
type LogEntry struct {
	Kind      LogKind
	ToolName  string
	Text      string
	Timestamp time.Time
}

// PermissionRequest is sent from the session goroutine to the TUI when a tool
// call requires user approval.
type PermissionRequest struct {
	ToolName    string
	RawInput    []byte
	Description string // human-readable summary shown in the TUI
}

// PermissionResponse is sent from the TUI back to the blocked session goroutine.
type PermissionResponse struct {
	Approved bool
}

const maxLogs = 500

// Session holds both the runtime state of an agent session and the channels
// used to communicate between the session goroutine and the TUI.
type Session struct {
	ID     string
	Config config.SessionConfig

	// Channels — created in New(), never replaced.
	PermCh  chan PermissionRequest  // session → TUI: permission needed
	RespCh  chan PermissionResponse // TUI → session: decision
	DoneCh  chan struct{}           // closed when Run() returns

	// kill cancels only this session's context. Set by SetKill after New().
	kill func()

	// Mutable state — guarded by mu.
	mu        sync.RWMutex
	state     State
	logs      []LogEntry
	turn      int
	startedAt time.Time
	err       error

	// pauseCh is nil when running; set to a new channel by Pause() and closed
	// by Resume(). The run loop selects on it at each turn boundary.
	pauseCh chan struct{}

	// Cumulative token usage across all turns.
	inputTokens      int64
	outputTokens     int64
	cacheWriteTokens int64 // tokens written to the prompt cache (billed ~1.25x input)
	cacheReadTokens  int64 // tokens served from the prompt cache (billed ~0.1x input)

	// Batch progress counters (non-zero only for batch sessions).
	batchTotal     int
	batchSucceeded int
	batchPending   int

	// Spin detection — only accessed from Run() goroutine, no mutex needed.
	lastToolName    string
	lastToolInput   string
	consecutiveCount int
}

// New creates a Session. Call Run() in a goroutine to start the agent loop.
func New(id string, cfg config.SessionConfig) *Session {
	return &Session{
		ID:        id,
		Config:    cfg,
		PermCh:    make(chan PermissionRequest, 1),
		RespCh:    make(chan PermissionResponse, 1),
		DoneCh:    make(chan struct{}),
		state:     StateStarting,
		startedAt: time.Now(),
	}
}

// --- Thread-safe accessors used by the TUI ---

func (s *Session) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *Session) Turn() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turn
}

func (s *Session) Err() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (s *Session) StartedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.startedAt
}

// Logs returns a copy of the log buffer. Safe to call from any goroutine.
func (s *Session) Logs() []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LogEntry, len(s.logs))
	copy(out, s.logs)
	return out
}

// --- Internal mutators called only from Run() ---

func (s *Session) setState(st State) {
	s.mu.Lock()
	s.state = st
	s.mu.Unlock()
}

func (s *Session) setTurn(t int) {
	s.mu.Lock()
	s.turn = t
	s.mu.Unlock()
}

func (s *Session) setErr(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

// SetBatchProgress records the latest batch status counts for TUI display.
func (s *Session) SetBatchProgress(total, succeeded, pending int) {
	s.mu.Lock()
	s.batchTotal = total
	s.batchSucceeded = succeeded
	s.batchPending = pending
	s.mu.Unlock()
}

// BatchProgress returns the current batch progress counters.
func (s *Session) BatchProgress() (total, succeeded, pending int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.batchTotal, s.batchSucceeded, s.batchPending
}

// TokenUsage returns the cumulative uncached token counts across all turns.
// Cached tokens are reported separately by CacheUsage.
func (s *Session) TokenUsage() (inputTokens, outputTokens int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inputTokens, s.outputTokens
}

// Usage returns the full token breakdown for this session.
func (s *Session) Usage() pricing.Usage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return pricing.Usage{
		Input:      s.inputTokens,
		Output:     s.outputTokens,
		CacheWrite: s.cacheWriteTokens,
		CacheRead:  s.cacheReadTokens,
	}
}

// CostUSD returns the estimated spend for this session so far.
func (s *Session) CostUSD() float64 {
	return pricing.Estimate(s.Config.Model, s.Usage(), s.Config.Batch)
}

// CacheUsage returns the cumulative prompt-cache token counts across all turns.
// These are billed at different rates than ordinary input tokens: writes at
// ~1.25x and reads at ~0.1x.
func (s *Session) CacheUsage() (writeTokens, readTokens int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cacheWriteTokens, s.cacheReadTokens
}

// addUsage accumulates usage from one API response. Note that Usage.InputTokens
// counts only the uncached remainder — total prompt size is the sum of all
// three input figures.
func (s *Session) addUsage(u anthropic.Usage) {
	s.mu.Lock()
	s.inputTokens += u.InputTokens
	s.outputTokens += u.OutputTokens
	s.cacheWriteTokens += u.CacheCreationInputTokens
	s.cacheReadTokens += u.CacheReadInputTokens
	s.mu.Unlock()
}

// Pause signals the session goroutine to pause before its next API call.
// No-op if the session is not running.
func (s *Session) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateRunning && s.pauseCh == nil {
		s.pauseCh = make(chan struct{})
	}
}

// Resume unblocks a paused session. No-op if not paused.
func (s *Session) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pauseCh != nil {
		close(s.pauseCh)
		s.pauseCh = nil
	}
}

// SetKill stores the cancel function for this session's context.
// Call once after New(), before launching Run().
func (s *Session) SetKill(cancel func()) {
	s.mu.Lock()
	s.kill = cancel
	s.mu.Unlock()
}

// Kill cancels this session's context, causing Run() to exit on the next turn.
func (s *Session) Kill() {
	s.mu.RLock()
	fn := s.kill
	s.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (s *Session) appendLog(e LogEntry) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	s.mu.Lock()
	s.logs = append(s.logs, e)
	if len(s.logs) > maxLogs {
		s.logs = s.logs[len(s.logs)-maxLogs:]
	}
	s.mu.Unlock()
}
