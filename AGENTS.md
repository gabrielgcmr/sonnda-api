<!-- AGENTS.md -->
# AGENTS.md

Simple instructions for coding agents working on this repo.

## General

- The project is being done by a solo developer.
- The project was started while the developer was learning Go, REST, and other concepts. Please promptly point out any parts that do not follow best practices.
- Call out any assumptions or open questions before finishing.
- Follow the existing error-handling and logging architecture described below.
- Always start every source file you create or modify with a one-line header comment containing the workspace-relative path to that file, formatted as "path/to/file".
  - Use the language's comment syntax (Go/TS/JS: //, HTML/Markdown: <!-- -->, CSS: /**/).
  - Example: // internal/features/patient/profile/service.go.
  - Skip only when the format does not support comments or the file is auto-generated.

## Stack

- Backend: API REST in Go, using Gin with Huma for route definitions and OpenAPI.
- **Database**: PostgreSQL hosted on Supabase, accessed through pgx and sqlc; Redis is available through the infrastructure adapter.
- **Auth**: Supabase JWT authentication.
- **File storage**: Google Cloud Storage (GCS).
- **Document processing**: command-based text extraction and Gemini structured extraction. A Document AI adapter also exists; check application wiring before treating it as active.

## Architecture

- The codebase is organized primarily by business feature under `internal/features`. The migration is incremental; do not assume every feature has the same internal package layout.
- Feature-specific models, contracts, services, HTTP handlers, and persistence adapters belong with their owning feature where applicable. Current feature areas include `account`, `auth`, `documentprocessing`, and `patient` (`access`, `profile`, and `exam`).
- `documentprocessing` owns document workflows, extraction coordination, snapshots, and the file-storage contract. Its specialized packages include `domain`, `extraction`, `labextraction`, `textextraction`, `http`, and `postgres`.
- `internal/domain` contains shared domain concepts used by multiple features, currently including demographics. Feature-specific domain models live under their feature.
- `internal/application/bootstrap` composes modules and dependencies. `internal/application/usecase` contains cross-feature workflows, including patient creation and laboratory-document confirmation.
- `internal/api` owns Gin/Huma setup, shared middleware and helpers, route composition, and HTTP error translation. Feature handlers remain in their feature packages. Huma route registrations are the OpenAPI source of truth; export `artifacts/openapi.json` locally and use each consumer's Taskfile command to regenerate clients.
- `internal/infrastructure` contains concrete adapters: PostgreSQL and generated sqlc code under `database/postgres`, plus `auth`, `filestorage`, `redis`, `documentai`, `gemini`, and `textextraction`.
- `internal/kernel` contains cross-cutting application errors, persistence errors, and observability. `internal/config` owns environment and integration configuration.
- Patient access checks live in `internal/features/patient/access`; the checker permits the patient owner or an account with an active grant. It does not currently implement action-level, account-type, or professional-kind authorization.

## Error Handling (MANDATORY)

This project uses a **centralized error contract** based on `AppError`.

### Core rules

- **Do NOT return raw strings as error contracts.**
- **Do NOT expose `err.Error()` in HTTP responses.**
- **Do NOT manually build error JSON in handlers or middleware.**

### AppError

- Application-level errors must be represented as `*apperr.AppError`.
- Location: `internal/kernel/apperr`
- `AppError` contains:
  - `Kind` (`ErrorKind`) - stable, machine-readable application code
  - `Message` - safe, human-readable message
  - `Cause` - optional internal error (wrapped with `%w`)
- **Prefer using helper constructors** from `internal/kernel/apperr/factory.go` instead of manually constructing them.
- Services/use cases **must return `AppError` for known failures** (validation, conflicts, not found, infra errors).
- Domain **never** imports HTTP, Gin, or `apperr`.
- Huma handlers translate application failures with `humaerror.From(err)` from `internal/api/humaerror`; call it directly instead of defining local `toHumaError` wrappers.
- Huma middleware writes application failures with `humaerror.Write(api, ctx, err)`. HTTP-only validation errors may use the standard `huma.Error*` helpers.
- Responses use Huma's standard Problem Details model. Keep internal causes and application error codes in diagnostics, not in the public JSON.
- Register `humaerror.Transform` before Huma's schema transformers to observe errors returned by handlers.
- Preserve one access log per request, one detailed log with causes for application 5xx failures, and no detailed 4xx logs. Recovery logs the panic stacktrace once and uses the same Huma response writer.
- Existing legacy Gin code may keep `presenter.ErrorResponder(c, err)`; do not introduce it into Huma handlers or delete the legacy presenter without migrating its consumers.

---

## Logging

- The app uses `log/slog` via `internal/kernel/observability` (request-scoped logger is injected by HTTP middleware).
- Configure with `LOG_LEVEL` (`debug|info|warn|error`) and `LOG_FORMAT` (`text|json|pretty`).

## Cursor Cloud specific instructions
- Use `/usr/local/bin/go` (Go 1.26.9). `go test ./...` is the API check. PDF text extraction needs `pdftotext` from `poppler-utils`.
- PostgreSQL 17 is local. `policy-rc.d` blocks `service postgresql start`; start the cluster with `sudo pg_ctlcluster 17 main start`. The database is `sonnda`, the role is `ubuntu`, and the socket URL is `postgres://ubuntu@/sonnda?host=/var/run/postgresql`.
- Migrations in `supabase/migrations` expect the roles `anon`, `authenticated`, and `service_role`, plus `auth.uid()`. The API reads `.env` (see `.env.example`). `SUPABASE_JWT_ISSUER` must be a reachable OpenID issuer; the hosted project URL is `PUBLIC_SUPABASE_URL` in the web app's `.env.example`. A local service-account file is enough for `GOOGLE_APPLICATION_CREDENTIALS` to boot the API. Object uploads still need a real Cloud Storage bucket.
- `make dev` serves `http://127.0.0.1:8080`. `GET /healthz` is public. Other routes require a Supabase bearer token.
- When `sonnda-svelte` is checked out next to this repo, install with `bun install --frozen-lockfile`, copy `.env.example` to `.env`, and run `bun run dev -- --host 127.0.0.1 --port 5173`. `bun run validate` is the web check.
