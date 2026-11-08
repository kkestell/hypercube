// Command hypercube runs teams of coding agents. `hypercube ui` is the app
// process: it manages sessions and attaches to their holders. `hypercube mcp`
// serves harnesses configured by earlier versions, forwarding to the backend.
// `hypercube hold` serves backends of earlier versions during replacement.
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/kkestell/hypercube/internal/buildinfo"
	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/mcpbridge"
)

const usage = "usage: hypercube [version | ui [port] | mcp | hold [-terminal -cols N -rows N] <dir> <command> [args...]]"

func main() {
	var err error
	switch {
	case len(os.Args) > 1 && os.Args[1] == "hold":
		err = hold.Main(os.Args[2:])
	case len(os.Args) == 2 && os.Args[1] == "version":
		fmt.Println(buildinfo.String())
	case len(os.Args) > 1 && os.Args[1] == "ui":
		err = serveUI(os.Args[2:])
	case len(os.Args) == 2 && os.Args[1] == "mcp":
		err = mcpbridge.Main(os.Stdin, os.Stdout)
	default:
		err = errors.New(usage)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube:", err)
		os.Exit(1)
	}
}

func parsePort(args []string) (string, error) {
	if len(args) == 0 {
		return "4280", nil
	}
	if len(args) > 1 {
		return "", errors.New("ui accepts only one port")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid port %q", args[0])
	}
	return args[0], nil
}
