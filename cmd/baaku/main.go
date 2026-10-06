package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/shuvo-paul/baaku/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("baaku: load config: %v", err)
	}
	_ = cfg // DB pool and router wire up here once routes exist.

	r := chi.NewRouter()
	// routes go here

	addr := ":" + port()
	log.Printf("baaku: listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("baaku: %v", err)
	}
}

func port() string {
	if v := os.Getenv("APP_PORT"); v != "" {
		return v
	}
	return "8000"
}
