// Package demo provides deterministic, synthetic data for product demonstrations.
package demo

import (
	"context"

	"github.com/maxbeizer/gh-msft/internal/calendar"
	"github.com/maxbeizer/gh-msft/internal/mail"
	"github.com/maxbeizer/gh-msft/internal/mstime"
)

var demoMailbox = mail.Address{Name: "Casey Example", Email: "casey@example.test"}

// MailProvider serves static, non-production messages for gh-msft demo.
type MailProvider struct{}

// ListInbox returns the synthetic inbox in newest-first order.
func (MailProvider) ListInbox(_ context.Context, top int, _ bool) ([]mail.Message, error) {
	messages := []mail.Message{
		demoMail("demo-project-update", "Project update", "Avery Chen", "avery@example.test", "2026-08-01T10:42:00Z", false),
		demoMail("demo-build-green", "The build is green (please don't look directly at it)", "Build Bot", "build-bot@example.test", "2026-08-01T10:17:00Z", false),
		demoMail("demo-plant", "Action required: name the office plant", "Workplace Team", "workplace@example.test", "2026-08-01T09:48:00Z", false),
		demoMail("demo-newsletter", "August engineering news", "Contoso Newsletter", "newsletter@example.test", "2026-08-01T09:15:00Z", false),
		demoMail("demo-duck", "Fwd: the rubber duck has opinions", "Morgan Lee", "morgan@example.test", "2026-08-01T08:52:00Z", false),
		demoMail("demo-lunch", "Lunch next week?", "Jamie Patel", "jamie@example.test", "2026-07-31T16:08:00Z", true),
		demoMail("demo-bake-sale", "Bake sale postmortem: zero regrets", "Community Crew", "community@example.test", "2026-07-31T14:25:00Z", true),
		demoMail("demo-meeting", "Meeting notes: this could have been a message", "Priya Shah", "priya@example.test", "2026-07-31T11:05:00Z", true),
		demoMail("demo-keyboard", "Your keyboard has reached 10,000 steps", "Wellness Bot", "wellness@example.test", "2026-07-30T15:30:00Z", true),
		demoMail("demo-ship-it", "Ship it, but perhaps after lunch", "Release Team", "release@example.test", "2026-07-30T12:15:00Z", true),
		demoMail("demo-snacks", "Snack drawer inventory: looking optimistic", "Office Manager", "office@example.test", "2026-07-29T16:40:00Z", true),
		demoMail("demo-focus", "Focus time is a meeting with future you", "Calendar Concierge", "calendar@example.test", "2026-07-29T09:00:00Z", true),
	}
	if top > 0 && top < len(messages) {
		return messages[:top], nil
	}
	return messages, nil
}

// GetDetail returns a synthetic message detail.
func (MailProvider) GetDetail(_ context.Context, id string) (mail.Detail, error) {
	message := demoMessage(id)
	return mail.NewDetail(message, demoMailBody(id)), nil
}

// Archive accepts the synthetic archive action without mutating external state.
func (MailProvider) Archive(context.Context, string) error {
	return nil
}

// Body returns a synthetic plain-text message body.
func (MailProvider) Body(_ context.Context, id string) (string, error) {
	return demoMailBody(id), nil
}

func demoMail(id, subject, name, email, received string, isRead bool) mail.Message {
	return mail.Message{
		ID:       id,
		Subject:  subject,
		From:     mail.Address{Name: name, Email: email},
		To:       []mail.Address{demoMailbox},
		Received: mstime.Parse(received),
		IsRead:   isRead,
	}
}

func demoMessage(id string) mail.Message {
	messages, _ := MailProvider{}.ListInbox(context.Background(), 0, false)
	for _, message := range messages {
		if message.ID == id {
			return message
		}
	}
	return mail.Message{ID: id, Subject: "Demo message", To: []mail.Address{demoMailbox}}
}

func demoMailBody(id string) string {
	if id == "demo-duck" {
		return "The duck recommends adding a test before changing the code.\n\nThe duck is, regrettably, right."
	}
	if id == "demo-build-green" {
		return "All checks passed.\n\nNo one is sure why, so please enjoy this moment responsibly."
	}
	return "A short, fictional message body for the gh-msft demo.\n\nEverything in this inbox is static, safe to share, and a little more cheerful than a real Monday."
}

// CalendarProvider serves static, non-production calendar events for gh-msft demo.
type CalendarProvider struct{}

// Upcoming returns the synthetic calendar in chronological order.
func (CalendarProvider) Upcoming(_ context.Context, top int) ([]calendar.Event, error) {
	events := []calendar.Event{
		demoEvent("demo-standup", "Daily standup (bring one fact)", "2026-08-03T14:00:00Z", "2026-08-03T14:15:00Z", "Avery Chen"),
		demoEvent("demo-focus", "Focus time: turning coffee into code", "2026-08-03T14:30:00Z", "2026-08-03T15:30:00Z", "Casey Example"),
		demoEvent("demo-triage", "Bug triage: nobody panic (yet)", "2026-08-03T16:00:00Z", "2026-08-03T16:30:00Z", "Morgan Lee"),
		demoEvent("demo-planning", "Planning session", "2026-08-03T17:00:00Z", "2026-08-03T18:00:00Z", "Jamie Patel"),
		demoEvent("demo-meeting-reduction", "Meeting about reducing meetings", "2026-08-04T14:00:00Z", "2026-08-04T14:30:00Z", "Priya Shah"),
		demoEvent("demo-design-review", "Design review: make it pop, tastefully", "2026-08-04T16:00:00Z", "2026-08-04T17:00:00Z", "Avery Chen"),
		demoEvent("demo-lunch-and-learn", "Lunch and learn: keyboard shortcuts", "2026-08-05T16:00:00Z", "2026-08-05T17:00:00Z", "Build Bot"),
		demoEvent("demo-calendar-hold", "Calendar hold: defend this block", "2026-08-06T14:00:00Z", "2026-08-06T15:00:00Z", "Casey Example"),
		demoEvent("demo-retro", "Retro: keep, stop, start, snack", "2026-08-07T15:00:00Z", "2026-08-07T16:00:00Z", "Morgan Lee"),
		{
			ID:        "demo-quiet-day",
			Subject:   "Quiet day (a bold experiment)",
			Start:     mstime.Parse("2026-08-10T00:00:00Z"),
			End:       mstime.Parse("2026-08-11T00:00:00Z"),
			IsAllDay:  true,
			Organizer: "Calendar Concierge",
		},
	}
	if top > 0 && top < len(events) {
		return events[:top], nil
	}
	return events, nil
}

func demoEvent(id, subject, start, end, organizer string) calendar.Event {
	return calendar.Event{
		ID:        id,
		Subject:   subject,
		Start:     mstime.Parse(start),
		End:       mstime.Parse(end),
		Organizer: organizer,
	}
}

// GetDetail returns a synthetic calendar event detail.
func (CalendarProvider) GetDetail(_ context.Context, id string) (calendar.Detail, error) {
	events, _ := CalendarProvider{}.Upcoming(context.Background(), 0)
	for _, event := range events {
		if event.ID == id {
			return calendar.Detail{
				ID:              event.ID,
				Subject:         event.Subject,
				Start:           event.Start,
				End:             event.End,
				IsAllDay:        event.IsAllDay,
				Organizer:       demoParticipant(event.Organizer),
				Attendees:       []calendar.Participant{demoMailboxParticipant(), {Name: "Avery Chen", Email: "avery@example.test"}, {Name: "Morgan Lee", Email: "morgan@example.test"}},
				Location:        demoEventLocation(id),
				Body:            demoEventBody(id),
				BodyPreview:     "A fictional event used only by the gh-msft demo.",
				JoinURL:         "https://example.test/meeting",
				WebLink:         "https://example.test/calendar",
				IsOnlineMeeting: true,
			}, nil
		}
	}
	return calendar.Detail{ID: id, Subject: "Demo event"}, nil
}

func demoParticipant(name string) calendar.Participant {
	return calendar.Participant{Name: name, Email: "calendar@example.test"}
}

func demoMailboxParticipant() calendar.Participant {
	return calendar.Participant{Name: demoMailbox.Name, Email: demoMailbox.Email}
}

func demoEventLocation(id string) string {
	if id == "demo-focus" || id == "demo-calendar-hold" {
		return "Do not disturb"
	}
	return "Conference room (probably)"
}

func demoEventBody(id string) string {
	if id == "demo-standup" {
		return "Bring one fact, one blocker, and one opinion about whether tabs are better than spaces.\n\nNo slides. The rubber duck will take notes."
	}
	if id == "demo-meeting-reduction" {
		return "Objective: determine whether this meeting can be replaced by a message.\n\nIronically, we will need a follow-up meeting to decide."
	}
	return "A fictional event used only by the gh-msft demo.\n\nNo calendar data, meeting links, or attendees come from a real account."
}
