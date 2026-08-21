package cli

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// spinner writes an animated, single-line progress indicator to a writer while a
// slow operation (notably WorkIQ startup) runs, so the CLI doesn't look hung. It
// writes to stderr only when that stderr is an interactive terminal; otherwise it
// is a no-op, keeping piped/scripted output clean.
type spinner struct {
	w       io.Writer
	enabled bool
	width   func() int

	mu       sync.Mutex
	messages []string
	stop     chan struct{}
	done     chan struct{}
	started  bool
}

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

const (
	framesPerMessage   = 20
	spinnerSafetyWidth = 1
)

// newSpinner returns a spinner writing to w. It is enabled only when w is (or
// wraps) an interactive terminal whose width can be handled safely.
func newSpinner(w io.Writer, messages ...string) *spinner {
	return newSpinnerWithTerminal(w, spinnerEnabled(w), messages...)
}

func newSpinnerWithTerminal(w io.Writer, enabled bool, messages ...string) *spinner {
	return &spinner{
		w:        w,
		enabled:  enabled,
		width:    func() int { return terminalWidth(w) },
		messages: messages,
	}
}

func spinnerEnabled(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fd := f.Fd()
	return supportsSpinner(
		isatty.IsTerminal(fd),
		isatty.IsCygwinTerminal(fd),
		terminalWidth(w),
	)
}

func supportsSpinner(nativeTerminal, cygwinTerminal bool, width int) bool {
	return nativeTerminal || (cygwinTerminal && width > 0)
}

func terminalWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return width
}

// start begins animating. It is safe to call when disabled (no-op).
func (s *spinner) start() {
	if s == nil || !s.enabled || s.started {
		return
	}
	s.started = true
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go s.run()
}

func (s *spinner) run() {
	defer close(s.done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	start := time.Now()
	frame := 0
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			msg := s.messageAt(frame)
			elapsed := time.Since(start).Truncate(time.Second)
			fmt.Fprintf(s.w, "\r\033[K%s", s.renderFrame(frame, msg, elapsed))
			frame++
		}
	}
}

func (s *spinner) renderFrame(frame int, message string, elapsed time.Duration) string {
	width := 0
	if s.width != nil {
		width = s.width()
	}
	return formatSpinnerFrame(spinnerFrames[frame%len(spinnerFrames)], message, elapsed, width)
}

func formatSpinnerFrame(frame rune, message string, elapsed time.Duration, terminalWidth int) string {
	prefix := fmt.Sprintf("%c ", frame)
	suffix := fmt.Sprintf(" (%s)", elapsed)
	line := prefix + message + suffix
	if terminalWidth <= 0 {
		return line
	}

	maxWidth := terminalWidth - spinnerSafetyWidth
	if maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(line) <= maxWidth {
		return line
	}

	messageWidth := maxWidth - runewidth.StringWidth(prefix) - runewidth.StringWidth(suffix)
	if messageWidth <= 0 {
		return runewidth.Truncate(string(frame), maxWidth, "")
	}
	tail := "…"
	if runewidth.StringWidth(tail) > messageWidth {
		tail = ""
	}
	return prefix + runewidth.Truncate(message, messageWidth, tail) + suffix
}

// setMessages updates the messages shown next to the spinner. The spinner cycles
// through the supplied messages on a fixed cadence, keeping progress copy
// deterministic for both users and tests.
func (s *spinner) setMessages(messages ...string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.messages = messages
	s.mu.Unlock()
}

func (s *spinner) messageAt(frame int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) == 0 {
		return ""
	}
	return s.messages[(frame/framesPerMessage)%len(s.messages)]
}

// stopSpinner halts animation and clears the line. Safe to call multiple times or
// when disabled.
func (s *spinner) stopSpinner() {
	if s == nil || !s.enabled || !s.started {
		return
	}
	s.started = false
	close(s.stop)
	<-s.done
	fmt.Fprint(s.w, "\r\033[K")
}
