# baaku-go

Go port of the Laravel app in `reference/` (git submodule — source of truth for
original behavior, schema, and content). This repo is the port; never invent
behavior here that `reference/` doesn't have.

## Architecture

- Service-Repository pattern: HTTP handlers parse/validate/render only; services own
  business logic and state transitions; repositories are thin wrappers over
  sqlc-generated queries. No inline SQL outside `internal/database/queries/`.

## Tech stack

- Go 1.27, chi (router), pgx/v5 (Postgres), goose (SQL migrations, embedded via `embed.FS`), sqlc (type-safe queries, run via `go tool sqlc`)
- Views: templ components + htmx for reactivity (HTML over the wire, no JS framework)
- Dev: air (live reload), godotenv (`.env`)

## Directory structure

```
cmd/baaku/      web server entrypoint
cmd/migrate/    goose migration CLI
internal/config/   env/.env config loading
internal/database/ pgx pool + embedded *.sql migrations + sqlc queries/generated
reference/      Laravel app (git submodule — source of truth)
```
