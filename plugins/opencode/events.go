package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

// nativeEvent reads only the main session's public stream. Child sessions have
// their own sessionID and cannot finish this agent's turn or enter its output.
type nativeEvent struct {
	Type string `json:"type"`
	Data struct {
		SessionID          string          `json:"sessionID"`
		AssistantMessageID string          `json:"assistantMessageID"`
		ID                 string          `json:"id"`
		InboxID            string          `json:"inboxID"`
		Name               string          `json:"name"`
		Ordinal            int             `json:"ordinal"`
		Delta              string          `json:"delta"`
		Text               string          `json:"text"`
		Input              json.RawMessage `json:"input"`
		Content            []toolContent   `json:"content"`
		Finish             string          `json:"finish"`
		Error              struct {
			Message string `json:"message"`
		} `json:"error"`
		Cost   float64 `json:"cost"`
		Tokens struct {
			Input  int `json:"input"`
			Output int `json:"output"`
			Cache  struct {
				Read  int `json:"read"`
				Write int `json:"write"`
			} `json:"cache"`
		} `json:"tokens"`
		Form form `json:"form"`
	} `json:"data"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
	URI  string `json:"uri"`
	MIME string `json:"mime"`
}

// The event stream includes unrelated domains with different payload shapes
// (for example, instructions.updated has an object-valued delta). Decode only
// events this adapter consumes rather than imposing one schema on all events.
func (e *nativeEvent) UnmarshalJSON(b []byte) error {
	var envelope struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return err
	}
	e.Type = envelope.Type
	switch e.Type {
	case "session.inbox.enqueued", "session.inbox.delivered", "session.inbox.cancelled",
		"session.execution.started", "session.execution.succeeded", "session.execution.failed", "session.execution.interrupted",
		"session.step.started", "session.step.ended", "session.text.delta", "session.text.ended", "session.reasoning.delta", "session.reasoning.ended",
		"session.tool.input.started", "session.tool.input.ended", "session.tool.called", "session.tool.success", "session.tool.failed",
		"session.usage.updated", "permission.asked", "form.created", "form.replied", "form.cancelled":
		return json.Unmarshal(envelope.Data, &e.Data)
	}
	return nil
}

func (ag *agent) handle(e nativeEvent) error {
	if e.Type == "plugin.updated" {
		select {
		case ag.catalogChanged <- struct{}{}:
		default:
		}
		return nil
	}
	// Native children share this private server. Their permissions and
	// questions must still be answered while the parent waits on their tool.
	if e.Type == "permission.asked" {
		// Match ACP's allow-once behavior; configured denials are not asked.
		return ag.server.call(ag.ctx, "POST", sessionPath(e.Data.SessionID)+"/permission/"+e.Data.ID+"/reply", map[string]string{"decision": "once"}, nil)
	}
	if e.Type == "form.created" {
		return ag.form(e.Data.Form)
	}
	if e.Type == "form.replied" || e.Type == "form.cancelled" {
		ag.mu.Lock()
		defer ag.mu.Unlock()
		items, _ := ag.requests.Close(func(r *request) bool { return r.id == e.Data.ID && (e.Type == "form.cancelled" || !r.replying) })
		if len(items) > 0 {
			ag.emit(harness.Event{Items: items})
		}
		return nil
	}
	if e.Data.SessionID != ag.session {
		return nil
	}
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	d := e.Data
	switch e.Type {
	case "session.inbox.enqueued":
		ag.pending[d.InboxID] = true
	case "session.inbox.delivered", "session.inbox.cancelled":
		delete(ag.pending, d.InboxID)
	case "session.execution.started":
		ag.last, ag.finish = nil, ""
		ev.State = harness.Working
	case "session.execution.succeeded", "session.execution.failed", "session.execution.interrupted":
		ev = ag.ended(e)
	case "session.step.started":
		ag.last, ag.finish = nil, ""
	case "session.step.ended":
		ag.finish = d.Finish
		ag.used = d.Tokens.Input + d.Tokens.Output + d.Tokens.Cache.Read + d.Tokens.Cache.Write
		ev.Usage = &harness.Usage{Context: ag.used, Window: ag.window, Cost: ag.cost}
	case "session.text.delta", "session.text.ended", "session.reasoning.delta", "session.reasoning.ended":
		kind := harness.KindText
		if strings.HasPrefix(e.Type, "session.reasoning.") {
			kind = harness.KindReasoning
		}
		id := fmt.Sprintf("%s:%s:%d", d.AssistantMessageID, kind, d.Ordinal)
		it := harness.Item{ID: id, Kind: kind, Text: d.Delta, Append: true}
		if strings.HasSuffix(e.Type, ".ended") {
			it.Text, it.Append, it.Done = d.Text, false, true
			delete(ag.open, id)
		} else {
			ag.open[id] = harness.Item{ID: id, Kind: kind}
		}
		if kind == harness.KindText && !slices.Contains(ag.last, id) {
			ag.last = append(ag.last, id)
		}
		ev.Items = append(ev.Items, it)
	case "session.tool.input.started", "session.tool.input.ended", "session.tool.called", "session.tool.success", "session.tool.failed":
		id := d.AssistantMessageID + ":" + d.ID
		it := ag.tools[id]
		if it.Done {
			break
		}
		it.ID, it.Kind = id, harness.KindTool
		if d.Name != "" {
			it.Tool = d.Name
			if tool, ok := strings.CutPrefix(d.Name, "hypercube_"); ok {
				it.Server, it.Tool = "hypercube", tool
			}
		}
		if e.Type == "session.tool.input.ended" && json.Valid([]byte(d.Text)) {
			it.Input = json.RawMessage(d.Text)
		}
		if len(d.Input) > 0 {
			it.Input = d.Input
		}
		if e.Type == "session.tool.success" || e.Type == "session.tool.failed" {
			it.Done, it.Failed = true, e.Type == "session.tool.failed"
			it.Output, it.Images = toolOutput(d.Content)
			if it.Output == "" && it.Failed {
				it.Output = d.Error.Message
			}
			ag.tools[id] = harness.Item{Done: true}
		} else {
			ag.tools[id] = it
		}
		ag.last = nil
		ev.Items = append(ev.Items, it)
	case "session.usage.updated":
		ag.cost = d.Cost
		ev.Usage = &harness.Usage{Context: ag.used, Window: ag.window, Cost: ag.cost}
	}
	if ev.State != "" || len(ev.Items) > 0 || ev.Usage != nil {
		ag.emit(ev)
	}
	return nil
}

// ended is driven by native execution completion, never by input responses.
// A correction can therefore be admitted without inventing an idle transition.
func (ag *agent) ended(e nativeEvent) harness.Event {
	ev := harness.Event{State: harness.Idle, InputPending: len(ag.pending) > 0}
	for id, it := range ag.open {
		it.Append, it.Done = true, true
		ev.Items = append(ev.Items, it)
		delete(ag.open, id)
	}
	// An answer already being sent finishes its own item after the HTTP
	// response, which can arrive after the native execution has ended.
	items, _ := ag.requests.Close(func(r *request) bool { return !r.replying })
	ev.Items = append(ev.Items, items...)
	if e.Type == "session.execution.failed" {
		ev.Items = append(ev.Items, ag.errors.Item(e.Data.Error.Message))
	}
	if e.Type == "session.execution.succeeded" {
		switch ag.finish {
		case "length":
			ev.Items = append(ev.Items, ag.errors.Item("The model stopped at its output limit."))
		case "content-filter":
			ev.Items = append(ev.Items, ag.errors.Item("The model refused the request."))
		default:
			ev.Final = slices.Clone(ag.last)
		}
	}
	return ev
}

func toolOutput(content []toolContent) (string, []harness.Image) {
	var text []string
	var images []harness.Image
	for _, c := range content {
		if c.Type == "text" {
			text = append(text, c.Text)
		}
		if c.Type == "file" && strings.HasPrefix(c.MIME, "image/") {
			if data, ok := strings.CutPrefix(c.URI, "data:"+c.MIME+";base64,"); ok {
				images = append(images, harness.Image{MIMEType: c.MIME, Data: data})
			}
		}
	}
	return strings.Join(text, "\n"), images
}
