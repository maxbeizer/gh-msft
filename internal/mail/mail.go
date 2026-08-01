// Package mail defines a backend-agnostic mail provider and a WorkIQ-backed
// implementation. Keeping the interface separate from the transport lets the UI
// and commands stay decoupled from WorkIQ (and allows a direct-Graph provider
// later, if delegated scopes ever become available).
package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/maxbeizer/gh-msft/internal/mstime"
	"github.com/maxbeizer/gh-msft/internal/workiq"
)

// Address is a mail participant.
type Address struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Message is a simplified inbox message.
type Message struct {
	ID       string      `json:"id"`
	Subject  string      `json:"subject"`
	From     Address     `json:"from"`
	To       []Address   `json:"to"`
	Received mstime.Time `json:"received"`
	IsRead   bool        `json:"isRead"`
}

// Detail is a message and its plain-text body.
type Detail struct {
	ID       string      `json:"id"`
	Subject  string      `json:"subject"`
	From     Address     `json:"from"`
	To       []Address   `json:"to"`
	Received mstime.Time `json:"received"`
	IsRead   bool        `json:"isRead"`
	Body     string      `json:"body"`
}

// NewDetail combines a message's metadata and body for presentation.
func NewDetail(m Message, body string) Detail {
	return Detail{
		ID:       m.ID,
		Subject:  m.Subject,
		From:     m.From,
		To:       m.To,
		Received: m.Received,
		IsRead:   m.IsRead,
		Body:     body,
	}
}

// Provider reads and mutates mail.
type Provider interface {
	// ListInbox returns up to top recent messages, newest first. When all is
	// false it reads the Inbox folder only; when true it reads all mail folders.
	ListInbox(ctx context.Context, top int, all bool) ([]Message, error)
	// GetDetail returns message metadata and its plain-text body by id.
	GetDetail(ctx context.Context, id string) (Detail, error)
	// Archive moves the message with the given id to the Archive folder.
	Archive(ctx context.Context, id string) error
	// Body returns the plain-text body of the message with the given id.
	Body(ctx context.Context, id string) (string, error)
}

// graphClient is the subset of the WorkIQ client this package needs. Defined here
// so tests can supply a fake without a live WorkIQ process.
type graphClient interface {
	Fetch(ctx context.Context, entityURLs ...string) ([]workiq.FetchResult, error)
	DoAction(ctx context.Context, actionURL string, jsonBody any) (json.RawMessage, error)
}

// WorkIQProvider implements Provider using the WorkIQ Graph proxy.
type WorkIQProvider struct {
	c graphClient
}

// NewWorkIQProvider builds a provider over the given WorkIQ client.
func NewWorkIQProvider(c graphClient) *WorkIQProvider {
	return &WorkIQProvider{c: c}
}

// graphMessage mirrors the relevant Microsoft Graph message fields.
type graphMessage struct {
	ID               string `json:"id"`
	Subject          string `json:"subject"`
	ReceivedDateTime string `json:"receivedDateTime"`
	IsRead           bool   `json:"isRead"`
	Body             struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
	From struct {
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"from"`
	ToRecipients []struct {
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"toRecipients"`
}

type graphMessageCollection struct {
	Value []graphMessage `json:"value"`
}

func (p *WorkIQProvider) ListInbox(ctx context.Context, top int, all bool) ([]Message, error) {
	if top <= 0 {
		top = 25
	}
	base := "/me/mailFolders/inbox/messages"
	if all {
		base = "/me/messages"
	}
	url := fmt.Sprintf("%s?$select=subject,from,toRecipients,receivedDateTime,isRead&$top=%d&$orderby=receivedDateTime desc", base, top)
	results, err := p.c.Fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	var coll graphMessageCollection
	if err := json.Unmarshal(results[0].Data, &coll); err != nil {
		return nil, fmt.Errorf("mail: decode messages: %w", err)
	}
	msgs := make([]Message, 0, len(coll.Value))
	for _, gm := range coll.Value {
		msgs = append(msgs, messageFromGraph(gm))
	}
	return msgs, nil
}

// GetDetail retrieves the metadata and body for one message in a single request.
func (p *WorkIQProvider) GetDetail(ctx context.Context, id string) (Detail, error) {
	if id == "" {
		return Detail{}, fmt.Errorf("mail: GetDetail requires a message id")
	}
	url := fmt.Sprintf("/me/messages/%s?$select=id,subject,from,toRecipients,receivedDateTime,isRead,body", id)
	results, err := p.c.Fetch(ctx, url)
	if err != nil {
		return Detail{}, err
	}
	if len(results) == 0 {
		return Detail{}, fmt.Errorf("mail: message %q was not found", id)
	}
	var gm graphMessage
	if err := json.Unmarshal(results[0].Data, &gm); err != nil {
		return Detail{}, fmt.Errorf("mail: decode message: %w", err)
	}
	body := gm.Body.Content
	if strings.EqualFold(gm.Body.ContentType, "html") {
		body = htmlToText(body)
	}
	return NewDetail(messageFromGraph(gm), body), nil
}

func messageFromGraph(gm graphMessage) Message {
	to := make([]Address, 0, len(gm.ToRecipients))
	for _, r := range gm.ToRecipients {
		to = append(to, Address{
			Name:  r.EmailAddress.Name,
			Email: r.EmailAddress.Address,
		})
	}
	return Message{
		ID:      gm.ID,
		Subject: gm.Subject,
		From: Address{
			Name:  gm.From.EmailAddress.Name,
			Email: gm.From.EmailAddress.Address,
		},
		To:       to,
		Received: mstime.Parse(gm.ReceivedDateTime),
		IsRead:   gm.IsRead,
	}
}

func (p *WorkIQProvider) Archive(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("mail: Archive requires a message id")
	}
	url := fmt.Sprintf("/me/messages/%s/move", id)
	_, err := p.c.DoAction(ctx, url, map[string]string{"DestinationId": "archive"})
	return err
}

func (p *WorkIQProvider) Body(ctx context.Context, id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("mail: Body requires a message id")
	}
	url := fmt.Sprintf("/me/messages/%s?$select=body", id)
	results, err := p.c.Fetch(ctx, url)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", nil
	}
	var gm graphMessage
	if err := json.Unmarshal(results[0].Data, &gm); err != nil {
		return "", fmt.Errorf("mail: decode body: %w", err)
	}
	content := gm.Body.Content
	if strings.EqualFold(gm.Body.ContentType, "html") {
		content = htmlToText(content)
	}
	return content, nil
}

var (
	htmlScriptStyleRE = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	htmlBreakRE       = regexp.MustCompile(`(?i)<(br|/p|/div|/tr|/li|/h[1-6])[^>]*>`)
	htmlTagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	htmlBlankRE       = regexp.MustCompile(`\n{3,}`)
)

// htmlToText turns an HTML mail body into readable plain text.
func htmlToText(s string) string {
	s = htmlScriptStyleRE.ReplaceAllString(s, "")
	s = htmlBreakRE.ReplaceAllString(s, "\n")
	s = htmlTagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r", "")
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t")
	}
	s = strings.Join(lines, "\n")
	s = htmlBlankRE.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
