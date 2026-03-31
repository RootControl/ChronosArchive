package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosarchive/chronosarchive/config"
	"github.com/chronosarchive/chronosarchive/session"
)

// key returns a tea.KeyMsg for the given string.
func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func keySpecial(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func newTestModel() Model {
	return NewModel(nil)
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func noopLaunch() LaunchFunc { return func(_ LaunchOpts) {} }

// --- Form open/close ---

func TestFormOpens_WhenLaunchSet(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))
	if !m.formOpen {
		t.Fatal("form should be open after pressing 'a'")
	}
}

func TestFormDoesNotOpen_WithoutLaunch(t *testing.T) {
	m := newTestModel()
	m = update(m, key("a"))
	if m.formOpen {
		t.Fatal("form should not open when launch is nil")
	}
}

func TestFormDefaults(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	if m.formModel != "claude-sonnet-4-6" {
		t.Errorf("default model: got %q, want claude-sonnet-4-6", m.formModel)
	}
	// All permissions default to false — must be explicitly enabled.
	if m.formApproveReads {
		t.Error("auto-approve reads should default to false")
	}
	if m.formApproveBash {
		t.Error("auto-approve bash should default to false")
	}
	if m.formApproveWrites {
		t.Error("auto-approve writes should default to false")
	}
	if m.formApproveWeb {
		t.Error("auto-approve web should default to false")
	}
	if m.formApproveHTTP {
		t.Error("auto-approve http should default to false")
	}
	if m.formApproveFileOps {
		t.Error("auto-approve file-ops should default to false")
	}
	if m.formThinking {
		t.Error("thinking should default to false")
	}
}

func TestFormClosesOnEsc(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))
	m = update(m, keySpecial(tea.KeyEsc))
	if m.formOpen {
		t.Fatal("form should be closed after esc")
	}
}

func TestFormEsc_ClearsFields(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))
	for _, ch := range "hello" {
		m = update(m, key(string(ch)))
	}
	m = update(m, keySpecial(tea.KeyEsc))
	if m.formProject != "" {
		t.Errorf("formProject should be cleared, got %q", m.formProject)
	}
	if m.formField != 0 {
		t.Errorf("formField should reset to 0, got %d", m.formField)
	}
}

// --- Field navigation ---

func TestFormTab_CyclesFields(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	for i := 1; i < 12; i++ {
		m = update(m, keySpecial(tea.KeyTab))
		if m.formField != i {
			t.Errorf("after %d tabs: formField = %d, want %d", i, m.formField, i)
		}
	}
	// wraps back to 0
	m = update(m, keySpecial(tea.KeyTab))
	if m.formField != 0 {
		t.Errorf("tab wrap: formField = %d, want 0", m.formField)
	}
}

// --- Text input ---

func TestFormTyping_ProjectField(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	for _, ch := range "/tmp/proj" {
		m = update(m, key(string(ch)))
	}
	if m.formProject != "/tmp/proj" {
		t.Errorf("formProject: got %q, want /tmp/proj", m.formProject)
	}
}

func TestFormBackspace(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	m = update(m, key("h"))
	m = update(m, key("i"))
	m = update(m, keySpecial(tea.KeyBackspace))
	if m.formProject != "h" {
		t.Errorf("after backspace: got %q, want %q", m.formProject, "h")
	}
}

// --- Bool toggles ---

func TestFormToggle_ApproveReads(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	for i := 0; i < 4; i++ {
		m = update(m, keySpecial(tea.KeyTab))
	}
	m = update(m, key(" "))
	if !m.formApproveReads {
		t.Error("toggle should have set formApproveReads to true")
	}
}

func TestFormToggle_ApproveBash(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	for i := 0; i < 5; i++ {
		m = update(m, keySpecial(tea.KeyTab))
	}
	m = update(m, key(" "))
	if !m.formApproveBash {
		t.Error("toggle should have set formApproveBash to true")
	}
}

func TestFormToggle_ApproveWrites(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	for i := 0; i < 6; i++ {
		m = update(m, keySpecial(tea.KeyTab))
	}
	m = update(m, key(" "))
	if !m.formApproveWrites {
		t.Error("toggle should have set formApproveWrites to true")
	}
}

func TestFormToggle_ApproveWeb(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	for i := 0; i < 7; i++ {
		m = update(m, keySpecial(tea.KeyTab))
	}
	m = update(m, key(" "))
	if !m.formApproveWeb {
		t.Error("toggle should have set formApproveWeb to true")
	}
}

func TestFormToggle_ApproveHTTP(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	for i := 0; i < 8; i++ {
		m = update(m, keySpecial(tea.KeyTab))
	}
	m = update(m, key(" "))
	if !m.formApproveHTTP {
		t.Error("toggle should have set formApproveHTTP to true")
	}
}

func TestFormToggle_ApproveFileOps(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	for i := 0; i < 9; i++ {
		m = update(m, keySpecial(tea.KeyTab))
	}
	m = update(m, key(" "))
	if !m.formApproveFileOps {
		t.Error("toggle should have set formApproveFileOps to true")
	}
}

func TestFormToggle_Thinking(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))

	for i := 0; i < 10; i++ {
		m = update(m, keySpecial(tea.KeyTab))
	}
	m = update(m, key(" "))
	if !m.formThinking {
		t.Error("toggle should have set formThinking to true")
	}
}

// --- Submit ---

func TestFormEnter_DoesNotLaunch_WhenFieldsEmpty(t *testing.T) {
	launched := false
	m := newTestModel()
	m.SetLaunch(func(_ LaunchOpts) { launched = true })
	m = update(m, key("a"))
	m = update(m, keySpecial(tea.KeyEnter))
	if launched {
		t.Error("should not launch when project/goal empty")
	}
	if !m.formOpen {
		t.Error("form should remain open")
	}
}

func TestFormEnter_Launches_WhenValid(t *testing.T) {
	var got LaunchOpts
	m := newTestModel()
	m.SetLaunch(func(opts LaunchOpts) { got = opts })
	m = update(m, key("a"))

	for _, ch := range "/tmp/p" {
		m = update(m, key(string(ch)))
	}
	m = update(m, keySpecial(tea.KeyTab))
	for _, ch := range "buildit" {
		m = update(m, key(string(ch)))
	}
	m = update(m, keySpecial(tea.KeyEnter))

	if m.formOpen {
		t.Error("form should be closed after successful submit")
	}
	if got.Project != "/tmp/p" {
		t.Errorf("project: got %q", got.Project)
	}
	if got.Goal != "buildit" {
		t.Errorf("goal: got %q", got.Goal)
	}
	if got.Model != "claude-sonnet-4-6" {
		t.Errorf("model: got %q", got.Model)
	}
	// All permissions default false.
	if got.ApproveReads || got.ApproveBash || got.ApproveWrites {
		t.Errorf("permissions should default false: reads=%v bash=%v writes=%v", got.ApproveReads, got.ApproveBash, got.ApproveWrites)
	}
	if got.Thinking {
		t.Error("thinking should default false")
	}
}

func TestFormEnter_ClearsFieldsAfterSubmit(t *testing.T) {
	m := newTestModel()
	m.SetLaunch(noopLaunch())
	m = update(m, key("a"))
	for _, ch := range "/p" {
		m = update(m, key(string(ch)))
	}
	m = update(m, keySpecial(tea.KeyTab))
	for _, ch := range "g" {
		m = update(m, key(string(ch)))
	}
	m = update(m, keySpecial(tea.KeyEnter))

	if m.formProject != "" || m.formGoal != "" || m.formModel != "" {
		t.Error("fields should be cleared after submit")
	}
}

// --- NewSessionMsg ---

func TestNewSessionMsg_AddsSession(t *testing.T) {
	m := newTestModel()
	sc := config.SessionConfig{Name: "new", ProjectPath: "/tmp", Goal: "g", Model: "claude-sonnet-4-6", MaxTurns: 50}
	s := session.New("s99", sc)

	m = update(m, NewSessionMsg{Session: s})

	if len(m.order) != 1 {
		t.Fatalf("expected 1 session, got %d", len(m.order))
	}
	if m.order[0] != "s99" {
		t.Errorf("order[0]: got %q, want s99", m.order[0])
	}
	if _, ok := m.views["s99"]; !ok {
		t.Error("view not created for new session")
	}
	if _, ok := m.sessions["s99"]; !ok {
		t.Error("session not registered in model")
	}
}

func TestNewSessionMsg_SelectsNewSession(t *testing.T) {
	m := newTestModel()
	sc := config.SessionConfig{Name: "n", ProjectPath: "/tmp", Goal: "g", Model: "claude-sonnet-4-6", MaxTurns: 50}

	m = update(m, NewSessionMsg{Session: session.New("s1", sc)})
	m = update(m, NewSessionMsg{Session: session.New("s2", sc)})

	if m.selectedIdx != 1 {
		t.Errorf("selectedIdx: got %d, want 1 (last added)", m.selectedIdx)
	}
}

func TestNewSessionMsg_ViewHasCorrectName(t *testing.T) {
	m := newTestModel()
	sc := config.SessionConfig{Name: "my-session", ProjectPath: "/tmp", Goal: "g", Model: "claude-sonnet-4-6", MaxTurns: 50}
	s := session.New("s1", sc)

	m = update(m, NewSessionMsg{Session: s})

	sv := m.views["s1"]
	if sv.name != "my-session" {
		t.Errorf("view name: got %q, want my-session", sv.name)
	}
	if sv.state != session.StateStarting {
		t.Errorf("view state: got %v, want StateStarting", sv.state)
	}
}

// --- Normal key events pass through when form is closed ---

func TestQuit_WhenFormClosed(t *testing.T) {
	m := newTestModel()
	_, cmd := m.Update(key("q"))
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestNavigation_UpDown(t *testing.T) {
	sc := config.SessionConfig{Name: "s", ProjectPath: "/tmp", Goal: "g", Model: "claude-sonnet-4-6", MaxTurns: 50}
	m := NewModel([]*session.Session{
		session.New("s0", sc),
		session.New("s1", sc),
	})
	m = update(m, key("j"))
	if m.selectedIdx != 1 {
		t.Errorf("after j: selectedIdx = %d, want 1", m.selectedIdx)
	}
	m = update(m, key("k"))
	if m.selectedIdx != 0 {
		t.Errorf("after k: selectedIdx = %d, want 0", m.selectedIdx)
	}
}
