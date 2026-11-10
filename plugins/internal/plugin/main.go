package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

// discoveryTimeout bounds a native harness's answer to `models`.
const discoveryTimeout = 60 * time.Second

// Plugin is what a plugin's commands run. Product is the harness's name and
// Executable the native command it runs.
type Plugin struct {
	Product    string
	Executable string
	Serve      func(io.Reader, io.Writer) error
	Models     func(context.Context, harness.ModelsRequest) (harness.Catalog, error)
	// Generate answers one prompt. A plugin that cannot leaves it nil.
	Generate func(context.Context, harness.GenerateRequest) (string, error)
}

// Main runs the command named by the plugin's arguments and exits.
func (p Plugin) Main() {
	name := "hypercube-harness"
	if len(os.Args) > 0 {
		name = os.Args[0]
	}
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s serve | describe | models | generate\n", name)
		os.Exit(1)
	}
	if err := p.run(os.Args[1], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		os.Exit(1)
	}
}

func (p Plugin) run(command string, in io.Reader, out io.Writer) error {
	switch command {
	case "serve":
		return p.Serve(in, out)
	case "describe":
		_, err := exec.LookPath(p.Executable)
		return json.NewEncoder(out).Encode(harness.Description{Name: p.Product, Installed: err == nil})
	case "models":
		var req harness.ModelsRequest
		if err := json.NewDecoder(in).Decode(&req); err != nil {
			return fmt.Errorf("malformed models request: %w", err)
		}
		if _, err := exec.LookPath(p.Executable); err != nil {
			return fmt.Errorf("%s is not installed: install %s", p.Product, p.Executable)
		}
		// Hypercube cancels discovery with SIGTERM; the harness's cleanup,
		// such as deleting a temporary native session, still runs.
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
		defer cancel()
		c, err := p.Models(ctx, req)
		if err != nil {
			return err
		}
		c.Model = req.Model
		return json.NewEncoder(out).Encode(c)
	case "generate":
		var req harness.GenerateRequest
		if err := json.NewDecoder(in).Decode(&req); err != nil {
			return fmt.Errorf("malformed generate request: %w", err)
		}
		if p.Generate == nil {
			return fmt.Errorf("%s does not run utility prompts", p.Product)
		}
		if _, err := exec.LookPath(p.Executable); err != nil {
			return fmt.Errorf("%s is not installed: install %s", p.Product, p.Executable)
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		text, err := p.Generate(ctx, req)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(harness.Generation{Text: text})
	}
	return fmt.Errorf("unknown command %q", command)
}
