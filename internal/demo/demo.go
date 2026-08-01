// Package demo provides deterministic, synthetic data for product demonstrations.
package demo

import (
	"context"

	"github.com/maxbeizer/gh-msft/internal/calendar"
	"github.com/maxbeizer/gh-msft/internal/mail"
	"github.com/maxbeizer/gh-msft/internal/mstime"
)

// MailProvider serves static, non-production messages for gh-msft demo.
type MailProvider struct{}

// ListInbox returns the synthetic inbox in newest-first order.
func (MailProvider) ListInbox(context.Context, int, bool) ([]mail.Message, error) {
	return []mail.Message{
		{
			ID:       "demo-project-update",
			Subject:  "Project update",
			From:     mail.Address{Name: "Avery Chen", Email: "avery@example.test"},
			Received: mstime.Parse("2026-08-01T10:42:00Z"),
		},
		{
			ID:       "demo-newsletter",
			Subject:  "August engineering news",
			From:     mail.Address{Name: "Contoso Newsletter", Email: "newsletter@example.test"},
			Received: mstime.Parse("2026-08-01T09:15:00Z"),
			IsRead:   true,
		},
		{
			ID:       "demo-lunch",
			Subject:  "Lunch next week?",
			From:     mail.Address{Name: "Morgan Lee", Email: "morgan@example.test"},
			Received: mstime.Parse("2026-07-31T16:08:00Z"),
			IsRead:   true,
		},
	}, nil
}

// GetDetail returns a synthetic message detail.
func (MailProvider) GetDetail(_ context.Context, id string) (mail.Detail, error) {
	message := demoMessage(id)
	return mail.NewDetail(message, "A short, fictional message body for the gh-msft demo."), nil
}

// Archive accepts the synthetic archive action without mutating external state.
func (MailProvider) Archive(context.Context, string) error {
	return nil
}

// Body returns a synthetic plain-text message body.
func (MailProvider) Body(context.Context, string) (string, error) {
	return "A short, fictional message body for the gh-msft demo.", nil
}

func demoMessage(id string) mail.Message {
	messages, _ := MailProvider{}.ListInbox(context.Background(), 0, false)
	for _, message := range messages {
		if message.ID == id {
			return message
		}
	}
	return mail.Message{ID: id, Subject: "Demo message"}
}

// CalendarProvider serves static, non-production calendar events for gh-msft demo.
type CalendarProvider struct{}

// Upcoming returns the synthetic calendar in chronological order.
func (CalendarProvider) Upcoming(context.Context, int) ([]calendar.Event, error) {
	return []calendar.Event{
		{
			ID:        "demo-standup",
			Subject:   "Team standup",
			Start:     mstime.Parse("2026-08-03T14:00:00Z"),
			End:       mstime.Parse("2026-08-03T14:15:00Z"),
			Organizer: "Avery Chen",
		},
		{
			ID:        "demo-planning",
			Subject:   "Planning session",
			Start:     mstime.Parse("2026-08-03T16:00:00Z"),
			End:       mstime.Parse("2026-08-03T17:00:00Z"),
			Organizer: "Morgan Lee",
		},
	}, nil
}

// GetDetail returns a synthetic calendar event detail.
func (CalendarProvider) GetDetail(_ context.Context, id string) (calendar.Detail, error) {
	events, _ := CalendarProvider{}.Upcoming(context.Background(), 0)
	for _, event := range events {
		if event.ID == id {
			return calendar.Detail{
				ID:        event.ID,
				Subject:   event.Subject,
				Start:     event.Start,
				End:       event.End,
				Organizer: calendar.Participant{Name: event.Organizer, Email: "avery@example.test"},
				Location:  "Conference room",
				Body:      "A fictional event used only by the gh-msft demo.",
				JoinURL:   "https://example.test/meeting",
				WebLink:   "https://example.test/calendar",
			}, nil
		}
	}
	return calendar.Detail{ID: id, Subject: "Demo event"}, nil
}
