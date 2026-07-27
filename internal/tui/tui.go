// Package tui provides an interactive terminal inbox for gh-msft built on
// Bubble Tea. It consumes the mail.Provider interface so it is decoupled from
// WorkIQ and unit-testable with a fake provider.
package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/maxbeizer/gh-msft/internal/mail"
)

// Message types exchanged with the Bubble Tea runtime.
type messagesLoadedMsg struct{ messages []mail.Message }
type bodyLoadedMsg struct {
	id   string
	body string
}
type archivedMsg struct{ id string }
type errMsg struct{ err error }

// Model is the inbox TUI state.
type Model struct {
	provider mail.Provider
	top      int

	messages []mail.Message
	cursor   int
	loading  bool
	err      error
	status   string
	showHelp bool
	quitting bool

	viewing     bool
	body        string
	bodyLoading bool

	width  int
	height int
}

// New builds an inbox model for the given provider.
func New(provider mail.Provider, top int) Model {
	if top <= 0 {
		top = 50
	}
	return Model{provider: provider, top: top, loading: true}
}

// Init kicks off the initial inbox load.
func (m Model) Init() tea.Cmd {
	return m.loadCmd()
}

func (m Model) loadCmd() tea.Cmd {
	provider, top := m.provider, m.top
	return func() tea.Msg {
		msgs, err := provider.ListInbox(context.Background(), top)
		if err != nil {
			return errMsg{err}
		}
		return messagesLoadedMsg{msgs}
	}
}

func archiveCmd(provider mail.Provider, id string) tea.Cmd {
	return func() tea.Msg {
		if err := provider.Archive(context.Background(), id); err != nil {
			return errMsg{err}
		}
		return archivedMsg{id}
	}
}

func bodyCmd(provider mail.Provider, id string) tea.Cmd {
	return func() tea.Msg {
		body, err := provider.Body(context.Background(), id)
		if err != nil {
			return errMsg{err}
		}
		return bodyLoadedMsg{id: id, body: body}
	}
}

// Update satisfies tea.Model. The concrete-typed update method below carries the
// real logic so it can be unit-tested without interface boxing.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := m.update(msg)
	return nm, cmd
}

// update advances the model. It is a pure function (aside from returned commands)
// so key handling can be unit-tested directly.
func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, tea.ClearScreen

	case messagesLoadedMsg:
		m.messages = msg.messages
		m.loading = false
		m.err = nil
		m.clampCursor()
		return m, nil

	case bodyLoadedMsg:
		m.body = msg.body
		m.bodyLoading = false
		return m, nil

	case errMsg:
		m.err = msg.err
		m.loading = false
		m.bodyLoading = false
		m.viewing = false
		return m, nil

	case archivedMsg:
		m.removeByID(msg.id)
		m.status = "archived"
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.viewing {
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc", "enter", "q", "backspace":
			m.viewing = false
			m.body = ""
			m.bodyLoading = false
			return m, nil
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "enter":
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		m.viewing = true
		m.bodyLoading = true
		m.body = ""
		m.status = ""
		return m, bodyCmd(m.provider, sel.ID)
	case "j", "down":
		m.moveCursor(1)
		return m, nil
	case "k", "up":
		m.moveCursor(-1)
		return m, nil
	case "g", "home":
		m.cursor = 0
		return m, nil
	case "G", "end":
		m.cursor = len(m.messages) - 1
		m.clampCursor()
		return m, nil
	case "r":
		// Local read toggle (visual only; provider has no mark-read yet).
		if sel := m.selected(); sel != nil {
			m.messages[m.cursor].IsRead = !sel.IsRead
		}
		return m, nil
	case "a":
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		m.status = "archiving…"
		return m, archiveCmd(m.provider, sel.ID)
	case "R":
		m.loading = true
		m.status = "refreshing…"
		return m, m.loadCmd()
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	m.clampCursor()
}

func (m *Model) clampCursor() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.messages) {
		m.cursor = len(m.messages) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// selected returns the highlighted message or nil when the inbox is empty.
func (m *Model) selected() *mail.Message {
	if m.cursor < 0 || m.cursor >= len(m.messages) {
		return nil
	}
	return &m.messages[m.cursor]
}

func (m *Model) removeByID(id string) {
	for i, msg := range m.messages {
		if msg.ID == id {
			m.messages = append(m.messages[:i], m.messages[i+1:]...)
			break
		}
	}
	m.clampCursor()
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	unreadStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	helpStyle     = lipgloss.NewStyle().Faint(true)
)

// View renders the current state.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.\n", m.err)
	}
	if m.loading {
		return "Loading inbox…\n"
	}
	if m.viewing {
		return m.detailView()
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Inbox (%d)", len(m.messages))))
	b.WriteString("\n\n")

	if len(m.messages) == 0 {
		b.WriteString("  (no messages)\n")
	}
	for i, msg := range m.messages {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		marker := "●"
		if msg.IsRead {
			marker = " "
		}
		from := msg.From.Name
		if from == "" {
			from = msg.From.Email
		}
		line := fmt.Sprintf("%s%s %-22s %s", cursor, marker, truncate(from, 22), truncate(msg.Subject, m.subjectWidth()))
		switch {
		case i == m.cursor:
			line = selectedStyle.Render(line)
		case !msg.IsRead:
			line = unreadStyle.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(helpStyle.Render(m.status))
		b.WriteString("\n")
	}
	if m.showHelp {
		b.WriteString(helpStyle.Render("j/k move · enter open · a archive · r toggle read · R refresh · g/G top/bottom · ? help · q quit"))
	} else {
		b.WriteString(helpStyle.Render("enter open · ? help · q quit"))
	}
	b.WriteString("\n")
	return b.String()
}

// detailView renders the body of the selected message.
func (m Model) detailView() string {
	sel := m.selected()
	if sel == nil {
		return "No message.\n\nPress esc to go back.\n"
	}

	var b strings.Builder
	width := m.width
	if width <= 0 {
		width = 80
	}
	b.WriteString(titleStyle.Render(lipgloss.NewStyle().Width(width).Render(sel.Subject)))
	b.WriteString("\n\n")
	if !sel.Received.IsZero() {
		b.WriteString(helpStyle.Render("Received: " + sel.Received.Time.Local().Format("Jan 02 15:04")))
		b.WriteString("\n")
	}
	b.WriteString(helpStyle.Width(width).Render("From: " + formatAddr(sel.From)))
	b.WriteString("\n")
	b.WriteString(helpStyle.Width(width).Render("To: " + formatAddrs(sel.To)))
	b.WriteString("\n")
	b.WriteString("\n")
	switch {
	case m.bodyLoading:
		b.WriteString("Loading message…\n")
	case m.body == "":
		b.WriteString("(empty message)\n")
	default:
		b.WriteString(lipgloss.NewStyle().Width(width).Render(m.body))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("esc/enter back · ctrl+c quit"))
	b.WriteString("\n")
	return b.String()
}

func formatAddr(a mail.Address) string {
	switch {
	case a.Name != "" && a.Email != "":
		return fmt.Sprintf("%s <%s>", a.Name, a.Email)
	case a.Email != "":
		return a.Email
	default:
		return a.Name
	}
}

func formatAddrs(as []mail.Address) string {
	if len(as) == 0 {
		return "-"
	}
	parts := make([]string, len(as))
	for i, a := range as {
		parts[i] = formatAddr(a)
	}
	return strings.Join(parts, ", ")
}

// subjectWidth returns how many columns the subject may use, derived from the
// terminal width so a resize shows more or less of the title. The fixed prefix
// (cursor, marker, from column, and separating spaces) is 27 columns.
func (m Model) subjectWidth() int {
	const prefix = 27
	if m.width <= 0 {
		return 50
	}
	w := m.width - prefix
	if w < 10 {
		w = 10
	}
	return w
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// Run starts the interactive inbox program against the given provider.
func Run(provider mail.Provider, top int) error {
	p := tea.NewProgram(New(provider, top))
	_, err := p.Run()
	return err
}
