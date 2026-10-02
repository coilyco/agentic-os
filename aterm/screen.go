package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// screen is the text a terminal would show now, rebuilt from the program's output.
// A reader's approximation: moves and erases, no color or mouse. docs/aterm-daemon.md
type screen struct {
	rows, cols  int
	cells       [][]rune
	row, col    int
	savedR      int
	savedC      int
	main        [][]rune
	alternate   bool
	parser      *ansi.Parser
	wrapPending bool
	top, bottom int
}

// blank marks a cell nothing was written to, and tail the right half of a wide rune.
const (
	blank = rune(0)
	tail  = rune(-1)
)

func newScreen(rows, cols int) *screen {
	s := &screen{}
	s.cells = makeCells(max(rows, 1), max(cols, 1))
	s.rows, s.cols = len(s.cells), len(s.cells[0])
	s.bottom = s.rows - 1
	s.parser = ansi.NewParser()
	s.parser.SetHandler(ansi.Handler{
		Print:     s.print,
		Execute:   s.execute,
		HandleCsi: s.csi,
		HandleEsc: s.esc,
	})
	return s
}

func makeCells(rows, cols int) [][]rune {
	cells := make([][]rune, rows)
	for index := range cells {
		cells[index] = make([]rune, cols)
	}
	return cells
}

// write feeds terminal output into the screen.
func (s *screen) write(data []byte) { s.parser.Parse(data) }

// resize keeps what fits of the old contents from the top left corner.
func (s *screen) resize(rows, cols int) {
	rows, cols = max(rows, 1), max(cols, 1)
	if rows == s.rows && cols == s.cols {
		return
	}
	next := makeCells(rows, cols)
	for index := 0; index < min(rows, s.rows); index++ {
		copy(next[index], s.cells[index])
	}
	s.cells, s.rows, s.cols = next, rows, cols
	s.top, s.bottom = 0, rows-1
	s.row, s.col = min(s.row, rows-1), min(s.col, cols-1)
}

func (s *screen) print(r rune) {
	width := max(runewidth.RuneWidth(r), 1)
	if s.wrapPending || s.col+width > s.cols {
		s.col = 0
		s.lineFeed()
	}
	s.wrapPending = false
	s.cells[s.row][s.col] = r
	if width == 2 && s.col+1 < s.cols {
		s.cells[s.row][s.col+1] = tail
	}
	s.col += width
	if s.col >= s.cols {
		s.col = s.cols - 1
		s.wrapPending = true
	}
}

func (s *screen) execute(b byte) {
	switch b {
	case '\r':
		s.col, s.wrapPending = 0, false
	case '\n', '\v', '\f':
		s.lineFeed()
	case '\b':
		s.col, s.wrapPending = max(s.col-1, 0), false
	case '\t':
		s.col, s.wrapPending = min((s.col/8+1)*8, s.cols-1), false
	}
}

func (s *screen) lineFeed() {
	switch {
	case s.row == s.bottom:
		s.scrollUp(s.top, s.bottom, 1)
	case s.row < s.rows-1:
		s.row++
	}
}

// scrollUp deletes n lines at top within the region and blanks the bottom ones.
func (s *screen) scrollUp(top, bottom, n int) {
	n = min(n, bottom-top+1)
	copy(s.cells[top:], s.cells[top+n:bottom+1])
	for index := bottom - n + 1; index <= bottom; index++ {
		s.cells[index] = make([]rune, s.cols)
	}
}

// scrollDown inserts n blank lines at top within the region, pushing the rest down.
func (s *screen) scrollDown(top, bottom, n int) {
	n = min(n, bottom-top+1)
	copy(s.cells[top+n:bottom+1], s.cells[top:bottom+1-n])
	for index := top; index < top+n; index++ {
		s.cells[index] = make([]rune, s.cols)
	}
}

func (s *screen) esc(cmd ansi.Cmd) {
	switch cmd.Final() {
	case '7':
		s.savedR, s.savedC = s.row, s.col
	case '8':
		s.row, s.col = min(s.savedR, s.rows-1), min(s.savedC, s.cols-1)
	case 'M':
		if s.row > 0 {
			s.row--
		}
	case 'c':
		s.clear(0, s.rows)
		s.row, s.col = 0, 0
	}
}

// csi handles the sequences a TUI repaints with: moves, erases, line and cell
// inserts and deletes, scrolls, and the alternate screen.
func (s *screen) csi(cmd ansi.Cmd, params ansi.Params) {
	s.wrapPending = false
	arg := func(index, fallback int) int {
		value, _, _ := params.Param(index, fallback)
		return value
	}
	n := max(arg(0, 1), 1)
	switch int(cmd.Prefix())<<8 | int(cmd.Final()) {
	case 'A':
		s.row = max(s.row-n, 0)
	case 'B', 'e':
		s.row = min(s.row+n, s.rows-1)
	case 'C', 'a':
		s.col = min(s.col+n, s.cols-1)
	case 'D':
		s.col = max(s.col-n, 0)
	case 'E':
		s.row, s.col = min(s.row+n, s.rows-1), 0
	case 'F':
		s.row, s.col = max(s.row-n, 0), 0
	case 'G', '`':
		s.col = min(n-1, s.cols-1)
	case 'd':
		s.row = min(n-1, s.rows-1)
	case 'H', 'f':
		s.row, s.col = min(max(arg(0, 1), 1)-1, s.rows-1), min(max(arg(1, 1), 1)-1, s.cols-1)
	case 'J':
		s.eraseDisplay(arg(0, 0))
	case 'K':
		s.eraseLine(arg(0, 0))
	case 'L':
		if s.row >= s.top && s.row <= s.bottom {
			s.scrollDown(s.row, s.bottom, n)
		}
	case 'M':
		if s.row >= s.top && s.row <= s.bottom {
			s.scrollUp(s.row, s.bottom, n)
		}
	case 'r':
		s.setRegion(arg(0, 1)-1, arg(1, s.rows)-1)
	case 'P':
		s.deleteCells(n)
	case '@':
		s.insertCells(n)
	case 'X':
		s.eraseCells(s.col, s.col+n)
	case 'S':
		s.scrollUp(s.top, s.bottom, n)
	case 'T':
		s.scrollDown(s.top, s.bottom, n)
	case 's':
		s.savedR, s.savedC = s.row, s.col
	case 'u':
		s.row, s.col = min(s.savedR, s.rows-1), min(s.savedC, s.cols-1)
	case '?'<<8 | 'h', '?'<<8 | 'l':
		s.privateMode(cmd, params)
	}
}

// privateMode switches the alternate screen, the only mode that changes what
// "the screen" means.
func (s *screen) privateMode(cmd ansi.Cmd, params ansi.Params) {
	on := cmd.Final() == 'h'
	for index := range params {
		mode, _, _ := params.Param(index, 0)
		if mode != 47 && mode != 1047 && mode != 1049 {
			continue
		}
		if on && !s.alternate {
			s.main, s.alternate = s.cells, true
			s.cells = makeCells(s.rows, s.cols)
			s.top, s.bottom = 0, s.rows-1
		} else if !on && s.alternate {
			s.cells, s.alternate = s.main, false
			s.resize(s.rows, s.cols)
		}
	}
}

func (s *screen) clear(from, to int) {
	for index := max(from, 0); index < min(to, s.rows); index++ {
		clear(s.cells[index])
	}
}

func (s *screen) eraseDisplay(mode int) {
	switch mode {
	case 0:
		s.eraseCells(s.col, s.cols)
		s.clear(s.row+1, s.rows)
	case 1:
		s.clear(0, s.row)
		s.eraseCells(0, s.col+1)
	default:
		s.clear(0, s.rows)
	}
}

func (s *screen) eraseLine(mode int) {
	switch mode {
	case 0:
		s.eraseCells(s.col, s.cols)
	case 1:
		s.eraseCells(0, s.col+1)
	default:
		s.eraseCells(0, s.cols)
	}
}

func (s *screen) eraseCells(from, to int) {
	for index := max(from, 0); index < min(to, s.cols); index++ {
		s.cells[s.row][index] = blank
	}
}

// setRegion sets the scroll margins, or resets them when they make no region,
// and homes the cursor as the terminal does.
func (s *screen) setRegion(top, bottom int) {
	s.top, s.bottom = 0, s.rows-1
	if top >= 0 && bottom < s.rows && top < bottom {
		s.top, s.bottom = top, bottom
	}
	s.row, s.col = 0, 0
}

func (s *screen) deleteCells(n int) {
	n = min(n, s.cols-s.col)
	line := s.cells[s.row]
	copy(line[s.col:], line[s.col+n:])
	clear(line[s.cols-n:])
}

func (s *screen) insertCells(n int) {
	n = min(n, s.cols-s.col)
	line := s.cells[s.row]
	copy(line[s.col+n:], line[s.col:s.cols-n])
	clear(line[s.col : s.col+n])
}

// text is the screen as lines, trailing blanks trimmed and trailing empty
// lines dropped.
func (s *screen) text() []string {
	lines := make([]string, 0, s.rows)
	for _, row := range s.cells {
		var line strings.Builder
		for _, r := range row {
			switch r {
			case tail:
			case blank:
				line.WriteByte(' ')
			default:
				line.WriteRune(r)
			}
		}
		lines = append(lines, strings.TrimRight(line.String(), " "))
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
