package harness

import (
	"encoding/json"
	"errors"
	"fmt"
)

// A plugin's `serve` command speaks JSON Lines on stdin and stdout; stderr
// carries diagnostics. Hypercube writes a Command per line. The plugin writes a
// Record per line: a Reply to a command, or an Event. A plugin exits when its
// stdin closes.

// Command names.
const (
	CmdStart     = "start"
	CmdInput     = "input"
	CmdInterrupt = "interrupt"
	CmdAnswer    = "answer"
)

// Item kinds. Plugins send every kind but User and Status, which Hypercube writes for the
// text and images it delivers.
const (
	KindUser      = "user"
	KindStatus    = "status"
	KindText      = "text"
	KindReasoning = "reasoning"
	KindTool      = "tool"
	KindError     = "error"
	KindAttention = "attention"
)

// Work states of the agent's main conversation.
const (
	Working = "working"
	Idle    = "idle"
)

// Command is one request to the plugin. The plugin answers each with a Reply
// carrying the same ID. Resent commands whose ID is not greater than the last
// ID read are ignored. IDs increase across launches. Start is set for
// CmdStart, Text and Images for CmdInput, and Request, Choice, and Text for
// CmdAnswer. An input has text, images, or both.
type Command struct {
	ID      int64      `json:"id"`
	Command string     `json:"command"`
	Start   *StartArgs `json:"start,omitempty"`
	Text    string     `json:"text,omitempty"`
	Images  []Image    `json:"images,omitempty"`
	Request string     `json:"request,omitempty"`
	Choice  string     `json:"choice,omitempty"`
}

// Image is a picture in a prompt or in the transcript: its MIME type and its
// bytes in base64, without a data URL prefix.
type Image struct {
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"`
}

// StartArgs tells the plugin which harness to start. The plugin replies once
// the harness is ready for input.
type StartArgs struct {
	// Hypercube is the executable the harness runs for `mcp`.
	Hypercube string `json:"hypercube"`
	// Env is the environment for Hypercube processes the harness starts.
	Env     map[string]string `json:"env"`
	Session string            `json:"session"`
	Model   string            `json:"model"`
	Effort  string            `json:"effort"`
	// Conversation is the harness's saved conversation to resume, if any.
	Conversation string `json:"conversation"`
	// Dir is the agent's working directory.
	Dir string `json:"dir"`
	// Instructions tell the model about its team and tools.
	Instructions string `json:"instructions"`
	// StateDir is Hypercube's state directory, where a plugin may keep files.
	StateDir string `json:"state_dir"`
}

// Reply answers a command: success, or the reason it failed. A reply to input
// means the native harness accepted the text, including into its own queue.
type Reply struct {
	ID    int64  `json:"id"`
	Error string `json:"error,omitempty"`
}

// Record is one line from the plugin. Exactly one field is set.
type Record struct {
	Reply *Reply `json:"reply,omitempty"`
	Event *Event `json:"event,omitempty"`
}

// Event reports what happened in the native conversation. Each field is
// optional.
type Event struct {
	// Conversation is the native conversation ID to resume later.
	Conversation string `json:"conversation,omitempty"`
	// State is the main agent's work state. Native subagents do not change it.
	State string `json:"state,omitempty"`
	Items []Item `json:"items,omitempty"`
	Usage *Usage `json:"usage,omitempty"`
}

// Usage is what the conversation has used: the tokens of its latest request,
// its model's context window, and its cost in US dollars, when known.
type Usage struct {
	Context int     `json:"context,omitempty"`
	Window  int     `json:"window,omitempty"`
	Cost    float64 `json:"cost,omitempty"`
}

// Item is one entry of the transcript. A later item with the same ID replaces
// the earlier one, or extends its Text when Append is set and the earlier item
// is not Done. IDs are unique within one launch of the plugin. A native subagent's
// items name the item that started it as their Parent.
//
// An attention item asks the user: Text is the prompt, Choices are the options,
// Notes[i] optionally describes Choices[i], and Free allows typed text. It stays open until an event sets Done, with the
// Answer when there is one.
//
// An item's Images are shown with its text: the pictures a user sent, or the
// pictures a model or tool produced. An item may have images and no text.
//
// A tool item from an MCP server names the Server, and Tool is the server's
// own name for the tool. A user item that delivers another agent's message
// names that session's ID in From, and Text is the message alone.
type Item struct {
	// At is when Hypercube first received the item, in Unix milliseconds.
	At      int64           `json:"at,omitempty"`
	ID      string          `json:"id"`
	Parent  string          `json:"parent,omitempty"`
	Kind    string          `json:"kind"`
	Done    bool            `json:"done,omitempty"`
	Append  bool            `json:"append,omitempty"`
	Text    string          `json:"text,omitempty"`
	Server  string          `json:"server,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Output  string          `json:"output,omitempty"`
	Failed  bool            `json:"failed,omitempty"`
	Choices []string        `json:"choices,omitempty"`
	Notes   []string        `json:"notes,omitempty"`
	Free    bool            `json:"free,omitempty"`
	Answer  string          `json:"answer,omitempty"`
	From    string          `json:"from,omitempty"`
	Images  []Image         `json:"images,omitempty"`
}

// Line returns the command as one line of the plugin's input.
func (c Command) Line() ([]byte, error) {
	b, err := json.Marshal(c)
	return append(b, '\n'), err
}

// decode parses one plugin line and checks that it is well formed.
func decode(line []byte) (Record, error) {
	var r Record
	if err := json.Unmarshal(line, &r); err != nil {
		return r, fmt.Errorf("malformed record: %w", err)
	}
	switch {
	case r.Reply != nil && r.Event != nil:
		return r, errors.New("malformed record: both a reply and an event")
	case r.Reply != nil:
		if r.Reply.ID <= 0 {
			return r, errors.New("malformed record: reply without an ID")
		}
	case r.Event != nil:
		if err := r.Event.check(); err != nil {
			return r, fmt.Errorf("malformed record: %w", err)
		}
	default:
		return r, errors.New("malformed record: neither a reply nor an event")
	}
	return r, nil
}

func (e Event) check() error {
	if e.State != "" && e.State != Working && e.State != Idle {
		return fmt.Errorf("unknown state %q", e.State)
	}
	for _, it := range e.Items {
		if it.ID == "" {
			return errors.New("item without an ID")
		}
		switch it.Kind {
		case KindText, KindReasoning, KindTool, KindError, KindAttention:
		default:
			return fmt.Errorf("item %q has unknown kind %q", it.ID, it.Kind)
		}
	}
	return nil
}
