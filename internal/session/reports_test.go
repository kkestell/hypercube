package session

import (
	"strconv"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestChildReportsWaitForTheFamilyWithoutBlockingMessages(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	one, two := f.child(parent, "one", ""), f.child(parent, "two", "")
	f.input(two.ID, "!work")
	f.until("second child working", func() bool { return !f.idle(two.ID) })
	answer := strings.Repeat("界", 450)
	f.input(one.ID, "!report "+strconv.Quote(answer))
	f.until("first child's report queued", func() bool { return len(f.queued(parent.ID)) == 1 })
	if got := f.queued(parent.ID)[0]; !got.Report || got.Text != "one finished a turn:\n"+strings.Repeat("界", 400)+"…" {
		t.Fatalf("report %+v", got)
	}
	if len(f.inputs(parent.ID)) != 1 {
		t.Fatal("parent resumed before all children were idle")
	}
	if _, err := f.rt.SendMessage(one.ID, parent.ID, "an explicit update"); err != nil {
		t.Fatal(err)
	}
	f.until("explicit update passes the report", func() bool { return len(f.inputs(parent.ID)) == 2 && f.idle(parent.ID) })
	f.input(two.ID, "!answer")
	f.until("one report batch delivered", func() bool {
		return len(f.queued(parent.ID)) == 0 && len(f.inputs(parent.ID)) == 3 && f.idle(parent.ID)
	})
	batch := f.inputs(parent.ID)[2]
	for _, want := range []string{"one finished a turn", "two finished a turn", "the answer", strings.Repeat("界", 400)} {
		if !strings.Contains(batch, want) {
			t.Fatalf("batch lacks %q: %s", want, batch)
		}
	}
	if strings.Contains(batch, "looking") || strings.Contains(batch, answer) {
		t.Fatal("report included progress or the full long answer")
	}
	var automatic bool
	for _, item := range f.transcript(parent.ID) {
		automatic = automatic || item.Automatic
	}
	if !automatic {
		t.Fatal("report was recorded as a user prompt")
	}
}

func TestReportArrivingDuringDeliveryIsNotLost(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "child", "")
	f.input(child.ID, "!report "+strconv.Quote("!hold first report"))
	f.until("parent holds the first report input", func() bool { return len(f.inputs(parent.ID)) == 2 })
	f.input(child.ID, "!report "+strconv.Quote("second report"))
	f.until("new report has its own queued row", func() bool { return len(f.queued(parent.ID)) == 2 })
	if err := f.rt.Interrupt(parent.ID); err != nil {
		t.Fatal(err)
	}
	f.until("second report delivered after the first", func() bool { return len(f.inputs(parent.ID)) == 3 && len(f.queued(parent.ID)) == 0 })
	if got := f.inputs(parent.ID)[2]; !strings.Contains(got, "second report") || strings.Contains(got, "first report") {
		t.Fatal(got)
	}
}

func TestChildQuestionAndParentWorkDelayReports(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	one, two := f.child(parent, "one", ""), f.child(parent, "two", "")
	f.input(two.ID, "!ask")
	f.until("child's question", func() bool { return len(f.rt.Requests()) == 1 })
	f.input(one.ID, "!answer")
	f.until("report waiting for the question", func() bool { return len(f.queued(parent.ID)) == 1 })
	f.input(parent.ID, "!work")
	f.until("parent working", func() bool { return !f.idle(parent.ID) })
	request := f.rt.Requests()[0]
	if err := f.rt.Answer(two.ID, request.Item.ID, "yes", ""); err != nil {
		t.Fatal(err)
	}
	f.until("child idle after answer", func() bool { return f.idle(two.ID) })
	if len(f.inputs(parent.ID)) != 2 {
		t.Fatal("report interrupted the parent's work")
	}
	f.input(parent.ID, "!finish")
	f.until("parent receives report after finishing", func() bool { return len(f.inputs(parent.ID)) == 4 && len(f.queued(parent.ID)) == 0 })
}

func TestDeletingAChildKeepsItsPendingReportDeliverable(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	one, two := f.child(parent, "one", ""), f.child(parent, "two", "")
	f.input(two.ID, "!work")
	f.until("second child working", func() bool { return !f.idle(two.ID) })
	f.input(one.ID, "!answer")
	f.until("report queued", func() bool { return len(f.queued(parent.ID)) == 1 })
	if _, err := f.rt.EndChild(parent.ID, "one"); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Delete(one.ID); err != nil {
		t.Fatal(err)
	}
	f.input(two.ID, "!finish")
	f.until("deleted child's report delivered", func() bool { return len(f.inputs(parent.ID)) == 2 && len(f.queued(parent.ID)) == 0 })
	if !strings.Contains(f.inputs(parent.ID)[1], "one finished a turn") {
		t.Fatal(f.inputs(parent.ID))
	}
}

func TestPendingChildInputSurvivesBackendReattachment(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	one, two := f.child(parent, "one", ""), f.child(parent, "two", "")
	f.input(two.ID, "!accept")
	f.input(one.ID, "!answer")
	f.until("report waits for pending child input", func() bool { return len(f.queued(parent.ID)) == 1 })
	f.restart()
	f.input(parent.ID, "continue")
	f.until("parent idle after reconnect", func() bool { return f.idle(parent.ID) && len(f.inputs(parent.ID)) == 2 })
	if len(f.queued(parent.ID)) != 1 {
		t.Fatal("pending input was forgotten on reconnect")
	}
	f.input(two.ID, "!answer")
	f.until("reports delivered after pending work", func() bool { return len(f.inputs(parent.ID)) == 3 && len(f.queued(parent.ID)) == 0 })
}

func TestForkChildUsesItsOwnConversationOnRestart(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "fork", Title: "Fork", Profile: harness.Profile{Harness: "test", Model: "m", Effort: "low"}, Dir: ".", Fork: true})
	if err != nil {
		t.Fatal(err)
	}
	f.until("child's saved conversation", func() bool { return f.storedSession(child.ID).Conversation != "" })
	if start := f.log(child.ID, "start")[0]; start.Conversation != "" || start.Fork != f.storedSession(parent.ID).Conversation {
		t.Fatalf("start %+v", start)
	}
	if err := f.rt.Stop(parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Start(parent.ID); err != nil {
		t.Fatal(err)
	}
	starts := f.log(child.ID, "start")
	if starts[1].Fork != "" || starts[1].Conversation != f.storedSession(child.ID).Conversation {
		t.Fatalf("resume %+v", starts[1])
	}
}
