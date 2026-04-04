package tui

import "github.com/chronosarchive/chronosarchive/session"

// TickMsg triggers spinner animation and elapsed-time refresh.
// All other message types are defined in the session package (session.StateMsg,
// session.LogMsg, session.PermissionMsg, session.DoneMsg) to avoid circular imports.
type TickMsg struct{}

// NewSessionMsg is sent to the TUI when a new session has been created and its
// goroutine launched. The TUI adds it to the session list.
type NewSessionMsg struct {
	Session *session.Session
}

// RetrySessionMsg is sent to the TUI when an existing session is being retried.
// The TUI replaces the old session in-place rather than appending a new row.
type RetrySessionMsg struct {
	Session *session.Session
}

// LaunchFunc is called by the TUI when the user submits the add-session form.
// It must create the session, start its goroutine, and send a NewSessionMsg back.
type LaunchFunc func(opts LaunchOpts)

// RetryFunc is called by the TUI when the user presses [r] on a failed/done session.
// It must create a new session with the same config and send a RetrySessionMsg back.
type RetryFunc func(old *session.Session)

// LaunchOpts carries all configurable fields from the add-session form.
type LaunchOpts struct {
	Project        string
	Goal           string
	Name           string
	Model          string
	ApproveReads   bool
	ApproveBash    bool
	ApproveWrites  bool
	ApproveWeb     bool
	ApproveHTTP    bool
	ApproveFileOps bool
	Thinking       bool
	ThinkingBudget int
}
