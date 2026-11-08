// Command hypercube-mcp is the stdio MCP server each agent's harness runs. It
// forwards requests to the Hypercube backend at HYPERCUBE_URL for the session
// in HYPERCUBE_SESSION_ID.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kkestell/hypercube/internal/mcpbridge"
)

func main() {
	err := errors.New("usage: hypercube-mcp")
	if len(os.Args) == 1 {
		err = mcpbridge.Main(os.Stdin, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-mcp:", err)
		os.Exit(1)
	}
}
