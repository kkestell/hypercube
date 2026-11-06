package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// handle takes what codex sends: notifications, and requests for the user.
func (ag *agent) handle(m plugin.Message) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	if len(m.ID) > 0 {
		ag.serverRequest(&ev, m)
	} else {
		ag.notification(&ev, m)
	}
	ag.send(ev)
}

// rateLimits merges sparse updates into the account's general windows.
// A model-specific quota has its own limit ID and is left out.
func (ag *agent) rateLimits(params json.RawMessage) []harness.Limit {
	type window struct {
		UsedPercent        int    `json:"usedPercent"`
		WindowDurationMins *int   `json:"windowDurationMins"`
		ResetsAt           *int64 `json:"resetsAt"`
	}
	var p struct {
		RateLimits struct {
			LimitID   *string `json:"limitId"`
			Primary   *window `json:"primary"`
			Secondary *window `json:"secondary"`
		} `json:"rateLimits"`
	}
	if json.Unmarshal(params, &p) != nil || p.RateLimits.LimitID != nil && *p.RateLimits.LimitID != "codex" {
		return nil
	}
	at := time.Now().UnixMilli()
	var limits []harness.Limit
	for i, w := range []*window{p.RateLimits.Primary, p.RateLimits.Secondary} {
		if w == nil {
			continue
		}
		l := ag.limits[i]
		if w.WindowDurationMins != nil {
			if l.Minutes != 0 && l.Minutes != *w.WindowDurationMins {
				l.Resets = 0
			}
			l.Minutes = *w.WindowDurationMins
		}
		l.Used, l.At = w.UsedPercent, at
		if w.ResetsAt != nil {
			l.Resets = *w.ResetsAt
		}
		ag.limits[i] = l
		if l.Minutes > 0 {
			limits = append(limits, l)
		}
	}
	return limits
}

// threadItem is the part of an item the translation reads. Fields that differ
// in shape between item types stay raw.
type threadItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Summary   []string        `json:"summary"`
	Content   json.RawMessage `json:"content"`
	Command   string          `json:"command"`
	Cwd       string          `json:"cwd"`
	Output    *string         `json:"aggregatedOutput"`
	ExitCode  *int            `json:"exitCode"`
	Status    string          `json:"status"`
	Changes   []fileChange    `json:"changes"`
	Server    string          `json:"server"`
	Namespace string          `json:"namespace"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result"` // an MCP call's toolResult, or an image generation's base64 data
	Error     *struct {
		Message string `json:"message"`
	} `json:"error"`
	ContentItems      []contentBlock  `json:"contentItems"`
	Success           *bool           `json:"success"`
	Query             string          `json:"query"`
	Path              string          `json:"path"`
	SavedPath         string          `json:"savedPath"`
	Failure           json.RawMessage `json:"failure"`
	Prompt            string          `json:"prompt"`
	Model             string          `json:"model"`
	ReceiverThreadIDs []string        `json:"receiverThreadIds"`
	Kind              string          `json:"kind"`
	AgentThreadID     string          `json:"agentThreadId"`
	AgentPath         string          `json:"agentPath"`
	Phase             string          `json:"phase"`
	Delivery          string          `json:"delivery"`
	Questions         []struct {
		Title   string   `json:"title"`
		Options []string `json:"options"`
	} `json:"questions"`
}

func (ag *agent) notification(ev *harness.Event, m plugin.Message) {
	switch m.Method {
	case "mcpServer/startupStatus/updated":
		var p struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		// Without its server, the agent cannot message its team.
		if json.Unmarshal(m.Params, &p) == nil && p.Name == "hypercube" && p.Status == "failed" {
			ag.fail(ev, "The hypercube MCP server failed to start: "+p.Error)
		}
		return
	case "account/rateLimits/updated":
		ev.Limits = ag.rateLimits(m.Params)
		return
	}
	var n struct {
		ThreadID     string          `json:"threadId"`
		TurnID       string          `json:"turnId"`
		ItemID       string          `json:"itemId"`
		Delta        string          `json:"delta"`
		SummaryIndex int             `json:"summaryIndex"`
		RequestID    json.RawMessage `json:"requestId"`
		Item         *threadItem     `json:"item"`
		Turn         *struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
				Details string `json:"additionalDetails"`
			} `json:"error"`
		} `json:"turn"`
		TokenUsage *struct {
			Last struct {
				Input int `json:"inputTokens"`
			} `json:"last"`
			Window int `json:"modelContextWindow"`
		} `json:"tokenUsage"`
	}
	if json.Unmarshal(m.Params, &n) != nil {
		return
	}
	if m.Method == "serverRequest/resolved" {
		// The request ended without an answer from us.
		ev.Items = append(ev.Items, ag.closeRequests(func(r *request) bool { return string(r.id) == string(n.RequestID) }, false)...)
		return
	}
	main := n.ThreadID == ag.thread
	child, known := ag.children[n.ThreadID]
	parent := child.parent
	if !main && !known {
		return
	}
	switch m.Method {
	case "turn/started":
		if n.Turn != nil {
			ev.Items = append(ev.Items, ag.closeAsyncQuestions(n.ThreadID)...)
			if main {
				ag.startTurn(ev, n.Turn.ID)
			} else if n.Turn.ID != child.ended {
				child.turn = n.Turn.ID
				ag.children[n.ThreadID] = child
			}
		}
	case "turn/completed":
		// A native subagent's turns do not change the agent's state.
		if main && n.Turn != nil {
			msg := ""
			if e := n.Turn.Error; e != nil {
				msg = strings.TrimSpace(e.Message + "\n" + e.Details)
			}
			ag.endTurn(ev, n.Turn.ID, n.Turn.Status, msg)
		} else if n.Turn != nil {
			child.ended = n.Turn.ID
			if child.turn == n.Turn.ID {
				child.turn = ""
			}
			ag.children[n.ThreadID] = child
		}
	case "item/started":
		if n.Item != nil {
			ag.item(ev, n.ThreadID, parent, *n.Item, false)
		}
	case "item/completed":
		if n.Item != nil {
			if n.Item.Type == "userMessage" {
				ev.Items = append(ev.Items, ag.closeAsyncQuestions(n.ThreadID)...)
			}
			ag.item(ev, n.ThreadID, parent, *n.Item, true)
			ag.answered(main, n.TurnID, *n.Item)
		}
	case "item/agentMessage/delta":
		ag.stream(ev, parent, harness.KindText, n.ItemID, n.Delta)
	case "item/reasoning/summaryTextDelta":
		ag.stream(ev, parent, harness.KindReasoning, n.ItemID, n.Delta)
	case "item/reasoning/summaryPartAdded":
		if n.SummaryIndex > 0 {
			ag.stream(ev, parent, harness.KindReasoning, n.ItemID, "\n\n")
		}
	case "thread/tokenUsage/updated":
		// A native subagent's requests do not count toward the agent's context.
		if main && n.TokenUsage != nil && n.TokenUsage.Last.Input > 0 {
			ev.Usage = &harness.Usage{Context: n.TokenUsage.Last.Input, Window: n.TokenUsage.Window}
		}
	}
}

// answered notes a main message that Codex marks as the turn's final answer.
// Messages of an earlier turn that complete late do not count.
func (ag *agent) answered(main bool, turn string, it threadItem) {
	if main && it.Type == "agentMessage" && it.Phase == "final_answer" && turn == ag.turn && !slices.Contains(ag.final, it.ID) {
		ag.final = append(ag.final, it.ID)
	}
}

// startTurn marks a main turn as running. ag.mu is held.
func (ag *agent) startTurn(ev *harness.Event, id string) {
	if id == "" || id == ag.turn || id == ag.ended {
		return
	}
	ag.turn = id
	ag.final = nil
	ev.State = harness.Working
}

// endTurn finishes a main turn: nothing is left streaming or waiting. The
// agent is idle unless a newer turn has started, and a completed turn reports
// its final answer with it. An interrupted turn is the user's own doing; a
// failed one is an error.
func (ag *agent) endTurn(ev *harness.Event, id, status, message string) {
	ag.ended = id
	if ag.turn == id || ag.turn == "" {
		ag.turn = ""
		if status == "completed" {
			ev.Final = ag.final
		}
		ag.final = nil
		for id, it := range ag.open {
			it.Append, it.Done = true, true
			ev.Items = append(ev.Items, it)
			delete(ag.open, id)
		}
		for id, it := range ag.running {
			it.Done = true
			ev.Items = append(ev.Items, it)
			ag.finished[id] = true
			delete(ag.running, id)
		}
		ev.Items = append(ev.Items, ag.closeRequests(blockingRequest, false)...)
		ev.State = harness.Idle
	}
	if status == "failed" {
		if message == "" {
			message = "The turn failed."
		}
		ag.fail(ev, message)
	}
}

func (ag *agent) fail(ev *harness.Event, text string) {
	ev.Items = append(ev.Items, ag.errs.Item(text))
}

// stream extends a message or reasoning item with the next piece of its text.
func (ag *agent) stream(ev *harness.Event, parent, kind, id, delta string) {
	if id == "" || delta == "" || ag.finished[id] {
		return
	}
	ag.open[id] = harness.Item{ID: id, Parent: parent, Kind: kind}
	ev.Items = append(ev.Items, harness.Item{ID: id, Parent: parent, Kind: kind, Text: delta, Append: true})
}

// item translates an item that started or completed. Codex may report an
// item's completion more than once; the first one counts.
func (ag *agent) item(ev *harness.Event, thread, parent string, it threadItem, completed bool) {
	if it.ID == "" || ag.finished[it.ID] {
		return
	}
	switch it.Type {
	case "agentMessage":
		if completed {
			ag.agentMessage(ev, thread, parent, it)
		}
	case "reasoning":
		if completed {
			text := strings.Join(it.Summary, "\n\n")
			if text == "" {
				var content []string
				_ = json.Unmarshal(it.Content, &content)
				text = strings.Join(content, "\n\n")
			}
			ag.complete(ev, harness.Item{ID: it.ID, Parent: parent, Kind: harness.KindReasoning, Text: text})
		}
	case "subAgentActivity":
		// The subagent's thread reports its items under this one.
		if it.Kind == "started" && it.AgentThreadID != "" {
			child := ag.children[it.AgentThreadID]
			child.parent = it.ID
			ag.children[it.AgentThreadID] = child
			ag.tool(ev, parent, it, completed)
		}
	case "collabAgentToolCall":
		for _, thread := range it.ReceiverThreadIDs {
			child := ag.children[thread]
			child.parent = it.ID
			ag.children[thread] = child
		}
		ag.tool(ev, parent, it, completed)
	case "imageGeneration":
		if completed {
			ag.generatedImage(ev, parent, it)
		}
	case "commandExecution", "fileChange", "mcpToolCall", "dynamicToolCall", "webSearch", "imageView":
		ag.tool(ev, parent, it, completed)
	}
}

func (ag *agent) agentMessage(ev *harness.Event, thread, parent string, it threadItem) {
	ag.complete(ev, harness.Item{ID: it.ID, Parent: parent, Kind: harness.KindText, Text: it.Text})
	if it.Delivery != "async" {
		return
	}
	for i, q := range it.Questions {
		item := harness.Item{ID: fmt.Sprintf("question:%s/%d", it.ID, i), Parent: parent, Kind: harness.KindAttention, Text: q.Title, Choices: q.Options, Free: true, Async: true}
		r := &request{async: true, thread: thread}
		ag.requests.Add(r, []harness.Item{item})
		ev.Items = append(ev.Items, item)
	}
}

// complete finishes an item. A message that streamed has its text replaced by
// the final text; an empty one that never streamed, and has no images, is not
// shown.
func (ag *agent) complete(ev *harness.Event, item harness.Item) {
	streamed := ag.finish(item.ID)
	if item.Kind != harness.KindTool && item.Text == "" && len(item.Images) == 0 && !streamed {
		return
	}
	item.Done = true
	ev.Items = append(ev.Items, item)
}

// finish records that an item has completed, and reports whether it streamed.
func (ag *agent) finish(id string) (streamed bool) {
	_, streamed = ag.open[id]
	delete(ag.open, id)
	delete(ag.running, id)
	ag.finished[id] = true
	return streamed
}
