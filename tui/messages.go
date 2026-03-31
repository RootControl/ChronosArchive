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

// LaunchFunc is called by the TUI when the user submits the add-session form.
// It must create the session, start its goroutine, and send a NewSessionMsg back.
type LaunchFunc func(opts LaunchOpts)

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
