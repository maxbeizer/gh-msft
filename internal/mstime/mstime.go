// Package mstime parses Microsoft Graph date/time strings, which come in several
// shapes (RFC3339, an offset-less form with 7 fractional digits, etc.) and marshals
// them back as stable RFC3339 for --json output.
package mstime

import (
	"strings"
	"time"
)

// Time wraps time.Time with Graph-friendly parsing and JSON encoding.
type Time struct{ time.Time }

var layouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.0000000Z07:00",
	"2006-01-02T15:04:05.0000000",
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02T15:04:05",
}

// Parse converts a Graph date/time string into a Time. Offset-less values are
// interpreted as UTC (Graph returns those alongside a separate timeZone field,
// which is UTC for the queries this tool issues). An unparseable or empty input
// yields the zero Time.
func Parse(s string) Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return Time{}
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return Time{t}
		}
	}
	return Time{}
}

// MarshalJSON emits RFC3339 (or null for the zero value).
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + t.Format(time.RFC3339) + `"`), nil
}
