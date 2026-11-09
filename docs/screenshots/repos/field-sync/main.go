// Command field-sync serves notebook changes to Field Notes devices.
package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":9090", "address to listen on")
	path := flag.String("db", "sync.db", "database file")
	flag.Parse()
	store, err := OpenStore(*path)
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /changes", requireToken(store, http.HandlerFunc(store.ListChanges)))
	mux.Handle("POST /changes", requireToken(store, http.HandlerFunc(store.PushChanges)))
	log.Printf("serving on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
