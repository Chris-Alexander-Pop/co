package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Chris-Alexander-Pop/co/internal/httpapi"
	"github.com/Chris-Alexander-Pop/co/internal/queue"
)

func main() {
	token := os.Getenv("CO_TOKEN")
	if token == "" {
		log.Fatal("CO_TOKEN is required")
	}
	root := os.Getenv("CO_ROOT")
	if root == "" {
		root = "/var/lib/co"
	}
	addr := os.Getenv("CO_ADDR")
	if addr == "" {
		addr = ":8787"
	}
	q, err := queue.New(root, nil)
	if err != nil {
		log.Fatal(err)
	}
	srv := &httpapi.Server{Q: q, Token: token}
	log.Printf("co-queue listening on %s (%d cpus)", addr, q.Host().CPUs)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
