package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maxbeizer/gh-msft/internal/mail"
	"github.com/maxbeizer/gh-msft/internal/mstime"
)

// inboxOf builds a deterministic inbox of n unread messages with IDs "1".."n".
func inboxOf(n int) []mail.Message {
	messages := make([]mail.Message, 0, n)
	for i := 1; i <= n; i++ {
		messages = append(messages, mail.Message{
			ID:       fmt.Sprintf("%d", i),
			Subject:  fmt.Sprintf("Subject %d", i),
			From:     mail.Address{Name: fmt.Sprintf("Sender %d", i)},
			Received: mstime.Parse("2026-01-02T15:00:00Z"),
		})
	}
	return messages
}

func loadedInbox(t *testing.T, n int) Model {
	t.Helper()
	m := New(&fakeProvider{inbox: inboxOf(n)}, 10, false)
	m, _ = m.update(messagesLoadedMsg{inboxOf(n)})
	return m
}

var (
	shiftDown = tea.KeyMsg{Type: tea.KeyShiftDown}
	shiftUp   = tea.KeyMsg{Type: tea.KeyShiftUp}
	plainDown = tea.KeyMsg{Type: tea.KeyDown}
	plainUp   = tea.KeyMsg{Type: tea.KeyUp}
)

// selectedIDs returns the marked IDs in inbox order for stable assertions.
func selectedIDs(m Model) []string {
	var ids []string
	for _, message := range m.messages {
		if m.marks.has(message.ID) {
			ids = append(ids, message.ID)
		}
	}
	return ids
}

func assertSelection(t *testing.T, m Model, want []string) {
	t.Helper()
	got := selectedIDs(m)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("selection = %v, want %v", got, want)
	}
	if m.marks.len() != len(want) {
		t.Errorf("selection size = %d, want %d", m.marks.len(), len(want))
	}
}

// AC1, AC2, AC3, AC4, AC5, AC6, AC7: shift-arrow selection semantics.
func TestShiftArrowSelection(t *testing.T) {
	tests := []struct {
		name         string
		messageCount int
		startCursor  int
		keys         []tea.KeyMsg
		wantSelected []string
		wantCursor   int
	}{
		{
			name:         "AC1 shift+down marks the current message and advances",
			messageCount: 5,
			keys:         []tea.KeyMsg{shiftDown},
			wantSelected: []string{"1"},
			wantCursor:   1,
		},
		{
			name:         "AC2 shift+up marks the current message and retreats",
			messageCount: 5,
			startCursor:  2,
			keys:         []tea.KeyMsg{shiftUp},
			wantSelected: []string{"3"},
			wantCursor:   1,
		},
		{
			name:         "AC3 a plain arrow passes over a message",
			messageCount: 5,
			keys:         []tea.KeyMsg{shiftDown, shiftDown, plainDown, shiftDown},
			wantSelected: []string{"1", "2", "4"},
			wantCursor:   4,
		},
		{
			name:         "AC4 shift is momentary so plain arrows never extend",
			messageCount: 5,
			keys:         []tea.KeyMsg{shiftDown, plainDown, shiftDown, plainDown, plainDown},
			wantSelected: []string{"1", "3"},
			wantCursor:   4,
		},
		{
			name:         "AC5 shift+down at the bottom marks without moving",
			messageCount: 3,
			startCursor:  2,
			keys:         []tea.KeyMsg{shiftDown},
			wantSelected: []string{"3"},
			wantCursor:   2,
		},
		{
			name:         "AC6 shift+up at the top marks without moving",
			messageCount: 3,
			keys:         []tea.KeyMsg{shiftUp},
			wantSelected: []string{"1"},
			wantCursor:   0,
		},
		{
			name:         "AC7 re-marking a message is idempotent",
			messageCount: 5,
			keys:         []tea.KeyMsg{shiftDown, plainUp, shiftDown},
			wantSelected: []string{"1"},
			wantCursor:   1,
		},
		{
			name:         "AC7 marking at a boundary repeatedly stays idempotent",
			messageCount: 3,
			startCursor:  2,
			keys:         []tea.KeyMsg{shiftDown, shiftDown, shiftDown},
			wantSelected: []string{"3"},
			wantCursor:   2,
		},
		{
			name:         "shift+up builds a selection upwards",
			messageCount: 5,
			startCursor:  4,
			keys:         []tea.KeyMsg{shiftUp, shiftUp, plainUp, shiftUp},
			wantSelected: []string{"2", "4", "5"},
			wantCursor:   0,
		},
		{
			name:         "J mirrors shift+down",
			messageCount: 5,
			keys:         []tea.KeyMsg{key("J"), key("J")},
			wantSelected: []string{"1", "2"},
			wantCursor:   2,
		},
		{
			name:         "K mirrors shift+up",
			messageCount: 5,
			startCursor:  2,
			keys:         []tea.KeyMsg{key("K")},
			wantSelected: []string{"3"},
			wantCursor:   1,
		},
		{
			name:         "shift+down on an empty inbox is a no-op",
			messageCount: 0,
			keys:         []tea.KeyMsg{shiftDown},
			wantSelected: nil,
			wantCursor:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loadedInbox(t, tt.messageCount)
			m.cursor = tt.startCursor
			for _, k := range tt.keys {
				m, _ = m.update(k)
			}
			assertSelection(t, m, tt.wantSelected)
			if m.cursor != tt.wantCursor {
				t.Errorf("cursor = %d, want %d", m.cursor, tt.wantCursor)
			}
		})
	}
}

// AC8: r marks every selected message read, reports a count, and clears.
func TestMarkSelectionAsRead(t *testing.T) {
	m := loadedInbox(t, 5)
	m, _ = m.update(shiftDown)
	m, _ = m.update(plainDown)
	m, _ = m.update(shiftDown)
	assertSelection(t, m, []string{"1", "3"})

	m, _ = m.update(key("r"))

	if !m.messages[0].IsRead || !m.messages[2].IsRead {
		t.Error("selected messages should be read")
	}
	if m.messages[1].IsRead {
		t.Error("the passed-over message should stay unread")
	}
	if m.messages[3].IsRead || m.messages[4].IsRead {
		t.Error("unselected messages should stay unread")
	}
	if m.status != "marked 2 as read" {
		t.Errorf("status = %q, want %q", m.status, "marked 2 as read")
	}
	assertSelection(t, m, nil)
}

// AC9: bulk mark-as-read sets rather than toggles.
func TestMarkSelectionAsReadIsNotAToggle(t *testing.T) {
	m := loadedInbox(t, 3)
	m.messages[2].IsRead = true
	m, _ = m.update(shiftDown)
	m, _ = m.update(plainDown)
	m, _ = m.update(shiftDown)
	assertSelection(t, m, []string{"1", "3"})

	m, _ = m.update(key("r"))

	if !m.messages[0].IsRead || !m.messages[2].IsRead {
		t.Error("every selected message should end up read")
	}
}

// AC10: r without a selection keeps toggling the cursor row.
func TestReadKeyWithoutSelectionTogglesCursorRow(t *testing.T) {
	m := loadedInbox(t, 3)
	m.cursor = 1

	m, _ = m.update(key("r"))
	if !m.messages[1].IsRead {
		t.Error("r should mark the cursor row read")
	}
	if m.messages[0].IsRead || m.messages[2].IsRead {
		t.Error("r should not touch other messages")
	}

	m, _ = m.update(key("r"))
	if m.messages[1].IsRead {
		t.Error("r should still toggle back to unread when nothing is selected")
	}
}

// AC11, AC12, AC13: the selection clears when it would become misleading.
func TestSelectionClearingTriggers(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyMsg
	}{
		{"AC11 esc clears an unapplied selection", []tea.KeyMsg{{Type: tea.KeyEsc}}},
		{"AC12 switching views clears the selection", []tea.KeyMsg{{Type: tea.KeyTab}}},
		{"AC13 refreshing clears the selection", []tea.KeyMsg{key("R")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loadedInbox(t, 5)
			m.cal = &fakeCal{events: sampleEvents()}
			m, _ = m.update(shiftDown)
			m, _ = m.update(shiftDown)
			assertSelection(t, m, []string{"1", "2"})

			for _, k := range tt.keys {
				m, _ = m.update(k)
			}

			assertSelection(t, m, nil)
			for i, message := range m.messages {
				if message.IsRead {
					t.Errorf("message %d should still be unread", i+1)
				}
			}
		})
	}
}

// AC14: archiving prunes the archived message out of the selection.
func TestArchivePrunesSelection(t *testing.T) {
	m := loadedInbox(t, 3)
	m, _ = m.update(shiftDown)
	m, _ = m.update(shiftDown)
	assertSelection(t, m, []string{"1", "2"})

	m, _ = m.update(archivedMsg{id: "1"})

	if len(m.messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(m.messages))
	}
	assertSelection(t, m, []string{"2"})
}

// A refresh that replaces the list must drop marks for messages that are gone
// while keeping marks for messages that survive.
func TestReloadPrunesSelection(t *testing.T) {
	m := loadedInbox(t, 3)
	m, _ = m.update(shiftDown)
	m, _ = m.update(shiftDown)
	assertSelection(t, m, []string{"1", "2"})

	m, _ = m.update(messagesLoadedMsg{inboxOf(1)})

	assertSelection(t, m, []string{"1"})
}

// AC16: shift-arrows do nothing in calendar mode.
func TestCalendarModeIgnoresShiftArrows(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m.cal = &fakeCal{events: sampleEvents()}
	m.mode = calendarMode
	m, _ = m.update(eventsLoadedMsg{sampleEvents()})

	m, _ = m.update(shiftDown)
	m, _ = m.update(shiftUp)

	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	if m.marks.len() != 0 {
		t.Errorf("selection size = %d, want 0", m.marks.len())
	}
}

// AC15: the selection is visible in the list and the title.
func TestSelectionIsRendered(t *testing.T) {
	m := loadedInbox(t, 3)
	m.width, m.height = 100, 24
	m, _ = m.update(shiftDown)
	m, _ = m.update(plainDown)
	m, _ = m.update(shiftDown)
	assertSelection(t, m, []string{"1", "3"})

	out := m.View()
	if !strings.Contains(out, "2 selected") {
		t.Errorf("view should report the selection count, got:\n%s", out)
	}

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "Subject 1"), strings.Contains(line, "Subject 3"):
			if !strings.Contains(line, markGlyph) {
				t.Errorf("selected row should show %q, got %q", markGlyph, line)
			}
		case strings.Contains(line, "Subject 2"):
			if strings.Contains(line, markGlyph) {
				t.Errorf("passed-over row should not show %q, got %q", markGlyph, line)
			}
		}
	}
}

func TestTitleOmitsSelectionCountWhenEmpty(t *testing.T) {
	m := loadedInbox(t, 3)
	if strings.Contains(m.mailTitle(), "selected") {
		t.Errorf("title = %q, should not mention a selection", m.mailTitle())
	}
}

func TestHelpMentionsSelection(t *testing.T) {
	m := loadedInbox(t, 3)
	if !strings.Contains(m.expandedHelp(), "select") {
		t.Errorf("expanded help should document selection, got %q", m.expandedHelp())
	}
	if !strings.Contains(m.compactHelp(), "select") {
		t.Errorf("compact help should document selection, got %q", m.compactHelp())
	}
}
