// Package harness defines the protocol between Hypercube and its harness
// plugins. A plugin is an executable named hypercube-harness-<name>. Its
// `serve` command keeps a native harness alive across turns and translates
// between the harness and the commands and records in this file; its
// `describe` and `models` commands are described in discovery.go. A plugin
// builds the harness's command line from per-launch arguments, environment,
// and overrides, without changing the user's settings, credentials, or HOME.
package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
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
	CmdConfigure = "configure"
)

// Item kinds. Hypercube writes User, State, and Proposal; plugins send the
// remaining kinds.
const (
	KindUser      = "user"
	KindState     = "state"
	KindText      = "text"
	KindReasoning = "reasoning"
	KindTool      = "tool"
	KindError     = "error"
	KindAttention = "attention"
	KindProposal  = "proposal"
)

// Work states of the agent's main conversation.
const (
	Working = "working"
	Idle    = "idle"
)

// Command is one request to the plugin. The plugin answers each with a Reply
// carrying the same ID. Resent commands whose ID is not greater than the last
// ID read are ignored. IDs increase across launches. Start is set for
// CmdStart, Text and Images for CmdInput, and Request, Choices, and Text for
// CmdAnswer, and Selection for CmdConfigure. An input has text, images, or both.
type Command struct {
	ID      int64      `json:"id"`
	Command string     `json:"command"`
	Start   *StartArgs `json:"start,omitempty"`
	Text    string     `json:"text,omitempty"`
	Images  []Image    `json:"images,omitempty"`
	// Request is the attention item's ID within this plugin launch, which may
	// differ from the native request ID when a request asks several questions.
	Request string `json:"request,omitempty"`
	// Choice is retained for replaying commands saved by older versions.
	Choice    string     `json:"choice,omitempty"`
	Choices   []string   `json:"choices,omitempty"`
	Selection *Selection `json:"selection,omitempty"`
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
	// MCP is the MCP bridge executable the harness runs, with Env, to give
	// the agent its Hypercube tools.
	MCP string `json:"mcp,omitempty"`
	// Hypercube is set instead of MCP in starts saved by earlier versions:
	// the backend executable, which the harness ran with the argument mcp.
	Hypercube string `json:"hypercube,omitempty"`
	// Env is the environment for the MCP bridge.
	Env     map[string]string `json:"env"`
	Session string            `json:"session"`
	Model   string            `json:"model"`
	Effort  string            `json:"effort"`
	// Conversation is the harness's saved conversation to resume, if any.
	Conversation string `json:"conversation"`
	// Fork copies a native conversation on first launch, instead of resuming it.
	Fork string `json:"fork,omitempty"`
	// ForkDir is the source conversation's working directory.
	ForkDir string `json:"fork_dir,omitempty"`
	// Dir is the agent's working directory.
	Dir string `json:"dir"`
	// Instructions tell the model about its team and tools.
	Instructions string `json:"instructions"`
	// StateDir is Hypercube's state directory, where a plugin may keep files.
	StateDir string `json:"state_dir"`
}

// Selection is a model and effort selected within the session's harness.
type Selection struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// Reply answers a command: success, or the reason it failed. A reply to input
// means the native harness accepted the text, including into its own queue.
type Reply struct {
	ID    int64  `json:"id"`
	Error string `json:"error,omitempty"`
	// Selection reports the accepted settings, including after a partial failure.
	// When absent, the selection did not change.
	Selection *Selection `json:"selection,omitempty"`
}

// Record is one line from the plugin. Exactly one field is set.
type Record struct {
	Reply *Reply `json:"reply,omitempty"`
	Event *Event `json:"event,omitempty"`
}

// Event reports what happened in the native conversation. Each field is
// optional.
type Event struct {
	// Conversation is a harness-owned saved reference to resume later.
	Conversation string `json:"conversation,omitempty"`
	// State is the main agent's work state. Native subagents do not change it.
	State string `json:"state,omitempty"`
	// InputPending on Idle means another native prompt is queued to start.
	InputPending bool `json:"input_pending,omitempty"`
	// Final names the main agent's text and image items that are the response
	// to the turn that has just ended. It accompanies State Idle, once the turn
	// succeeded, and is left out when a turn was interrupted, failed, or has no
	// identifiable response.
	Final []string `json:"final,omitempty"`
	Items []Item   `json:"items,omitempty"`
	Usage *Usage   `json:"usage,omitempty"`
	// Limits are the account's use of the harness's usage windows, for the
	// windows the harness reported.
	Limits []Limit `json:"limits,omitempty"`
}

// Usage reports the conversation's estimated context use, model context
// window, and cost in US dollars, when known. Context comes from the latest
// model request or step; whether it includes output tokens depends on the
// harness. See docs/harness-support.md for each harness's accounting.
type Usage struct {
	Context int     `json:"context,omitempty"`
	Window  int     `json:"window,omitempty"`
	Cost    float64 `json:"cost,omitempty"`
}

// Limit is the account's use of one usage window of the harness, such as five
// hours or seven days. Minutes is the window's length, which names it; Used is
// the percent of the limit used; Resets is when the window resets, in Unix
// seconds, or zero when unknown. At is when the plugin saw the harness's
// report, in Unix milliseconds: Hypercube keeps the newest reading of each
// window.
type Limit struct {
	Minutes int   `json:"minutes"`
	Used    int   `json:"used"`
	Resets  int64 `json:"resets,omitempty"`
	At      int64 `json:"at"`
}

// Item is one entry of the transcript. A later item with the same ID replaces
// the earlier one, or extends its Text when Append is set and the earlier item
// is not Done. IDs are unique within one launch of the plugin. When the native
// protocol identifies a subagent's starting tool item, its nested items name
// that item as their Parent. Other output and questions depend on the harness.
//
// An attention item asks the user: Text is the prompt, Choices are the options,
// Notes[i] optionally describes Choices[i], Multi allows several selections,
// and Free allows typed text. It stays open until an event sets Done, with the
// Answer when there is one. The agent waits for the answer.
//
// An approval attention item names the tool it asks to use in Grant, the
// harness's own name for the tool, and its first choice allows the request
// once. A user can allow the tool for good, and Hypercube then answers later
// approvals for that tool with the first choice. The item keeps its Grant when
// it finishes.
//
// An offer is an attention item Hypercube writes in a top-level session's
// transcript: it asks whether a new task gets its own child session, and its
// Text is the task's title. Its Input holds the settings of the child its
// accepted answer started.
//
// An item's Images are shown with its text: the pictures a user sent, or the
// pictures a model or tool produced. An item may have images and no text.
//
// A state item is Hypercube's record of a work state change, in the order the
// transcript shows it: State is the new work state, and an idle one's Final
// names the items that answered the turn it ended.
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
	Multi   bool            `json:"multi,omitempty"`
	Notes   []string        `json:"notes,omitempty"`
	Free    bool            `json:"free,omitempty"`
	Answer  string          `json:"answer,omitempty"`
	From    string          `json:"from,omitempty"`
	Images  []Image         `json:"images,omitempty"`
	State   string          `json:"state,omitempty"`
	Final   []string        `json:"final,omitempty"`
	Grant   string          `json:"grant,omitempty"`
	Offer   bool            `json:"offer,omitempty"`
	// Automatic distinguishes host-generated input from the user's prompts.
	Automatic bool `json:"automatic,omitempty"`
}

// CheckAnswer validates selections received from the UI or plugin commands.
func CheckAnswer(item Item, choices []string, text string) error {
	if len(choices) > 1 && !item.Multi {
		return errors.New("the question allows only one choice")
	}
	for i, choice := range choices {
		if !slices.Contains(item.Choices, choice) {
			return fmt.Errorf("unknown choice %q", choice)
		}
		if slices.Contains(choices[:i], choice) {
			return fmt.Errorf("choice %q was selected more than once", choice)
		}
	}
	if len(choices) == 0 {
		if strings.TrimSpace(text) == "" {
			return errors.New("the answer is empty")
		}
		if !item.Free {
			return errors.New("the question needs one of its choices")
		}
	}
	return nil
}

// Line returns the command as one line of the plugin's input.
func (c Command) Line() ([]byte, error) {
	b, err := json.Marshal(c)
	return append(b, '\n'), err
}

// DecodeRecord parses one line of a plugin's output and checks that it is a
// well-formed reply or event.
func DecodeRecord(line []byte) (Record, error) {
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
	if len(e.Final) > 0 && e.State != Idle {
		return errors.New("final items without the idle state")
	}
	if slices.Contains(e.Final, "") {
		return errors.New("final item without an ID")
	}
	for _, l := range e.Limits {
		if l.Minutes <= 0 || l.Used < 0 || l.At <= 0 {
			return fmt.Errorf("limit %+v needs a positive window, nonnegative use, and an observation time", l)
		}
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
		if it.Grant != "" && (it.Kind != KindAttention || len(it.Choices) == 0) {
			return fmt.Errorf("item %q has a grant but is not an attention item with choices", it.ID)
		}
	}
	return nil
}
