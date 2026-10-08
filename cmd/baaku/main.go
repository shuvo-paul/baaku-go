package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/database"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/handler"
	"github.com/shuvo-paul/baaku/internal/logger"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/repository/confirm"
	"github.com/shuvo-paul/baaku/internal/repository/session"
	"github.com/shuvo-paul/baaku/internal/repository/twofactor"
	"github.com/shuvo-paul/baaku/internal/repository/user"
	"github.com/shuvo-paul/baaku/internal/service/login"
	tfasvc "github.com/shuvo-paul/baaku/internal/service/twofactor"
	"github.com/shuvo-paul/baaku/internal/service/twofactorchallenge"
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
	key, err := cfg.App.KeyBytes()
	if err != nil {
		l.Fatal().Err(err).Msg("baaku: APP_KEY (base64: 32 bytes) required — CSRF and 2FA encrypt with it")
	}

	ctx := context.Background()
	pool, err := database.OpenPool(ctx, cfg.Postgres.DSN())
	if err != nil {
		l.Fatal().Err(err).Msg("baaku: open pool")
	}
	defer pool.Close()
	q := generated.New(pool)

	users := user.NewRepo(q, pool)
	sessStore := session.NewStore(q)
	twofaRepo := twofactor.NewRepo(users, q)
	confirmRepo := confirm.NewRepo(q, time.Duration(cfg.Auth.PasswordTimeout)*time.Second)
	twofa := tfasvc.NewService(twofaRepo, twofaRepo, confirmRepo, key, cfg.App.Name)

	authSvc := login.New(users, sessStore, users)
	challengeSvc := twofactorchallenge.New(sessStore, twofa)
	lh := handler.NewLogin(authSvc, challengeSvc, cfg.Session, cfg.App.Name)

	r := chi.NewRouter()
	r.Use(middleware.Session(cfg.Session.Cookie, sessStore))
	r.Use(middleware.CSRF(key))

	r.Get("/login", lh.ShowLogin)
	r.With(middleware.LoginThrottle(5, time.Minute)).Post("/login", lh.Login)
	r.Post("/logout", lh.Logout)
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
