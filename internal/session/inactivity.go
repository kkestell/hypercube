package session

import (
	"context"
	"time"

	"github.com/kkestell/hypercube/internal/store"
)

func (r *Runtime) recordActivity(session string) error {
	return r.save(func(tx *write) error {
		return tx.TouchSession(session, time.Now().UnixMilli())
	})
}

// ViewSession exempts a page's selected session from the idle timeout until
// the returned function is called when the page switches or disconnects.
func (r *Runtime) ViewSession(id string) func() {
	if id == "" {
		return func() {}
	}
	r.mu.Lock()
	r.viewers[id]++
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.viewers[id]--
		if r.viewers[id] == 0 {
			delete(r.viewers, id)
		}
	}
}

// StopIdleSessions stops sessions after ten minutes without activity. A selected
// session or a turn still running in any agent keeps it alive. It runs until ctx ends.
func (r *Runtime) StopIdleSessions(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			r.stopIdleSessions(now)
		case <-ctx.Done():
			return
		}
	}
}

func (r *Runtime) stopIdleSessions(now time.Time) {
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return
	}
	var expired []string
	err := r.db.Read(func(tx *store.Tx) error {
		sessions, err := tx.Sessions("")
		if err != nil {
			return err
		}
		for _, s := range sessions {
			if !s.Running || r.viewers[s.ID] > 0 || now.UnixMilli()-s.LastActive < idleTimeout.Milliseconds() {
				continue
			}
			agents, err := tx.Agents(s.ID)
			if err != nil {
				return err
			}
			busy := false
			for _, a := range agents {
				l := r.live[a.ID]
				busy = busy || a.Activity == store.Busy || (l != nil && (l.starting || l.delivering))
			}
			if !busy {
				expired = append(expired, s.ID)
			}
		}
		return nil
	})
	var lives []*live
	if err == nil {
		for _, id := range expired {
			stopped, stopErr := r.stopLocked(id)
			if stopErr != nil {
				logf("stopping idle session %s: %v", id, stopErr)
			}
			lives = append(lives, stopped...)
		}
	}
	r.mu.Unlock()
	if err != nil {
		logf("checking idle sessions: %v", err)
		return
	}
	stopClients(lives)
	for _, id := range expired {
		r.Terminals.Stop(id)
	}
}
