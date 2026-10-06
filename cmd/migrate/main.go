// Command migrate is a standalone migration CLI backed by goose and the SQL
// files embedded in internal/database.
//
// Usage: migrate [up|down|status|version]
package main

import (
	"fmt"
	"os"

	"github.com/pressly/goose/v3"
	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "up", "down", "status", "version":
	default:
		return fmt.Errorf("usage: migrate [up|down|status|version]")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.OpenSQL(cfg.Postgres.DSN())
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(database.FS)
	goose.SetDialect("postgres")
	// The embedded FS has the .sql files at its root (fs.go lives in
	// internal/database/), so the goose dir is ".".
	return goose.Run(cmd, db, ".")
}
