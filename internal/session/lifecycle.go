package session

import (
	"cmp"
	"errors"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// Each session wakes and parks on its own. A session wakes when the user
// views it or prompts it, or a message reaches it, and parks once it has been
// eligible to park for idleTimeout.

// View counts a page viewing the session and wakes it. A viewed session does
// not park. The returned function ends the view.
func (r *Runtime) View(id string) (release func(), err error) {
	r.mu.Lock()
	r.viewers[id]++
	r.mu.Unlock()
	release = sync.OnceFunc(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.viewers[id]--
		if r.viewers[id] == 0 {
			delete(r.viewers, id)
			r.settleLocked(id)
		}
	})
	if _, err := r.wake(id); err != nil && !errors.Is(err, errClosing) && !errors.Is(err, errDeleting) && !errors.Is(err, errEnded) {
		release()
		return nil, err
	}
	return release, nil
}

// wake returns the session's launch, registering one that resumes its
// conversation when it has none. It does not wait for the plugin to start.
func (r *Runtime) wake(id string) (*live, error) {
	var s store.Session
	if err := r.db.Read(func(tx *store.Tx) error { var err error; s, err = tx.Session(id); return err }); err != nil {
		return nil, err
	}
	root := cmp.Or(s.Parent, s.ID)
	// A top-level checkout session shows the checkout's current branch; a
	// worktree session's branch is fixed when it is created.
	checkout := s.Parent == "" && s.Base == ""
	var branch string
	if checkout {
		branch, _ = workspace.Branch(s.Dir)
	}
	for {
		r.mu.Lock()
		if r.closing {
			r.mu.Unlock()
			return nil, errClosing
		}
		l := r.live[id]
		if l != nil && !l.stopping {
			r.mu.Unlock()
			return l, nil
		}
		if r.deleting[id] || r.deleting[root] {
			r.mu.Unlock()
			return nil, errDeleting
		}
		if l != nil {
			// run ends the launch through endLocked, which takes mu.
			done := l.done
			r.mu.Unlock()
			<-done
			continue
		}
		err := r.save(func(tx *write) error {
			s, err := tx.Session(id)
			if err != nil {
				return err
			}
			if s.Ended {
				return errEnded
			}
			if checkout {
				s.Branch = branch
			}
			l, err = r.decide(tx, s, root, 0)
			return err
		})
		if err == nil {
			r.live[id] = l
		}
		r.mu.Unlock()
		if err != nil {
			return nil, err
		}
		go r.run(l)
		return l, nil
	}
}

// settleLocked starts a fresh idle timeout when the session can park, and
// cancels its timer when it cannot. It runs wherever something that keeps a
// session awake can end. The caller holds mu.
func (r *Runtime) settleLocked(id string) {
	if t := r.idle[id]; t != nil {
		t.timer.Stop()
		delete(r.idle, id)
	}
	if !r.parkableLocked(id) {
		return
	}
	// The callback compares the timer's identity, which exists before the
	// timer can fire.
	t := &idleTimer{}
	t.timer = time.AfterFunc(r.idleTimeout, func() { r.park(id, t) })
	r.idle[id] = t
}

// idleTimer is a session's pending idle timeout. Runtime.mu guards timer.
type idleTimer struct{ timer *time.Timer }

// parkableLocked reports whether nothing keeps the session awake: no viewer,
// no turn, question, or command in progress, and no queued message. The
// caller holds mu.
func (r *Runtime) parkableLocked(id string) bool {
	l := r.live[id]
	if r.closing || l == nil || r.deleting[id] || r.deleting[l.root] || r.viewers[id] > 0 {
		return false
	}
	if l.ready == 0 || l.stopping || len(l.commands) > 0 || l.attention {
		return false
	}
	parkable := false
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		queued, err := tx.Queued(id)
		parkable = s.Activity == store.Idle && !s.InputPending && len(queued) == 0
		return err
	})
	if err != nil {
		logf("reading whether %s can park: %v", id, err)
		return false
	}
	return parkable
}

// park ends the session's agent when its timer is still current and the
// session can still park. A view, prompt, or message that arrives while the
// agent stops wakes it again once it has ended.
func (r *Runtime) park(id string, t *idleTimer) {
	r.mu.Lock()
	if r.idle[id] != t {
		r.mu.Unlock()
		return
	}
	delete(r.idle, id)
	if !r.parkableLocked(id) {
		r.mu.Unlock()
		return
	}
	lives := r.selectLaunches(func(l *live) bool { return l.id == id })
	r.mu.Unlock()
	r.stopLaunches(lives)
}

// stopIdleLocked stops every idle timer. The caller holds mu.
func (r *Runtime) stopIdleLocked() {
	for id, t := range r.idle {
		t.timer.Stop()
		delete(r.idle, id)
	}
}
