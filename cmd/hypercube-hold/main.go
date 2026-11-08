// Command hypercube-hold runs one child process independently of the
// Hypercube backend. It owns the child's input socket, retained output, and
// exit record in a holder directory; internal/hold's Start runs it.
package main

import (
	"fmt"
	"os"

	"github.com/kkestell/hypercube/internal/hold"
)

func main() {
	if err := hold.Main(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-hold:", err)
		os.Exit(1)
	}
}
