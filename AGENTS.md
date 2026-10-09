# baaku-go

Go port of the Laravel app in `reference/` (git submodule — source of truth for
original behavior, schema, and content). This repo is the port; never invent
behavior here that `reference/` doesn't have. Do not edit `reference/`

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

Follows Layered Architecture — layer-first naming, top layer first. All code
lives here; never write new code outside this layout.

```
internal/handler/    HTTP handlers: parse/validate/render only (empty until routes land)
internal/service/    business logic + domain types (one subpackage per feature)
   register/         user registration + validation
   login/            login/logout service
   passwordchange/   password change flow
   password/         shared bcrypt hashing + Laravel password rules
   user/             User, UserState, state machine — mirrors reference/app/Models/User.php
   passwordreset/    password reset broker
   emailverify/      signed email-verification URLs + MarkVerified
   session/          session ID + cookie helpers
   twofactor/        TOTP, recovery codes, 2FA service
internal/repository/ thin wrappers over sqlc-generated queries (one subpackage per feature)
   user/             users queries + registration transaction
   passwordreset/    password_reset_tokens
   twofactor/        2FA secret + replay-guard queries
   confirm/          password-confirmation cache
   session/          sessions table store
internal/middleware/ cross-cutting HTTP middleware (session, CSRF, …)
internal/config/     env/.env config loading
internal/database/   data layer: pgx pool + embedded *.sql migrations + sqlc queries/generated
internal/logger/     shared logging infrastructure
internal/mailer/     email rendering + sending
cmd/baaku/           web server entrypoint (router, handler wiring)
cmd/migrate/         goose migration CLI
reference/           Laravel app (git submodule — source of truth)
```

```
