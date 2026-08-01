// Package calendar defines a backend-agnostic calendar provider and a WorkIQ-backed
// implementation.
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/maxbeizer/gh-msft/internal/mstime"
	"github.com/maxbeizer/gh-msft/internal/plaintext"
	"github.com/maxbeizer/gh-msft/internal/workiq"
)

// Event is a simplified calendar event.
type Event struct {
	ID        string      `json:"id"`
	Subject   string      `json:"subject"`
	Start     mstime.Time `json:"start"`
	End       mstime.Time `json:"end"`
	IsAllDay  bool        `json:"isAllDay"`
	Organizer string      `json:"organizer"`
}

// Participant identifies an event organizer or attendee.
type Participant struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Detail contains the full event data rendered by the interactive TUI.
type Detail struct {
	ID              string        `json:"id"`
	Subject         string        `json:"subject"`
	Start           mstime.Time   `json:"start"`
	End             mstime.Time   `json:"end"`
	IsAllDay        bool          `json:"isAllDay"`
	Organizer       Participant   `json:"organizer"`
	Attendees       []Participant `json:"attendees"`
	Location        string        `json:"location"`
	Body            string        `json:"body"`
	BodyPreview     string        `json:"bodyPreview"`
	WebLink         string        `json:"webLink"`
	JoinURL         string        `json:"joinUrl"`
	IsOnlineMeeting bool          `json:"isOnlineMeeting"`
}

// Provider reads calendar data.
type Provider interface {
	// Upcoming returns up to top events starting from now, ordered by start time.
	Upcoming(ctx context.Context, top int) ([]Event, error)
	// GetDetail returns the full event data by id.
	GetDetail(ctx context.Context, id string) (Detail, error)
}

// graphClient is the subset of the WorkIQ client this package needs.
type graphClient interface {
	Fetch(ctx context.Context, entityURLs ...string) ([]workiq.FetchResult, error)
}

// WorkIQProvider implements Provider using the WorkIQ Graph proxy.
type WorkIQProvider struct {
	c   graphClient
	now func() time.Time
}

// NewWorkIQProvider builds a provider over the given WorkIQ client.
func NewWorkIQProvider(c graphClient) *WorkIQProvider {
	return &WorkIQProvider{c: c, now: time.Now}
}

type graphDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

type graphEvent struct {
	ID        string        `json:"id"`
	Subject   string        `json:"subject"`
	Start     graphDateTime `json:"start"`
	End       graphDateTime `json:"end"`
	IsAllDay  bool          `json:"isAllDay"`
	Organizer struct {
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"organizer"`
	Attendees []struct {
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"attendees"`
	Body struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
	BodyPreview string `json:"bodyPreview"`
	Location    struct {
		DisplayName string `json:"displayName"`
	} `json:"location"`
	WebLink         string `json:"webLink"`
	IsOnlineMeeting bool   `json:"isOnlineMeeting"`
	OnlineMeeting   struct {
		JoinURL string `json:"joinUrl"`
	} `json:"onlineMeeting"`
}

type graphEventCollection struct {
	Value []graphEvent `json:"value"`
}

func (p *WorkIQProvider) Upcoming(ctx context.Context, top int) ([]Event, error) {
	if top <= 0 {
		top = 25
	}
	now := p.now().UTC()
	end := now.Add(14 * 24 * time.Hour)
	// calendarView expands recurring instances and supports a time window ordered
	// by start; it is the right endpoint for an "upcoming" view.
	url := fmt.Sprintf(
		"/me/calendarView?startDateTime=%s&endDateTime=%s&$select=subject,start,end,isAllDay,organizer&$orderby=start/dateTime&$top=%d",
		now.Format(time.RFC3339), end.Format(time.RFC3339), top,
	)
	results, err := p.c.Fetch(ctx, url)
	if err != nil {
		// Fall back to /me/events if calendarView is unavailable.
		return p.upcomingViaEvents(ctx, top)
	}
	events, perr := parseEvents(results)
	if perr != nil {
		return nil, perr
	}
	return events, nil
}

func (p *WorkIQProvider) upcomingViaEvents(ctx context.Context, top int) ([]Event, error) {
	url := fmt.Sprintf("/me/events?$select=subject,start,end,isAllDay,organizer&$orderby=start/dateTime&$top=%d", top)
	results, err := p.c.Fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	return parseEvents(results)
}

// GetDetail retrieves one event with its body, attendees, and browser links.
func (p *WorkIQProvider) GetDetail(ctx context.Context, id string) (Detail, error) {
	if id == "" {
		return Detail{}, fmt.Errorf("calendar: GetDetail requires an event id")
	}
	url := fmt.Sprintf(
		"/me/events/%s?$select=subject,body,bodyPreview,organizer,attendees,start,end,location,webLink,onlineMeeting,isOnlineMeeting",
		url.PathEscape(id),
	)
	results, err := p.c.Fetch(ctx, url)
	if err != nil {
		return Detail{}, err
	}
	if len(results) == 0 {
		return Detail{}, fmt.Errorf("calendar: event %q was not found", id)
	}
	var event graphEvent
	if err := json.Unmarshal(results[0].Data, &event); err != nil {
		return Detail{}, fmt.Errorf("calendar: decode event: %w", err)
	}
	return detailFromGraph(event), nil
}

func parseEvents(results []workiq.FetchResult) ([]Event, error) {
	if len(results) == 0 {
		return nil, nil
	}
	var coll graphEventCollection
	if err := json.Unmarshal(results[0].Data, &coll); err != nil {
		return nil, fmt.Errorf("calendar: decode events: %w", err)
	}
	events := make([]Event, 0, len(coll.Value))
	for _, ge := range coll.Value {
		events = append(events, eventFromGraph(ge))
	}
	return events, nil
}

func eventFromGraph(event graphEvent) Event {
	organizer := event.Organizer.EmailAddress.Name
	if organizer == "" {
		organizer = event.Organizer.EmailAddress.Address
	}
	return Event{
		ID:        event.ID,
		Subject:   event.Subject,
		Start:     mstime.Parse(event.Start.DateTime),
		End:       mstime.Parse(event.End.DateTime),
		IsAllDay:  event.IsAllDay,
		Organizer: organizer,
	}
}

func detailFromGraph(event graphEvent) Detail {
	attendees := make([]Participant, 0, len(event.Attendees))
	for _, attendee := range event.Attendees {
		attendees = append(attendees, Participant{
			Name:  attendee.EmailAddress.Name,
			Email: attendee.EmailAddress.Address,
		})
	}
	body := event.Body.Content
	if strings.EqualFold(event.Body.ContentType, "html") {
		body = plaintext.HTMLToText(body)
	}
	return Detail{
		ID:              event.ID,
		Subject:         event.Subject,
		Start:           mstime.Parse(event.Start.DateTime),
		End:             mstime.Parse(event.End.DateTime),
		IsAllDay:        event.IsAllDay,
		Organizer:       Participant{Name: event.Organizer.EmailAddress.Name, Email: event.Organizer.EmailAddress.Address},
		Attendees:       attendees,
		Location:        event.Location.DisplayName,
		Body:            body,
		BodyPreview:     event.BodyPreview,
		WebLink:         event.WebLink,
		JoinURL:         event.OnlineMeeting.JoinURL,
		IsOnlineMeeting: event.IsOnlineMeeting,
	}
}
