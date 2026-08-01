package demo

import (
	"context"
	"testing"
)

func TestMailProviderFixtures(t *testing.T) {
	messages, err := (MailProvider{}).ListInbox(context.Background(), 5, false)
	if err != nil {
		t.Fatalf("ListInbox() error = %v", err)
	}
	if len(messages) != 5 {
		t.Fatalf("ListInbox() returned %d messages, want 5", len(messages))
	}
	if messages[0].ID != "demo-project-update" {
		t.Errorf("first message = %q, want %q", messages[0].ID, "demo-project-update")
	}

	detail, err := (MailProvider{}).GetDetail(context.Background(), "demo-duck")
	if err != nil {
		t.Fatalf("GetDetail() error = %v", err)
	}
	if detail.Body == "" {
		t.Error("GetDetail() returned an empty fixture body")
	}
}

func TestCalendarProviderFixtures(t *testing.T) {
	events, err := (CalendarProvider{}).Upcoming(context.Background(), 4)
	if err != nil {
		t.Fatalf("Upcoming() error = %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("Upcoming() returned %d events, want 4", len(events))
	}
	if events[0].ID != "demo-standup" {
		t.Errorf("first event = %q, want %q", events[0].ID, "demo-standup")
	}

	detail, err := (CalendarProvider{}).GetDetail(context.Background(), "demo-standup")
	if err != nil {
		t.Fatalf("GetDetail() error = %v", err)
	}
	if len(detail.Attendees) == 0 || detail.Body == "" {
		t.Error("GetDetail() returned an incomplete fixture")
	}
}
