package tui

import "testing"

func TestSelectionAdd(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		wantLen int
	}{
		{"empty", nil, 0},
		{"single id", []string{"a"}, 1},
		{"distinct ids", []string{"a", "b", "c"}, 3},
		{"duplicate ids collapse", []string{"a", "a", "a"}, 1},
		{"blank ids are ignored", []string{""}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s selection
			for _, id := range tt.ids {
				s.add(id)
			}
			if got := s.len(); got != tt.wantLen {
				t.Errorf("len() = %d, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestSelectionHas(t *testing.T) {
	tests := []struct {
		name  string
		added []string
		query string
		want  bool
	}{
		{"present", []string{"a", "b"}, "a", true},
		{"absent", []string{"a", "b"}, "c", false},
		{"empty selection", nil, "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s selection
			for _, id := range tt.added {
				s.add(id)
			}
			if got := s.has(tt.query); got != tt.want {
				t.Errorf("has(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestSelectionClear(t *testing.T) {
	var s selection
	s.add("a")
	s.add("b")
	s.clear()
	if got := s.len(); got != 0 {
		t.Errorf("len() after clear = %d, want 0", got)
	}
	if s.has("a") {
		t.Error("has(\"a\") after clear = true, want false")
	}
	s.add("c")
	if !s.has("c") {
		t.Error("selection is unusable after clear")
	}
}

func TestSelectionRetain(t *testing.T) {
	tests := []struct {
		name    string
		added   []string
		keep    []string
		wantHas map[string]bool
		wantLen int
	}{
		{
			name:    "drops ids that are gone",
			added:   []string{"a", "b", "c"},
			keep:    []string{"a", "c"},
			wantHas: map[string]bool{"a": true, "b": false, "c": true},
			wantLen: 2,
		},
		{
			name:    "keeping nothing empties the selection",
			added:   []string{"a", "b"},
			keep:    nil,
			wantHas: map[string]bool{"a": false, "b": false},
			wantLen: 0,
		},
		{
			name:    "unknown ids are not added",
			added:   []string{"a"},
			keep:    []string{"a", "z"},
			wantHas: map[string]bool{"a": true, "z": false},
			wantLen: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s selection
			for _, id := range tt.added {
				s.add(id)
			}
			s.retain(tt.keep)
			if got := s.len(); got != tt.wantLen {
				t.Errorf("len() = %d, want %d", got, tt.wantLen)
			}
			for id, want := range tt.wantHas {
				if got := s.has(id); got != want {
					t.Errorf("has(%q) = %v, want %v", id, got, want)
				}
			}
		})
	}
}
