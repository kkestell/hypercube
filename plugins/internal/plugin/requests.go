package plugin

import "github.com/kkestell/hypercube/internal/harness"

// Requests tracks the requests of a harness that wait for the user. A request
// asks one or more questions, each shown as an attention item; R holds what the
// harness keeps about a request. The zero value is ready to use, and the
// owner's lock guards it.
type Requests[R any] struct {
	open map[string]slot[R] // the unanswered items, by item ID
	left map[*R]int         // the unanswered items of each request
}

type slot[R any] struct {
	request *R
	index   int
	item    harness.Item
}

// Open is an unanswered item and the request it belongs to.
type Open[R any] struct {
	Request *R
	Index   int          // the item's place among its request's items
	Item    harness.Item // the item as it was added
	Last    bool         // no other item of the request is unanswered
}

// Add starts tracking a request whose questions are items.
func (q *Requests[R]) Add(r *R, items []harness.Item) {
	if q.open == nil {
		q.open, q.left = map[string]slot[R]{}, map[*R]int{}
	}
	for i, it := range items {
		q.open[it.ID] = slot[R]{request: r, index: i, item: it}
	}
	q.left[r] = len(items)
}

// Get returns the unanswered item with the given ID.
func (q *Requests[R]) Get(id string) (Open[R], bool) {
	s, ok := q.open[id]
	return Open[R]{Request: s.request, Index: s.index, Item: s.item, Last: q.left[s.request] == 1}, ok
}

// Done finishes the unanswered item with the given ID, which Get found, and
// returns it with its answer.
func (q *Requests[R]) Done(id, answer string) harness.Item {
	s := q.open[id]
	delete(q.open, id)
	if q.left[s.request]--; q.left[s.request] == 0 {
		delete(q.left, s.request)
	}
	s.item.Done, s.item.Answer = true, answer
	return s.item
}

// Close stops tracking the requests that match, which nobody will answer. It
// returns their unanswered items, finished, and the requests.
func (q *Requests[R]) Close(match func(*R) bool) ([]harness.Item, []*R) {
	var items []harness.Item
	for id, s := range q.open {
		if match(s.request) {
			s.item.Done = true
			items = append(items, s.item)
			delete(q.open, id)
		}
	}
	var closed []*R
	for r := range q.left {
		if match(r) {
			closed = append(closed, r)
			delete(q.left, r)
		}
	}
	return items, closed
}

// Notes returns notes for an item's Notes field: nil when no option has one.
func Notes(notes []string) []string {
	for _, note := range notes {
		if note != "" {
			return notes
		}
	}
	return nil
}
