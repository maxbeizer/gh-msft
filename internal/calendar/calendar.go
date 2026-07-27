// Package calendar defines a backend-agnostic calendar provider and a WorkIQ-backed
// implementation.
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/maxbeizer/gh-msft/internal/mstime"
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

// Provider reads calendar data.
type Provider interface {
	// Upcoming returns up to top events starting from now, ordered by start time.
	Upcoming(ctx context.Context, top int) ([]Event, error)
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
		organizer := ge.Organizer.EmailAddress.Name
		if organizer == "" {
			organizer = ge.Organizer.EmailAddress.Address
		}
		events = append(events, Event{
			ID:        ge.ID,
			Subject:   ge.Subject,
			Start:     mstime.Parse(ge.Start.DateTime),
			End:       mstime.Parse(ge.End.DateTime),
			IsAllDay:  ge.IsAllDay,
			Organizer: organizer,
		})
	}
	return events, nil
}
