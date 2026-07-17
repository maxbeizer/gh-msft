package cli

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

// spinner writes an animated, single-line progress indicator to a writer while a
// slow operation (notably WorkIQ startup) runs, so the CLI doesn't look hung. It
// writes to stderr only when that stderr is an interactive terminal; otherwise it
// is a no-op, keeping piped/scripted output clean.
type spinner struct {
	w       io.Writer
	enabled bool

	mu      sync.Mutex
	msg     string
	stop    chan struct{}
	done    chan struct{}
	started bool
}

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// newSpinner returns a spinner writing to w. It is enabled only when w is (or
// wraps) an interactive terminal, which we detect via os.Stderr.
func newSpinner(w io.Writer, msg string) *spinner {
	return &spinner{w: w, msg: msg, enabled: isTerminal(w)}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
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
			s.mu.Lock()
			msg := s.msg
			s.mu.Unlock()
			elapsed := time.Since(start).Truncate(time.Second)
			fmt.Fprintf(s.w, "\r\033[K%c %s (%s)", spinnerFrames[frame%len(spinnerFrames)], msg, elapsed)
			frame++
		}
	}
}

// setMessage updates the text shown next to the spinner.
func (s *spinner) setMessage(msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.msg = msg
	s.mu.Unlock()
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
