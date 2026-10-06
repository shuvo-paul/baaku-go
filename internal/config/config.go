// Package config loads application configuration from the environment,
// optionally seeded from a .env file in the working directory.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Postgres Postgres
	Session  Session
	// TODO: Mail, GoogleDrive sections come later — add fields here.
}

type Postgres struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

// Session mirrors reference/config/session.php (database driver).
type Session struct {
	Cookie   string // SESSION_COOKIE
	Lifetime int    // SESSION_LIFETIME, minutes
	Secure   bool   // SESSION_SECURE_COOKIE
	HttpOnly bool   // SESSION_HTTP_ONLY
	SameSite string // SESSION_SAME_SITE
}

// Load reads .env if present (existing env vars win), then builds Config
// from environment variables matching the Laravel reference app.
func Load() (*Config, error) {
	loadDotEnv(".env")

	cfg := &Config{
		Postgres: Postgres{
			Host:     getenv("DB_HOST", "127.0.0.1"),
			Port:     getenv("DB_PORT", "5432"),
			User:     getenv("DB_USERNAME", "baaku"),
			Password: getenv("DB_PASSWORD", ""),
			DBName:   getenv("DB_DATABASE", "baaku_db"),
		},
		// Laravel derives the cookie name from APP_NAME; baaku has no
		// APP_NAME env yet, so the default is fixed here.
		Session: Session{
			Cookie:   getenv("SESSION_COOKIE", "baaku-session"),
			Lifetime: getenvInt("SESSION_LIFETIME", 120),
			Secure:   getenvBool("SESSION_SECURE_COOKIE", false),
			HttpOnly: getenvBool("SESSION_HTTP_ONLY", true),
			SameSite: getenv("SESSION_SAME_SITE", "lax"),
		},
	}
	return cfg, nil
}

// DSN returns the Postgres connection string.
func (p Postgres) DSN() string {
	// An empty password omits the segment entirely: pgx's connstring parser
	// treats the token after an empty "password=" as its value, silently
	// dropping dbname.
	dsn := fmt.Sprintf("host=%s port=%s user=%s", p.Host, p.Port, p.User)
	if p.Password != "" {
		dsn += " password=" + p.Password
	}
	return dsn + " dbname=" + p.DBName
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := getenv(key, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	if v := getenv(key, ""); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

// loadDotEnv parses a simple KEY=VALUE .env file into the process env without
// overwriting existing variables. No-op if the file does not exist.
// ponytail: no ${VAR} interpolation / export prefix support — add godotenv
// if the .env ever needs either.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}
