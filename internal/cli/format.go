package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/maxbeizer/gh-msft/internal/calendar"
	"github.com/maxbeizer/gh-msft/internal/mail"
)

const maxSubjectLen = 60

// writeMessages renders inbox messages as a table or JSON.
func writeMessages(w io.Writer, msgs []mail.Message, asJSON bool) error {
	if asJSON {
		return writeJSON(w, msgs)
	}
	if len(msgs) == 0 {
		fmt.Fprintln(w, "No messages.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "\tRECEIVED\tFROM\tSUBJECT")
	for _, m := range msgs {
		marker := "●" // unread
		if m.IsRead {
			marker = " "
		}
		from := m.From.Name
		if from == "" {
			from = m.From.Email
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			marker,
			formatLocal(m.Received.Time),
			truncate(from, 24),
			truncate(m.Subject, maxSubjectLen),
		)
	}
	return tw.Flush()
}

// writeEvents renders calendar events as a table or JSON.
func writeEvents(w io.Writer, events []calendar.Event, asJSON bool) error {
	if asJSON {
		return writeJSON(w, events)
	}
	if len(events) == 0 {
		fmt.Fprintln(w, "No upcoming events.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "WHEN\tSUBJECT\tORGANIZER")
	for _, e := range events {
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			formatEventWhen(e),
			truncate(e.Subject, maxSubjectLen),
			truncate(e.Organizer, 24),
		)
	}
	return tw.Flush()
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func formatLocal(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("Jan 02 15:04")
}

func formatEventWhen(e calendar.Event) string {
	start := e.Start.Time
	if start.IsZero() {
		return "-"
	}
	if e.IsAllDay {
		return start.Format("Mon Jan 02") + " all day"
	}
	s := start.Local()
	if e.End.IsZero() {
		return s.Format("Mon Jan 02 15:04")
	}
	end := e.End.Local()
	if sameDay(s, end) {
		return fmt.Sprintf("%s-%s", s.Format("Mon Jan 02 15:04"), end.Format("15:04"))
	}
	return fmt.Sprintf("%s-%s", s.Format("Mon Jan 02 15:04"), end.Format("Mon Jan 02 15:04"))
}

// sameDay reports whether a and b fall on the same calendar day.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
