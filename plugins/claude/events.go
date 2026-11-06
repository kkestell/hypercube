package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// answer is the main agent's newest message and the text items its blocks
// became. A successful result settles it as the response to a turn.
type answer struct {
	message string
	texts   []string
	settled bool
}

// message is the part of a stream line the translation reads.
type message struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	State     string          `json:"state"`
	SessionID string          `json:"session_id"`
	Parent    string          `json:"parent_tool_use_id"`
	Model     string          `json:"model"`
	APIID     string          `json:"api_message_id"`
	Event     json.RawMessage `json:"event"`
	Message   struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   struct {
			Input         int `json:"input_tokens"`
			CacheRead     int `json:"cache_read_input_tokens"`
			CacheCreation int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
	Response  struct {
		Subtype   string          `json:"subtype"`
		RequestID string          `json:"request_id"`
		Error     string          `json:"error"`
		Response  json.RawMessage `json:"response"`
	} `json:"response"`
	IsError        bool    `json:"is_error"`
	Result         string  `json:"result"`
	Errors         []any   `json:"errors"`
	TerminalReason string  `json:"terminal_reason"`
	Cost           float64 `json:"total_cost_usd"`
	ModelUsage     map[string]struct {
		ContextWindow int `json:"contextWindow"`
	} `json:"modelUsage"`
	RateLimitInfo struct {
		UnifiedWindows map[string]struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    int64   `json:"resetsAt"`
		} `json:"unifiedWindows"`
	} `json:"rate_limit_info"`
}

// block is a content block of an assistant or user message.
type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Source    struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source"`
}

// image is the picture of an image block, if it is an inline one.
func (b block) image() (harness.Image, bool) {
	if b.Type != "image" || b.Source.Type != "base64" || b.Source.Data == "" || !strings.HasPrefix(b.Source.MediaType, "image/") {
		return harness.Image{}, false
	}
	return harness.Image{MIMEType: b.Source.MediaType, Data: b.Source.Data}, true
}

// line translates one line of claude's output.
func (c *claude) line(line []byte) {
	var m message
	if err := json.Unmarshal(line, &m); err != nil {
		fmt.Fprintf(os.Stderr, "hypercube-harness-claude: unreadable claude output: %v\n", err)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var ev harness.Event
	switch m.Type {
	case "system":
		switch m.Subtype {
		case "init":
			c.model = m.Model
		case "session_state_changed":
			switch m.State {
			case "running":
				ev.State = harness.Working
			case "idle":
				ev.State, ev.Final = harness.Idle, c.final
				c.final = nil
			}
		}
	case "stream_event":
		if m.Parent == "" {
			c.mainMessage(m.APIID)
			c.stream(&ev, m)
		}
	case "assistant":
		c.assistant(&ev, m)
	case "user":
		c.toolResults(&ev, m)
	case "control_response":
		if pending := c.configuring; pending != nil && m.Response.RequestID == pending.id {
			var err error
			if m.Response.Subtype == "error" {
				err = errors.New(m.Response.Error)
			} else {
				c.model = pending.model
			}
			pending.reply <- err
			c.configuring = nil
		}
	case "control_request":
		c.control(&ev, m)
	case "control_cancel_request":
		items, _ := c.requests.Close(func(r *request) bool { return r.id == m.RequestID })
		ev.Items = append(ev.Items, items...)
	case "result":
		c.result(&ev, m)
	case "rate_limit_event":
		ev.Limits = limits(m)
	}
	if ev.Conversation != "" || ev.State != "" || len(ev.Items) > 0 || ev.Usage != nil || len(ev.Limits) > 0 {
		c.emit(ev)
	}
}

// stream shows text and reasoning as they arrive. Claude's complete blocks
// follow and finish the items.
func (c *claude) stream(ev *harness.Event, m message) {
	var e struct {
		Type  string `json:"type"`
		Index int    `json:"index"`
		Delta struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"delta"`
	}
	if json.Unmarshal(m.Event, &e) != nil || e.Type != "content_block_delta" || m.APIID == "" {
		return
	}
	item := harness.Item{ID: fmt.Sprintf("%s:%d", m.APIID, e.Index), Append: true}
	switch e.Delta.Type {
	case "text_delta":
		item.Kind, item.Text = harness.KindText, e.Delta.Text
	case "thinking_delta":
		item.Kind, item.Text = harness.KindReasoning, e.Delta.Thinking
	}
	if item.Text == "" {
		return
	}
	c.open[item.ID] = harness.Item{ID: item.ID, Kind: item.Kind}
	ev.Items = append(ev.Items, item)
}

// mainMessage notes a record of the main agent's message. A new message
// supersedes a response that an earlier result confirmed.
func (c *claude) mainMessage(id string) {
	if id != "" && id != c.latest.message {
		c.latest = answer{message: id}
		c.final = nil
	}
}

func (c *claude) assistant(ev *harness.Event, m message) {
	var blocks []block
	if json.Unmarshal(m.Message.Content, &blocks) != nil {
		return
	}
	if m.Parent == "" {
		c.mainMessage(m.Message.ID)
		if m.Message.Model != "" {
			c.model = m.Message.Model
		}
	}
	for _, b := range blocks {
		id := fmt.Sprintf("%s:%d", m.Message.ID, c.blocks[m.Message.ID])
		c.blocks[m.Message.ID]++
		_, streamed := c.open[id]
		delete(c.open, id)
		switch b.Type {
		case "text":
			if streamed || strings.TrimSpace(b.Text) != "" {
				ev.Items = append(ev.Items, harness.Item{ID: id, Parent: m.Parent, Kind: harness.KindText, Text: b.Text, Done: true})
				if m.Parent == "" {
					c.latest.texts = append(c.latest.texts, id)
				}
			}
		case "thinking":
			if streamed || strings.TrimSpace(b.Thinking) != "" {
				ev.Items = append(ev.Items, harness.Item{ID: id, Parent: m.Parent, Kind: harness.KindReasoning, Text: b.Thinking, Done: true})
			}
		case "tool_use":
			// AskUserQuestion is shown as its attention requests.
			if b.Name == "AskUserQuestion" {
				continue
			}
			item := harness.Item{ID: b.ID, Parent: m.Parent, Kind: harness.KindTool, Input: b.Input}
			item.Server, item.Tool = plugin.MCPTool(b.Name)
			c.tools[b.ID] = item
			ev.Items = append(ev.Items, item)
		}
	}
	// A native subagent's requests do not count toward the agent's context.
	if u := m.Message.Usage; m.Parent == "" && u.Input+u.CacheRead+u.CacheCreation > 0 {
		c.usage.Context = u.Input + u.CacheRead + u.CacheCreation
		usage := c.usage
		ev.Usage = &usage
	}
}

func (c *claude) toolResults(ev *harness.Event, m message) {
	var blocks []block
	if json.Unmarshal(m.Message.Content, &blocks) != nil {
		return // a prompt, which Hypercube already shows
	}
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
		}
		item, ok := c.tools[b.ToolUseID]
		if !ok {
			continue
		}
		delete(c.tools, b.ToolUseID)
		item.Output, item.Images = resultText(b.Content)
		item.Failed, item.Done = b.IsError, true
		ev.Items = append(ev.Items, item)
	}
}

// resultText is the text and the images of a tool result, which claude sends
// as a string or as blocks.
func resultText(raw json.RawMessage) (string, []harness.Image) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return string(raw), nil
	}
	var parts []string
	var images []harness.Image
	for _, b := range blocks {
		if img, ok := b.image(); ok {
			images = append(images, img)
		} else if b.Type == "text" {
			parts = append(parts, b.Text)
		} else {
			parts = append(parts, "["+b.Type+"]")
		}
	}
	return strings.Join(parts, "\n"), images
}

// windows are the lengths in minutes of the subscription windows claude
// reports, by its name for each.
var windows = []struct {
	name    string
	minutes int
}{{"five_hour", 300}, {"seven_day", 10080}}

// limits are the windows a rate limit event reports, as seen now.
func limits(m message) []harness.Limit {
	at := time.Now().UnixMilli()
	var limits []harness.Limit
	for _, w := range windows {
		if u, ok := m.RateLimitInfo.UnifiedWindows[w.name]; ok {
			limits = append(limits, harness.Limit{Minutes: w.minutes, Used: int(math.Round(u.Utilization * 100)), Resets: u.ResetsAt, At: at})
		}
	}
	return limits
}

// summary is what a result line says about how its turn ended.
func (m message) summary() string {
	if m.Result != "" {
		return m.Result
	}
	if len(m.Errors) > 0 {
		return fmt.Sprint(m.Errors...)
	}
	return m.Subtype
}

// result ends a turn: nothing is left streaming or waiting, and the turn's
// usage is final.
func (c *claude) result(ev *harness.Event, m message) {
	// Claude names its session at startup but saves it only once a turn
	// runs, so the conversation is reported from a turn's result; an agent
	// that never took a turn has nothing to resume.
	if m.SessionID != "" && m.SessionID != c.conversation {
		c.conversation = m.SessionID
		ev.Conversation = m.SessionID
	}
	for id, it := range c.open {
		it.Append, it.Done = true, true
		ev.Items = append(ev.Items, it)
		delete(c.open, id)
	}
	items, _ := c.requests.Close(func(*request) bool { return true })
	ev.Items = append(ev.Items, items...)
	// A successful result confirms the newest main message as the response; a
	// result with no newer message, such as a bookkeeping one, leaves it.
	switch {
	case m.IsError:
		c.final = nil
		c.latest.settled = true
	case !c.latest.settled:
		c.final = slices.Clone(c.latest.texts)
		c.latest.settled = true
	}
	// An interrupted turn is the user's own doing.
	if m.IsError && !strings.HasPrefix(m.TerminalReason, "aborted") {
		ev.Items = append(ev.Items, c.errors.Item(m.summary()))
	}
	window := m.ModelUsage[c.model].ContextWindow
	if len(m.ModelUsage) == 1 {
		for _, u := range m.ModelUsage {
			window = u.ContextWindow
		}
	}
	if window > 0 {
		c.usage.Window = window
	}
	c.usage.Cost = m.Cost
	usage := c.usage
	ev.Usage = &usage
}
