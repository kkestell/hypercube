// Command hypercube-harness-claude runs Claude Code as a Hypercube agent.
// `serve` keeps one `claude -p` process with stream-JSON input and output
// alive across turns, and translates its stream into protocol events.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

const startTimeout = 60 * time.Second

func main() {
	plugin.Plugin{Product: "Claude Code", Executable: "claude", Serve: serve, Models: models}.Main()
}

func serve(in io.Reader, out io.Writer) error {
	return plugin.Serve(in, out, func(a harness.StartArgs, emit func(harness.Event)) (plugin.Agent, error) {
		c, err := startClaude(a, emit)
		if err != nil {
			return plugin.Agent{}, err
		}
		c.mu.Lock()
		conversation := c.conversation
		c.mu.Unlock()
		return plugin.Agent{
			Process: c.proc, Conversation: conversation,
			Configure: c.configure, Input: c.input, Interrupt: c.interrupt, Answer: c.answer, Stop: c.proc.Stop,
		}, nil
	})
}

// claude is a running `claude -p` process and what its stream has told us.
type claude struct {
	proc  *plugin.Process
	stdin io.Writer
	emit  func(harness.Event)

	writing sync.Mutex // serializes writes to stdin
	counter int        // control request IDs; guarded by writing

	// mu guards the translation state below. Events are emitted with it held,
	// so they keep the order of the stream and of answers.
	mu           sync.Mutex
	models       []nativeModel // what initialize offered; set before start returns
	conversation string
	model        string
	configuring  *configuration
	blocks       map[string]int          // content blocks seen per assistant message
	open         map[string]harness.Item // streamed items that no complete block has finished
	tools        map[string]harness.Item // tool items, by tool use ID
	requests     plugin.Requests[request]
	errors       plugin.Errors
	usage        harness.Usage
	latest       answer   // the main agent's newest message
	final        []string // the text items a successful result confirmed, until idle reports them
}

func startClaude(a harness.StartArgs, emit func(harness.Event)) (*claude, error) {
	command, args, err := plugin.MCPCommand(a)
	if err != nil {
		return nil, err
	}
	mcp := map[string]any{"mcpServers": map[string]any{"hypercube": map[string]any{"command": command, "args": args, "env": a.Env}}}
	mcpJSON, err := json.Marshal(mcp)
	if err != nil {
		return nil, err
	}
	// The prompt snapshot would freeze the instructions of the first launch.
	argv := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--permission-prompt-tool", "stdio", "--permission-mode", "auto", "--system-prompt-snapshot", "off",
		"--mcp-config", string(mcpJSON), "--model", a.Model}
	if a.Effort != "" {
		argv = append(argv, "--effort", a.Effort)
	}
	if a.Instructions != "" {
		argv = append(argv, "--append-system-prompt", a.Instructions)
	}
	if a.Conversation != "" {
		argv = append(argv, "--resume", a.Conversation)
	} else if a.Fork != "" {
		// A stable destination keeps a first launch that has not yet reported
		// its conversation from choosing another ID on restart.
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(a.Session)))
		id := fmt.Sprintf("%s-%s-4%s-8%s-%s", hash[:8], hash[8:12], hash[13:16], hash[17:20], hash[20:32])
		argv = append(argv, "--resume", a.Fork, "--fork-session", "--session-id", id)
	}
	cmd := exec.Command("claude", argv...)
	cmd.Dir = a.Dir
	// Claude reports idle and working states only to a host that asks for them.
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SDK_READS_SESSION_STATE=1")
	p, err := plugin.StartProcess("Claude Code", cmd)
	if err != nil {
		return nil, err
	}
	c := &claude{
		proc: p, stdin: p.Stdin, emit: emit, conversation: a.Conversation, model: a.Model,
		blocks: map[string]int{}, open: map[string]harness.Item{}, tools: map[string]harness.Item{},
	}
	ready := make(chan error, 1)
	go c.read(ready)
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	if err := c.initialize(ctx, ready); err != nil {
		p.Stop()
		return nil, err
	}
	// Claude accepts any model name at launch and fails only on the first prompt.
	if _, err := check(c.models, harness.Selection{Model: a.Model, Effort: a.Effort}); err != nil {
		p.Stop()
		return nil, err
	}
	return c, nil
}

// initialize asks claude to start and waits until read reports the answer.
func (c *claude) initialize(ctx context.Context, ready <-chan error) error {
	// A failed write means claude has exited. read reports why on ready, which
	// can include a result line that a write error does not carry.
	_ = c.send(map[string]any{"type": "control_request", "request_id": "initialize", "request": map[string]any{"subtype": "initialize"}})
	select {
	case err := <-ready:
		return err
	case <-ctx.Done():
		return errors.New("claude did not become ready")
	}
}

// Startup states of claude's output.
const (
	starting = iota // waiting for the response to initialize
	started         // translating every line
	failed          // discarding every line
)

// read translates claude's output once claude has started, and sends on
// ready whether it did. During startup, a result line is the reason a start
// fails, such as a conversation that cannot be resumed, and is not
// translated.
func (c *claude) read(ready chan<- error) {
	state := starting
	var failure error
	err := c.proc.Read(func(line []byte) {
		switch state {
		case started:
			c.line(line)
			return
		case failed:
			return
		}
		var m message
		if json.Unmarshal(line, &m) != nil {
			c.line(line) // reports the unreadable line
			return
		}
		switch {
		case m.Type == "control_response" && m.Response.RequestID == "initialize":
			if m.Response.Subtype == "error" {
				failure = errors.New(m.Response.Error)
			} else if failure == nil {
				c.models, failure = initialized(m.Response.Response)
			}
			state = started
			if failure != nil {
				state = failed
			}
			ready <- failure
		case m.Type == "result":
			failure = errors.New(m.summary())
		default:
			c.line(line)
		}
	})
	if state == starting {
		if failure == nil {
			failure = err
		}
		ready <- failure
	}
}

func (c *claude) send(v any) error {
	c.writing.Lock()
	defer c.writing.Unlock()
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("claude is not running: %w", err)
	}
	return nil
}

// input sends a prompt: plain text, or content blocks when it has images.
func (c *claude) input(text string, images []harness.Image) error {
	var content any = text
	if len(images) > 0 {
		blocks := make([]map[string]any, 0, len(images)+1)
		if text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": text})
		}
		for _, img := range images {
			blocks = append(blocks, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": img.MIMEType, "data": img.Data}})
		}
		content = blocks
	}
	return c.send(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}})
}

func (c *claude) interrupt() error {
	c.writing.Lock()
	c.counter++
	id := fmt.Sprintf("interrupt-%d", c.counter)
	c.writing.Unlock()
	return c.send(map[string]any{"type": "control_request", "request_id": id, "request": map[string]any{"subtype": "interrupt"}})
}
