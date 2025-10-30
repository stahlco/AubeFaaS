package main

import (
	"aube/pkg/rproxy"
	"log"
	"net/http"
	"os"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("rproxy: ")

	proxy := rproxy.New()

	err := proxy.Add("reverse_echo", "ws://localhost:8001")
	if err != nil {
		log.Printf("adding test function to proxy faile with error: %v", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:    ":8083",
		Handler: proxy,
	}

	log.Printf("started on addr: %s", server.Addr)

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
