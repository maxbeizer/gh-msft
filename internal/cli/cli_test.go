package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/maxbeizer/gh-msft/internal/calendar"
	"github.com/maxbeizer/gh-msft/internal/mail"
	"github.com/maxbeizer/gh-msft/internal/mstime"
)

type fakeMail struct {
	inbox      []mail.Message
	listErr    error
	archived   []string
	archiveErr error
}

func (f *fakeMail) ListInbox(ctx context.Context, top int) ([]mail.Message, error) {
	return f.inbox, f.listErr
}

func (f *fakeMail) Archive(ctx context.Context, id string) error {
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
	return f.events, f.err
}

func factoryFor(m mail.Provider, c calendar.Provider) Factory {
	return func(ctx context.Context) (*Providers, error) {
		return &Providers{Mail: m, Cal: c, Close: func() {}}, nil
	}
}

func run(t *testing.T, factory Factory, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd(factory)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return buf.String(), err
}

func TestMailListTable(t *testing.T) {
	fm := &fakeMail{inbox: []mail.Message{
		{ID: "1", Subject: "Hello world", From: mail.Address{Name: "Alice"}, IsRead: false, Received: mstime.Parse("2026-07-17T21:36:12Z")},
	}}
	out, err := run(t, factoryFor(fm, &fakeCal{}), "mail", "list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "Hello world") || !strings.Contains(out, "Alice") {
		t.Errorf("output missing content:\n%s", out)
	}
	if !strings.Contains(out, "SUBJECT") {
		t.Errorf("output missing header:\n%s", out)
	}
}

func TestMailListJSON(t *testing.T) {
	fm := &fakeMail{inbox: []mail.Message{
		{ID: "1", Subject: "Hello", From: mail.Address{Name: "Alice", Email: "a@x.com"}, IsRead: true, Received: mstime.Parse("2026-07-17T21:36:12Z")},
	}}
	out, err := run(t, factoryFor(fm, &fakeCal{}), "mail", "list", "--json")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var got []mail.Message
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Subject != "Hello" {
		t.Errorf("unexpected JSON: %+v", got)
	}
}

func TestMailListEmpty(t *testing.T) {
	out, err := run(t, factoryFor(&fakeMail{}, &fakeCal{}), "mail", "list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "No messages") {
		t.Errorf("expected empty message, got:\n%s", out)
	}
}

func TestMailArchiveArgs(t *testing.T) {
	fm := &fakeMail{}
	out, err := run(t, factoryFor(fm, &fakeCal{}), "mail", "archive", "ID1", "ID2")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(fm.archived) != 2 || fm.archived[0] != "ID1" || fm.archived[1] != "ID2" {
		t.Errorf("archived = %v", fm.archived)
	}
	if !strings.Contains(out, "archived ID1") {
		t.Errorf("missing confirmation:\n%s", out)
	}
}

func TestMailArchiveNoIDsErrors(t *testing.T) {
	_, err := run(t, factoryFor(&fakeMail{}, &fakeCal{}), "mail", "archive")
	if err == nil {
		t.Fatal("expected error when no ids provided")
	}
}

func TestMailArchivePropagatesError(t *testing.T) {
	fm := &fakeMail{archiveErr: errors.New("boom")}
	_, err := run(t, factoryFor(fm, &fakeCal{}), "mail", "archive", "X")
	if err == nil {
		t.Fatal("expected archive error to propagate")
	}
}

func TestCalTable(t *testing.T) {
	fc := &fakeCal{events: []calendar.Event{
		{ID: "E1", Subject: "Standup", Organizer: "Max", Start: mstime.Parse("2026-07-20T16:30:00.0000000"), End: mstime.Parse("2026-07-20T17:00:00.0000000")},
	}}
	out, err := run(t, factoryFor(&fakeMail{}, fc), "cal")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "Standup") || !strings.Contains(out, "Max") {
		t.Errorf("output missing content:\n%s", out)
	}
}

func TestCalTableAllDay(t *testing.T) {
	fc := &fakeCal{events: []calendar.Event{
		{ID: "H1", Subject: "Company Holiday", Organizer: "HR", IsAllDay: true,
			Start: mstime.Parse("2026-07-20T00:00:00.0000000"), End: mstime.Parse("2026-07-21T00:00:00.0000000")},
	}}
	out, err := run(t, factoryFor(&fakeMail{}, fc), "cal")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "all day") {
		t.Errorf("all-day event should render \"all day\":\n%s", out)
	}
	if strings.Contains(out, "00:00") {
		t.Errorf("all-day event should not show a time:\n%s", out)
	}
}

func TestCalJSON(t *testing.T) {
	fc := &fakeCal{events: []calendar.Event{
		{ID: "E1", Subject: "Standup", Organizer: "Max", Start: mstime.Parse("2026-07-20T16:30:00.0000000")},
	}}
	out, err := run(t, factoryFor(&fakeMail{}, fc), "cal", "--json")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var got []calendar.Event
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Subject != "Standup" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestFactoryErrorSurfacesHint(t *testing.T) {
	failing := func(ctx context.Context) (*Providers, error) {
		return nil, errors.New("spawn failed")
	}
	_, err := run(t, failing, "mail", "list")
	if err == nil || !strings.Contains(err.Error(), "WorkIQ") {
		t.Fatalf("expected WorkIQ hint in error, got %v", err)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly-ten", 11, "exactly-ten"},
		{"this is a long subject line", 10, "this is a…"},
		{"multi\nline", 20, "multi line"},
	}
	for _, tt := range tests {
		if got := truncate(tt.in, tt.n); got != tt.want {
			t.Errorf("truncate(%q,%d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}
