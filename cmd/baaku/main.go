package main

import (
	"embed"
	"io/fs"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/logger"
)

// Built assets (make assets). all: keeps .gitkeep so go build works pre-build.
//
//go:embed all:static
var staticFS embed.FS

func main() {
	l := logger.New()

	cfg, err := config.Load()
	if err != nil {
		l.Fatal().Err(err).Msg("baaku: load config")
	}
	_ = cfg // DB pool and router wire up here once routes exist.

	r := chi.NewRouter()
	// routes go here

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		l.Fatal().Err(err).Msg("baaku: static fs")
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(static))))

	addr := ":" + port()
	l.Info().Str("addr", addr).Msg("baaku: listening")
	if err := http.ListenAndServe(addr, r); err != nil {
		l.Fatal().Err(err).Msg("baaku: serve")
	}
}

func port() string {
	if v := os.Getenv("APP_PORT"); v != "" {
		return v
	}
	return "8000"
}
