// Package ui is the Bubble Tea dashboard.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/visualabsai/agentdeck/internal/agent"
	"github.com/visualabsai/agentdeck/internal/session"
	"github.com/visualabsai/agentdeck/internal/tmux"
)

// ---------- styles ----------

var (
	cText    = lipgloss.AdaptiveColor{Light: "#1F2328", Dark: "#E6EDF3"}
	cMuted   = lipgloss.AdaptiveColor{Light: "#6E7781", Dark: "#7D8590"}
	cFaint   = lipgloss.AdaptiveColor{Light: "#D0D7DE", Dark: "#30363D"}
	cAccent  = lipgloss.Color("#7C6CF0")
	cWorking = lipgloss.Color("#3FB950")
	cWaiting = lipgloss.Color("#F0883E")
	cIdle    = lipgloss.Color("#8B949E")
	cShell   = lipgloss.Color("#58A6FF")
	cDanger  = lipgloss.Color("#F85149")

	sTitle   = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sMuted   = lipgloss.NewStyle().Foreground(cMuted)
	sText    = lipgloss.NewStyle().Foreground(cText)
	sPanel   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cFaint).Padding(0, 1)
	sPanelHi = sPanel.Copy().BorderForeground(cAccent)
	sSel     = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#EEF0FF", Dark: "#1E2130"}).Bold(true)
	sKey     = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sBadge   = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	sDanger  = lipgloss.NewStyle().Foreground(cDanger).Bold(true)
	sPrompt  = lipgloss.NewStyle().Foreground(cAccent)
)

func statusStyle(s session.Status) (string, lipgloss.Style) {
	switch s {
	case session.StatusWorking:
		return "●", lipgloss.NewStyle().Foreground(cWorking)
	case session.StatusWaiting:
		return "◐", lipgloss.NewStyle().Foreground(cWaiting).Bold(true)
	case session.StatusIdle:
		return "○", lipgloss.NewStyle().Foreground(cIdle)
	case session.StatusShell:
		return "$", lipgloss.NewStyle().Foreground(cShell)
	}
	return "?", lipgloss.NewStyle().Foreground(cMuted)
}

// ---------- model ----------

type mode int

const (
	modeList mode = iota
	modeNew
	modeSend
	modeKill
)

type refreshMsg struct {
	sessions []session.Session
	err      error
}
type tickMsg time.Time
type flashMsg struct{}

type Model struct {
	sessions []session.Session
	cursor   int
	width    int
	height   int
	mode     mode
	err      error
	flash    string

	// new-session form
	agentIdx  int
	dirInput  textinput.Model
	promptIn  textinput.Model
	formField int

	sendIn textinput.Model
}

// New builds the initial model.
func New() Model {
	dir := textinput.New()
	dir.Placeholder = "~/code/project"
	dir.Prompt = ""
	wd, _ := os.Getwd()
	dir.SetValue(wd)
	dir.CharLimit = 512

	pr := textinput.New()
	pr.Placeholder = "optional first prompt"
	pr.Prompt = ""
	pr.CharLimit = 2000

	send := textinput.New()
	send.Placeholder = "message to type into the session"
	send.Prompt = ""
	send.CharLimit = 2000

	return Model{dirInput: dir, promptIn: pr, sendIn: send}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refresh, tick())
}

func refresh() tea.Msg {
	s, err := session.Discover()
	return refreshMsg{sessions: s, err: err}
}

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func clearFlash() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return flashMsg{} })
}

func (m Model) current() *session.Session {
	if len(m.sessions) == 0 || m.cursor >= len(m.sessions) {
		return nil
	}
	return &m.sessions[m.cursor]
}

// ---------- update ----------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		w := m.inputWidth()
		m.dirInput.Width, m.promptIn.Width, m.sendIn.Width = w, w, w
		return m, nil
	case tickMsg:
		return m, tea.Batch(refresh, tick())
	case flashMsg:
		m.flash = ""
		return m, nil
	case refreshMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		// keep cursor on the same session if it moved
		var keep string
		if c := m.current(); c != nil {
			keep = c.Name
		}
		m.sessions = msg.sessions
		m.cursor = 0
		for i, s := range m.sessions {
			if s.Name == keep {
				m.cursor = i
			}
		}
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeNew:
			return m.updateNew(msg)
		case modeSend:
			return m.updateSend(msg)
		case modeKill:
			return m.updateKill(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.sessions)-1 {
			m.cursor++
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		if len(m.sessions) > 0 {
			m.cursor = len(m.sessions) - 1
		}
	case "r":
		return m, refresh
	case "enter", "a":
		if c := m.current(); c != nil {
			name := c.Name
			return m, tea.ExecProcess(tmux.AttachCmd(name), func(err error) tea.Msg {
				return refresh()
			})
		}
	case "n":
		m.mode = modeNew
		m.formField = 0
		m.promptIn.SetValue("")
		m.setDir()
		return m, nil
	case "s":
		if c := m.current(); c != nil {
			m.mode = modeSend
			m.sendIn.SetValue("")
			return m, m.sendIn.Focus()
		}
	case "x", "d", "delete":
		if m.current() != nil {
			m.mode = modeKill
		}
	case "1", "2", "3", "4", "5":
		// quick-launch agent N in the selected session's directory
		idx := int(msg.String()[0] - '1')
		agents := agent.All()
		if idx < len(agents) {
			m.mode = modeNew
			m.agentIdx = idx
			m.formField = 1
			m.promptIn.SetValue("")
			m.setDir()
			return m, m.dirInput.Focus()
		}
	}
	return m, nil
}

func (m Model) updateNew(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	agents := agent.All()
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.dirInput.Blur()
		m.promptIn.Blur()
		return m, nil
	case "tab", "down":
		m.formField = (m.formField + 1) % 3
		return m, m.focusForm()
	case "shift+tab", "up":
		m.formField = (m.formField + 2) % 3
		return m, m.focusForm()
	case "enter":
		if m.formField < 2 {
			m.formField++
			return m, m.focusForm()
		}
		a := agents[m.agentIdx]
		name, err := session.Launch(a.ID, m.dirInput.Value(), "", m.promptIn.Value())
		m.mode = modeList
		m.dirInput.Blur()
		m.promptIn.Blur()
		if err != nil {
			m.flash = sDanger.Render("✗ " + err.Error())
		} else {
			m.flash = lipgloss.NewStyle().Foreground(cWorking).Render("✓ started " + name)
		}
		return m, tea.Batch(refresh, clearFlash())
	}
	if m.formField == 0 {
		switch msg.String() {
		case "left", "h":
			m.agentIdx = (m.agentIdx + len(agents) - 1) % len(agents)
		case "right", "l", " ":
			m.agentIdx = (m.agentIdx + 1) % len(agents)
		}
		return m, nil
	}
	var cmd tea.Cmd
	if m.formField == 1 {
		m.dirInput, cmd = m.dirInput.Update(msg)
	} else {
		m.promptIn, cmd = m.promptIn.Update(msg)
	}
	return m, cmd
}

// setDir pre-fills the directory with the selected session's. SetValue leaves
// the cursor where it was unless the field was empty, which would make the next
// keystroke edit the middle of the path, so park it at the end.
func (m *Model) setDir() {
	c := m.current()
	if c == nil || c.Dir == "" {
		return
	}
	m.dirInput.SetValue(c.Dir)
	m.dirInput.CursorEnd()
}

func (m *Model) focusForm() tea.Cmd {
	m.dirInput.Blur()
	m.promptIn.Blur()
	switch m.formField {
	case 1:
		return m.dirInput.Focus()
	case 2:
		return m.promptIn.Focus()
	}
	return nil
}

func (m Model) updateSend(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.sendIn.Blur()
		return m, nil
	case "enter":
		c := m.current()
		m.mode = modeList
		m.sendIn.Blur()
		if c == nil {
			return m, nil
		}
		if err := tmux.Send(c.Name, m.sendIn.Value()); err != nil {
			m.flash = sDanger.Render("✗ " + err.Error())
		} else {
			m.flash = sMuted.Render("→ sent to " + c.Name)
		}
		return m, tea.Batch(refresh, clearFlash())
	}
	var cmd tea.Cmd
	m.sendIn, cmd = m.sendIn.Update(msg)
	return m, cmd
}

func (m Model) updateKill(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		m.mode = modeList
		if c := m.current(); c != nil {
			if err := session.Kill(c.Name); err != nil {
				m.flash = sDanger.Render("✗ " + err.Error())
			} else {
				m.flash = sMuted.Render("✕ killed " + c.Name)
			}
		}
		return m, tea.Batch(refresh, clearFlash())
	default:
		m.mode = modeList
	}
	return m, nil
}

// ---------- view ----------

// Panels are two columns of border plus two of padding, so a panel of total
// width W has W-4 columns for its content.
const (
	panelChrome = 4
	minPanelW   = 24 // below this a panel shows nothing useful
)

// layout returns the total width of each panel for the current terminal size.
// When the terminal is too narrow for two panels listW is 0 and the detail
// panel takes the whole width.
func (m Model) layout() (listW, detailW int) {
	if m.width < 2*minPanelW+2 {
		return 0, m.width
	}
	listW = m.width * 38 / 100
	if listW < minPanelW {
		listW = minPanelW
	}
	if listW > 56 {
		listW = 56
	}
	detailW = m.width - listW - 2
	if detailW < minPanelW {
		detailW = minPanelW
		listW = m.width - detailW - 2
	}
	return listW, detailW
}

// inputWidth is the number of columns the form's text inputs may use. It has to
// be pushed into the textinput models from Update: they only recompute their
// horizontal scroll window there, not at render time.
func (m Model) inputWidth() int {
	_, detailW := m.layout()
	return detailW - panelChrome - 6
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	header := m.viewHeader()
	footer := m.viewFooter()

	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 1
	// A bordered panel needs a row of content between its two border rows.
	if bodyH < 3 {
		return clip(lipgloss.JoinVertical(lipgloss.Left, header, footer), m.width, m.height)
	}

	formMode := m.mode == modeNew || m.mode == modeSend || m.mode == modeKill
	panelStyle := func(focused bool) lipgloss.Style {
		if focused {
			return sPanelHi
		}
		return sPanel
	}

	listW, detailW := m.layout()
	var body string
	if listW == 0 {
		// Too narrow for two panels: show the one that matters right now.
		inner, focused := m.viewList(detailW-panelChrome, bodyH-2), false
		if formMode {
			inner, focused = m.viewDetail(detailW-panelChrome, bodyH-2), true
		}
		body = panelStyle(focused).Width(detailW - 2).Height(bodyH - 2).Render(inner)
	} else {
		left := m.viewList(listW-panelChrome, bodyH-2)
		right := m.viewDetail(detailW-panelChrome, bodyH-2)
		leftPanel := sPanel.Width(listW - 2).Height(bodyH - 2).Render(left)
		rightPanel := panelStyle(formMode).Width(detailW - 2).Height(bodyH - 2).Render(right)
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
	}
	return clip(lipgloss.JoinVertical(lipgloss.Left, header, body, footer), m.width, m.height)
}

func (m Model) viewHeader() string {
	working, waiting := 0, 0
	for _, s := range m.sessions {
		switch s.Status {
		case session.StatusWorking:
			working++
		case session.StatusWaiting:
			waiting++
		}
	}
	left := sTitle.Render("✦ agentdeck")
	stats := fmt.Sprintf("%d sessions", len(m.sessions))
	if working > 0 {
		stats += " · " + lipgloss.NewStyle().Foreground(cWorking).Render(fmt.Sprintf("%d working", working))
	}
	if waiting > 0 {
		stats += " · " + lipgloss.NewStyle().Foreground(cWaiting).Bold(true).Render(fmt.Sprintf("%d waiting", waiting))
	}
	if m.flash != "" {
		stats = m.flash + "   " + stats
	}
	if m.err != nil {
		stats = sDanger.Render(m.err.Error())
	}
	right := sMuted.Render(stats + "  " + time.Now().Format("15:04"))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return " " + left + strings.Repeat(" ", gap) + right
}

func (m Model) viewList(w, h int) string {
	if len(m.sessions) == 0 {
		msg := "No tmux sessions.\n\nPress " + sKey.Render("n") + " to start an agent."
		if !tmux.Available() {
			msg = sDanger.Render("tmux not found") + "\n\nInstall tmux first."
		}
		return sMuted.Render(msg)
	}
	rowsPerItem := 2
	visible := h / rowsPerItem
	if visible < 1 {
		visible = 1
	}
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	var b strings.Builder
	for i := start; i < len(m.sessions) && i < start+visible; i++ {
		s := m.sessions[i]
		glyph, st := statusStyle(s.Status)
		icon, name := "  ", s.Name
		var badge string
		if s.Agent != nil {
			icon = lipgloss.NewStyle().Foreground(lipgloss.Color(s.Agent.Color)).Render(s.Agent.Icon) + " "
			badge = lipgloss.NewStyle().Foreground(lipgloss.Color(s.Agent.Color)).Render(s.Agent.ID)
		} else {
			badge = sMuted.Render(s.Process)
		}
		line1 := st.Render(glyph) + " " + icon + sText.Bold(true).Render(truncate(name, w-14))
		line1 = truncate(padRight(line1, w-lipgloss.Width(badge)-1)+badge, w)
		meta := shorten(s.Dir) + " · " + ago(s.Activity)
		if s.Attached {
			meta += " · attached"
		}
		line2 := "    " + sMuted.Render(truncate(meta, w-4))
		if i == m.cursor {
			line1 = sSel.Width(w).Render(line1)
			line2 = sSel.Width(w).Render(line2)
		}
		b.WriteString(line1 + "\n" + line2 + "\n")
	}
	if len(m.sessions) > visible {
		b.WriteString(sMuted.Render(fmt.Sprintf("%d/%d", m.cursor+1, len(m.sessions))))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) viewDetail(w, h int) string {
	switch m.mode {
	case modeNew:
		return m.viewNewForm(w)
	case modeKill:
		c := m.current()
		if c == nil {
			// The session went away while the confirmation was up.
			return sMuted.Render("That session is gone.")
		}
		return sDanger.Render("Kill session "+c.Name+"?") + "\n\n" +
			sKey.Render("y") + sMuted.Render(" confirm   ") + sKey.Render("any other key") + sMuted.Render(" cancel")
	}
	c := m.current()
	if c == nil {
		return sMuted.Render("Select a session to see its output.")
	}
	glyph, st := statusStyle(c.Status)
	title := sText.Bold(true).Render(c.Name)
	if c.Agent != nil {
		title = sBadge.Background(lipgloss.Color(c.Agent.Color)).Render(c.Agent.Icon+" "+c.Agent.Name) + "  " + title
	}
	line2 := st.Render(glyph+" "+c.Status.String()) + sMuted.Render(" · "+shorten(c.Dir)+" · started "+ago(c.Created))
	if c.Attached {
		line2 += sMuted.Render(" · attached")
	}
	if c.Prompt != "" {
		line2 += "\n" + sMuted.Render("prompt: ") + sText.Render(truncate(c.Prompt, w-8))
	}
	head := title + "\n" + line2 + "\n" + sMuted.Render(strings.Repeat("─", w)) + "\n"

	tailH := h - lipgloss.Height(head) - 1
	if m.mode == modeSend {
		tailH -= 3
	}
	if tailH < 1 {
		tailH = 1
	}
	lines := strings.Split(c.Tail, "\n")
	if len(lines) > tailH {
		lines = lines[len(lines)-tailH:]
	}
	for i := range lines {
		lines[i] = truncate(lines[i], w)
	}
	body := strings.Join(lines, "\n")

	if m.mode == modeSend {
		body += strings.Repeat("\n", max(0, tailH-len(lines))) + "\n" +
			sPrompt.Render("→ ") + m.sendIn.View() + "\n" + sMuted.Render("enter send · esc cancel")
	}
	return head + body
}

func (m Model) viewNewForm(w int) string {
	agents := agent.All()
	var b strings.Builder
	b.WriteString(sTitle.Render("New session") + "\n\n")

	// agent picker
	label := func(i int, s string) string {
		if m.formField == i {
			return sKey.Render("▸ " + s)
		}
		return sMuted.Render("  " + s)
	}
	b.WriteString(label(0, "Agent") + "\n   ")
	for i, a := range agents {
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(a.Color)).Padding(0, 1)
		if i == m.agentIdx {
			st = st.Background(lipgloss.Color(a.Color)).Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
		}
		b.WriteString(st.Render(a.Icon+" "+strings.Title(a.ID)) + " ")
	}
	b.WriteString("\n\n")

	b.WriteString(label(1, "Directory") + "\n   " + field(m.dirInput, w-4) + "\n\n")
	b.WriteString(label(2, "Prompt") + "\n   " + field(m.promptIn, w-4) + "\n\n")

	a := agents[m.agentIdx]
	cmd := append([]string{a.Command}, a.Args...)
	b.WriteString(sMuted.Render("will run: ") + sText.Render(strings.Join(cmd, " ")) + "\n\n")
	b.WriteString(sMuted.Render("←/→ pick agent · tab next field · enter start · esc cancel"))
	return b.String()
}

func (m Model) viewFooter() string {
	k := func(key, desc string) string { return sKey.Render(key) + sMuted.Render(" "+desc) }
	var parts []string
	switch m.mode {
	case modeNew:
		parts = []string{k("tab", "field"), k("←/→", "agent"), k("enter", "start"), k("esc", "back")}
	case modeSend:
		parts = []string{k("enter", "send"), k("esc", "back")}
	case modeKill:
		parts = []string{k("y", "kill"), k("esc", "back")}
	default:
		parts = []string{k("↑/↓", "move"), k("enter", "attach"), k("n", "new"), k("1-5", "quick new"), k("s", "send"), k("x", "kill"), k("r", "refresh"), k("q", "quit")}
	}
	return " " + strings.Join(parts, sMuted.Render("  ·  "))
}

// ---------- helpers ----------

func shorten(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home) {
		p = "~" + strings.TrimPrefix(p, home)
	}
	parts := strings.Split(p, string(filepath.Separator))
	if len(parts) > 3 {
		p = filepath.Join(parts[0], "…", parts[len(parts)-2], parts[len(parts)-1])
		if parts[0] == "" {
			p = "/" + p
		}
	}
	return p
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// truncate shortens s to w display columns. Pane captures are full of escape
// sequences, so it cuts with an ANSI-aware truncator rather than by rune.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// clip is the last guard before the frame reaches the terminal: no line may be
// wider than the screen and no more lines than it is tall, or the display wraps
// and smears. Correct layout should make this a no-op.
func clip(view string, w, h int) string {
	lines := strings.Split(view, "\n")
	if h > 0 && len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = ansi.Truncate(l, w, "")
		}
	}
	return strings.Join(lines, "\n")
}

// field renders a text input clipped to w columns. textinput only recomputes
// its horizontal scroll window inside Update, so a long pre-filled value (a deep
// project path, say) would otherwise render in full and wrap the form panel.
func field(ti textinput.Model, w int) string {
	return truncate(ti.View(), w)
}

func padRight(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
