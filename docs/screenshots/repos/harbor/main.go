// Command harbor builds a static site from Markdown pages.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: harbor build | serve [-addr :8080]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "build":
		site := Site{Content: "content", Templates: "templates", Out: "public"}
		n, err := site.Build()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("built %d pages\n", n)
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		addr := fs.String("addr", ":8080", "address to listen on")
		fs.Parse(os.Args[2:])
		log.Printf("serving public/ on %s", *addr)
		log.Fatal(http.ListenAndServe(*addr, http.FileServer(http.Dir("public"))))
	default:
		fmt.Fprintf(os.Stderr, "harbor: unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}
