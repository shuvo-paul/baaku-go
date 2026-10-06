package database

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"

	_ "github.com/jackc/pgx/v5/stdlib" // register "pgx" driver for OpenSQL
)

// OpenSQL opens a database/sql pool; required by goose.
func OpenSQL(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn)
}

// OpenPool opens a native pgx pool for the app server.
func OpenPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, dsn)
}
