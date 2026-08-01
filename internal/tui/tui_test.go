package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maxbeizer/gh-msft/internal/calendar"
	"github.com/maxbeizer/gh-msft/internal/mail"
	"github.com/maxbeizer/gh-msft/internal/mstime"
)

type fakeProvider struct {
	inbox      []mail.Message
	listErr    error
	archived   []string
	archiveErr error
	gotAll     bool
	body       string
	bodyErr    error
	bodyIDs    []string
}

func (f *fakeProvider) ListInbox(ctx context.Context, top int, all bool) ([]mail.Message, error) {
	f.gotAll = all
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.inbox, nil
}

func (f *fakeProvider) GetDetail(ctx context.Context, id string) (mail.Detail, error) {
	for _, message := range f.inbox {
		if message.ID == id {
			return mail.NewDetail(message, f.body), nil
		}
	}
	return mail.Detail{}, errors.New("message not found")
}

func (f *fakeProvider) Archive(ctx context.Context, id string) error {
	if f.archiveErr != nil {
		return f.archiveErr
	}
	f.archived = append(f.archived, id)
	return nil
}

type fakeCal struct {
	events []calendar.Event
	err    error
}

func (f *fakeCal) Upcoming(ctx context.Context, top int) ([]calendar.Event, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events, nil
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

func sampleEvents() []calendar.Event {
	return []calendar.Event{
		{ID: "e1", Subject: "Standup", Organizer: "Alice",
			Start: mstime.Parse("2026-01-02T15:00:00Z"), End: mstime.Parse("2026-01-02T15:15:00Z")},
		{ID: "e2", Subject: "1:1", Organizer: "Bob",
			Start: mstime.Parse("2026-01-02T16:00:00Z"), End: mstime.Parse("2026-01-02T16:30:00Z")},
	}
}

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestLoadedPopulatesMessages(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	if m.loading {
		t.Fatal("should not be loading after load")
	}
	if len(m.messages) != 3 {
		t.Fatalf("got %d messages", len(m.messages))
	}
}

func TestNavigationClamps(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
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
	m := New(fp, 10, false)
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
	m := New(fp, 10, false)
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
	m := New(&fakeProvider{}, 10, false)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	if m.messages[0].IsRead {
		t.Fatal("precondition: first should be unread")
	}
	m, _ = m.update(key("r"))
	if !m.messages[0].IsRead {
		t.Error("r should toggle read state")
	}
}

func TestTabTogglesModeAndLoadsEvents(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m.cal = &fakeCal{events: sampleEvents()}
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	if m.mode != mailMode {
		t.Fatalf("mode = %d, want mailMode", m.mode)
	}

	tab := tea.KeyMsg{Type: tea.KeyTab}
	// First tab: switch to calendar; events not loaded yet so a load command runs.
	m, cmd := m.update(tab)
	if m.mode != calendarMode {
		t.Errorf("after tab, mode = %d, want calendarMode", m.mode)
	}
	if cmd == nil {
		t.Fatal("expected events load command on first switch to calendar")
	}
	res := cmd()
	ev, ok := res.(eventsLoadedMsg)
	if !ok {
		t.Fatalf("expected eventsLoadedMsg, got %T", res)
	}
	m, _ = m.update(ev)
	if len(m.events) != 2 {
		t.Errorf("got %d events, want 2", len(m.events))
	}

	// Second tab: back to mail; already loaded so no command.
	m, cmd = m.update(tab)
	if m.mode != mailMode {
		t.Errorf("after second tab, mode = %d, want mailMode", m.mode)
	}
	if cmd != nil {
		t.Error("mail already loaded; expected no command on switch back")
	}

	// Third tab: back to calendar; already loaded so no command.
	m, cmd = m.update(tab)
	if cmd != nil {
		t.Error("calendar already loaded; expected no reload command")
	}
}

func TestEventWhenSingleVsMultiDay(t *testing.T) {
	single := calendar.Event{
		Start: mstime.Parse("2026-01-02T09:00:00Z"),
		End:   mstime.Parse("2026-01-02T17:00:00Z"),
	}
	got := eventWhen(single)
	parts := strings.SplitN(got, " - ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected a range, got %q", got)
	}
	if len(parts[1]) != len("15:04") {
		t.Errorf("single-day end should be time-only, got %q (full %q)", parts[1], got)
	}

	multi := calendar.Event{
		Start: mstime.Parse("2026-01-02T09:00:00Z"),
		End:   mstime.Parse("2026-01-05T17:00:00Z"),
	}
	got = eventWhen(multi)
	parts = strings.SplitN(got, " - ", 2)
	if len(parts) != 2 || len(parts[1]) == len("15:04") {
		t.Errorf("multi-day end should include a date, got %q", got)
	}
}

func TestCalendarViewRendersEvents(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m.cal = &fakeCal{events: sampleEvents()}
	m.mode = calendarMode
	_ = m.View() // loading calendar
	m, _ = m.update(eventsLoadedMsg{sampleEvents()})
	out := m.View() // calendar list
	if !strings.Contains(out, "Standup") {
		t.Errorf("calendar view missing event subject; got:\n%s", out)
	}
	if !strings.Contains(out, " - ") {
		t.Errorf("calendar view should show start - end range; got:\n%s", out)
	}
	m, _ = m.update(eventsLoadedMsg{nil})
	_ = m.View() // empty calendar
}

func TestStartInCalendarModeLoadsEvents(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m.cal = &fakeCal{events: sampleEvents()}
	m.mode = calendarMode
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should load events when starting in calendar mode")
	}
	if _, ok := cmd().(eventsLoadedMsg); !ok {
		t.Error("Init in calendar mode should produce eventsLoadedMsg")
	}
}

func TestCalendarViewShowsAllDay(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m.cal = &fakeCal{}
	m.mode = calendarMode
	allDay := []calendar.Event{
		{ID: "h", Subject: "Company Holiday", IsAllDay: true,
			Start: mstime.Parse("2026-01-02T00:00:00Z"), End: mstime.Parse("2026-01-03T00:00:00Z")},
	}
	m, _ = m.update(eventsLoadedMsg{allDay})
	out := m.View()
	if !strings.Contains(out, "all day") {
		t.Errorf("all-day event should render \"all day\"; got:\n%s", out)
	}
	if strings.Contains(out, "00:00") {
		t.Errorf("all-day event should not show a time; got:\n%s", out)
	}
}

func TestQuitSetsFlag(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m, cmd := m.update(key("q"))
	if !m.quitting {
		t.Error("q should set quitting")
	}
	if cmd == nil {
		t.Error("q should return tea.Quit command")
	}
}

func TestLoadErrorSetsErr(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
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
	m := New(fp, 10, false)
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
	m := New(&fakeProvider{}, 10, false)
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
	m := New(fp, 10, false)
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
	m := New(fp, 10, false)
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
	m := New(&fakeProvider{}, 10, false)
	_ = m.View() // loading
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	_ = m.View() // list
	m, _ = m.update(key("?"))
	_ = m.View() // help
	m, _ = m.update(errMsg{errors.New("x")})
	_ = m.View() // error
}

func TestEmptyInboxSelectedNil(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
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
			m := New(&fakeProvider{}, 10, false)
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

	m := New(&fakeProvider{}, 10, false)
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
