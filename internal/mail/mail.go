// Package mail defines a backend-agnostic mail provider and a WorkIQ-backed
// implementation. Keeping the interface separate from the transport lets the UI
// and commands stay decoupled from WorkIQ (and allows a direct-Graph provider
// later, if delegated scopes ever become available).
package mail

import (
	"context"
	"encoding/json"
	"fmt"

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
	Received mstime.Time `json:"received"`
	IsRead   bool        `json:"isRead"`
}

// Provider reads and mutates mail.
type Provider interface {
	// ListInbox returns up to top recent messages, newest first. When all is
	// false it reads the Inbox folder only; when true it reads all mail folders.
	ListInbox(ctx context.Context, top int, all bool) ([]Message, error)
	// Archive moves the message with the given id to the Archive folder.
	Archive(ctx context.Context, id string) error
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
	From             struct {
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"from"`
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
	url := fmt.Sprintf("%s?$select=subject,from,receivedDateTime,isRead&$top=%d&$orderby=receivedDateTime desc", base, top)
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
		msgs = append(msgs, Message{
			ID:      gm.ID,
			Subject: gm.Subject,
			From: Address{
				Name:  gm.From.EmailAddress.Name,
				Email: gm.From.EmailAddress.Address,
			},
			Received: mstime.Parse(gm.ReceivedDateTime),
			IsRead:   gm.IsRead,
		})
	}
	return msgs, nil
}

func (p *WorkIQProvider) Archive(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("mail: Archive requires a message id")
	}
	url := fmt.Sprintf("/me/messages/%s/move", id)
	_, err := p.c.DoAction(ctx, url, map[string]string{"DestinationId": "archive"})
	return err
}
