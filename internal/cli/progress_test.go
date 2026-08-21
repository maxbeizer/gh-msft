package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
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

func TestFormatSpinnerFrame(t *testing.T) {
	tests := []struct {
		name          string
		message       string
		terminalWidth int
		want          string
		wantTruncated bool
	}{
		{
			name:          "normal width",
			message:       "Starting WorkIQ",
			terminalWidth: 80,
			want:          "⠋ Starting WorkIQ (1s)",
		},
		{
			name:          "narrow width",
			message:       "Starting WorkIQ",
			terminalWidth: 16,
			wantTruncated: true,
		},
		{
			name:          "unicode display width",
			message:       "界界界界",
			terminalWidth: 12,
			wantTruncated: true,
		},
		{
			name:          "unknown width",
			message:       "Starting WorkIQ",
			terminalWidth: 0,
			want:          "⠋ Starting WorkIQ (1s)",
		},
		{
			name:          "extremely narrow width",
			message:       "Starting WorkIQ",
			terminalWidth: 4,
			want:          "⠋",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSpinnerFrame('⠋', tt.message, time.Second, tt.terminalWidth)
			if tt.want != "" && got != tt.want {
				t.Errorf("formatSpinnerFrame() = %q, want %q", got, tt.want)
			}
			if tt.wantTruncated && (!strings.Contains(got, "…") || !strings.HasSuffix(got, " (1s)")) {
				t.Errorf("formatSpinnerFrame() = %q, want a truncated message with elapsed time", got)
			}
			if tt.terminalWidth > 0 && runewidth.StringWidth(got) >= tt.terminalWidth {
				t.Errorf("formatSpinnerFrame() width = %d, must be less than terminal width %d", runewidth.StringWidth(got), tt.terminalWidth)
			}
		})
	}
}

func TestSpinnerRenderFrameReadsCurrentWidth(t *testing.T) {
	widths := []int{80, 16}
	sp := newSpinnerWithTerminal(&bytes.Buffer{}, true, "Starting WorkIQ")
	sp.width = func() int {
		width := widths[0]
		widths = widths[1:]
		return width
	}

	wide := sp.renderFrame(0, "Starting WorkIQ", time.Second)
	narrow := sp.renderFrame(0, "Starting WorkIQ", time.Second)

	if strings.Contains(wide, "…") {
		t.Errorf("wide frame unexpectedly truncated: %q", wide)
	}
	if !strings.Contains(narrow, "…") {
		t.Errorf("narrow frame was not truncated: %q", narrow)
	}
}

func TestSupportsSpinner(t *testing.T) {
	tests := []struct {
		name     string
		native   bool
		cygwin   bool
		width    int
		expected bool
	}{
		{"native terminal with width", true, false, 80, true},
		{"native terminal with unknown width", true, false, 0, true},
		{"cygwin terminal with width", false, true, 80, true},
		{"cygwin terminal with unknown width", false, true, 0, false},
		{"non-terminal", false, false, 80, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportsSpinner(tt.native, tt.cygwin, tt.width); got != tt.expected {
				t.Errorf("supportsSpinner() = %t, want %t", got, tt.expected)
			}
		})
	}
}

func TestSpinnerNilSafe(t *testing.T) {
	var sp *spinner
	// None of these should panic on a nil spinner.
	sp.start()
	sp.setMessages("x")
	sp.stopSpinner()
}
