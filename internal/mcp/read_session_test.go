package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func saveReadItems(t *testing.T, db *store.DB, items []harness.Item) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error {
		for i, it := range items {
			it.ID = fmt.Sprint(i + 1)
			data, err := json.Marshal(it)
			if err != nil {
				return err
			}
			if _, err := tx.PutEntry("b", store.Entry{Seq: int64(i + 1), Item: it.ID, Data: data}); err != nil {
				return err
			}
		}
		return nil
	})
}

func TestReadSessionAcrossBatches(t *testing.T) {
	t.Parallel()
	db, rt := family(t, false, t.TempDir(), t.TempDir())
	items := make([]harness.Item, 160)
	for i := range items {
		items[i] = harness.Item{Kind: harness.KindReasoning, Text: "filtered"}
	}
	long := strings.Repeat("界", 20000)
	items[0] = harness.Item{Kind: harness.KindUser, Text: "brief"}
	items[69] = harness.Item{Kind: harness.KindText, Text: long}
	items[139] = harness.Item{Kind: harness.KindText, Text: long}
	items[149] = harness.Item{Kind: harness.KindState, State: harness.Idle, Final: []string{"140"}}
	saveReadItems(t, db, items)
	for _, test := range []struct {
		cursor int64
		want   string
	}{
		{0, "Earlier entries remain: call read_session with cursor 140 to read them.\n\nAssistant: " + long + "\n\n(turn finished)"},
		{-5, "Earlier entries remain: call read_session with cursor 140 to read them.\n\nAssistant: " + long + "\n\n(turn finished)"},
		{140, "User: brief\n\nAssistant: " + long},
		{70, "User: brief"},
		{1, "No entries."},
	} {
		got, err := readSession(db, rt, "session-1", readSessionArgs{To: "w1", Cursor: test.cursor})
		if err != nil || got != test.want {
			t.Fatalf("cursor %d: got %.160q (%v), want %.160q", test.cursor, got, err, test.want)
		}
	}
}

func TestReadSessionRuneBudget(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		items []harness.Item
		want  string
	}{
		{
			name: "oversized first entry",
			items: []harness.Item{
				{Kind: harness.KindUser, Text: "earlier"},
				{Kind: harness.KindText, Text: strings.Repeat("界", readBudget+10)},
			},
			want: "Earlier entries remain: call read_session with cursor 2 to read them.\n\nAssistant: " + strings.Repeat("界", readBudget-len("Assistant: ")) + "…",
		},
		{
			name: "full page with only filtered older entries",
			items: []harness.Item{
				{Kind: harness.KindReasoning, Text: "hidden"},
				{Kind: harness.KindState, State: harness.Working},
				{Kind: harness.KindText, Parent: "native", Text: "hidden"},
				{Kind: harness.KindText, Text: strings.Repeat("é", readBudget-len("Assistant: "))},
			},
			want: "Assistant: " + strings.Repeat("é", readBudget-len("Assistant: ")),
		},
		{
			name: "separator counts at exact boundary",
			items: []harness.Item{
				{Kind: harness.KindUser, Text: "a"},
				{Kind: harness.KindText, Text: strings.Repeat("é", readBudget-len("Assistant: ")-len("User: a")-2)},
			},
			want: "User: a\n\nAssistant: " + strings.Repeat("é", readBudget-len("Assistant: ")-len("User: a")-2),
		},
		{
			name: "separator pushes older entry over boundary",
			items: []harness.Item{
				{Kind: harness.KindUser, Text: "a"},
				{Kind: harness.KindText, Text: strings.Repeat("é", readBudget-len("Assistant: ")-len("User: a")-1)},
			},
			want: "Earlier entries remain: call read_session with cursor 2 to read them.\n\nAssistant: " + strings.Repeat("é", readBudget-len("Assistant: ")-len("User: a")-1),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, rt := family(t, false, t.TempDir(), t.TempDir())
			saveReadItems(t, db, test.items)
			got, err := readSession(db, rt, "session-1", readSessionArgs{To: "w1"})
			if err != nil || got != test.want {
				t.Fatalf("got %.160q (%v), want %.160q (runes %d vs %d)", got, err, test.want, len([]rune(got)), len([]rune(test.want)))
			}
		})
	}
}

func TestReadSessionCompactFormatting(t *testing.T) {
	t.Parallel()
	db, rt := family(t, false, t.TempDir(), t.TempDir())
	saveReadItems(t, db, []harness.Item{
		{Kind: harness.KindUser, Text: "prompt"},
		{Kind: harness.KindUser, Text: "generated", Automatic: true, From: "ignored"},
		{Kind: harness.KindUser, Text: "message", From: "sender"},
		{Kind: harness.KindText, Text: "answer"},
		{Kind: harness.KindTool, Tool: "shell", Output: "omitted"},
		{Kind: harness.KindTool, Tool: "read", Failed: true},
		{Kind: harness.KindError, Text: "error"},
		{Kind: harness.KindAttention, Text: "question"},
		{Kind: harness.KindAttention, Text: "question", Answer: "yes"},
		{Kind: harness.KindProposal, Text: "suggestion"},
		{Kind: harness.KindState, State: harness.Idle},
		{Kind: harness.KindState, State: harness.Idle, Final: []string{"answer"}},
		{Kind: harness.KindReasoning, Text: "omitted"},
		{Kind: harness.KindState, State: harness.Working},
		{Kind: harness.KindText, Parent: "native", Text: "omitted"},
	})
	want := strings.Join([]string{
		"User: prompt", "Hypercube: generated", "Message from sender: message", "Assistant: answer",
		"Tool: shell", "Tool: read (failed)", "Error: error", "Question: question",
		"Question: question\nAnswer: yes", "Suggested session: suggestion",
		"(turn ended without a final response)", "(turn finished)",
	}, "\n\n")
	got, err := readSession(db, rt, "session-1", readSessionArgs{To: "w1"})
	if err != nil || got != want {
		t.Fatalf("got %q (%v), want %q", got, err, want)
	}
}
