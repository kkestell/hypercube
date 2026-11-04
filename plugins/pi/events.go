package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// sessionStats is Pi's total usage, including compaction and tool results.
type sessionStats struct {
	Cost         float64 `json:"cost"`
	ContextUsage *struct {
		Tokens int `json:"tokens"`
	} `json:"contextUsage"`
}

// block is a text or reasoning block of an assistant message. It is shown
// once it has text beyond whitespace.
type block struct {
	kind  string
	text  string
	shown bool
}

// assistant is what a finished assistant message says about how it ended, and
// the text items it showed.
type assistant struct {
	stop, err string
	texts     []string
}

func (ag *agent) send(ev harness.Event) {
	if ev.Conversation != "" || ev.State != "" || len(ev.Items) > 0 || ev.Usage != nil {
		ag.emit(ev)
	}
}

// line translates one line of pi's output.
func (ag *agent) line(line []byte) {
	var h struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(line, &h); err != nil {
		fmt.Fprintf(os.Stderr, "hypercube-harness-pi: unreadable pi output: %v\n", err)
		return
	}
	if h.Type == "response" {
		ag.response(line, h.ID)
		return
	}
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	switch h.Type {
	case "agent_start":
		ev.State = harness.Working
		ag.texts = nil
	case "agent_settled":
		ag.settled(&ev, line)
	case "message_start":
		var m struct {
			Message struct{ Role string } `json:"message"`
		}
		if json.Unmarshal(line, &m) == nil && m.Message.Role == "assistant" {
			ag.messages++
			ag.texts = nil
		}
	case "message_update":
		ag.update(&ev, line)
	case "message_end":
		ag.messageEnd(&ev, line)
	case "tool_execution_start":
		ag.toolStart(&ev, line)
	case "tool_execution_end":
		ag.toolEnd(&ev, line)
	case "extension_ui_request":
		ag.dialog(&ev, line)
	case "extension_error":
		var m struct{ Error string }
		if json.Unmarshal(line, &m) == nil {
			ag.fail(&ev, "Extension error: "+m.Error)
		}
	}
	ag.send(ev)
}

func (ag *agent) fail(ev *harness.Event, text string) {
	ev.Items = append(ev.Items, ag.errs.Item(text))
}

// settled ends a run: Pi has nothing more to do on its own. A run that ended
// in a provider error says so; an aborted one is the user's own doing.
func (ag *agent) settled(ev *harness.Event, line []byte) {
	var m struct{ Aborted bool }
	_ = json.Unmarshal(line, &m)
	last := ag.last
	ag.last = assistant{}
	if last.stop == "error" && !m.Aborted {
		text := last.err
		if text == "" {
			text = "The model request failed."
		}
		ag.fail(ev, text)
	}
	// Only a message that ended normally in an uninterrupted run is a response.
	if !m.Aborted && last.stop == "stop" {
		ev.Final = last.texts
	}
	ev.State = harness.Idle
	// The reader must keep draining Pi's output to receive the response.
	if err := ag.write(map[string]any{"id": statsID, "type": "get_session_stats"}); err != nil {
		ag.fail(ev, err.Error())
	}
}

// stats refreshes the total after a run, including usage outside assistant messages.
func (ag *agent) stats(r response) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	var stats sessionStats
	if r.err != "" {
		ag.fail(&ev, "get_session_stats: "+r.err)
	} else if err := json.Unmarshal(r.data, &stats); err != nil {
		ag.fail(&ev, "get_session_stats: "+err.Error())
	} else {
		ag.cost = stats.Cost
		ev.Usage = &harness.Usage{Context: ag.context, Window: ag.window, Cost: ag.cost}
	}
	ag.send(ev)
}

// update shows text and reasoning as they arrive.
func (ag *agent) update(ev *harness.Event, line []byte) {
	var m struct {
		Update struct {
			Type    string `json:"type"`
			Index   int    `json:"contentIndex"`
			Delta   string `json:"delta"`
			Content string `json:"content"`
		} `json:"assistantMessageEvent"`
	}
	if json.Unmarshal(line, &m) != nil {
		return
	}
	u := m.Update
	id := fmt.Sprintf("m%d:%d", ag.messages, u.Index)
	switch u.Type {
	case "text_start":
		ag.blocks[id] = &block{kind: harness.KindText}
	case "thinking_start":
		ag.blocks[id] = &block{kind: harness.KindReasoning}
	case "text_delta", "thinking_delta":
		b := ag.blocks[id]
		if b == nil {
			return
		}
		b.text += u.Delta
		if strings.TrimSpace(b.text) == "" {
			return
		}
		item := harness.Item{ID: id, Kind: b.kind, Text: u.Delta, Append: true}
		if !b.shown {
			item.Text, item.Append = b.text, false
			b.shown = true
		}
		ev.Items = append(ev.Items, item)
	case "text_end", "thinking_end":
		b := ag.blocks[id]
		if b == nil {
			return
		}
		delete(ag.blocks, id)
		if b.shown || strings.TrimSpace(u.Content) != "" {
			ev.Items = append(ev.Items, harness.Item{ID: id, Kind: b.kind, Text: u.Content, Done: true})
			if b.kind == harness.KindText {
				ag.texts = append(ag.texts, id)
			}
		}
	}
}

// messageEnd records a finished assistant message: how it ended, and the
// usage of its request. Pi saves the session from its first message.
func (ag *agent) messageEnd(ev *harness.Event, line []byte) {
	var m struct {
		Message struct {
			Role         string `json:"role"`
			StopReason   string `json:"stopReason"`
			ErrorMessage string `json:"errorMessage"`
			Usage        struct {
				TotalTokens int `json:"totalTokens"`
				Cost        struct {
					Total float64 `json:"total"`
				} `json:"cost"`
			} `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &m) != nil {
		return
	}
	switch m.Message.Role {
	case "user":
	case "assistant":
		// A message cut short has no end for its blocks.
		for id, b := range ag.blocks {
			if b.shown {
				ev.Items = append(ev.Items, harness.Item{ID: id, Kind: b.kind, Append: true, Done: true})
			}
			delete(ag.blocks, id)
		}
		ag.last = assistant{stop: m.Message.StopReason, err: m.Message.ErrorMessage, texts: ag.texts}
		ag.cost += m.Message.Usage.Cost.Total
		if t := m.Message.Usage.TotalTokens; t > 0 {
			ag.context = t
		}
		ev.Usage = &harness.Usage{Context: ag.context, Window: ag.window, Cost: ag.cost}
	default:
		return
	}
	if !ag.announced {
		ag.announced = true
		ev.Conversation = ag.session
	}
}

func (ag *agent) toolStart(ev *harness.Event, line []byte) {
	var m struct {
		ID   string          `json:"toolCallId"`
		Name string          `json:"toolName"`
		Args json.RawMessage `json:"args"`
	}
	if json.Unmarshal(line, &m) != nil {
		return
	}
	item := harness.Item{ID: m.ID, Kind: harness.KindTool, Input: m.Args}
	item.Server, item.Tool = plugin.MCPTool(m.Name)
	ag.tools[m.ID] = item
	ev.Items = append(ev.Items, item)
}

func (ag *agent) toolEnd(ev *harness.Event, line []byte) {
	var m struct {
		ID     string `json:"toolCallId"`
		Result struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Data     string `json:"data"`
				MimeType string `json:"mimeType"`
			} `json:"content"`
		} `json:"result"`
		IsError bool `json:"isError"`
	}
	if json.Unmarshal(line, &m) != nil {
		return
	}
	item, ok := ag.tools[m.ID]
	if !ok {
		return
	}
	delete(ag.tools, m.ID)
	var parts []string
	for _, c := range m.Result.Content {
		switch c.Type {
		case "text":
			parts = append(parts, c.Text)
		case "image":
			item.Images = append(item.Images, harness.Image{MIMEType: c.MimeType, Data: c.Data})
		default:
			parts = append(parts, "["+c.Type+"]")
		}
	}
	item.Output, item.Failed, item.Done = strings.Join(parts, "\n"), m.IsError, true
	ev.Items = append(ev.Items, item)
}
