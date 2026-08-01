package cli

import (
	"bytes"
	"testing"
)

func TestSpinnerDisabledOnNonTerminal(t *testing.T) {
	var buf bytes.Buffer
	sp := newSpinner(&buf, "working…")
	if sp.enabled {
		t.Fatal("spinner should be disabled when writer is not a terminal")
	}
	sp.start()
	sp.setMessages("still working…")
	sp.stopSpinner()
	if buf.Len() != 0 {
		t.Errorf("disabled spinner wrote %q, want nothing", buf.String())
	}
}

func TestSpinnerMessageRotation(t *testing.T) {
	sp := newSpinnerWithTerminal(&bytes.Buffer{}, true, "first", "second", "third")
	if !sp.enabled {
		t.Fatal("spinner should be enabled for an interactive terminal")
	}

	tests := []struct {
		name  string
		frame int
		want  string
	}{
		{"initial message", 0, "first"},
		{"same message before interval", framesPerMessage - 1, "first"},
		{"second message", framesPerMessage, "second"},
		{"third message", framesPerMessage * 2, "third"},
		{"wraps to first message", framesPerMessage * 3, "first"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sp.messageAt(tt.frame); got != tt.want {
				t.Errorf("messageAt(%d) = %q, want %q", tt.frame, got, tt.want)
			}
		})
	}
}

func TestSpinnerMessageUpdates(t *testing.T) {
	sp := newSpinnerWithTerminal(&bytes.Buffer{}, true, "starting")
	sp.setMessages("loading", "still loading")

	if got := sp.messageAt(0); got != "loading" {
		t.Errorf("messageAt(0) = %q, want loading", got)
	}
	if got := sp.messageAt(framesPerMessage); got != "still loading" {
		t.Errorf("messageAt(framesPerMessage) = %q, want still loading", got)
	}
}

func TestSpinnerNilSafe(t *testing.T) {
	var sp *spinner
	// None of these should panic on a nil spinner.
	sp.start()
	sp.setMessages("x")
	sp.stopSpinner()
}
