package server

import "math"

const (
	barRows   = 1 // the status bar, at the bottom of the terminal
	titleRows = 1 // the row above each pane
)

// rect is a pane's screen in terminal cells, 0-based. Its title row is y-1.
type rect struct{ x, y, cols, rows int }

// layout places the first of a window's panes that fit a terminal of cols by
// rows. The first pane is the main pane, on the left, taking split of the
// width; the others stack on the right, and one column between them is a
// divider. A collapsed stacked pane
// gets only its title row, and the others share the rest in equal heights.
// The result may be shorter than panes. It must be called with the server's
// mutex held.
func layout(panes []*pane, split float64, cols, rows int) []rect {
	n := len(panes)
	area := rows - barRows
	if cols < 1 || area < 2 {
		return nil
	}
	main := rect{x: 0, y: titleRows, cols: cols, rows: area - titleRows}
	// A terminal under three columns has no room for a stack; that is unlikely.
	if n == 1 || cols < 3 {
		return []rect{main}
	}
	main.cols = min(max(int(math.Round(float64(cols)*split)), 1), cols-2)
	rects := []rect{main}
	x := main.cols + 1
	// Show stacked panes in order while each fits: a collapsed pane needs its
	// title row, and an open one a screen row too.
	stack := panes[1:]
	need, shown, open := 0, 0, 0
	for _, p := range stack {
		h := titleRows
		if !p.collapsed {
			h++
		}
		if need+h > area {
			break
		}
		need, shown = need+h, shown+1
		if !p.collapsed {
			open++
		}
	}
	extra := area - need
	y, j := 0, 0
	for _, p := range stack[:shown] {
		h := 0
		if !p.collapsed {
			h = 1 + extra/open
			if j < extra%open {
				h++
			}
			j++
		}
		rects = append(rects, rect{x: x, y: y + titleRows, cols: cols - x, rows: h})
		y += titleRows + h
	}
	return rects
}
