package main

import (
	"fmt"
	"log"
	"net"

	"github.com/wricardo/ai-http-bin/internal/server"
)

const port = "8082"

func main() {
	baseURL := fmt.Sprintf("http://localhost:%s", port)

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("AI HTTP Bin running on :%s", port)
	log.Printf("API spec (markdown):  %s/", baseURL)
	log.Printf("REST API:             %s/api/tokens", baseURL)
	log.Printf("GraphQL playground:   %s/playground", baseURL)
	log.Printf("Webhook receiver:     %s/<token-id>", baseURL)

	if err := server.New(baseURL).Serve(ln); err != nil {
		log.Fatal(err)
	}
}
