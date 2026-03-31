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
type LaunchFunc func(project, goal, name, model string, approveReads, approveBash, approveWrites bool)
