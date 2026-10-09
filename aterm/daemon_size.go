package main

import "sync"

// ptySizeFeature says the daemon sizes a PTY to the largest client and sends it
// `size`. See the tooling-aterm-client skill, references/sizing.md.
const ptySizeFeature = "pty-size"

// clientSize is the box one client can show. A client that scales renders a
// larger PTY by panning, and one that does not would mis-wrap it.
type clientSize struct {
	rows, cols int
	scales     bool
}

// sizeState is what each attached client said it can show, and the size chosen.
type sizeState struct {
	mu         sync.Mutex
	reported   map[*conn]clientSize
	rows, cols int
}

// chooseSize is the largest reported box, held to the smallest client that cannot scale.
func chooseSize(reported map[*conn]clientSize) (rows, cols int, ok bool) {
	var capRows, capCols int
	for _, box := range reported {
		rows, cols = max(rows, box.rows), max(cols, box.cols)
		if box.scales {
			continue
		}
		if capRows == 0 || box.rows < capRows {
			capRows = box.rows
		}
		if capCols == 0 || box.cols < capCols {
			capCols = box.cols
		}
	}
	if capRows > 0 {
		rows = min(rows, capRows)
	}
	if capCols > 0 {
		cols = min(cols, capCols)
	}
	return rows, cols, len(reported) > 0
}

// report records what client c can show and re-chooses the PTY size. A nil
// scales keeps what the client's attach said.
func (s *ptySession) report(c *conn, rows, cols int, scales *bool) {
	if rows <= 0 || cols <= 0 {
		return
	}
	s.size.mu.Lock()
	if s.size.reported == nil {
		s.size.reported = map[*conn]clientSize{}
	}
	scaling := s.size.reported[c].scales
	if scales != nil {
		scaling = *scales
	}
	s.size.reported[c] = clientSize{clampSize(rows, 24), clampSize(cols, 80), scaling}
	s.size.mu.Unlock()
	s.refit()
}

// refit applies the chosen size when it changed and tells every attached client.
func (s *ptySession) refit() {
	s.size.mu.Lock()
	defer s.size.mu.Unlock()
	rows, cols, ok := chooseSize(s.size.reported)
	if !ok || (rows == s.size.rows && cols == s.size.cols) {
		return
	}
	s.size.rows, s.size.cols = rows, cols
	if s.holder != nil {
		_ = s.holder.write(frame{Type: "resize", Rows: rows, Cols: cols})
	}
	s.mu.Lock()
	if s.scr != nil {
		s.scr.resize(rows, cols)
	}
	s.mu.Unlock()
	s.sendTo(s.clientList(), frame{Type: "size", Session: s.name, Rows: rows, Cols: cols})
}

// sizeFrame is the size now chosen, for a client that just attached.
func (s *ptySession) sizeFrame() (frame, bool) {
	s.size.mu.Lock()
	defer s.size.mu.Unlock()
	return frame{Type: "size", Session: s.name, Rows: s.size.rows, Cols: s.size.cols}, s.size.rows > 0
}

// forgetSize drops a client that detached or went away, and re-chooses without it.
func (s *ptySession) forgetSize(c *conn) {
	s.size.mu.Lock()
	delete(s.size.reported, c)
	s.size.mu.Unlock()
	s.refit()
}
