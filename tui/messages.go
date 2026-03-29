package tui

// TickMsg triggers spinner animation and elapsed-time refresh.
// All other message types are defined in the session package (session.StateMsg,
// session.LogMsg, session.PermissionMsg, session.DoneMsg) to avoid circular imports.
type TickMsg struct{}
