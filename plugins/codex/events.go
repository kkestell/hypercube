package main

import (
	"encoding/json"
	"fmt"
	"strings"

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
	Result    *toolResult     `json:"result"`
	Error     *struct {
		Message string `json:"message"`
	} `json:"error"`
	ContentItems      []contentBlock `json:"contentItems"`
	Success           *bool          `json:"success"`
	Query             string         `json:"query"`
	Path              string         `json:"path"`
	Prompt            string         `json:"prompt"`
	Model             string         `json:"model"`
	ReceiverThreadIDs []string       `json:"receiverThreadIds"`
	Kind              string         `json:"kind"`
	AgentThreadID     string         `json:"agentThreadId"`
	AgentPath         string         `json:"agentPath"`
}

type fileChange struct {
	Path string          `json:"path"`
	Kind json.RawMessage `json:"kind"`
	Diff string          `json:"diff"`
}

type toolResult struct {
	Content           []contentBlock  `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (ag *agent) notification(ev *harness.Event, m plugin.Message) {
	if m.Method == "mcpServer/startupStatus/updated" {
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
	}
	var n struct {
		ThreadID     string          `json:"threadId"`
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
	parent, known := ag.children[n.ThreadID]
	if !main && !known {
		return
	}
	switch m.Method {
	case "turn/started":
		if main && n.Turn != nil {
			ag.startTurn(ev, n.Turn.ID)
		}
	case "turn/completed":
		// A native subagent's turns do not change the agent's state.
		if main && n.Turn != nil {
			msg := ""
			if e := n.Turn.Error; e != nil {
				msg = strings.TrimSpace(e.Message + "\n" + e.Details)
			}
			ag.endTurn(ev, n.Turn.ID, n.Turn.Status, msg)
		}
	case "item/started":
		if n.Item != nil {
			ag.item(ev, parent, *n.Item, false)
		}
	case "item/completed":
		if n.Item != nil {
			ag.item(ev, parent, *n.Item, true)
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

// startTurn marks a main turn as running. ag.mu is held.
func (ag *agent) startTurn(ev *harness.Event, id string) {
	if id == "" || id == ag.turn || id == ag.ended {
		return
	}
	ag.turn = id
	ev.State = harness.Working
}

// endTurn finishes a main turn: nothing is left streaming or waiting. The
// agent is idle unless a newer turn has started. An interrupted turn is the
// user's own doing; a failed one is an error.
func (ag *agent) endTurn(ev *harness.Event, id, status, message string) {
	ag.ended = id
	if ag.turn == id || ag.turn == "" {
		ag.turn = ""
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
		ev.Items = append(ev.Items, ag.closeRequests(func(*request) bool { return true }, false)...)
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
	ag.errors++
	ev.Items = append(ev.Items, harness.Item{ID: fmt.Sprintf("error-%d", ag.errors), Kind: harness.KindError, Text: text, Done: true})
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
func (ag *agent) item(ev *harness.Event, parent string, it threadItem, completed bool) {
	if it.ID == "" || ag.finished[it.ID] {
		return
	}
	switch it.Type {
	case "agentMessage":
		if completed {
			ag.complete(ev, harness.Item{ID: it.ID, Parent: parent, Kind: harness.KindText, Text: it.Text})
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
			ag.children[it.AgentThreadID] = it.ID
			ag.tool(ev, parent, it, completed)
		}
	case "collabAgentToolCall":
		for _, thread := range it.ReceiverThreadIDs {
			ag.children[thread] = it.ID
		}
		ag.tool(ev, parent, it, completed)
	case "commandExecution", "fileChange", "mcpToolCall", "dynamicToolCall", "webSearch", "imageView":
		ag.tool(ev, parent, it, completed)
	}
}

// complete finishes an item. A message that streamed has its text replaced by
// the final text; an empty one that never streamed is not shown.
func (ag *agent) complete(ev *harness.Event, item harness.Item) {
	_, streamed := ag.open[item.ID]
	delete(ag.open, item.ID)
	delete(ag.running, item.ID)
	ag.finished[item.ID] = true
	if item.Kind != harness.KindTool && item.Text == "" && !streamed {
		return
	}
	item.Done = true
	ev.Items = append(ev.Items, item)
}

func (ag *agent) tool(ev *harness.Event, parent string, it threadItem, completed bool) {
	item := harness.Item{ID: it.ID, Parent: parent, Kind: harness.KindTool}
	var input any
	switch it.Type {
	case "commandExecution":
		item.Tool, input = "shell", map[string]any{"command": it.Command, "cwd": it.Cwd}
		if it.Output != nil {
			item.Output = *it.Output
		}
		item.Failed = it.Status == "failed" || it.Status == "declined" || it.ExitCode != nil && *it.ExitCode != 0
	case "fileChange":
		paths := make([]string, len(it.Changes))
		for i, c := range it.Changes {
			paths[i] = c.Path
		}
		item.Tool, input = "apply_patch", map[string]any{"path": strings.Join(paths, ", "), "changes": it.Changes}
		item.Failed = it.Status == "failed" || it.Status == "declined"
	case "mcpToolCall":
		item.Tool = "mcp__" + it.Server + "__" + it.Tool
		item.Input = it.Arguments
		item.Failed = it.Status == "failed" || it.Error != nil
		switch {
		case it.Error != nil:
			item.Output = it.Error.Message
		case it.Result != nil:
			item.Output = blocksText(it.Result.Content)
			if item.Output == "" && len(it.Result.StructuredContent) > 0 {
				item.Output = string(it.Result.StructuredContent)
			}
		}
	case "dynamicToolCall":
		item.Tool = it.Tool
		if it.Namespace != "" {
			item.Tool = it.Namespace + "__" + it.Tool
		}
		item.Input = it.Arguments
		item.Output = blocksText(it.ContentItems)
		item.Failed = it.Status == "failed" || it.Success != nil && !*it.Success
	case "webSearch":
		item.Tool, input = "web_search", map[string]any{"query": it.Query}
	case "imageView":
		item.Tool, input = "view_image", map[string]any{"path": it.Path}
	case "subAgentActivity":
		item.Tool, input = "subagent", map[string]any{"agent": it.AgentPath, "model": it.Model}
	case "collabAgentToolCall":
		item.Tool, input = "agent_"+it.Tool, map[string]any{"prompt": it.Prompt}
		item.Failed = it.Status == "failed"
	}
	if input != nil {
		item.Input = json.RawMessage(plugin.JSONString(input))
	}
	if it.Status == "declined" && item.Output == "" {
		item.Output = "Declined."
	}
	if completed {
		ag.complete(ev, item)
		return
	}
	ag.running[it.ID] = item
	ev.Items = append(ev.Items, item)
}

func blocksText(blocks []contentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
