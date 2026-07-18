package mail

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/maxbeizer/gh-msft/internal/workiq"
)

// fakeGraph implements graphClient with canned data and call capture.
type fakeGraph struct {
	fetchData     string
	fetchErr      error
	gotFetchURLs  []string
	doActionURL   string
	doActionBody  any
	doActionErr   error
	doActionCalls int
}

func (f *fakeGraph) Fetch(ctx context.Context, urls ...string) ([]workiq.FetchResult, error) {
	f.gotFetchURLs = urls
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return []workiq.FetchResult{{Data: json.RawMessage(f.fetchData), StatusCode: 200}}, nil
}

func (f *fakeGraph) DoAction(ctx context.Context, actionURL string, body any) (json.RawMessage, error) {
	f.doActionCalls++
	f.doActionURL = actionURL
	f.doActionBody = body
	if f.doActionErr != nil {
		return nil, f.doActionErr
	}
	return json.RawMessage(`{}`), nil
}

// sampleInbox is a trimmed capture of a real /me/messages response.
const sampleInbox = `{
  "value": [
    {"id":"AAA1","subject":"Nifty Stand Up","isRead":false,"receivedDateTime":"2026-07-17T21:36:12Z",
     "from":{"emailAddress":{"name":"Ada Lovelace","address":"ada@example.com"}}},
    {"id":"AAA2","subject":"Accepted: Meeting","isRead":true,"receivedDateTime":"2026-07-17T21:42:23Z",
     "from":{"emailAddress":{"name":"Jonathan Otalora","address":"stephenotalora@github.com"}}}
  ]
}`

func TestListInboxParsesMessages(t *testing.T) {
	fg := &fakeGraph{fetchData: sampleInbox}
	p := NewWorkIQProvider(fg)

	msgs, err := p.ListInbox(context.Background(), 5, false)
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].ID != "AAA1" || msgs[0].Subject != "Nifty Stand Up" {
		t.Errorf("msg0 = %+v", msgs[0])
	}
	if msgs[0].From.Name != "Ada Lovelace" || msgs[0].From.Email != "ada@example.com" {
		t.Errorf("msg0 from = %+v", msgs[0].From)
	}
	if msgs[0].IsRead {
		t.Errorf("msg0 should be unread")
	}
	if msgs[0].Received.IsZero() {
		t.Errorf("msg0 received time not parsed")
	}
	// Verify the request selected the right fields and top.
	if len(fg.gotFetchURLs) != 1 {
		t.Fatalf("expected 1 fetch url, got %v", fg.gotFetchURLs)
	}
	if want := "$top=5"; !contains(fg.gotFetchURLs[0], want) {
		t.Errorf("fetch url %q missing %q", fg.gotFetchURLs[0], want)
	}
}

func TestListInboxDefaultsToInboxFolder(t *testing.T) {
	fg := &fakeGraph{fetchData: sampleInbox}
	p := NewWorkIQProvider(fg)
	if _, err := p.ListInbox(context.Background(), 5, false); err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if want := "/me/mailFolders/inbox/messages"; !contains(fg.gotFetchURLs[0], want) {
		t.Errorf("fetch url %q missing %q", fg.gotFetchURLs[0], want)
	}
}

func TestListInboxAllReadsAllMail(t *testing.T) {
	fg := &fakeGraph{fetchData: sampleInbox}
	p := NewWorkIQProvider(fg)
	if _, err := p.ListInbox(context.Background(), 5, true); err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if contains(fg.gotFetchURLs[0], "mailFolders") {
		t.Errorf("fetch url %q should not scope to a folder", fg.gotFetchURLs[0])
	}
	if want := "/me/messages"; !contains(fg.gotFetchURLs[0], want) {
		t.Errorf("fetch url %q missing %q", fg.gotFetchURLs[0], want)
	}
}

func TestListInboxEmpty(t *testing.T) {
	fg := &fakeGraph{fetchData: `{"value":[]}`}
	p := NewWorkIQProvider(fg)
	msgs, err := p.ListInbox(context.Background(), 5, false)
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("want 0 messages, got %d", len(msgs))
	}
}

func TestListInboxMalformedDegradesGracefully(t *testing.T) {
	// Missing/garbled fields should not panic; bad JSON is an error.
	fg := &fakeGraph{fetchData: `{"value":[{"id":"X"}]}`}
	p := NewWorkIQProvider(fg)
	msgs, err := p.ListInbox(context.Background(), 5, false)
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if len(msgs) != 1 || msgs[0].ID != "X" {
		t.Fatalf("unexpected: %+v", msgs)
	}
	if !msgs[0].Received.IsZero() {
		t.Errorf("missing received should be zero time")
	}
}

func TestArchiveSendsMoveAction(t *testing.T) {
	fg := &fakeGraph{}
	p := NewWorkIQProvider(fg)
	if err := p.Archive(context.Background(), "MSG123"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if fg.doActionCalls != 1 {
		t.Fatalf("expected 1 do_action call, got %d", fg.doActionCalls)
	}
	if fg.doActionURL != "/me/messages/MSG123/move" {
		t.Errorf("action url = %q", fg.doActionURL)
	}
	body, _ := json.Marshal(fg.doActionBody)
	if !contains(string(body), "archive") {
		t.Errorf("action body = %s, want DestinationId archive", body)
	}
}

func TestArchiveRequiresID(t *testing.T) {
	p := NewWorkIQProvider(&fakeGraph{})
	if err := p.Archive(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestArchivePropagatesError(t *testing.T) {
	fg := &fakeGraph{doActionErr: errors.New("nope")}
	p := NewWorkIQProvider(fg)
	if err := p.Archive(context.Background(), "X"); err == nil {
		t.Fatal("expected error from DoAction")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
