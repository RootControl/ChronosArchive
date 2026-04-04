package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chronosarchive/chronosarchive/config"
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

	// Called when the user presses [r] on a failed/done session.
	retry RetryFunc

	// Add-session form overlay.
	formOpen           bool
	formField          int // 0=project 1=goal 2=name 3=model 4=reads 5=bash 6=writes 7=web 8=http 9=fileops 10=thinking 11=thinkingBudget
	formProject        string
	formGoal           string
	formName           string
	formModel          string
	formApproveReads   bool
	formApproveBash    bool
	formApproveWrites  bool
	formApproveWeb     bool
	formApproveHTTP    bool
	formApproveFileOps bool
	formThinking       bool
	formThinkingBudget string
	formTemplateName   string // name of the template currently loaded into the form

	// Template cycling state (populated when form opens).
	templates   []config.Template
	templateIdx int

	// Brief status flash shown in the status bar (e.g. "saved template X").
	statusMsg    string
	statusMsgTTL int // decrements on each TickMsg; message clears at 0

	// Log search state.
	logSearching bool
	logSearch    string
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

// SetRetry provides the callback used when the user retries a failed/done session.
func (m *Model) SetRetry(fn RetryFunc) {
	m.retry = fn
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
				m.resetForm()

			case "tab", "down":
				m.formField = (m.formField + 1) % 12

			case "shift+tab", "up":
				m.formField = (m.formField + 11) % 12

			case "enter":
				if m.formProject != "" && m.formGoal != "" {
					name := m.formName
					if name == "" {
						name = fmt.Sprintf("session-%d", len(m.order)+1)
					}
					if m.launch != nil {
						budget := 10000
						if m.formThinkingBudget != "" {
							fmt.Sscanf(m.formThinkingBudget, "%d", &budget)
						}
						m.launch(LaunchOpts{
							Project:        m.formProject,
							Goal:           m.formGoal,
							Name:           name,
							Model:          m.formModel,
							ApproveReads:   m.formApproveReads,
							ApproveBash:    m.formApproveBash,
							ApproveWrites:  m.formApproveWrites,
							ApproveWeb:     m.formApproveWeb,
							ApproveHTTP:    m.formApproveHTTP,
							ApproveFileOps: m.formApproveFileOps,
							Thinking:       m.formThinking,
							ThinkingBudget: budget,
						})
					}
					m.resetForm()
				}

			case " ":
				switch m.formField {
				case 4:
					m.formApproveReads = !m.formApproveReads
				case 5:
					m.formApproveBash = !m.formApproveBash
				case 6:
					m.formApproveWrites = !m.formApproveWrites
				case 7:
					m.formApproveWeb = !m.formApproveWeb
				case 8:
					m.formApproveHTTP = !m.formApproveHTTP
				case 9:
					m.formApproveFileOps = !m.formApproveFileOps
				case 10:
					m.formThinking = !m.formThinking
				}

			case "ctrl+t":
				if len(m.templates) > 0 {
					m.templateIdx = (m.templateIdx + 1) % len(m.templates)
					t := m.templates[m.templateIdx]
					m.formTemplateName = t.Name
					if t.Goal != "" {
						m.formGoal = t.Goal
					}
					if t.Model != "" {
						m.formModel = t.Model
					}
					m.formApproveReads = t.ToolPermissions.AutoApproveReads
					m.formApproveBash = t.ToolPermissions.AutoApproveBash
					m.formApproveWrites = t.ToolPermissions.AutoApproveWrites
					m.formApproveWeb = t.ToolPermissions.AutoApproveWebFetch
					m.formApproveHTTP = t.ToolPermissions.AutoApproveHTTP
					m.formApproveFileOps = t.ToolPermissions.AutoApproveFileOps
					m.formThinking = t.Thinking
					if t.ThinkingBudget > 0 {
						m.formThinkingBudget = fmt.Sprintf("%d", t.ThinkingBudget)
					}
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
				case 11:
					if len(m.formThinkingBudget) > 0 {
						m.formThinkingBudget = m.formThinkingBudget[:len(m.formThinkingBudget)-1]
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
					case 11:
						if ch >= "0" && ch <= "9" {
							m.formThinkingBudget += ch
						}
					}
				}
			}
			break
		}

		// Log search mode: capture input before the global key switch.
		if m.logSearching {
			switch msg.String() {
			case "esc", "enter":
				m.logSearching = false
				m.refreshLogViewport()
			case "backspace":
				if len(m.logSearch) > 0 {
					m.logSearch = m.logSearch[:len(m.logSearch)-1]
					m.refreshLogViewport()
				}
			default:
				if ch := msg.String(); len(ch) == 1 {
					m.logSearch += ch
					m.refreshLogViewport()
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
				m.formApproveReads = false
				m.formApproveBash = false
				m.formApproveWrites = false
				m.formApproveWeb = false
				m.formApproveHTTP = false
				m.formApproveFileOps = false
				m.formThinking = false
				m.formThinkingBudget = ""
				m.formTemplateName = ""
				m.templateIdx = -1
				m.templates, _ = config.LoadTemplates()
			}

		case "/":
			m.logSearching = true
			m.logSearch = ""
			m.focus = panelDetail
			m.refreshLogViewport()

		case "up", "k":
			if m.selectedIdx > 0 {
				m.selectedIdx--
				m.logSearching = false
				m.logSearch = ""
				m.refreshLogViewport()
			}

		case "down", "j":
			if m.selectedIdx < len(m.order)-1 {
				m.selectedIdx++
				m.logSearching = false
				m.logSearch = ""
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

		case "e":
			if sid := m.selectedSessionID(); sid != "" {
				if s, ok := m.sessions[sid]; ok {
					if sv, ok2 := m.views[sid]; ok2 {
						path, err := exportLog(s, sv.logs)
						if err != nil {
							m.statusMsg = "export failed: " + err.Error()
						} else {
							m.statusMsg = "exported → " + path
						}
						m.statusMsgTTL = 8 // ~4 s
					}
				}
			}

		case "p":
			if sid := m.selectedSessionID(); sid != "" {
				if s, ok := m.sessions[sid]; ok {
					switch m.views[sid].state {
					case session.StateRunning:
						s.Pause()
					case session.StatePaused:
						s.Resume()
					}
				}
			}

		case "r":
			if sid := m.selectedSessionID(); sid != "" {
				if sv := m.views[sid]; sv.state == session.StateFailed || sv.state == session.StateDone {
					if s, ok := m.sessions[sid]; ok && m.retry != nil {
						m.retry(s)
					}
				}
			}

		case "T":
			if sid := m.selectedSessionID(); sid != "" {
				if s, ok := m.sessions[sid]; ok {
					tmpl := config.TemplateFromSession(s.Config)
					if err := config.SaveTemplate(tmpl); err != nil {
						m.statusMsg = "template save failed: " + err.Error()
					} else {
						m.statusMsg = fmt.Sprintf("saved template %q", tmpl.Name)
					}
					m.statusMsgTTL = 6 // ~3 s at 500 ms tick
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

	case RetrySessionMsg:
		s := msg.Session
		// Replace the old session reference without changing list order.
		m.sessions[s.ID] = s
		if sv, ok := m.views[s.ID]; ok {
			sv.state = session.StateStarting
			sv.logs = nil
			sv.turn = 0
			sv.err = nil
			sv.startedAt = s.StartedAt()
		}
		delete(m.pendingPerms, s.ID)
		m.refreshLogViewport()

	// --- TUI-internal messages ---

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case TickMsg:
		if m.statusMsgTTL > 0 {
			m.statusMsgTTL--
			if m.statusMsgTTL == 0 {
				m.statusMsg = ""
			}
		}
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

	templateHint := styleGray.Render("[ctrl+t] load template")
	if m.formTemplateName != "" {
		templateHint = styleGray.Render("template: ") + styleGreen.Render(m.formTemplateName) + styleGray.Render("  [ctrl+t] next")
	} else if len(m.templates) == 0 {
		templateHint = styleGray.Render("(no saved templates)")
	}
	title := styleBold.Render("Add Session") + "  " + styleGray.Render("[tab] next  [space] toggle  [enter] launch  [esc] cancel") + "  " + templateHint

	thinkingBudget := m.formThinkingBudget
	if thinkingBudget == "" {
		thinkingBudget = "10000"
	}

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
		styleGray.Render("── Permissions ──────────────────────────"),
		label("Auto-approve reads", m.formField == 4) + "  " + boolVal(m.formApproveReads, m.formField == 4),
		label("Auto-approve bash", m.formField == 5) + "  " + boolVal(m.formApproveBash, m.formField == 5),
		label("Auto-approve writes", m.formField == 6) + "  " + boolVal(m.formApproveWrites, m.formField == 6),
		label("Auto-approve web_fetch", m.formField == 7) + "  " + boolVal(m.formApproveWeb, m.formField == 7),
		label("Auto-approve http_request", m.formField == 8) + "  " + boolVal(m.formApproveHTTP, m.formField == 8),
		label("Auto-approve file ops", m.formField == 9) + "  " + boolVal(m.formApproveFileOps, m.formField == 9),
		"",
		styleGray.Render("── Thinking ─────────────────────────────"),
		label("Enable thinking", m.formField == 10) + "  " + boolVal(m.formThinking, m.formField == 10),
		label("Thinking budget (tokens)", m.formField == 11),
		"  " + textVal(thinkingBudget, m.formField == 11),
	}

	if m.formProject == "" || m.formGoal == "" {
		rows = append(rows, "", styleGray.Render("  * required"))
	}

	return styleFormBox.Render(strings.Join(rows, "\n"))
}

func (m *Model) resetForm() {
	m.formOpen = false
	m.formField = 0
	m.formProject = ""
	m.formGoal = ""
	m.formName = ""
	m.formModel = ""
	m.formApproveReads = false
	m.formApproveBash = false
	m.formApproveWrites = false
	m.formApproveWeb = false
	m.formApproveHTTP = false
	m.formApproveFileOps = false
	m.formThinking = false
	m.formThinkingBudget = ""
	m.formTemplateName = ""
}

func (m Model) renderHeader() string {
	title := styleBold.Render("ChronosArchive")
	help := styleGray.Render("[↑↓/jk] nav  [tab] panel  [y/n] approve  [a] add  [p] pause  [r] retry  [/] search  [e] export  [T] template  [q] quit")
	space := strings.Repeat(" ", max(0, m.width-lipgloss.Width(title)-lipgloss.Width(help)-2))
	return styleHeader.Width(m.width).Render(title + space + help)
}

func (m Model) renderStatusBar() string {
	if m.statusMsg != "" {
		return styleStatusBar.Width(m.width).Render("  " + m.statusMsg)
	}
	running, waiting, paused, done := 0, 0, 0, 0
	for _, sv := range m.views {
		switch sv.state {
		case session.StateRunning:
			running++
		case session.StateWaitingPermission:
			waiting++
		case session.StatePaused:
			paused++
		case session.StateDone, session.StateFailed:
			done++
		}
	}
	txt := fmt.Sprintf("  %d running  %d waiting  %d paused  %d done  (total: %d)", running, waiting, paused, done, len(m.order))
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
	case session.StatePaused:
		return styleYellow.Render("⏸")
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
	if s, ok := m.sessions[sid]; ok {
		in, out := s.TokenUsage()
		if in+out > 0 {
			cost := estimateCost(s.Config.Model, in, out)
			heading += "  " + styleGray.Render(fmt.Sprintf("%dk tok  $%.4f", (in+out)/1000, cost))
		}
	}
	switch sv.state {
	case session.StateRunning:
		heading += "  " + styleGray.Render("[p] pause")
	case session.StatePaused:
		heading += "  " + styleGray.Render("[p] resume")
	case session.StateFailed, session.StateDone:
		heading += "  " + styleGray.Render("[r] retry")
	}
	parts = append(parts, heading, "")

	if req, ok := m.pendingPerms[sid]; ok {
		parts = append(parts, m.renderPermPrompt(req))
	}

	if m.logSearching {
		cursor := styleBlue.Render("█")
		parts = append(parts, styleGray.Render("search: ")+styleGreen.Render(m.logSearch)+cursor+styleGray.Render("  [esc] clear"))
	} else if m.logSearch != "" {
		parts = append(parts, styleGray.Render("filter: ")+styleGreen.Render(m.logSearch)+styleGray.Render("  [/] edit  [↑↓] nav clears"))
	}

	parts = append(parts, m.logViewport.View())
	return strings.Join(parts, "\n")
}

func (m Model) renderPermPrompt(req session.PermissionMsg) string {
	title := stylePermTitle.Render("PERMISSION REQUIRED")
	tool := "Tool:   " + styleToolName.Render(req.ToolName)
	detail := "Action: " + req.Description
	keys := styleGreen.Render("[y] Approve") + "  " + styleRed.Render("[n] Deny")
	lines := []string{title, tool, detail}
	if preview := permPreview(req.ToolName, req.RawInput); preview != "" {
		lines = append(lines, "", preview)
	}
	lines = append(lines, "", keys)
	return stylePermBox.Render(strings.Join(lines, "\n"))
}

// permPreview returns a short content preview for write/edit tool calls.
func permPreview(toolName string, rawInput []byte) string {
	var m map[string]any
	if err := json.Unmarshal(rawInput, &m); err != nil {
		return ""
	}
	const maxLines = 8
	switch toolName {
	case "edit_file":
		old, _ := m["old_string"].(string)
		new, _ := m["new_string"].(string)
		if old == "" && new == "" {
			return ""
		}
		var sb strings.Builder
		for _, line := range splitHead(old, maxLines) {
			sb.WriteString(styleRed.Render("- " + line) + "\n")
		}
		for _, line := range splitHead(new, maxLines) {
			sb.WriteString(styleGreen.Render("+ " + line) + "\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	case "write_file":
		content, _ := m["content"].(string)
		if content == "" {
			return ""
		}
		var sb strings.Builder
		for _, line := range splitHead(content, maxLines) {
			sb.WriteString(styleGray.Render("  " + line) + "\n")
		}
		lines := strings.Count(content, "\n") + 1
		if lines > maxLines {
			sb.WriteString(styleGray.Render(fmt.Sprintf("  … (%d more lines)", lines-maxLines)))
		}
		return strings.TrimRight(sb.String(), "\n")
	}
	return ""
}

func splitHead(s string, n int) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		return lines[:n]
	}
	return lines
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
	filter := strings.ToLower(m.logSearch)
	for _, e := range sv.logs {
		line := formatLogEntry(e)
		if filter != "" && !strings.Contains(strings.ToLower(e.Text), filter) {
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	m.logViewport.SetContent(sb.String())
	if filter == "" {
		m.logViewport.GotoBottom()
	}
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
