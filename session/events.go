package session

// The following types are sent from session goroutines to the Bubble Tea
// program via prog.Send(). The tui package imports session and switches on
// these types in its Update function. Sessions never import tui.

// StateMsg is sent when a session transitions to a new State.
type StateMsg struct {
	SessionID string
	NewState  State
}

// LogMsg is sent when a session appends a log entry.
type LogMsg struct {
	SessionID string
	Entry     LogEntry
}

// PermissionMsg is sent when a session needs user approval before executing a
// tool. The session goroutine is blocked on RespCh until a response arrives.
type PermissionMsg struct {
	SessionID   string
	ToolName    string
	RawInput    []byte
	Description string
}

// DoneMsg is sent when a session's Run() function returns (success or error).
type DoneMsg struct {
	SessionID string
	Err       error
}
