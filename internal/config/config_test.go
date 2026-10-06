package config

import (
	"os"
	"path/filepath"
	"testing"
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

	cfg, err := Load()
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

	cfg, err := Load()
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
