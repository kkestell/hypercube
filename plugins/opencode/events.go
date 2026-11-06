package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// ended finishes a turn: nothing is left streaming or waiting. The agent is
// idle once no prompt is in flight and no input waits.
func (ag *agent) ended(stopReason string, err error) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	for id, it := range ag.open {
		it.Append, it.Done = true, true
		ev.Items = append(ev.Items, it)
		delete(ag.open, id)
	}
	ev.Items = append(ev.Items, ag.closeRequests()...)
	stopping := ag.ctx.Err() != nil
	select {
	case <-ag.rpc.Done():
		stopping = true // Serve reports the process's exit reason.
	default:
	}
	// A cancelled turn is the user's own doing.
	text := ""
	switch {
	case stopping:
		// The agent is stopping.
	case err != nil:
		text = err.Error()
	case stopReason == "max_tokens":
		text = "The model stopped at its output limit."
	case stopReason == "refusal":
		text = "The model refused the request."
	}
	if text != "" {
		ev.Items = append(ev.Items, ag.errors.Item(text))
	}
	ev.State, ev.InputPending = harness.Idle, len(ag.queued) > 0
	// Each successful prompt has its own final, even when the next prompt
	// is already queued. InputPending keeps the host from treating this
	// transition as a chance to resume the parent.
	if !stopping && err == nil && stopReason == "end_turn" && ag.last != "" {
		ev.Final = []string{ag.last}
	}
	ag.emit(ev)
	if len(ag.queued) > 0 {
		blocks := ag.queued
		ag.queued = nil
		ag.emit(harness.Event{State: harness.Working})
		ag.prompt(blocks)
	} else {
		ag.busy = false
	}
}

// handle takes what opencode sends: updates, permission requests, and questions
// for the user. It runs on the connection's reading goroutine, so it answers
// what it can at once and leaves a form to be answered by the user.
func (ag *agent) handle(m plugin.Message) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	switch {
	case m.Method == "session/update":
		ag.update(&ev, m.Params)
	case m.Method == "$/cancel_request":
		ag.withdraw(&ev, m.Params)
	case m.ID == nil:
		// Other notifications carry nothing the plugin shows.
	case m.Method == "session/request_permission":
		result, err := permission(m.Params)
		if err != nil {
			_ = ag.rpc.RespondError(m.ID, -32602, err.Error())
		} else {
			_ = ag.rpc.Respond(m.ID, result)
		}
	case m.Method == "elicitation/create":
		ag.form(&ev, m.ID, m.Params)
	default:
		_ = ag.rpc.RespondError(m.ID, -32601, "Method not found")
	}
	if len(ev.Items) > 0 || ev.Usage != nil {
		ag.emit(ev)
	}
}

// sessionUpdate is the part of a session/update notification that the plugin
// reads. Content is one content block for a chunk and a list of tool call
// content for a tool call.
type sessionUpdate struct {
	SessionID string `json:"sessionId"`
	Update    struct {
		SessionUpdate string          `json:"sessionUpdate"`
		MessageID     string          `json:"messageId"`
		Content       json.RawMessage `json:"content"`
		ToolCallID    string          `json:"toolCallId"`
		Name          string          `json:"name"`
		Status        string          `json:"status"`
		RawInput      json.RawMessage `json:"rawInput"`
		Used          int             `json:"used"`
		Size          int             `json:"size"`
		Cost          *struct {
			Amount float64 `json:"amount"`
		} `json:"cost"`
		Meta struct {
			Child json.RawMessage `json:"opencode/child-session"`
		} `json:"_meta"`
	} `json:"update"`
}

// contentBlock is a block of a message or of a tool's result.
type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

// toolContent is an entry of a tool call's content: a content block, or
// another type such as a diff.
type toolContent struct {
	Type    string       `json:"type"`
	Content contentBlock `json:"content"`
}

func (ag *agent) update(ev *harness.Event, params json.RawMessage) {
	var p sessionUpdate
	if json.Unmarshal(params, &p) != nil || p.SessionID != ag.session {
		return
	}
	u := p.Update
	// ACP does not identify the tool that launched a child. Keep its output
	// out of the main transcript rather than guessing its parent from a title.
	if len(u.Meta.Child) > 0 && string(u.Meta.Child) != "null" {
		return
	}
	switch u.SessionUpdate {
	case "agent_message_chunk", "agent_thought_chunk":
		kind := harness.KindText
		if u.SessionUpdate == "agent_thought_chunk" {
			kind = harness.KindReasoning
		}
		var content contentBlock
		if json.Unmarshal(u.Content, &content) != nil {
			return
		}
		ag.chunk(ev, kind, u.MessageID, content)
	case "tool_call", "tool_call_update":
		var content []toolContent
		if len(u.Content) > 0 && json.Unmarshal(u.Content, &content) != nil {
			return
		}
		ag.toolCall(ev, u.ToolCallID, u.Name, u.Status, u.RawInput, content)
	case "usage_update":
		if u.Used == 0 {
			return
		}
		usage := harness.Usage{Context: u.Used, Window: u.Size}
		if u.Cost != nil {
			usage.Cost = u.Cost.Amount
		}
		ev.Usage = &usage
	}
}

// chunk extends a streamed message or reasoning item.
func (ag *agent) chunk(ev *harness.Event, kind, messageID string, content contentBlock) {
	if content.Type != "text" || content.Text == "" {
		return
	}
	id := fmt.Sprintf("%s:%d", kind, ag.turn)
	if messageID != "" {
		id = messageID
	}
	if kind == harness.KindText {
		ag.last = messageID
	}
	ev.Items = append(ev.Items, harness.Item{ID: id, Kind: kind, Text: content.Text, Append: true})
	ag.open[id] = harness.Item{ID: id, Kind: kind}
}

// toolCall reports a tool call or an update to it. OpenCode names the tool in
// a field of its own, and its input is kept as OpenCode wrote it.
func (ag *agent) toolCall(ev *harness.Event, id, name, status string, input json.RawMessage, content []toolContent) {
	if id == "" {
		return
	}
	t, known := ag.tools[id]
	if !known {
		t = harness.Item{ID: id, Kind: harness.KindTool}
		// Text before a tool call is progress.
		ag.last = ""
	}
	if t.Done {
		return
	}
	if name != "" {
		t.Tool = name
		// OpenCode names an MCP tool <server>_<tool>.
		if tool, ok := strings.CutPrefix(name, "hypercube_"); ok {
			t.Server, t.Tool = "hypercube", tool
		}
	}
	if len(input) > 0 {
		t.Input = input
	}
	if status == "completed" || status == "failed" {
		t.Done, t.Failed = true, status == "failed"
		t.Output, t.Images = toolOutput(content)
	}
	if t.Done {
		// Remember completion to ignore later updates without retaining payloads.
		ag.tools[id] = harness.Item{Done: true}
	} else {
		ag.tools[id] = t
	}
	ev.Items = append(ev.Items, t)
}

// toolOutput is the text and images of a tool's result. An edit is shown as a
// diff, which has neither.
func toolOutput(content []toolContent) (string, []harness.Image) {
	var parts []string
	var images []harness.Image
	for _, c := range content {
		if c.Type != "content" {
			continue
		}
		switch b := c.Content; b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "image":
			images = append(images, harness.Image{MIMEType: b.MIMEType, Data: b.Data})
		}
	}
	return strings.Join(parts, "\n"), images
}
