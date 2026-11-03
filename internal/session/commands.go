package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// record reserves an ID and the input's position before any bytes are sent.
// The caller holds mu; send serializes all recorded lines in database order.
func (r *Runtime) record(tx *write, l *live, cmd harness.Command, message, seq int64, item *harness.Item) (int64, error) {
	c := store.Command{Session: l.id, Command: cmd.Command, Message: message, Seq: seq}
	var err error
	if item != nil {
		// The input's time is when it was sent, not when the plugin accepted it.
		item.At = time.Now().UnixMilli()
		if c.Item, err = json.Marshal(item); err != nil {
			return 0, err
		}
	}
	if c.ID, err = tx.AddCommand(c); err != nil {
		return 0, err
	}
	cmd.ID = c.ID
	line, err := cmd.Line()
	if err != nil {
		return 0, err
	}
	c.Line = string(line)
	if err = tx.SetCommandLine(c.ID, c.Line); err != nil {
		return 0, err
	}
	l.commands[c.ID] = c
	if message == 0 {
		l.waiters[c.ID] = make(chan error, 1)
	}
	return c.ID, nil
}

// send holds the launch's send lock across reading and writing its commands.
// Recording is serialized by mu, so every sender sees the same ID order.
func (r *Runtime) send(l *live) {
	l.send.Lock()
	defer l.send.Unlock()
	r.mu.Lock()
	if !r.current(l) || r.closing || l.client == nil {
		r.mu.Unlock()
		return
	}
	var commands []store.Command
	err := r.db.Read(func(tx *store.Tx) error { var err error; commands, err = tx.Commands(l.id); return err })
	client := l.client
	r.mu.Unlock()
	if err != nil {
		logf("reading commands: %v", err)
		return
	}
	for _, c := range commands {
		if c.ID <= l.sent {
			continue
		}
		if err = client.Send([]byte(c.Line)); err != nil {
			return
		}
		l.sent = c.ID
	}
}

func (r *Runtime) wait(l *live, id int64, timeout time.Duration) error {
	r.mu.Lock()
	ch := l.waiters[id]
	r.mu.Unlock()
	if ch == nil {
		return errors.New("command has no waiter")
	}
	defer func() { r.mu.Lock(); delete(l.waiters, id); r.mu.Unlock() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-ch:
		return err
	case <-l.done:
		select {
		case err := <-ch:
			return err
		default:
		}
		if l.cause != nil {
			return l.cause
		}
		return errStopped
	case <-timer.C:
		return fmt.Errorf("no reply within %s", timeout)
	}
}

// reply applies acceptance even when the original caller has already stopped
// waiting. The output position is committed by apply in this transaction.
func (r *Runtime) reply(tx *write, l *live, reply harness.Reply) error {
	c := l.commands[reply.ID]
	var failure error
	if reply.Error != "" {
		failure = errors.New(reply.Error)
	}
	if c.Command == harness.CmdStart {
		if failure != nil {
			l.cause = failure
			go l.client.Stop()
		} else {
			s, err := tx.Session(l.id)
			if err != nil {
				return err
			}
			s.Ready = time.Now().UnixMilli()
			l.ready = s.Ready
			if s.Activity == store.Exited {
				s.Activity = store.Idle
			}
			if err = tx.SaveSession(s); err != nil {
				return err
			}
		}
	} else if failure != nil {
		text := "Agent command failed: "
		if c.Message != 0 {
			text = "Delivering a message failed: "
		}
		if err := r.note(tx, l, harness.KindError, text+failure.Error()); err != nil {
			return err
		}
	} else if c.Command == harness.CmdInput {
		if c.Message != 0 {
			if err := tx.Dequeue(c.Message); err != nil {
				return err
			}
		}
		var item harness.Item
		if err := json.Unmarshal(c.Item, &item); err != nil {
			return err
		}
		if err := tx.putItem(l.id, c.Seq, item); err != nil {
			return err
		}
	}
	return tx.DeleteCommand(reply.ID)
}

// recordRequest records an interactive command. The caller holds mu.
func (r *Runtime) recordRequest(l *live, cmd harness.Command, item *harness.Item, release bool) (int64, error) {
	var id int64
	err := r.save(func(tx *write) error {
		if release {
			s, err := tx.Root(l.id)
			if err != nil {
				return err
			}
			s.AwaitingUser = false
			if err = tx.SaveSession(s); err != nil {
				return err
			}
		}
		var seq int64
		if item != nil {
			seq = r.nextSeq(l.id)
		}
		var err error
		id, err = r.record(tx, l, cmd, 0, seq, item)
		return err
	})
	return id, err
}

func (r *Runtime) request(l *live, id int64) error {
	go r.send(l)
	return r.wait(l, id, r.requestTimeout)
}
