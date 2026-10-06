package config_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/shuvo-paul/baaku/internal/config"
)

func TestLoadFromDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "# comment\nDB_HOST=db.internal\nDB_PORT=5433\nDB_USERNAME=baaku\nDB_PASSWORD=secret\nDB_DATABASE=baaku\nAUTH_PASSWORD_TIMEOUT=3600\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	old, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_HOST", "real.host") // real env must win over .env

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Host != "real.host" {
		t.Errorf("Host = %q, want real.host (env overrides .env)", cfg.Postgres.Host)
	}
	if cfg.Postgres.Port != "5433" {
		t.Errorf("Port = %q, want 5433", cfg.Postgres.Port)
	}
	if cfg.Postgres.Password != "secret" {
		t.Errorf("Password = %q, want secret", cfg.Postgres.Password)
	}
	if cfg.Auth.PasswordTimeout != 3600 {
		t.Errorf("PasswordTimeout = %d, want 3600", cfg.Auth.PasswordTimeout)
	}
	if cfg.Postgres.DSN() == "" {
		t.Error("DSN() is empty")
	}
}

func TestLoadDefaultsWithoutDotEnv(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	// loadDotEnv persists vars in the process env; clear any DB_* leakage.
	for _, k := range []string{"DB_HOST", "DB_PORT", "DB_USERNAME", "DB_PASSWORD", "DB_DATABASE", "AUTH_PASSWORD_TIMEOUT"} {
		t.Setenv(k, "")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Host != "127.0.0.1" || cfg.Postgres.DBName != "baaku_db" {
		t.Errorf("unexpected defaults: %+v", cfg.Postgres)
	}
	if cfg.Auth.PasswordTimeout != 10800 {
		t.Errorf("PasswordTimeout = %d, want default 10800", cfg.Auth.PasswordTimeout)
	}
}

func TestAppKeyBytes(t *testing.T) {
	raw := bytes.Repeat([]byte{0x7f}, 32)
	app := config.App{Key: "base64:" + base64.StdEncoding.EncodeToString(raw)}
	got, err := app.KeyBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Error("KeyBytes returned wrong key material")
	}

	// No prefix also decodes (plain base64 APP_KEY).
	app.Key = base64.StdEncoding.EncodeToString(raw)
	if _, err := app.KeyBytes(); err != nil {
		t.Errorf("plain base64 APP_KEY rejected: %v", err)
	}

	app.Key = "base64:" + base64.StdEncoding.EncodeToString(raw[:16])
	if _, err := app.KeyBytes(); err == nil {
		t.Error("16-byte APP_KEY accepted, want error")
	}
	if _, err := (config.App{Key: "not-base64!!"}).KeyBytes(); err == nil {
		t.Error("garbage APP_KEY accepted, want error")
	}
}
