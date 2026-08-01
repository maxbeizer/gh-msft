package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/maxbeizer/gh-msft/internal/workiq"
)

type fakeGraph struct {
	byURL    map[string]string // substring match -> data
	fetchErr error
	calls    []string
}

func (f *fakeGraph) Fetch(ctx context.Context, urls ...string) ([]workiq.FetchResult, error) {
	f.calls = append(f.calls, urls...)
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	data := `{"value":[]}`
	for sub, d := range f.byURL {
		if strings.Contains(urls[0], sub) {
			data = d
			break
		}
	}
	return []workiq.FetchResult{{Data: json.RawMessage(data), StatusCode: 200}}, nil
}

// sampleEvents is a trimmed capture of a real /me/calendarView response.
const sampleEvents = `{
  "value": [
    {"id":"E1","subject":"Nifty Stand Up",
     "start":{"dateTime":"2026-07-20T16:30:00.0000000","timeZone":"UTC"},
     "end":{"dateTime":"2026-07-20T17:00:00.0000000","timeZone":"UTC"},
     "organizer":{"emailAddress":{"name":"Ada Lovelace","address":"ada@example.com"}}},
    {"id":"E2","subject":"Max:Meni 1:1",
     "start":{"dateTime":"2026-07-28T17:30:00.0000000","timeZone":"UTC"},
     "end":{"dateTime":"2026-07-28T18:00:00.0000000","timeZone":"UTC"},
     "organizer":{"emailAddress":{"name":"Meni Zalzman","address":"MeniZalzman@github.com"}}}
  ]
}`

func fixedNow() time.Time {
	return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
}

func TestUpcomingParsesCalendarView(t *testing.T) {
	fg := &fakeGraph{byURL: map[string]string{"calendarView": sampleEvents}}
	p := NewWorkIQProvider(fg)
	p.now = fixedNow

	events, err := p.Upcoming(context.Background(), 10)
	if err != nil {
		t.Fatalf("Upcoming: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].ID != "E1" || events[0].Subject != "Nifty Stand Up" {
		t.Errorf("event0 = %+v", events[0])
	}
	if events[0].Organizer != "Ada Lovelace" {
		t.Errorf("organizer = %q", events[0].Organizer)
	}
	if events[0].Start.IsZero() || events[0].End.IsZero() {
		t.Errorf("start/end not parsed: %+v", events[0])
	}
	if got := events[0].Start.UTC().Format(time.RFC3339); got != "2026-07-20T16:30:00Z" {
		t.Errorf("start = %s", got)
	}
	// It should have used calendarView with a time window.
	if len(fg.calls) == 0 || !strings.Contains(fg.calls[0], "calendarView") {
		t.Errorf("expected calendarView call, got %v", fg.calls)
	}
	if !strings.Contains(fg.calls[0], "startDateTime=2026-07-17") {
		t.Errorf("window start missing: %s", fg.calls[0])
	}
}

func TestUpcomingFallsBackToEvents(t *testing.T) {
	// First call (calendarView) errors; provider should retry /me/events.
	fg := &errThenOK{
		errURLSub: "calendarView",
		okData:    sampleEvents,
	}
	p := NewWorkIQProvider(fg)
	p.now = fixedNow

	events, err := p.Upcoming(context.Background(), 10)
	if err != nil {
		t.Fatalf("Upcoming: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if !fg.sawEvents {
		t.Errorf("expected fallback to /me/events")
	}
}

func TestUpcomingEmpty(t *testing.T) {
	fg := &fakeGraph{byURL: map[string]string{"calendarView": `{"value":[]}`}}
	p := NewWorkIQProvider(fg)
	p.now = fixedNow
	events, err := p.Upcoming(context.Background(), 10)
	if err != nil {
		t.Fatalf("Upcoming: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("want 0 events, got %d", len(events))
	}
}

func TestGetDetailParsesEventAndLinks(t *testing.T) {
	fg := &fakeGraph{byURL: map[string]string{"/me/events/E1": `{
		"id":"E1",
		"subject":"Planning",
		"start":{"dateTime":"2026-07-20T16:30:00.0000000","timeZone":"UTC"},
		"end":{"dateTime":"2026-07-20T17:00:00.0000000","timeZone":"UTC"},
		"organizer":{"emailAddress":{"name":"Ada Lovelace","address":"ada@example.com"}},
		"attendees":[{"emailAddress":{"name":"Grace Hopper","address":"grace@example.com"}}],
		"location":{"displayName":"Conference room"},
		"body":{"contentType":"HTML","content":"<p>Discuss <strong>roadmap</strong></p>"},
		"bodyPreview":"Discuss roadmap",
		"webLink":"https://outlook.office.com/calendar/item",
		"onlineMeeting":{"joinUrl":"https://teams.microsoft.com/l/meetup-join/abc"},
		"isOnlineMeeting":true
	}`}}
	p := NewWorkIQProvider(fg)

	got, err := p.GetDetail(context.Background(), "E1")
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	if got.Subject != "Planning" || got.Location != "Conference room" || got.Body != "Discuss roadmap" {
		t.Errorf("detail = %+v", got)
	}
	if got.Organizer.Email != "ada@example.com" || len(got.Attendees) != 1 || got.Attendees[0].Email != "grace@example.com" {
		t.Errorf("participants = organizer:%+v attendees:%+v", got.Organizer, got.Attendees)
	}
	if got.JoinURL != "https://teams.microsoft.com/l/meetup-join/abc" || got.WebLink == "" || !got.IsOnlineMeeting {
		t.Errorf("links = %+v", got)
	}
	if len(fg.calls) != 1 {
		t.Fatalf("fetch calls = %v, want one", fg.calls)
	}
	wantSelect := "$select=subject,body,bodyPreview,organizer,attendees,start,end,location,webLink,onlineMeeting,isOnlineMeeting"
	if !strings.Contains(fg.calls[0], wantSelect) {
		t.Errorf("fetch URL %q missing detail fields", fg.calls[0])
	}
	if strings.Contains(fg.calls[0], "onlineMeetingUrl") {
		t.Errorf("fetch URL %q must not use deprecated onlineMeetingUrl", fg.calls[0])
	}
}

func TestGetDetailRequiresID(t *testing.T) {
	p := NewWorkIQProvider(&fakeGraph{})
	if _, err := p.GetDetail(context.Background(), ""); err == nil {
		t.Fatal("expected an error for an empty event id")
	}
}

// errThenOK errors on the calendarView call and succeeds on /me/events.
type errThenOK struct {
	errURLSub string
	okData    string
	sawEvents bool
}

func (f *errThenOK) Fetch(ctx context.Context, urls ...string) ([]workiq.FetchResult, error) {
	if strings.Contains(urls[0], f.errURLSub) {
		return nil, errors.New("calendarView unavailable")
	}
	if strings.Contains(urls[0], "/me/events") {
		f.sawEvents = true
	}
	return []workiq.FetchResult{{Data: json.RawMessage(f.okData), StatusCode: 200}}, nil
}
