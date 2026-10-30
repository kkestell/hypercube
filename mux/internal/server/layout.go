package server

const (
	barRows   = 2 // rows at the top of the terminal: the session bar and the window bar
	titleRows = 1 // the row above each pane
)

// rect is a pane's screen in terminal cells, 0-based. Its title row is y-1.
type rect struct{ x, y, cols, rows int }

// layout places the first panes of a window of n panes that fit a terminal of
// cols by rows. The first pane is the main pane, on the left; the others stack
// on the right in equal heights, and one column between them is a divider.
// The result may be shorter than n.
func layout(n, cols, rows int) []rect {
	area := rows - barRows
	if cols < 1 || area < 2 {
		return nil
	}
	main := rect{x: 0, y: barRows + titleRows, cols: cols, rows: area - titleRows}
	// A terminal under three columns has no room for a stack; that is unlikely.
	if n == 1 || cols < 3 {
		return []rect{main}
	}
	main.cols = cols * 40 / 100
	rects := []rect{main}
	x := main.cols + 1
	y := barRows
	shown := min(n-1, area/2) // each stacked pane needs a title row and a screen row
	for i := range shown {
		h := area / shown
		if i < area%shown {
			h++
		}
		rects = append(rects, rect{x: x, y: y + titleRows, cols: cols - x, rows: h - titleRows})
		y += h
	}
	return rects
}
