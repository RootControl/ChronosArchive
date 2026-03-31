package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chronosarchive/chronosarchive/session"
)

const leftPanelWidth = 32

type panelFocus int

const (
	panelList   panelFocus = iota
	panelDetail
)

// sessionView is the TUI-side view of a session (display state only).
type sessionView struct {
	id        string
	name      string
	state     session.State
	logs      []session.LogEntry
	turn      int
	startedAt time.Time
	err       error
}

// Model is the root Bubble Tea model.
type Model struct {
	// Ordered list of session IDs for display.
	order []string
	// Display state for each session.
	views map[string]*sessionView
	// Live session references for sending permission responses.
	sessions map[string]*session.Session

	// Pending permission requests, keyed by session ID.
	pendingPerms map[string]session.PermissionMsg

	// Navigation
	selectedIdx int
	focus       panelFocus

	// UI components
	logViewport viewport.Model
	spinner     spinner.Model

	// Terminal size
	width  int
	height int

	// Set after tea.NewProgram() is created (via SetProgram).
	program *tea.Program

	// Called when the user submits the add-session form.
	launch LaunchFunc

	// Add-session form overlay.
	formOpen         bool
	formField        int // 0=project 1=goal 2=name 3=model 4=reads 5=bash 6=writes
	formProject      string
	formGoal         string
	formName         string
	formModel        string
	formApproveReads bool
	formApproveBash  bool
	formApproveWrites bool
}

// NewModel builds the initial Model from a set of sessions.
func NewModel(sessions []*session.Session) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleGreen

	order := make([]string, len(sessions))
	views := make(map[string]*sessionView, len(sessions))
	sessMap := make(map[string]*session.Session, len(sessions))

	for i, s := range sessions {
		order[i] = s.ID
		views[s.ID] = &sessionView{
			id:        s.ID,
			name:      s.Config.Name,
			state:     session.StateStarting,
			startedAt: s.StartedAt(),
		}
		sessMap[s.ID] = s
	}

	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle()

	m := Model{
		order:        order,
		views:        views,
		sessions:     sessMap,
		pendingPerms: make(map[string]session.PermissionMsg),
		spinner:      sp,
		logViewport:  vp,
	}
	return m
}

// SetProgram injects the tea.Program reference so the model can write to
// session response channels from the Update loop.
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

// SetLaunch provides the callback used when the user submits the add-session form.
func (m *Model) SetLaunch(fn LaunchFunc) {
	m.launch = fn
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()

	case tea.KeyMsg:
		if m.formOpen {
			switch msg.String() {
			case "esc":
				m.formOpen = false
				m.formProject, m.formGoal, m.formName, m.formModel = "", "", "", ""
				m.formApproveReads, m.formApproveBash, m.formApproveWrites = false, false, false
				m.formField = 0

			case "tab", "down":
				m.formField = (m.formField + 1) % 7

			case "shift+tab", "up":
				m.formField = (m.formField + 6) % 7

			case "enter":
				if m.formProject != "" && m.formGoal != "" {
					name := m.formName
					if name == "" {
						name = fmt.Sprintf("session-%d", len(m.order)+1)
					}
					if m.launch != nil {
						m.launch(m.formProject, m.formGoal, name, m.formModel,
							m.formApproveReads, m.formApproveBash, m.formApproveWrites)
					}
					m.formOpen = false
					m.formProject, m.formGoal, m.formName, m.formModel = "", "", "", ""
					m.formApproveReads, m.formApproveBash, m.formApproveWrites = false, false, false
					m.formField = 0
				}

			case " ":
				switch m.formField {
				case 4:
					m.formApproveReads = !m.formApproveReads
				case 5:
					m.formApproveBash = !m.formApproveBash
				case 6:
					m.formApproveWrites = !m.formApproveWrites
				}

			case "backspace":
				switch m.formField {
				case 0:
					if len(m.formProject) > 0 {
						m.formProject = m.formProject[:len(m.formProject)-1]
					}
				case 1:
					if len(m.formGoal) > 0 {
						m.formGoal = m.formGoal[:len(m.formGoal)-1]
					}
				case 2:
					if len(m.formName) > 0 {
						m.formName = m.formName[:len(m.formName)-1]
					}
				case 3:
					if len(m.formModel) > 0 {
						m.formModel = m.formModel[:len(m.formModel)-1]
					}
				}

			default:
				ch := msg.String()
				if len(ch) == 1 {
					switch m.formField {
					case 0:
						m.formProject += ch
					case 1:
						m.formGoal += ch
					case 2:
						m.formName += ch
					case 3:
						m.formModel += ch
					}
				}
			}
			break
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "a":
			if m.launch != nil {
				m.formOpen = true
				m.formField = 0
				m.formModel = "claude-sonnet-4-6"
				m.formApproveReads = true
				m.formApproveBash = true
				m.formApproveWrites = true
			}

		case "up", "k":
			if m.selectedIdx > 0 {
				m.selectedIdx--
				m.refreshLogViewport()
			}

		case "down", "j":
			if m.selectedIdx < len(m.order)-1 {
				m.selectedIdx++
				m.refreshLogViewport()
			}

		case "tab":
			if m.focus == panelList {
				m.focus = panelDetail
			} else {
				m.focus = panelList
			}

		case "y":
			if sid := m.selectedSessionID(); sid != "" {
				if req, ok := m.pendingPerms[sid]; ok {
					m.approvePermission(req.SessionID, true)
					delete(m.pendingPerms, sid)
				}
			}

		case "n":
			if sid := m.selectedSessionID(); sid != "" {
				if req, ok := m.pendingPerms[sid]; ok {
					m.approvePermission(req.SessionID, false)
					delete(m.pendingPerms, sid)
				}
			}

		case "pgup":
			m.logViewport.HalfViewUp()
		case "pgdown":
			m.logViewport.HalfViewDown()
		}

	// --- Session event messages (defined in session package) ---

	case session.StateMsg:
		if sv, ok := m.views[msg.SessionID]; ok {
			sv.state = msg.NewState
			if sv, ok2 := m.views[msg.SessionID]; ok2 {
				sv.turn = m.sessions[msg.SessionID].Turn()
			}
		}

	case session.LogMsg:
		if sv, ok := m.views[msg.SessionID]; ok {
			sv.logs = append(sv.logs, msg.Entry)
			if len(sv.logs) > 500 {
				sv.logs = sv.logs[1:]
			}
			if msg.SessionID == m.selectedSessionID() {
				m.refreshLogViewport()
			}
		}

	case session.PermissionMsg:
		m.pendingPerms[msg.SessionID] = msg
		if sv, ok := m.views[msg.SessionID]; ok {
			sv.state = session.StateWaitingPermission
		}
		// Auto-navigate to detail panel when the focused session needs permission.
		if msg.SessionID == m.selectedSessionID() {
			m.focus = panelDetail
		}

	case session.DoneMsg:
		if sv, ok := m.views[msg.SessionID]; ok {
			if msg.Err != nil {
				sv.state = session.StateFailed
				sv.err = msg.Err
			} else {
				sv.state = session.StateDone
			}
			sv.turn = m.sessions[msg.SessionID].Turn()
		}
		delete(m.pendingPerms, msg.SessionID)

	case NewSessionMsg:
		s := msg.Session
		m.order = append(m.order, s.ID)
		m.views[s.ID] = &sessionView{
			id:        s.ID,
			name:      s.Config.Name,
			state:     session.StateStarting,
			startedAt: s.StartedAt(),
		}
		m.sessions[s.ID] = s
		m.selectedIdx = len(m.order) - 1
		m.refreshLogViewport()

	// --- TUI-internal messages ---

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case TickMsg:
		cmds = append(cmds, tickCmd())

	default:
		var cmd tea.Cmd
		m.logViewport, cmd = m.logViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) approvePermission(sessionID string, approved bool) {
	s, ok := m.sessions[sessionID]
	if !ok {
		return
	}
	select {
	case s.RespCh <- session.PermissionResponse{Approved: approved}:
	default:
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}

	header := m.renderHeader()
	left := m.renderSessionList()
	right := m.renderDetail()
	statusBar := m.renderStatusBar()

	rightWidth := m.width - leftPanelWidth - 2
	if rightWidth < 10 {
		rightWidth = 10
	}

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		styleLeftPanel.Width(leftPanelWidth).Height(m.height-3).Render(left),
		styleRightPanel.Width(rightWidth).Height(m.height-3).Render(right),
	)

	base := lipgloss.JoinVertical(lipgloss.Left, header, body, statusBar)

	if m.formOpen {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderForm())
	}

	return base
}

func (m Model) renderForm() string {
	label := func(text string, active bool) string {
		if active {
			return styleFormActive.Render(text)
		}
		return styleFormLabel.Render(text)
	}
	textVal := func(val string, active bool) string {
		cursor := ""
		if active {
			cursor = styleFormCursor.Render("█")
		}
		if active {
			return styleFormActive.Render(val) + cursor
		}
		return styleFormInactive.Render(val)
	}
	boolVal := func(on bool, active bool) string {
		indicator := styleRed.Render("[✗]")
		if on {
			indicator = styleGreen.Render("[✓]")
		}
		hint := styleGray.Render(" [space] toggle")
		if active {
			return indicator + hint
		}
		return indicator
	}

	title := styleBold.Render("Add Session") + "  " + styleGray.Render("[tab] next  [space] toggle  [enter] launch  [esc] cancel")

	rows := []string{
		title,
		"",
		label("Project path *", m.formField == 0),
		"  " + textVal(m.formProject, m.formField == 0),
		"",
		label("Goal *", m.formField == 1),
		"  " + textVal(m.formGoal, m.formField == 1),
		"",
		label("Name (optional)", m.formField == 2),
		"  " + textVal(m.formName, m.formField == 2),
		"",
		label("Model", m.formField == 3),
		"  " + textVal(m.formModel, m.formField == 3),
		"",
		label("Auto-approve reads", m.formField == 4) + "  " + boolVal(m.formApproveReads, m.formField == 4),
		label("Auto-approve bash", m.formField == 5) + "  " + boolVal(m.formApproveBash, m.formField == 5),
		label("Auto-approve writes", m.formField == 6) + "  " + boolVal(m.formApproveWrites, m.formField == 6),
	}

	if m.formProject == "" || m.formGoal == "" {
		rows = append(rows, "", styleGray.Render("  * required"))
	}

	return styleFormBox.Render(strings.Join(rows, "\n"))
}

func (m Model) renderHeader() string {
	title := styleBold.Render("ChronosArchive")
	help := styleGray.Render("[↑↓/jk] nav  [tab] panel  [y/n] approve  [a] add  [q] quit")
	space := strings.Repeat(" ", max(0, m.width-lipgloss.Width(title)-lipgloss.Width(help)-2))
	return styleHeader.Width(m.width).Render(title + space + help)
}

func (m Model) renderStatusBar() string {
	running, waiting, done := 0, 0, 0
	for _, sv := range m.views {
		switch sv.state {
		case session.StateRunning:
			running++
		case session.StateWaitingPermission:
			waiting++
		case session.StateDone, session.StateFailed:
			done++
		}
	}
	txt := fmt.Sprintf("  %d running  %d waiting  %d done  (total: %d)", running, waiting, done, len(m.order))
	return styleStatusBar.Width(m.width).Render(txt)
}

func (m Model) renderSessionList() string {
	var sb strings.Builder
	sb.WriteString(styleBold.Render("SESSIONS") + "\n\n")

	for i, id := range m.order {
		sv := m.views[id]
		icon := stateIcon(sv.state, m.spinner)
		name := sv.name
		maxName := leftPanelWidth - 10
		if len(name) > maxName {
			name = name[:maxName]
		}
		turnStr := ""
		if sv.state == session.StateRunning || sv.state == session.StateWaitingPermission {
			turnStr = styleGray.Render(fmt.Sprintf(" t%d", sv.turn))
		}

		row := fmt.Sprintf(" %s %s%s", icon, name, turnStr)
		if _, hasPerm := m.pendingPerms[id]; hasPerm {
			row += " " + styleYellow.Render("!")
		}
		if i == m.selectedIdx {
			row = styleSelectedRow.Width(leftPanelWidth - 1).Render(row)
		}
		sb.WriteString(row + "\n")
	}
	return sb.String()
}

func stateIcon(s session.State, sp spinner.Model) string {
	switch s {
	case session.StateStarting:
		return styleGray.Render("○")
	case session.StateRunning:
		return styleGreen.Render(sp.View())
	case session.StateWaitingPermission:
		return styleYellow.Render("⏳")
	case session.StateDone:
		return styleGreen.Render("✓")
	case session.StateFailed:
		return styleRed.Render("✗")
	}
	return "?"
}

func (m Model) renderDetail() string {
	sid := m.selectedSessionID()
	if sid == "" {
		return styleGray.Render("No session selected")
	}
	sv := m.views[sid]

	var parts []string
	heading := styleBold.Render(sv.name) + "  " + stateIcon(sv.state, m.spinner)
	if sv.err != nil {
		heading += "  " + styleRed.Render(sv.err.Error())
	}
	parts = append(parts, heading, "")

	if req, ok := m.pendingPerms[sid]; ok {
		parts = append(parts, m.renderPermPrompt(req))
	}

	parts = append(parts, m.logViewport.View())
	return strings.Join(parts, "\n")
}

func (m Model) renderPermPrompt(req session.PermissionMsg) string {
	title := stylePermTitle.Render("PERMISSION REQUIRED")
	tool := "Tool:   " + styleToolName.Render(req.ToolName)
	detail := "Action: " + req.Description
	keys := styleGreen.Render("[y] Approve") + "  " + styleRed.Render("[n] Deny")
	content := strings.Join([]string{title, tool, detail, "", keys}, "\n")
	return stylePermBox.Render(content)
}

func (m *Model) resizeViewport() {
	rightWidth := m.width - leftPanelWidth - 4
	if rightWidth < 10 {
		rightWidth = 10
	}
	vpHeight := m.height - 7
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.logViewport.Width = rightWidth
	m.logViewport.Height = vpHeight
	m.refreshLogViewport()
}

func (m *Model) refreshLogViewport() {
	sid := m.selectedSessionID()
	if sid == "" {
		m.logViewport.SetContent("")
		return
	}
	sv := m.views[sid]
	var sb strings.Builder
	for _, e := range sv.logs {
		sb.WriteString(formatLogEntry(e))
		sb.WriteString("\n")
	}
	m.logViewport.SetContent(sb.String())
	m.logViewport.GotoBottom()
}

func formatLogEntry(e session.LogEntry) string {
	ts := e.Timestamp.Format("15:04:05")
	prefix := styleGray.Render(ts) + " "
	switch e.Kind {
	case session.LogToolCall:
		return prefix + styleToolName.Render("["+e.ToolName+"]") + " " + e.Text
	case session.LogToolResult:
		lines := strings.Split(e.Text, "\n")
		short := lines[0]
		if len(lines) > 1 {
			short += styleGray.Render(fmt.Sprintf(" (+%d lines)", len(lines)-1))
		}
		return prefix + styleResult.Render("  → "+short)
	case session.LogPermission:
		return prefix + styleYellow.Render("[perm] "+e.Text)
	case session.LogSystem:
		return prefix + styleGray.Render("[sys] "+e.Text)
	default:
		return prefix + e.Text
	}
}

func (m Model) selectedSessionID() string {
	if m.selectedIdx < 0 || m.selectedIdx >= len(m.order) {
		return ""
	}
	return m.order[m.selectedIdx]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
