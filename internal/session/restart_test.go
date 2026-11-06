package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/store"
)

func (f *fixture) restart() { f.t.Helper(); f.rt.Detach(); f.rt = f.newRuntime(); f.restore() }
func (f *fixture) holder(id string) *hold.Holder {
	f.t.Helper()
	s := f.storedSession(id)
	h, err := hold.Open(filepath.Join(store.Dir(), "run", s.Launch))
	if err != nil {
		f.t.Fatal(err)
	}
	return h
}
func TestRestartAppliesOutputAndStreamingOnce(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.input(s.ID, "!work")
	f.until("working", func() bool { return f.storedSession(s.ID).Activity == store.Busy })
	f.rt.Detach()
	f.recordDetachedInput(s.ID, "!usage", true)
	f.recordDetachedInput(s.ID, "!finish", true)
	f.until("offline inputs", func() bool { return len(f.inputs(s.ID)) == 4 })
	f.rt = f.newRuntime()
	f.restore()
	f.until("replayed state", func() bool {
		return f.storedSession(s.ID).Usage.Context == 45200 && f.storedSession(s.ID).Activity == store.Idle && strings.Contains(f.summary(s.ID), "text:finished")
	})
	if len(f.log(s.ID, "start")) != 1 {
		t.Fatal("restart launched another plugin")
	}
	count := 0
	for _, item := range f.transcript(s.ID) {
		if item.Text == "finished" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("streaming item appears %d times", count)
	}
	before := f.summary(s.ID)
	f.restart()
	f.until("same transcript", func() bool { return f.summary(s.ID) == before })
}
func (f *fixture) recordDetachedInput(id, text string, send bool) {
	f.t.Helper()
	var line []byte
	err := f.db.Tx(func(tx *store.Tx) error {
		seq, err := tx.LastSeq(id)
		if err != nil {
			return err
		}
		item, _ := json.Marshal(harness.Item{ID: "offline-" + store.NewID(), Kind: harness.KindUser, Text: text, Done: true})
		c, err := tx.AddCommand(store.Command{Session: id, Command: harness.CmdInput, Seq: seq + 1, Item: item})
		if err != nil {
			return err
		}
		line, _ = harness.Command{ID: c, Command: harness.CmdInput, Text: text}.Line()
		return tx.SetCommandLine(c, string(line))
	})
	if err != nil {
		f.t.Fatal(err)
	}
	if send {
		h := f.holder(id)
		defer h.Close()
		if _, err = h.Write(line); err != nil {
			f.t.Fatal(err)
		}
	}
}
func TestRestartAnswersPersistedAttention(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.input(s.ID, "!ask")
	f.until("attention", func() bool { return len(f.requests()) == 1 })
	id := f.requests()[0].Item.ID
	f.restart()
	f.until("restored attention", func() bool { return len(f.requests()) == 1 })
	if err := f.rt.Answer(s.ID, id, []string{"yes"}, ""); err != nil {
		t.Fatal(err)
	}
	f.until("answered", func() bool { return len(f.requests()) == 0 })
	if log := f.log(s.ID, "answer"); len(log) != 1 || log[0].Request != "q" {
		t.Fatal(log)
	}
}
func TestRestartKeepsHeldDeliveryAndResendsUnwrittenCommand(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.queue("user", s.ID, "!hold", "later")
	f.until("held delivery", func() bool { return len(f.inputs(s.ID)) == 2 })
	f.restart()
	f.until("attached", func() bool {
		f.rt.mu.Lock()
		defer f.rt.mu.Unlock()
		return f.rt.live[s.ID] != nil && f.rt.live[s.ID].client != nil
	})
	if len(f.queued(s.ID)) != 2 || len(f.inputs(s.ID)) != 2 {
		t.Fatal("held delivery repeated or removed")
	}
	if err := f.rt.Interrupt(s.ID); err != nil {
		t.Fatal(err)
	}
	f.until("delivered once", func() bool { return len(f.queued(s.ID)) == 0 && len(f.inputs(s.ID)) == 3 })
	count := 0
	for _, item := range f.transcript(s.ID) {
		if strings.Contains(item.Text, "!hold") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("held item written %d times", count)
	}
	f.rt.Detach()
	f.recordDetachedInput(s.ID, "not sent", false)
	f.rt = f.newRuntime()
	f.restore()
	f.until("unsent command delivered", func() bool { return len(f.inputs(s.ID)) == 4 && strings.HasSuffix(f.summary(s.ID), "text:ack") })
	if got := f.inputs(s.ID); got[3] != "not sent" {
		t.Fatal(got)
	}
}
func TestRestartDeliversMessageQueuedBeforeDelivery(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.rt.Detach()
	// The message was saved, but the app stopped before delivering it.
	f.queue("user", s.ID, "queued")
	f.rt = f.newRuntime()
	f.restore()
	f.until("delivered", func() bool { return len(f.queued(s.ID)) == 0 })
}
func TestRestartReadsExitedLaunchAndRelaunches(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.rt.Detach()
	f.recordDetachedInput(s.ID, "!crash", true)
	h := f.holder(s.ID)
	f.until("offline exit", func() bool { _, err := h.Exit(); return err == nil })
	f.rt = f.newRuntime()
	f.restore()
	f.until("resumed after exit", func() bool { return len(f.log(s.ID, "start")) == 2 && f.idle(s.ID) })
	if !strings.Contains(f.summary(s.ID), "The agent exited: ") || f.storedSession(s.ID).Quick != 1 {
		t.Fatalf("exit not applied: %s %+v", f.summary(s.ID), f.storedSession(s.ID))
	}
	if f.log(s.ID, "start")[1].Conversation != s.ID+"-conversation" {
		t.Fatal("conversation lost")
	}
}
func killHolder(t *testing.T, dir string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if err = syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	h, _ := hold.Open(dir)
	deadline := time.Now().Add(5 * time.Second)
	for h.Alive() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.Alive() {
		t.Fatal("holder still alive")
	}
}
func TestRestartLostLaunchStaysParkedUntilMessaged(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	child := f.child(s, "child", "")
	f.rt.Detach()
	f.recordDetachedInput(child.ID, "!usage", true)
	f.until("offline usage", func() bool { return len(f.inputs(child.ID)) == 1 })
	dir := filepath.Join(store.Dir(), "run", f.storedSession(child.ID).Launch)
	killHolder(t, dir)
	f.rt = f.newRuntime()
	f.restore()
	if f.awake(child.ID) || f.storedSession(child.ID).Launch != "" || len(f.log(child.ID, "start")) != 1 {
		t.Fatal("the lost child did not stay parked")
	}
	if !f.awake(s.ID) || len(f.log(s.ID, "start")) != 1 || f.storedSession(child.ID).Usage.Context != 45200 || strings.Contains(f.summary(child.ID), "exited") {
		t.Fatal("lost launch was not recovered without a note")
	}
	f.queue(s.ID, child.ID, "after the loss")
	f.until("the child to wake and take the message", func() bool {
		return len(f.queued(child.ID)) == 0 && len(f.log(child.ID, "start")) == 2
	})
}
func TestRestoreStartsDecidedLaunchAndRemovesOrphans(t *testing.T) {
	f := newFixture(t)
	f.seed()
	f.rt.mu.Lock()
	var l *live
	err := f.rt.save(func(tx *write) error {
		s, err := tx.Session("s")
		if err != nil {
			return err
		}
		s.Harness = "test"
		if err = tx.SaveSession(s); err != nil {
			return err
		}
		l, err = f.rt.decide(tx, s, "s", 0)
		return err
	})
	f.rt.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	original := strings.TrimSuffix(l.prefix, ":")
	orphan, err := hold.Start(filepath.Join(store.Dir(), "run", "orphan"), []string{"/bin/cat"}, os.Environ(), f.root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer orphan.Remove()
	f.restore()
	f.until("decided launch ready", func() bool { return f.idle("s") })
	if f.storedSession("s").Launch != original || len(f.log("s", "start")) != 1 {
		t.Fatal("recorded launch replaced")
	}
	if orphan.Alive() {
		t.Fatal("orphan alive")
	}
	if _, err = os.Stat(filepath.Join(store.Dir(), "run", "orphan")); !os.IsNotExist(err) {
		t.Fatal("orphan directory remains")
	}
}

func TestRestartReplaysUncommittedStreamingAppend(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.input(s.ID, "!work")
	f.input(s.ID, "!append")
	f.until("streaming append", func() bool { return strings.Contains(f.summary(s.ID), "text:working more") })
	f.restart()
	f.until("replayed append", func() bool { return strings.Contains(f.summary(s.ID), "text:working more") })
	count := func(text string) int {
		n := 0
		for _, item := range f.transcript(s.ID) {
			if item.Kind == harness.KindText && item.Text == text {
				n++
			}
		}
		return n
	}
	if count("working more") != 1 {
		t.Fatal(f.summary(s.ID))
	}
	f.input(s.ID, "!complete")
	f.until("completed append", func() bool { return count("working more done") == 1 })
	f.restart()
	f.until("completed item once", func() bool { return count("working more done") == 1 })
}
func TestParkAppliesAcceptanceBeforeReaderEnds(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.queue("user", s.ID, "!onclose")
	f.until("pending delivery", func() bool { return len(f.inputs(s.ID)) == 2 })
	f.park(s.ID)
	if len(f.queued(s.ID)) != 0 || !strings.Contains(f.summary(s.ID), "user:!onclose") {
		t.Fatal("parking discarded acceptance")
	}
	f.input(s.ID, "again")
	if len(f.inputs(s.ID)) != 3 {
		t.Fatal("accepted message repeated")
	}
	entries, err := os.ReadDir(filepath.Join(store.Dir(), "run"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("holder directories: %v %v", entries, err)
	}
	f.rt.Quit()
	entries, err = os.ReadDir(filepath.Join(store.Dir(), "run"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("Quit left directories: %v %v", entries, err)
	}
}
func TestQuickExitCountSurvivesRestarts(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	for exits := 1; exits <= maxQuickExits; exits++ {
		f.rt.Detach()
		f.recordDetachedInput(s.ID, "!crash", true)
		h := f.holder(s.ID)
		f.until("exit", func() bool { _, err := h.Exit(); return err == nil })
		f.rt = f.newRuntime()
		f.restore()
		if exits < maxQuickExits {
			f.until("next launch", func() bool { return len(f.log(s.ID, "start")) == exits+1 && f.idle(s.ID) })
		} else {
			f.until("third exit stays ended", func() bool { return f.storedSession(s.ID).Launch == "" })
		}
	}
	if len(f.log(s.ID, "start")) != maxQuickExits {
		t.Fatal("quick-exit limit lost")
	}
}

func TestDeliveryTimeoutHoldsLaterMessagesUntilAcceptance(t *testing.T) {
	f := newFixture(t)
	f.rt.requestTimeout = 2 * time.Second
	s := f.session()
	f.queue("user", s.ID, "!hold", "later")
	deadline := time.Now().Add(f.rt.requestTimeout + 5*time.Second)
	noted := func() int {
		n := 0
		for _, item := range f.transcript(s.ID) {
			if strings.Contains(item.Text, "later messages wait for it") {
				n++
			}
		}
		return n
	}
	for noted() == 0 && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if noted() != 1 || len(f.queued(s.ID)) != 2 || len(f.inputs(s.ID)) != 2 {
		t.Fatal("timeout did not hold delivery")
	}
	if err := f.rt.Interrupt(s.ID); err != nil {
		t.Fatal(err)
	}
	f.until("delivery resumes", func() bool { return len(f.queued(s.ID)) == 0 && len(f.inputs(s.ID)) == 3 })
	if noted() != 1 {
		t.Fatal("timeout noted more than once")
	}
}

func TestRestoreDecidedLaunchHasStartTimeout(t *testing.T) {
	f := newFixture(t)
	f.rt.startTimeout = 2 * time.Second
	t.Setenv("HYPERCUBE_TEST_START_HOLD", "1")
	f.seed()
	f.rt.mu.Lock()
	err := f.rt.save(func(tx *write) error {
		s, err := tx.Session("s")
		if err != nil {
			return err
		}
		s.Harness = "test"
		if err = tx.SaveSession(s); err != nil {
			return err
		}
		_, err = f.rt.decide(tx, s, "s", 0)
		return err
	})
	f.rt.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	f.restore()
	deadline := time.Now().Add(f.rt.startTimeout + 5*time.Second)
	for f.storedSession("s").Launch != "" && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(f.summary("s"), "Could not start the agent: the plugin did not start in time") || f.storedSession("s").Launch != "" {
		t.Fatal(f.summary("s"))
	}
}

func TestAdoptedIdleSessionParks(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.rt.Detach()
	f.rt = f.newRuntime()
	f.idleAfter(50 * time.Millisecond)
	f.restore()
	f.untilParked(s.ID)
	if len(f.log(s.ID, "start")) != 1 || f.summary(s.ID) != "user:start|text:ack" {
		t.Fatalf("parking the adopted session: %q", f.summary(s.ID))
	}
}

func TestDetachReleasesBlockedPluginSender(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	s = f.storedSession(s.ID)
	pid := f.log(s.ID, "start")[0].Pid
	h := f.holder(s.ID)
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(pid, syscall.SIGCONT)
		_ = h.Stop()
		_ = h.Remove()
	})
	f.rt.mu.Lock()
	l := f.rt.live[s.ID]
	// The holder reads a whole command before writing it to the plugin.
	// The second large line fills its socket once the first fills the pipe.
	for range 2 {
		_, err := f.rt.recordRequest(l, harness.Command{Command: harness.CmdInput, Text: strings.Repeat("x", 2<<20)}, nil)
		if err != nil {
			f.rt.mu.Unlock()
			t.Fatal(err)
		}
	}
	f.rt.mu.Unlock()
	sent := make(chan struct{})
	go func() { f.rt.send(l); close(sent) }()
	f.until("the sender holding its lock", func() bool {
		if l.send.TryLock() {
			l.send.Unlock()
			return false
		}
		return true
	})
	detached := make(chan struct{})
	go func() { f.rt.Detach(); close(detached) }()
	select {
	case <-detached:
	case <-time.After(5 * time.Second):
		t.Fatal("detach waited behind the blocked plugin writer")
	}
	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		t.Fatal("detach left its sender blocked")
	}
	if !h.Alive() || f.storedSession(s.ID).Launch != s.Launch || f.summary(s.ID) != "user:start|text:ack" {
		t.Fatal("detach stopped the holder or discarded its transcript checkpoint")
	}
}

func TestRestoreRegistersAllFamiliesBeforeServing(t *testing.T) {
	f := newFixture(t)
	first := f.session()
	child := f.child(first, "worker", "")
	other := f.session()
	first, child, other = f.storedSession(first.ID), f.storedSession(child.ID), f.storedSession(other.ID)
	f.rt.Detach()
	f.rt = f.newRuntime()
	var releases []func()
	t.Cleanup(func() {
		for _, release := range releases {
			release()
		}
	})
	err := f.rt.Restore(func() {
		// Requests admitted as soon as HTTP starts must find every saved
		// family, including the family not yet adopted by any reader.
		for _, s := range []store.Session{first, child, other} {
			release, err := f.rt.View(s.ID)
			if err != nil {
				t.Fatal(err)
			}
			releases = append(releases, release)
			if got := f.storedSession(s.ID).Launch; got != s.Launch {
				t.Errorf("early view replaced launch %s with %s", s.Launch, got)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []store.Session{first, child, other} {
		f.input(s.ID, "after restoration")
		f.until("the restored launch accepting input", func() bool { return f.idle(s.ID) })
		if len(f.log(s.ID, "start")) != 1 || f.storedSession(s.ID).Launch != s.Launch {
			t.Error("restoration replaced the registered launch")
		}
	}
}
