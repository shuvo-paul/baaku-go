// Package database embeds the goose SQL migration files so the migrate
// CLI runs from a single binary with no external files at runtime.
package database

import "embed"

//go:embed *.sql
var FS embed.FS
