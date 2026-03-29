package session

import (
	"sync"
	"time"

	"github.com/chronosarchive/chronosarchive/config"
)

// State represents the lifecycle state of a session.
type State int

const (
	StateStarting          State = iota
	StateRunning
	StateWaitingPermission
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

	// Mutable state — guarded by mu.
	mu        sync.RWMutex
	state     State
	logs      []LogEntry
	turn      int
	startedAt time.Time
	err       error
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
