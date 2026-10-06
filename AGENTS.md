# baaku-go

Go port of the Laravel app in `reference/` (git submodule — source of truth for
original behavior, schema, and content). This repo is the port; never invent
behavior here that `reference/` doesn't have.

## Architecture

- **Layered Architecture (mandatory):** every directory sits in a layer — HTTP
  handlers (parse/validate/render only) → services (business logic) → repositories
  (thin wrappers over sqlc-generated queries) → `internal/database`. No layer may
  reach across or skip layers: handlers never touch sqlc queries directly,
  services never render. Views (templ components) sit above handlers.
- **Layer names are canonical: `internal/handler/`, `internal/service/`,
  `internal/repository/`.** The directory structure must always follow this
  layout. New code (packages, features, queries) goes into the matching layer
  directory — never ad-hoc locations at `internal/` root or inside `cmd/`.
- Service-Repository pattern: repositories are the only code that calls
  sqlc-generated queries. No inline SQL outside `internal/database/queries/`.

## Tech stack

- Go 1.27, chi (router), pgx/v5 (Postgres), goose (SQL migrations, embedded via `embed.FS`), sqlc (type-safe queries, run via `go tool sqlc`)
- Views: templ components + htmx for reactivity (HTML over the wire, no JS framework)
- Dev: air (live reload), godotenv (`.env`)
- Tests: external test packages — file package ends with `_test` (e.g. `package auth_test`, importing the package under test). Never use the internal package name in `_test.go` files; existing internal-named tests migrate opportunistically.

## Directory structure

Follows Layered Architecture — layer-first naming, top layer first. This is the
canonical layout for all new code; existing packages under `internal/auth/`
(helpers mixed with handlers, `repo.go` next to domain types) migrate here
opportunistically — never write new code in the old layout.

```
internal/handler/    HTTP handlers: parse/validate/render only (auth.go, register.go, …)
internal/service/    business logic + domain types (one subpackage per feature)
   user/             User, UserState, state machine — mirrors reference/app/Models/User.php
   confirm/          email confirmation flow
   passwordreset/    password reset flow
   emailverify/      email verification flow
internal/repository/ thin wrappers over sqlc-generated queries (one subpackage per feature)
   user/
   passwordreset/
internal/middleware/ cross-cutting HTTP middleware (session, CSRF, …)
internal/config/     env/.env config loading
internal/database/   data layer: pgx pool + embedded *.sql migrations + sqlc queries/generated
internal/logger/     shared logging infrastructure
cmd/baaku/           web server entrypoint (router, handler wiring)
cmd/migrate/         goose migration CLI
reference/           Laravel app (git submodule — source of truth)
```
