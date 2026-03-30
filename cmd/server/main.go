package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/wricardo/ai-http-bin/internal/server"
	"golang.ngrok.com/ngrok"
	ngrokconfig "golang.ngrok.com/ngrok/config"
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if os.Getenv("NGROK_ENABLED") == "true" {
		runNgrok(ctx)
	} else {
		runLocal(ctx)
	}
}

func runLocal(ctx context.Context) {
	listenAddr := envOrDefault("PORT", "0")
	if listenAddr != "0" {
		listenAddr = ":" + listenAddr
	} else {
		listenAddr = ":0"
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	baseURL := fmt.Sprintf("http://localhost:%d", port)

	log.Printf("AI HTTP Bin running on :%d", port)
	log.Printf("API spec:         %s/", baseURL)
	log.Printf("REST API:         %s/api/tokens", baseURL)
	log.Printf("GraphQL:          %s/playground", baseURL)
	log.Printf("Webhook receiver: %s/<token-id>", baseURL)

	srv := server.New(baseURL)
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("serve error: %v", err)
		}
	}()

	<-ctx.Done()
	shutdown(srv)
}

func runNgrok(ctx context.Context) {
	token := envOrDefault("NGROK_TOKEN", "")
	if token == "" {
		log.Fatal("NGROK_TOKEN env var is required when NGROK_ENABLED=true")
	}
	domain := envOrDefault("NGROK_DOMAIN", "ai-http-bin.ngrok.app")
	baseURL := "https://" + domain

	listener, err := ngrok.Listen(ctx,
		ngrokconfig.HTTPEndpoint(ngrokconfig.WithDomain(domain)),
		ngrok.WithAuthtoken(token),
	)
	if err != nil {
		log.Fatalf("ngrok listen: %v", err)
	}

	log.Printf("AI HTTP Bin exposed at %s", baseURL)
	log.Printf("API spec:         %s/", baseURL)
	log.Printf("REST API:         %s/api/tokens", baseURL)
	log.Printf("GraphQL:          %s/playground", baseURL)
	log.Printf("Webhook receiver: %s/<token-id>", baseURL)

	srv := server.New(baseURL)
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("serve error: %v", err)
		}
	}()

	<-ctx.Done()
	shutdown(srv)
}

func shutdown(srv *http.Server) {
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
