# baaku-go

Go port of the Laravel app in `reference/` (git submodule — source of truth for
original behavior, schema, and content). This repo is the port; never invent
behavior here that `reference/` doesn't have.

## Tech stack

- Go 1.27, chi (router), pgx/v5 (Postgres), goose (SQL migrations, embedded via `embed.FS`)
- Views: templ components + htmx for reactivity (HTML over the wire, no JS framework)
- Dev: air (live reload), godotenv (`.env`)

## Directory structure

```
cmd/baaku/      web server entrypoint
cmd/migrate/    goose migration CLI
internal/config/   env/.env config loading
internal/database/ pgx pool + embedded *.sql migrations
reference/      Laravel app (git submodule — source of truth)
```
