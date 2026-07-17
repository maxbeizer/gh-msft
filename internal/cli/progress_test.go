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
	sp.setMessage("still working…")
	sp.stopSpinner()
	if buf.Len() != 0 {
		t.Errorf("disabled spinner wrote %q, want nothing", buf.String())
	}
}

func TestSpinnerNilSafe(t *testing.T) {
	var sp *spinner
	// None of these should panic on a nil spinner.
	sp.start()
	sp.setMessage("x")
	sp.stopSpinner()
}
