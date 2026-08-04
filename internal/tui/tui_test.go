package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/maxbeizer/gh-msft/internal/calendar"
	"github.com/maxbeizer/gh-msft/internal/mail"
	"github.com/maxbeizer/gh-msft/internal/mstime"
	"github.com/muesli/termenv"
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
	events    []calendar.Event
	err       error
	detail    calendar.Detail
	detailErr error
	detailIDs []string
}

func (f *fakeCal) Upcoming(ctx context.Context, top int) ([]calendar.Event, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events, nil
}

func (f *fakeCal) GetDetail(ctx context.Context, id string) (calendar.Detail, error) {
	f.detailIDs = append(f.detailIDs, id)
	if f.detailErr != nil {
		return calendar.Detail{}, f.detailErr
	}
	return f.detail, nil
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
		{ID: "1", Subject: "First", From: mail.Address{Name: "Alice"}, Received: mstime.Parse("2026-01-02T15:00:00Z"), IsRead: false},
		{ID: "2", Subject: "Second", From: mail.Address{Name: "Bob"}, Received: mstime.Parse("2026-01-02T16:00:00Z"), IsRead: true},
		{ID: "3", Subject: "Third", From: mail.Address{Name: "Carol"}, Received: mstime.Parse("2026-01-02T17:00:00Z"), IsRead: false},
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

func TestNavigationSupportsArrowKeys(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})

	m, _ = m.update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("down arrow: cursor = %d, want 1", m.cursor)
	}
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyHome})
	if m.cursor != 0 {
		t.Errorf("home: cursor = %d, want 0", m.cursor)
	}
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.cursor != 2 {
		t.Errorf("end: cursor = %d, want 2", m.cursor)
	}
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 1 {
		t.Errorf("up arrow: cursor = %d, want 1", m.cursor)
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
	if !strings.Contains(out, eventTimeLabel(sampleEvents()[0], false)) {
		t.Errorf("calendar view should show the event time range; got:\n%s", out)
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

func TestEnterOpensCalendarEventAndBrowserActions(t *testing.T) {
	fc := &fakeCal{
		events: sampleEvents(),
		detail: calendar.Detail{
			Subject:   "Standup",
			Start:     mstime.Parse("2026-01-02T15:00:00Z"),
			End:       mstime.Parse("2026-01-02T15:15:00Z"),
			Organizer: calendar.Participant{Name: "Alice", Email: "alice@example.com"},
			Attendees: []calendar.Participant{{Name: "Bob", Email: "bob@example.com"}},
			Location:  "Room 1",
			Body:      "Agenda",
			JoinURL:   "https://teams.microsoft.com/l/meetup-join/standup",
			WebLink:   "https://outlook.office.com/calendar/standup",
		},
	}
	var opened []string
	m := New(&fakeProvider{}, 10, false)
	m.cal = fc
	m.openURL = func(rawURL string) error {
		opened = append(opened, rawURL)
		return nil
	}
	m.mode = calendarMode
	m, _ = m.update(eventsLoadedMsg{sampleEvents()})

	m, cmd := m.update(key("enter"))
	if !m.viewing || !m.viewingEvent || !m.eventLoading {
		t.Fatal("enter should open a loading calendar detail view")
	}
	if cmd == nil {
		t.Fatal("enter should fetch event detail")
	}
	msg := cmd()
	detailMsg, ok := msg.(eventDetailLoadedMsg)
	if !ok {
		t.Fatalf("expected eventDetailLoadedMsg, got %T", msg)
	}
	if detailMsg.request != m.eventRequest {
		t.Errorf("detail request = %d, want %d", detailMsg.request, m.eventRequest)
	}
	m, _ = m.update(detailMsg)
	if m.eventLoading || len(fc.detailIDs) != 1 || fc.detailIDs[0] != "e1" {
		t.Errorf("detail loading state = %v, ids = %v", m.eventLoading, fc.detailIDs)
	}

	for _, want := range []string{"Organizer: Alice <alice@example.com>", "Attendees: Bob <bob@example.com>", "Location: Room 1", "Agenda", "Meeting link: available"} {
		if !strings.Contains(m.View(), want) {
			t.Errorf("calendar detail missing %q; got:\n%s", want, m.View())
		}
	}

	m, cmd = m.update(key("j"))
	if cmd == nil {
		t.Fatal("j should open the online-meeting join link")
	}
	m, _ = m.update(cmd())
	if len(opened) != 1 || opened[0] != fc.detail.JoinURL {
		t.Errorf("opened URLs = %v", opened)
	}
	if !strings.Contains(m.View(), "Opened meeting link in your browser.") {
		t.Errorf("success status missing; got:\n%s", m.View())
	}

	m, cmd = m.update(key("o"))
	if cmd == nil {
		t.Fatal("o should open the Outlook web link")
	}
	m, _ = m.update(cmd())
	if len(opened) != 2 || opened[1] != fc.detail.WebLink {
		t.Errorf("opened URLs = %v", opened)
	}
}

func TestStaleEventDetailResultsAreIgnored(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m.cal = &fakeCal{events: sampleEvents()}
	m.mode = calendarMode
	m, _ = m.update(eventsLoadedMsg{sampleEvents()})

	m, _ = m.update(key("enter"))
	firstRequest := m.eventRequest
	m, _ = m.update(key("esc"))
	m.cursor = 1
	m, _ = m.update(key("enter"))
	secondRequest := m.eventRequest
	if secondRequest == firstRequest {
		t.Fatal("each event detail request must have a new request id")
	}

	m, _ = m.update(eventDetailLoadedMsg{
		request: firstRequest,
		detail:  calendar.Detail{Subject: "Stale event"},
	})
	m, _ = m.update(eventDetailErrMsg{request: firstRequest, err: errors.New("stale failure")})
	if !m.viewing || !m.eventLoading || m.eventDetail.Subject != "" || m.err != nil {
		t.Fatalf("stale detail changed active view: %+v", m)
	}

	m, _ = m.update(eventDetailLoadedMsg{
		request: secondRequest,
		detail:  calendar.Detail{Subject: "Current event"},
	})
	if m.eventLoading || m.eventDetail.Subject != "Current event" {
		t.Errorf("current detail = %+v, loading = %v", m.eventDetail, m.eventLoading)
	}
}

func TestEventDetailCommandScopesErrorsToRequest(t *testing.T) {
	wantErr := errors.New("event unavailable")
	msg := eventDetailCmd(&fakeCal{detailErr: wantErr}, "e1", 7)()
	got, ok := msg.(eventDetailErrMsg)
	if !ok {
		t.Fatalf("expected eventDetailErrMsg, got %T", msg)
	}
	if got.request != 7 || !errors.Is(got.err, wantErr) {
		t.Errorf("event detail error = %+v, want request 7 with %v", got, wantErr)
	}
}

func TestCalendarDetailMissingAndFailedLinksSurfaceStatus(t *testing.T) {
	tests := []struct {
		name       string
		detail     calendar.Detail
		openURL    func(string) error
		key        string
		wantStatus string
		wantCmd    bool
	}{
		{
			name:       "missing meeting link",
			detail:     calendar.Detail{Subject: "Offline"},
			key:        "j",
			wantStatus: "This event has no meeting link.",
		},
		{
			name:   "browser launch failure",
			detail: calendar.Detail{Subject: "Planning", JoinURL: "https://teams.microsoft.com/l/meetup-join/abc"},
			openURL: func(string) error {
				return errors.New("browser unavailable")
			},
			key:        "j",
			wantStatus: "Could not open meeting link: browser unavailable",
			wantCmd:    true,
		},
		{
			name:       "missing Outlook link",
			detail:     calendar.Detail{Subject: "Planning"},
			key:        "o",
			wantStatus: "This event has no Outlook event.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(&fakeProvider{}, 10, false)
			m.loading = false
			m.viewing = true
			m.viewingEvent = true
			m.eventDetail = tt.detail
			if tt.openURL != nil {
				m.openURL = tt.openURL
			}

			m, cmd := m.update(key(tt.key))
			if (cmd != nil) != tt.wantCmd {
				t.Fatalf("command = %v, want command %v", cmd != nil, tt.wantCmd)
			}
			if cmd != nil {
				m, _ = m.update(cmd())
			}
			if m.status != tt.wantStatus {
				t.Errorf("status = %q, want %q", m.status, tt.wantStatus)
			}
			if !strings.Contains(m.View(), tt.wantStatus) {
				t.Errorf("detail view missing status %q; got:\n%s", tt.wantStatus, m.View())
			}
		})
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

func TestDetailViewSupportsVimAndArrowScrolling(t *testing.T) {
	fp := &fakeProvider{body: strings.Repeat("line\n", 30)}
	m := New(fp, 10, false)
	m, _ = m.update(tea.WindowSizeMsg{Width: 80, Height: 8})
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	m, cmd := m.update(key("enter"))
	m, _ = m.update(cmd())

	m, _ = m.update(key("j"))
	if m.detailOffset != 1 {
		t.Errorf("j: detailOffset = %d, want 1", m.detailOffset)
	}
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyDown})
	if m.detailOffset != 2 {
		t.Errorf("down arrow: detailOffset = %d, want 2", m.detailOffset)
	}
	m, _ = m.update(key("G"))
	if m.detailOffset != m.maxDetailOffset() {
		t.Errorf("G: detailOffset = %d, want %d", m.detailOffset, m.maxDetailOffset())
	}
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyHome})
	if m.detailOffset != 0 {
		t.Errorf("home: detailOffset = %d, want 0", m.detailOffset)
	}
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyEnd})
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyUp})
	if m.detailOffset != m.maxDetailOffset()-1 {
		t.Errorf("up arrow: detailOffset = %d, want %d", m.detailOffset, m.maxDetailOffset()-1)
	}
}

func TestDetailViewQClosesWithoutQuitting(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	m, _ = m.update(key("enter"))
	m, _ = m.update(key("q"))
	if m.viewing {
		t.Error("q should close the detail view")
	}
	if m.quitting {
		t.Error("q should not quit from the detail view")
	}
}

func TestRefreshClearsError(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m, _ = m.update(errMsg{errors.New("boom")})
	m, cmd := m.update(key("R"))
	if m.err != nil {
		t.Error("R should clear an error while retrying")
	}
	if !m.loading {
		t.Error("R should mark the model as loading")
	}
	if cmd == nil {
		t.Error("R should return a reload command")
	}
}

func TestContextualHelpMatchesMode(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	m, _ = m.update(key("?"))
	if !strings.Contains(m.View(), "a archive") {
		t.Error("mail help should include archive")
	}

	m.mode = calendarMode
	if strings.Contains(m.footer(), "a archive") {
		t.Error("calendar help should not include mail actions")
	}
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.showHelp {
		t.Error("esc should dismiss expanded help")
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
		{"unset falls back", 0, 31},
		{"narrow clamps to min", 20, 8},
		{"wide grows", 120, 67},
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

func TestRenderedStatesUseDesignSystem(t *testing.T) {
	tests := []struct {
		name  string
		model Model
		want  []string
	}{
		{
			name:  "mail list",
			model: Model{messages: sampleMessages(), mailLoaded: true, width: 100},
			want:  []string{"gh msft", "[Mail]", "Inbox · 3 messages · Inbox", "NEW", "> "},
		},
		{
			name:  "calendar list",
			model: Model{mode: calendarMode, events: sampleEvents(), calLoaded: true, width: 100},
			want:  []string{"gh msft", "[Calendar]", "Upcoming · 2 events", "Standup", "Help:"},
		},
		{
			name:  "loading",
			model: Model{loading: true, width: 100},
			want:  []string{"gh msft", "Loading inbox…"},
		},
		{
			name:  "error",
			model: Model{err: errors.New("connection refused"), width: 100},
			want:  []string{"gh msft", "Error", "connection refused", "Help: R retry · q quit"},
		},
		{
			name:  "message detail",
			model: Model{messages: sampleMessages(), mailLoaded: true, viewing: true, body: "Message body", width: 100},
			want:  []string{"gh msft", "Message", "First", "From: Alice", "Message body", "Help:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := tt.model.View()
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("View() missing %q; got:\n%s", want, out)
				}
			}
		})
	}
}

func TestNarrowViewRetainsTextualStateIndicators(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	m, _ = m.update(tea.WindowSizeMsg{Width: 30, Height: 24})

	out := m.View()
	for _, want := range []string{"gh msft", "NEW", "> ", "Help:"} {
		if !strings.Contains(out, want) {
			t.Errorf("narrow View() missing %q; got:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if got := lipgloss.Width(line); got > 30 {
			t.Errorf("narrow line width = %d, want <= 30: %q", got, line)
		}
	}
}

func TestMailViewShowsScanColumnsAndScope(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()[:1]})
	m, _ = m.update(tea.WindowSizeMsg{Width: 100, Height: 24})

	out := m.View()
	for _, want := range []string{"Inbox · 1 message · Inbox", "NEW", "Alice", "First", receivedDateTime(m.messages[0])} {
		if !strings.Contains(out, want) {
			t.Errorf("mail view missing %q:\n%s", want, out)
		}
	}

	m.all = true
	out = m.View()
	if !strings.Contains(out, "All mail") {
		t.Errorf("all-mail view should identify its scope:\n%s", out)
	}
}

func TestCalendarViewGroupsDaysAndShowsEventKinds(t *testing.T) {
	events := []calendar.Event{
		{ID: "same-day", Subject: "Planning", Organizer: "Alice", Start: mstime.Parse("2026-01-02T09:00:00Z"), End: mstime.Parse("2026-01-02T10:00:00Z")},
		{ID: "all-day", Subject: "Conference", IsAllDay: true, Start: mstime.Parse("2026-01-02T00:00:00Z"), End: mstime.Parse("2026-01-03T00:00:00Z")},
		{ID: "multi-day", Subject: "Offsite", Start: mstime.Parse("2026-01-03T09:00:00Z"), End: mstime.Parse("2026-01-05T17:00:00Z")},
	}
	m := New(&fakeProvider{}, 10, false)
	m.mode = calendarMode
	m, _ = m.update(eventsLoadedMsg{events})
	m, _ = m.update(tea.WindowSizeMsg{Width: 100, Height: 24})

	out := m.View()
	if got := strings.Count(out, calendarDayLabel(events[0])); got != 1 {
		t.Errorf("calendar should have one section per day, got %d:\n%s", got, out)
	}
	for _, want := range []string{"all day", eventTimeLabel(events[0], false), truncate(eventTimeLabel(events[2], false), 20)} {
		if !strings.Contains(out, want) {
			t.Errorf("calendar view missing %q:\n%s", want, out)
		}
	}
}

func TestNarrowListViewsFitTerminal(t *testing.T) {
	tests := []struct {
		name  string
		model Model
	}{
		{
			name: "mail",
			model: func() Model {
				m := New(&fakeProvider{}, 10, false)
				m, _ = m.update(messagesLoadedMsg{sampleMessages()[:1]})
				return m
			}(),
		},
		{
			name: "calendar",
			model: func() Model {
				m := New(&fakeProvider{}, 10, false)
				m.mode = calendarMode
				m, _ = m.update(eventsLoadedMsg{sampleEvents()[:1]})
				return m
			}(),
		},
	}
	for _, tt := range tests {
		for _, width := range []int{20, 32} {
			t.Run(tt.name+"/"+fmt.Sprint(width), func(t *testing.T) {
				m, _ := tt.model.update(tea.WindowSizeMsg{Width: width, Height: 24})
				for _, line := range strings.Split(strings.TrimSuffix(m.View(), "\n"), "\n") {
					if got := lipgloss.Width(line); got > width {
						t.Errorf("line width = %d, want <= %d: %q", got, width, line)
					}
				}
			})
		}
	}
}

func TestPanelsUseAvailableTerminalWidth(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m.mode = calendarMode
	m, _ = m.update(eventsLoadedMsg{sampleEvents()})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 24})

	maxWidth := 0
	for _, line := range strings.Split(strings.TrimSuffix(m.View(), "\n"), "\n") {
		maxWidth = maxInt(maxWidth, lipgloss.Width(line))
	}
	wantWidth := m.width - 2 // The screen keeps one column of horizontal margin on each side.
	if maxWidth != wantWidth {
		t.Errorf("widest rendered line = %d, want available width %d", maxWidth, wantWidth)
	}
}

func TestListsStayWithinTerminalHeightAndKeepSelectionVisible(t *testing.T) {
	events := make([]calendar.Event, 12)
	for i := range events {
		events[i] = calendar.Event{
			ID:      fmt.Sprintf("event-%d", i),
			Subject: fmt.Sprintf("Event %d", i),
			Start:   mstime.Parse(fmt.Sprintf("2026-01-%02dT09:00:00Z", i+1)),
			End:     mstime.Parse(fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)),
		}
	}
	m := New(&fakeProvider{}, len(events), false)
	m.mode = calendarMode
	m, _ = m.update(eventsLoadedMsg{events})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 12})
	m.cursor = len(events) - 1

	out := m.View()
	if strings.Contains(out, "Event 0") {
		t.Errorf("calendar viewport should not render off-screen events:\n%s", out)
	}
	if !strings.Contains(out, "Event 11") {
		t.Errorf("calendar viewport should keep the selected event visible:\n%s", out)
	}
	if lines := len(strings.Split(strings.TrimSuffix(out, "\n"), "\n")); lines > m.height {
		t.Errorf("calendar View() rendered %d lines, want <= terminal height %d:\n%s", lines, m.height, out)
	}
}

func TestMailListViewportKeepsSelectionVisible(t *testing.T) {
	messages := make([]mail.Message, 12)
	for i := range messages {
		messages[i] = mail.Message{
			ID:       fmt.Sprintf("message-%d", i),
			Subject:  fmt.Sprintf("Message %d", i),
			From:     mail.Address{Name: "Sender"},
			Received: mstime.Parse(fmt.Sprintf("2026-01-%02dT09:00:00Z", i+1)),
		}
	}
	m := New(&fakeProvider{}, len(messages), false)
	m, _ = m.update(messagesLoadedMsg{messages})
	m, _ = m.update(tea.WindowSizeMsg{Width: 120, Height: 12})
	m.cursor = len(messages) - 1

	out := m.View()
	if strings.Contains(out, "Message 0") {
		t.Errorf("mail viewport should not render off-screen messages:\n%s", out)
	}
	if !strings.Contains(out, "Message 11") {
		t.Errorf("mail viewport should keep the selected message visible:\n%s", out)
	}
}

func TestWideCalendarRowsKeepUsefulMetadata(t *testing.T) {
	m := Model{width: 160}
	event := calendar.Event{
		Subject:   "Planning the next iteration without hiding the event title",
		Organizer: "maxbeizer@github.com",
		Start:     mstime.Parse("2026-01-02T09:00:00Z"),
		End:       mstime.Parse("2026-01-02T10:00:00Z"),
	}

	out := m.calendarRow(0, event)
	for _, want := range []string{event.Subject, event.Organizer} {
		if !strings.Contains(out, want) {
			t.Errorf("wide calendar row missing %q:\n%s", want, out)
		}
	}
}

func TestExpandedHelpStaysWithinTerminalHeight(t *testing.T) {
	m := New(&fakeProvider{}, 10, false)
	m, _ = m.update(messagesLoadedMsg{sampleMessages()})
	m, _ = m.update(tea.WindowSizeMsg{Width: 80, Height: 12})
	m.showHelp = true

	out := m.View()
	if lines := len(strings.Split(strings.TrimSuffix(out, "\n"), "\n")); lines > m.height {
		t.Errorf("expanded help rendered %d lines, want <= terminal height %d:\n%s", lines, m.height, out)
	}
}

func TestTruncateRespectsDisplayWidth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{name: "wide rune", input: "界界", width: 3, want: "界…"},
		{name: "single column", input: "abc", width: 1, want: "…"},
		{name: "unchanged", input: "hello", width: 5, want: "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.input, tt.width); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
			}
		})
	}
}

func TestNoColorFallbackRemainsReadable(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })

	m := Model{messages: sampleMessages(), mailLoaded: true, width: 100}
	out := m.View()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("ASCII color profile should not emit ANSI colors; got:\n%s", out)
	}
	for _, want := range []string{"[Mail]", "NEW", "> ", "Help:"} {
		if !strings.Contains(out, want) {
			t.Errorf("no-color View() missing %q; got:\n%s", want, out)
		}
	}
}

func TestLoadingViewIdentifiesActiveScope(t *testing.T) {
	m := New(&fakeProvider{}, 10, true)
	m, _ = m.update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if out := m.View(); !strings.Contains(out, "All mail · Loading…") {
		t.Errorf("mail loading view missing scope and state:\n%s", out)
	}

	m.mode = calendarMode
	if out := m.View(); !strings.Contains(out, "Upcoming · Loading…") {
		t.Errorf("calendar loading view missing scope and state:\n%s", out)
	}
}
