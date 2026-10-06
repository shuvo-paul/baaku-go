// Package config loads application configuration from the environment,
// optionally seeded from a .env file in the working directory.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Postgres Postgres
	// TODO: Mail, GoogleDrive sections come later — add fields here.
}

type Postgres struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
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
	}
	return cfg, nil
}

// DSN returns the Postgres connection string.
func (p Postgres) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s",
		p.Host, p.Port, p.User, p.Password, p.DBName)
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
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
