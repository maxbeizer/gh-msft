// Package tui provides an interactive terminal inbox and calendar for gh-msft
// built on Bubble Tea. It consumes the mail.Provider and calendar.Provider
// interfaces so it is decoupled from WorkIQ and unit-testable with fakes.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/maxbeizer/gh-msft/internal/calendar"
	"github.com/maxbeizer/gh-msft/internal/mail"
)

// Message types exchanged with the Bubble Tea runtime.
type messagesLoadedMsg struct{ messages []mail.Message }
type eventsLoadedMsg struct{ events []calendar.Event }
type bodyLoadedMsg struct {
	id   string
	body string
}
type archivedMsg struct{ id string }
type errMsg struct{ err error }

// mode selects which view the TUI shows.
type mode int

const (
	mailMode     mode = iota // inbox list (default)
	calendarMode             // upcoming calendar events
)

// Model is the inbox/calendar TUI state.
type Model struct {
	provider mail.Provider
	cal      calendar.Provider
	top      int
	all      bool
	mode     mode

	messages   []mail.Message
	mailLoaded bool
	events     []calendar.Event
	calLoaded  bool

	cursor   int
	loading  bool
	err      error
	status   string
	showHelp bool
	quitting bool

	viewing      bool
	body         string
	bodyLoading  bool
	detailOffset int

	width  int
	height int
}

type calendarListRow struct {
	eventIndex int
	text       string
}

// New builds a model for the given mail provider. When all is true it loads all
// mail instead of only the inbox.
func New(provider mail.Provider, top int, all bool) Model {
	if top <= 0 {
		top = 50
	}
	return Model{provider: provider, top: top, all: all, loading: true}
}

// Init kicks off the initial load for the starting mode.
func (m Model) Init() tea.Cmd {
	if m.mode == calendarMode {
		return m.loadEventsCmd()
	}
	return m.loadCmd()
}

func (m Model) loadCmd() tea.Cmd {
	provider, top, all := m.provider, m.top, m.all
	return func() tea.Msg {
		msgs, err := provider.ListInbox(context.Background(), top, all)
		if err != nil {
			return errMsg{err}
		}
		return messagesLoadedMsg{msgs}
	}
}

func (m Model) loadEventsCmd() tea.Cmd {
	cal, top := m.cal, m.top
	return func() tea.Msg {
		if cal == nil {
			return eventsLoadedMsg{nil}
		}
		evs, err := cal.Upcoming(context.Background(), top)
		if err != nil {
			return errMsg{err}
		}
		return eventsLoadedMsg{evs}
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
		m.mailLoaded = true
		m.loading = false
		m.err = nil
		m.clampCursor()
		return m, nil

	case eventsLoadedMsg:
		m.events = msg.events
		m.calLoaded = true
		m.loading = false
		m.err = nil
		m.clampCursor()
		return m, nil

	case bodyLoadedMsg:
		m.body = msg.body
		m.bodyLoading = false
		m.clampDetailOffset()
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
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}
	if m.viewing {
		return m.handleDetailKey(msg)
	}
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "esc":
		m.showHelp = false
		return m, nil
	case "tab":
		m.toggleMode()
		m.cursor = 0
		if cmd := m.loadForModeCmd(); cmd != nil {
			m.loading = true
			return m, cmd
		}
		return m, nil
	case "enter":
		sel := m.selected()
		if sel == nil {
			return m, nil
		}
		m.viewing = true
		m.bodyLoading = true
		m.body = ""
		m.detailOffset = 0
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
		m.cursor = m.itemCount() - 1
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
		m.err = nil
		m.status = "refreshing…"
		if m.mode == calendarMode {
			return m, m.loadEventsCmd()
		}
		return m, m.loadCmd()
	}
	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q", "backspace":
		m.viewing = false
		m.body = ""
		m.bodyLoading = false
		m.detailOffset = 0
	case "j", "down":
		m.detailOffset++
		m.clampDetailOffset()
	case "k", "up":
		m.detailOffset--
		m.clampDetailOffset()
	case "g", "home":
		m.detailOffset = 0
	case "G", "end":
		m.detailOffset = m.maxDetailOffset()
	}
	return m, nil
}

// loadForModeCmd returns a load command when the current mode's data has not been
// fetched yet, or nil when it is already loaded.
func (m Model) loadForModeCmd() tea.Cmd {
	if m.mode == calendarMode && !m.calLoaded {
		return m.loadEventsCmd()
	}
	if m.mode == mailMode && !m.mailLoaded {
		return m.loadCmd()
	}
	return nil
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	m.clampCursor()
}

func (m *Model) toggleMode() {
	if m.mode == mailMode {
		m.mode = calendarMode
	} else {
		m.mode = mailMode
	}
}

// itemCount returns the number of rows in the active view.
func (m *Model) itemCount() int {
	if m.mode == calendarMode {
		return len(m.events)
	}
	return len(m.messages)
}

func (m *Model) clampCursor() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= m.itemCount() {
		m.cursor = m.itemCount() - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) clampDetailOffset() {
	if m.detailOffset < 0 {
		m.detailOffset = 0
	}
	if max := m.maxDetailOffset(); m.detailOffset > max {
		m.detailOffset = max
	}
}

// selected returns the highlighted mail message, or nil when not in mail mode or
// the inbox is empty. Calendar mode has no selectable mail actions yet.
func (m *Model) selected() *mail.Message {
	if m.mode != mailMode {
		return nil
	}
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

// View renders the current state.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.err != nil {
		return m.errorView()
	}
	if m.loading {
		return m.loadingView()
	}
	if m.viewing {
		return m.detailView()
	}
	if m.mode == calendarMode {
		return m.viewCalendar()
	}
	return m.viewMail()
}

// viewMail renders the inbox list.
func (m Model) viewMail() string {
	var b strings.Builder
	if len(m.messages) == 0 {
		empty := "No messages in Inbox."
		if m.all {
			empty = "No messages in all mail."
		}
		b.WriteString(styles.empty.Render(empty))
		b.WriteString("\n")
	}
	start, end := m.listRange(len(m.messages), m.cursor)
	for i := start; i < end; i++ {
		msg := m.messages[i]
		line := m.mailRow(i, msg)
		switch {
		case i == m.cursor:
			line = styles.selected.Render(line)
		case !msg.IsRead:
			line = styles.unread.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString(m.footer())
	return m.screen(m.chrome(m.mailTitle(), -1), b.String())
}

func (m Model) viewCalendar() string {
	var b strings.Builder
	if len(m.events) == 0 {
		b.WriteString(styles.empty.Render("No upcoming events."))
		b.WriteString("\n")
	} else {
		rows := m.calendarListRows()
		selectedRow := 0
		for i, row := range rows {
			if row.eventIndex == m.cursor {
				selectedRow = i
				break
			}
		}
		start, end := m.listRange(len(rows), selectedRow)
		for _, row := range rows[start:end] {
			b.WriteString(row.text)
			b.WriteString("\n")
		}
	}

	b.WriteString(m.footer())
	return m.screen(m.chrome(m.calendarTitle(), -1), b.String())
}

func (m Model) calendarListRows() []calendarListRow {
	rows := make([]calendarListRow, 0, len(m.events)*2)
	lastDay := ""
	for i, e := range m.events {
		day := calendarDayLabel(e)
		if day != lastDay {
			if lastDay != "" {
				rows = append(rows, calendarListRow{eventIndex: -1, text: ""})
			}
			rows = append(rows, calendarListRow{eventIndex: -1, text: styles.header.Render(day)})
			lastDay = day
		}
		line := m.calendarRow(i, e)
		if i == m.cursor {
			line = styles.selected.Render(line)
		}
		rows = append(rows, calendarListRow{eventIndex: i, text: line})
	}
	return rows
}

func (m Model) mailTitle() string {
	scope := "Inbox"
	if m.all {
		scope = "All mail"
	}
	return fmt.Sprintf("Inbox · %d %s · %s", len(m.messages), pluralize(len(m.messages), "message"), scope)
}

func (m Model) calendarTitle() string {
	return fmt.Sprintf("Upcoming · %d %s", len(m.events), pluralize(len(m.events), "event"))
}

func pluralize(count int, singular string) string {
	if count != 1 {
		return singular + "s"
	}
	return singular
}

func (m Model) mailRow(index int, msg mail.Message) string {
	cursor := "  "
	if index == m.cursor {
		cursor = "> "
	}
	state := "    "
	if !msg.IsRead {
		state = "NEW "
	}
	from := msg.From.Name
	if from == "" {
		from = msg.From.Email
	}
	width := m.listWidth()
	if m.isNarrow() {
		senderWidth := maxInt(4, width-len(cursor)-len(state)-6)
		firstLine := fmt.Sprintf("%s%s%s %s",
			cursor,
			state,
			truncate(from, senderWidth),
			receivedTime(msg),
		)
		secondLine := "    " + truncate(msg.Subject, maxInt(4, width-4))
		return truncate(firstLine, width) + "\n" + truncate(secondLine, width)
	}
	senderWidth := minInt(22, maxInt(12, width/4))
	subjectWidth := maxInt(8, width-senderWidth-21)
	return truncate(fmt.Sprintf("%s%s %-*s %s %s",
		cursor,
		state,
		senderWidth,
		truncate(from, senderWidth),
		truncate(msg.Subject, subjectWidth),
		receivedDateTime(msg),
	), width)
}

func (m Model) calendarRow(index int, e calendar.Event) string {
	cursor := "  "
	if index == m.cursor {
		cursor = "> "
	}
	width := m.listWidth()
	if m.isNarrow() {
		timeWidth := minInt(7, maxInt(4, width/2))
		subjectWidth := maxInt(4, width-len(cursor)-timeWidth-1)
		return truncate(fmt.Sprintf("%s%-*s %s", cursor, timeWidth, truncate(eventTimeLabel(e, true), timeWidth), truncate(e.Subject, subjectWidth)), width)
	}
	timeWidth := 20
	organizerWidth := 0
	if e.Organizer != "" && width >= 84 {
		organizerWidth = minInt(32, maxInt(20, width/4))
	}
	subjectWidth := width - timeWidth - 3
	if organizerWidth > 0 {
		subjectWidth -= 3 + organizerWidth
	}
	subjectWidth = maxInt(12, subjectWidth)
	line := fmt.Sprintf("%s%-*s %s", cursor, timeWidth, truncate(eventTimeLabel(e, false), timeWidth), truncate(e.Subject, subjectWidth))
	if organizerWidth > 0 {
		line += " " + styles.metadata.Render("· "+truncate(e.Organizer, organizerWidth))
	}
	return line
}

func receivedDateTime(msg mail.Message) string {
	if msg.Received.IsZero() {
		return "-"
	}
	return msg.Received.Time.Local().Format("Jan 02 15:04")
}

func receivedTime(msg mail.Message) string {
	if msg.Received.IsZero() {
		return "-"
	}
	return msg.Received.Time.Local().Format("15:04")
}

func calendarDayLabel(e calendar.Event) string {
	if e.Start.IsZero() {
		return "No date"
	}
	return e.Start.Time.Local().Format("Mon, Jan 02")
}

func eventTimeLabel(e calendar.Event, compact bool) string {
	if e.Start.IsZero() {
		return "-"
	}
	start := e.Start.Time.Local()
	if e.IsAllDay {
		if e.End.IsZero() || !e.End.Time.Local().After(start.AddDate(0, 0, 1)) {
			return "all day"
		}
		end := e.End.Time.Local().AddDate(0, 0, -1)
		if compact {
			return "all day→" + end.Format("Jan 02")
		}
		return "all day → " + end.Format("Jan 02")
	}
	if e.End.IsZero() {
		return start.Format("15:04")
	}
	end := e.End.Time.Local()
	if sameDay(start, end) {
		return start.Format("15:04") + "-" + end.Format("15:04")
	}
	if compact {
		return start.Format("Jan 02") + "→" + end.Format("Jan 02")
	}
	return start.Format("Jan 02 15:04") + " → " + end.Format("Jan 02 15:04")
}

// eventWhen formats an event's start and end for the calendar list. All-day
// events show the date followed by "all day". Same-day meetings show the end as
// a time only; multi-day meetings repeat the end date.
func eventWhen(e calendar.Event) string {
	if e.Start.IsZero() {
		return "-"
	}
	if e.IsAllDay {
		return e.Start.Format("Mon Jan 02") + " all day"
	}
	start := e.Start.Local()
	if e.End.IsZero() {
		return start.Format("Mon Jan 02 15:04")
	}
	end := e.End.Local()
	if sameDay(start, end) {
		return start.Format("Mon Jan 02 15:04") + " - " + end.Format("15:04")
	}
	return start.Format("Mon Jan 02 15:04") + " - " + end.Format("Mon Jan 02 15:04")
}

// sameDay reports whether a and b fall on the same calendar day.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// footer renders the shared status line and key hints for both views.
func (m Model) footer() string {
	var b strings.Builder
	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(styles.status.Render("Status: " + m.status))
		b.WriteString("\n")
	}
	var phrases []string
	if m.showHelp {
		phrases = strings.Split(m.expandedHelp(), " · ")
	} else {
		phrases = strings.Split(m.compactHelp(), " · ")
	}
	b.WriteString(styles.help.Render("Help: " + wrapPhrases(phrases, m.listWidth()-6)))
	b.WriteString("\n")
	return b.String()
}

func (m Model) compactHelp() string {
	if m.mode == calendarMode {
		return "j/k move · tab switch · ? help · q quit"
	}
	return "j/k move · enter open · tab switch · ? help · q quit"
}

func (m Model) expandedHelp() string {
	if m.mode == calendarMode {
		return "j/k or ↑/↓ move · g/G or home/end top/bottom · R refresh · tab mail · ?/esc close help · q quit"
	}
	return "j/k or ↑/↓ move · g/G or home/end top/bottom · enter open · a archive · r toggle read · R refresh · tab calendar · ?/esc close help · q quit"
}

// detailView renders the body of the selected message.
func (m Model) detailView() string {
	sel := m.selected()
	if sel == nil {
		return m.screen(m.chrome("Message", -1), styles.empty.Render("No message.")+"\n\n"+styles.help.Render("Help: esc to go back"))
	}

	var b strings.Builder
	lines := m.detailLines()
	m.clampDetailOffset()
	end := m.detailOffset + m.detailHeight()
	if end > len(lines) {
		end = len(lines)
	}
	b.WriteString(strings.Join(lines[m.detailOffset:end], "\n"))
	b.WriteString("\n")
	b.WriteString(styles.help.Render("Help: j/k or ↑/↓ scroll · g/G top/bottom · esc/enter/q back · ctrl+c quit"))
	b.WriteString("\n")
	return m.screen(m.chrome("Message", -1), b.String())
}

func (m Model) detailLines() []string {
	sel := m.selected()
	if sel == nil {
		return []string{"No message."}
	}

	width := m.listWidth()
	lines := []string{styles.header.Render(truncate(sel.Subject, width)), ""}
	if !sel.Received.IsZero() {
		lines = append(lines, styles.metadata.Render("Received: "+sel.Received.Time.Local().Format("Jan 02 15:04")))
	}
	lines = append(lines,
		styles.metadata.Render(truncate("From: "+formatAddr(sel.From), width)),
		styles.metadata.Render(truncate("To: "+formatAddrs(sel.To), width)),
		"",
	)
	switch {
	case m.bodyLoading:
		lines = append(lines, styles.loading.Render("Loading message…"))
	case m.body == "":
		lines = append(lines, styles.empty.Render("This message has no body."))
	default:
		lines = append(lines, strings.Split(lipgloss.NewStyle().Width(width).Render(m.body), "\n")...)
	}
	return lines
}

func (m Model) detailHeight() int {
	if m.height <= 0 {
		return 20
	}
	if height := m.height - 6; height > 0 {
		return height
	}
	return 1
}

func (m Model) listHeight() int {
	if m.height <= 0 {
		return 20
	}
	if height := m.height - 6; height > 0 {
		return height
	}
	return 1
}

func (m Model) listRange(total, focus int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	height := m.listHeight()
	if total <= height {
		return 0, total
	}
	if focus < 0 {
		focus = 0
	}
	if focus >= total {
		focus = total - 1
	}
	start := focus - height + 1
	if start < 0 {
		start = 0
	}
	if maxStart := total - height; start > maxStart {
		start = maxStart
	}
	return start, start + height
}

func (m Model) maxDetailOffset() int {
	max := len(m.detailLines()) - m.detailHeight()
	if max < 0 {
		return 0
	}
	return max
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

// subjectWidth returns how many columns the subject may use in a standard-width
// inbox row after reserving state, sender, and received-time columns.
func (m Model) subjectWidth() int {
	senderWidth := minInt(22, maxInt(12, m.listWidth()/4))
	return maxInt(8, m.listWidth()-senderWidth-21)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) loadingView() string {
	message := "Loading inbox…"
	if m.mode == calendarMode {
		message = "Loading calendar…"
	} else if m.all {
		message = "Loading all mail…"
	}
	content := styles.loading.Render(message) + "\n\n" + styles.help.Render("Help: R refresh · q quit")
	title := "Inbox · Loading…"
	if m.all {
		title = "All mail · Loading…"
	}
	if m.mode == calendarMode {
		title = "Upcoming · Loading…"
	}
	return m.screen(m.chrome(title, -1), content)
}

func (m Model) errorView() string {
	width := m.listWidth()
	content := styles.error.Render("Error") + "\n" +
		lipgloss.NewStyle().Width(width).Render(m.err.Error()) + "\n\n" +
		styles.help.Render("Help: R retry · q quit")
	return styles.app.Render(m.chrome("Error", -1)+"\n"+m.errorPanel(content)) + "\n"
}

func (m Model) chrome(title string, count int) string {
	if count >= 0 {
		title = fmt.Sprintf("%s (%d)", title, count)
	}
	if m.isNarrow() {
		return styles.chrome.Render(truncate("gh msft · "+title, m.listWidth()))
	}
	mailTab := styles.inactiveTab.Render("Mail")
	calendarTab := styles.inactiveTab.Render("Calendar")
	if m.mode == mailMode {
		mailTab = styles.activeTab.Render("[Mail]")
	} else {
		calendarTab = styles.activeTab.Render("[Calendar]")
	}
	prefixWidth := lipgloss.Width("gh msft  [Mail]  [Calendar]  ")
	title = truncate(title, maxInt(1, m.listWidth()-prefixWidth))
	return styles.chrome.Render("gh msft") + "  " + mailTab + "  " + calendarTab + "  " + styles.header.Render(title)
}

func (m Model) screen(chrome, content string) string {
	return styles.app.Render(chrome+"\n"+m.panel(content)) + "\n"
}

func (m Model) panel(content string) string {
	if m.isNarrow() {
		return styles.compactPanel.Width(m.listWidth()).Render(content)
	}
	return styles.panel.Width(m.listWidth()).Render(content)
}

func (m Model) errorPanel(content string) string {
	if m.isNarrow() {
		return styles.compactError.Width(m.listWidth()).Render(content)
	}
	return styles.errorPanel.Width(m.listWidth()).Render(content)
}

func (m Model) modeTitle() string {
	if m.mode == calendarMode {
		return "Calendar"
	}
	return "Inbox"
}

func (m Model) isNarrow() bool {
	return m.width > 0 && m.width < 54
}

func (m Model) listWidth() int {
	if m.width <= 0 {
		return 74
	}
	inset := 6
	if m.isNarrow() {
		inset = 4
	}
	width := m.width - inset
	if width < 12 {
		return 12
	}
	return width
}

func wrapPhrases(phrases []string, width int) string {
	if width <= 0 {
		return strings.Join(phrases, " · ")
	}
	var b strings.Builder
	lineWidth := 0
	for _, phrase := range phrases {
		phraseWidth := lipgloss.Width(phrase)
		if lineWidth == 0 {
			b.WriteString(phrase)
			lineWidth = phraseWidth
			continue
		}
		if lineWidth+3+phraseWidth > width {
			b.WriteString("\n")
			b.WriteString(phrase)
			lineWidth = phraseWidth
			continue
		}
		b.WriteString(" · ")
		b.WriteString(phrase)
		lineWidth += 3 + phraseWidth
	}
	return b.String()
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n == 1 {
		return string(runes[:1])
	}
	return string(runes[:n-1]) + "…"
}

// Run starts the interactive TUI against the given providers. When all is true
// the mail mode loads all messages; when startCalendar is true the TUI opens in
// calendar mode instead of mail mode.
func Run(mailP mail.Provider, calP calendar.Provider, top int, all bool, startCalendar bool) error {
	m := New(mailP, top, all)
	m.cal = calP
	if startCalendar {
		m.mode = calendarMode
	}
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
