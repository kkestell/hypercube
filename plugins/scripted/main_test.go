package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
	"github.com/kkestell/hypercube/internal/report"
)

const testScene = `{"workspaces": [{"sessions": [{"turns": [
  {"prompt": "Add retry", "end": "idle", "items": [
    {"id": "r", "kind": "reasoning", "text": "Look at the client.", "done": true},
    {"id": "a", "kind": "text", "text": "Added retry.", "done": true}]},
  {"prompt": "Ship it?", "end": "ask", "items": [
    {"id": "q", "kind": "attention", "text": "Push to main?", "choices": ["Push", "Wait"]}]},
  {"prompt": "Check limits", "end": "idle", "limits": [{"minutes": 300, "used": 40, "left": 90}], "items": [
    {"id": "l", "kind": "text", "text": "Checked.", "done": true}]}
]}]}]}`

func start(t *testing.T) *harnesstest.Serving {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scene.json")
	if err := os.WriteFile(path, []byte(testScene), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HYPERCUBE_SCENE", path)
	c := harnesstest.Serve(t, serve)
	c.Send(harness.Command{ID: 1, Command: harness.CmdStart, Start: &harness.StartArgs{}})
	c.Reply(1)
	return c
}

// events sends an input and returns the events that follow its reply, through
// the one that match accepts.
func events(c *harnesstest.Serving, id int64, text string, match func(harness.Event) bool) []harness.Event {
	c.Send(harness.Command{ID: id, Command: harness.CmdInput, Text: text})
	c.Reply(id)
	var got []harness.Event
	c.Until(func(r harness.Record) bool {
		got = append(got, *r.Event)
		return match(*r.Event)
	})
	return got
}

func TestTurns(t *testing.T) {
	c := start(t)
	idle := events(c, 2, "Message from lead:\nAdd retry\n\nThis message comes from another session.",
		func(ev harness.Event) bool { return ev.State == harness.Idle })
	want := []harness.Event{
		{State: harness.Working},
		{Items: []harness.Item{{ID: "r", Kind: harness.KindReasoning, Text: "Look at the client.", Done: true}}},
		{State: harness.Idle, Final: []string{"a"}, Items: []harness.Item{{ID: "a", Kind: harness.KindText, Text: "Added retry.", Done: true}}},
	}
	if !reflect.DeepEqual(idle, want) {
		t.Errorf("idle turn = %+v, want %+v", idle, want)
	}

	ask := events(c, 3, "Ship it?", func(ev harness.Event) bool { return len(ev.Items) > 0 })
	want = []harness.Event{
		{State: harness.Working},
		{Items: []harness.Item{{ID: "q", Kind: harness.KindAttention, Text: "Push to main?", Choices: []string{"Push", "Wait"}}}},
	}
	if !reflect.DeepEqual(ask, want) {
		t.Errorf("ask turn = %+v, want %+v", ask, want)
	}

	unmatched := events(c, 4, "Something else", func(harness.Event) bool { return true })
	if items := unmatched[0].Items; len(items) != 1 || items[0].Kind != harness.KindError {
		t.Errorf("unmatched input = %+v, want an error item", unmatched)
	}
}

// A turn's limits arrive with its end, resetting the given minutes after the
// reading.
func TestLimits(t *testing.T) {
	c := start(t)
	before := time.Now()
	got := events(c, 2, "Check limits", func(ev harness.Event) bool { return ev.State == harness.Idle })
	limits := got[len(got)-1].Limits
	if len(limits) != 1 || limits[0].Minutes != 300 || limits[0].Used != 40 {
		t.Fatalf("limits = %+v, want one 300-minute window 40%% used", limits)
	}
	if at := time.UnixMilli(limits[0].At); at.Before(before.Truncate(time.Millisecond)) || time.Since(at) > time.Minute {
		t.Errorf("reading at %v, want now", at)
	}
	if left := time.Unix(limits[0].Resets, 0).Sub(time.UnixMilli(limits[0].At)); left < 89*time.Minute || left > 90*time.Minute {
		t.Errorf("resets %v after the reading, want 90 minutes", left)
	}
}

// An idle parent of idle children receives their reports, which no scene
// scripts. The turn ends without an error.
func TestChildReports(t *testing.T) {
	c := start(t)
	got := events(c, 2, report.ReportsReady+" A report marks the end of a turn.\n\nReports:\n\nDone.",
		func(ev harness.Event) bool { return ev.State == harness.Idle })
	want := []harness.Event{{State: harness.Working}, {State: harness.Idle}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("report turn = %+v, want %+v", got, want)
	}
}
