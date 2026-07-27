package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maxbeizer/gh-msft/internal/mail"
)

type fakeProvider struct {
	inbox      []mail.Message
	listErr    error
	archived   []string
	archiveErr error
	body       string
	bodyErr    error
	bodyIDs    []string
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

func (f *fakeProvider) Body(ctx context.Context, id string) (string, error) {
	f.bodyIDs = append(f.bodyIDs, id)
	if f.bodyErr != nil {
		return "", f.bodyErr
	}
	return f.body, nil
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

func TestEnterOpensMessageAndLoadsBody(t *testing.T) {
	fp := &fakeProvider{body: "hello world"}
	m := New(fp, 10)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})

	m, cmd := m.update(key("enter"))
	if !m.viewing {
		t.Fatal("enter should open the detail view")
	}
	if !m.bodyLoading {
		t.Fatal("enter should mark body as loading")
	}
	if cmd == nil {
		t.Fatal("enter should return a body command")
	}

	res := cmd()
	bm, ok := res.(bodyLoadedMsg)
	if !ok {
		t.Fatalf("expected bodyLoadedMsg, got %T", res)
	}
	if bm.id != "1" || bm.body != "hello world" {
		t.Errorf("bodyLoadedMsg = %+v", bm)
	}
	if len(fp.bodyIDs) != 1 || fp.bodyIDs[0] != "1" {
		t.Errorf("provider body ids = %v", fp.bodyIDs)
	}

	m, _ = m.update(bm)
	if m.bodyLoading {
		t.Error("body should no longer be loading")
	}
	if m.body != "hello world" {
		t.Errorf("body = %q", m.body)
	}
}

func TestEnterOnEmptyInboxIsNoOp(t *testing.T) {
	m := New(&fakeProvider{}, 10)
	m, _ = m.update(messagesLoadedMsg{nil})
	m, cmd := m.update(key("enter"))
	if m.viewing {
		t.Error("enter on empty inbox should not open detail view")
	}
	if cmd != nil {
		t.Error("enter on empty inbox should be a no-op")
	}
}

func TestDetailViewKeysClose(t *testing.T) {
	fp := &fakeProvider{body: "body text"}
	m := New(fp, 10)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	m, _ = m.update(key("enter"))
	if !m.viewing {
		t.Fatal("precondition: should be viewing")
	}

	m, cmd := m.update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.viewing {
		t.Error("esc should close the detail view")
	}
	if m.quitting {
		t.Error("esc should not quit the program")
	}
	if cmd != nil {
		t.Error("esc should not return a command")
	}
}

func TestDetailViewBodyErrorSurfaces(t *testing.T) {
	fp := &fakeProvider{bodyErr: errors.New("boom")}
	m := New(fp, 10)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	_, cmd := m.update(key("enter"))
	if cmd == nil {
		t.Fatal("expected body command")
	}
	res := cmd()
	em, ok := res.(errMsg)
	if !ok {
		t.Fatalf("expected errMsg, got %T", res)
	}
	m2, _ := m.update(em)
	if m2.viewing {
		t.Error("body error should close the detail view")
	}
	if m2.err == nil {
		t.Error("body error should set err")
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

func TestSubjectWidthTracksResize(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  int
	}{
		{"unset falls back", 0, 50},
		{"narrow clamps to min", 20, 10},
		{"wide grows", 120, 93},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(&fakeProvider{}, 10)
			m, _ = m.update(tea.WindowSizeMsg{Width: tt.width, Height: 24})
			if got := m.subjectWidth(); got != tt.want {
				t.Errorf("subjectWidth() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestResizeShowsMoreSubject(t *testing.T) {
	long := "This is a very long email subject line that keeps going and going"
	msgs := []mail.Message{{ID: "1", Subject: long, From: mail.Address{Name: "Alice"}}}

	m := New(&fakeProvider{}, 10)
	m, _ = m.update(messagesLoadedMsg{msgs})

	m, _ = m.update(tea.WindowSizeMsg{Width: 60, Height: 24})
	narrow := m.View()
	m, _ = m.update(tea.WindowSizeMsg{Width: 200, Height: 24})
	wide := m.View()

	if len(wide) <= len(narrow) {
		t.Errorf("wide view (%d) should render more than narrow (%d)", len(wide), len(narrow))
	}
	if !strings.Contains(wide, long) {
		t.Error("wide view should show the full subject")
	}
}
