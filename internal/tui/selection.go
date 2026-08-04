package tui

// selection is a set of message IDs the user has marked for a bulk action.
//
// It is keyed by ID rather than by row index so that archiving a message or
// refreshing the list cannot silently repoint the selection at different rows.
// The zero value is an empty, usable selection.
type selection struct {
	ids map[string]struct{}
}

// add marks an ID. Adding an ID that is already marked is a no-op, so repeatedly
// selecting the same row (holding shift-arrow against a list boundary, say) is
// harmless. Blank IDs are ignored because they cannot identify a message.
func (s *selection) add(id string) {
	if id == "" {
		return
	}
	if s.ids == nil {
		s.ids = make(map[string]struct{})
	}
	s.ids[id] = struct{}{}
}

// has reports whether the ID is marked.
func (s selection) has(id string) bool {
	_, ok := s.ids[id]
	return ok
}

// clear unmarks everything.
func (s *selection) clear() {
	s.ids = nil
}

// len returns how many IDs are marked.
func (s selection) len() int {
	return len(s.ids)
}

// retain drops every marked ID that is absent from keep. It is used to prune the
// selection when messages leave the list.
func (s *selection) retain(keep []string) {
	if len(s.ids) == 0 {
		return
	}
	pruned := make(map[string]struct{}, len(s.ids))
	for _, id := range keep {
		if _, ok := s.ids[id]; ok {
			pruned[id] = struct{}{}
		}
	}
	if len(pruned) == 0 {
		s.ids = nil
		return
	}
	s.ids = pruned
}
