package tui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maxbeizer/gh-msft/internal/mail"
)

type fakeProvider struct {
	inbox      []mail.Message
	listErr    error
	archived   []string
	archiveErr error
}

func (f *fakeProvider) ListInbox(ctx context.Context, top int) ([]mail.Message, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.inbox, nil
}

func (f *fakeProvider) Archive(ctx context.Context, id string) error {
	if f.archiveErr != nil {
		return f.archiveErr
	}
	f.archived = append(f.archived, id)
	return nil
}

func sampleMessages() []mail.Message {
	return []mail.Message{
		{ID: "1", Subject: "First", From: mail.Address{Name: "Alice"}, IsRead: false},
		{ID: "2", Subject: "Second", From: mail.Address{Name: "Bob"}, IsRead: true},
		{ID: "3", Subject: "Third", From: mail.Address{Name: "Carol"}, IsRead: false},
	}
}

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestLoadedPopulatesMessages(t *testing.T) {
	m := New(&fakeProvider{}, 10)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	if m.loading {
		t.Fatal("should not be loading after load")
	}
	if len(m.messages) != 3 {
		t.Fatalf("got %d messages", len(m.messages))
	}
}

func TestNavigationClamps(t *testing.T) {
	m := New(&fakeProvider{}, 10)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})

	// Up at top stays at 0.
	m, _ = m.update(key("k"))
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	// Move down twice.
	m, _ = m.update(key("j"))
	m, _ = m.update(key("j"))
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor)
	}
	// Down at bottom stays at last.
	m, _ = m.update(key("j"))
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped)", m.cursor)
	}
	// G to bottom, g to top.
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if m.cursor != 0 {
		t.Errorf("g: cursor = %d, want 0", m.cursor)
	}
}

func TestArchiveKeyReturnsCommandAndArchivedRemoves(t *testing.T) {
	fp := &fakeProvider{}
	m := New(fp, 10)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})

	// Pressing 'a' on the first message should return a command.
	m2, cmd := m.update(key("a"))
	if cmd == nil {
		t.Fatal("expected archive command")
	}
	// Execute the command to drive the provider and get the resulting msg.
	resMsg := cmd()
	am, ok := resMsg.(archivedMsg)
	if !ok {
		t.Fatalf("expected archivedMsg, got %T", resMsg)
	}
	if am.id != "1" {
		t.Errorf("archived id = %q, want 1", am.id)
	}
	if len(fp.archived) != 1 || fp.archived[0] != "1" {
		t.Errorf("provider archived = %v", fp.archived)
	}
	// Feeding archivedMsg back removes the item from the list.
	m3, _ := m2.update(am)
	if len(m3.messages) != 2 {
		t.Fatalf("after archive, got %d messages, want 2", len(m3.messages))
	}
	for _, msg := range m3.messages {
		if msg.ID == "1" {
			t.Error("archived message still present")
		}
	}
}

func TestArchiveErrorSurfaces(t *testing.T) {
	fp := &fakeProvider{archiveErr: errors.New("nope")}
	m := New(fp, 10)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	_, cmd := m.update(key("a"))
	if cmd == nil {
		t.Fatal("expected command")
	}
	res := cmd()
	if _, ok := res.(errMsg); !ok {
		t.Fatalf("expected errMsg, got %T", res)
	}
}

func TestReadToggle(t *testing.T) {
	m := New(&fakeProvider{}, 10)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	if m.messages[0].IsRead {
		t.Fatal("precondition: first should be unread")
	}
	m, _ = m.update(key("r"))
	if !m.messages[0].IsRead {
		t.Error("r should toggle read state")
	}
}

func TestQuitSetsFlag(t *testing.T) {
	m := New(&fakeProvider{}, 10)
	m, cmd := m.update(key("q"))
	if !m.quitting {
		t.Error("q should set quitting")
	}
	if cmd == nil {
		t.Error("q should return tea.Quit command")
	}
}

func TestLoadErrorSetsErr(t *testing.T) {
	m := New(&fakeProvider{}, 10)
	m, _ = m.update(errMsg{errors.New("boom")})
	if m.err == nil {
		t.Error("expected err to be set")
	}
	if m.loading {
		t.Error("should not be loading after error")
	}
}

func TestViewDoesNotPanic(t *testing.T) {
	m := New(&fakeProvider{}, 10)
	_ = m.View() // loading
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	_ = m.View() // list
	m, _ = m.update(key("?"))
	_ = m.View() // help
	m, _ = m.update(errMsg{errors.New("x")})
	_ = m.View() // error
}

func TestEmptyInboxSelectedNil(t *testing.T) {
	m := New(&fakeProvider{}, 10)
	m, _ = m.update(messagesLoadedMsg{nil})
	if m.selected() != nil {
		t.Error("selected should be nil for empty inbox")
	}
	// Pressing 'a' on empty inbox is a no-op (no command).
	_, cmd := m.update(key("a"))
	if cmd != nil {
		t.Error("archive on empty inbox should be no-op")
	}
}
