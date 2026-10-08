<!-- internal/api/AGENTS.md -->
# AGENTS.md

Simple instructions for coding in api package

## Error handling (MANDATORY)

- Huma routes must return Huma errors (`huma.Error*`, `huma.NewError`, or `huma.WriteErr`) and use the standard Huma Problem Details response.
- Convert application errors with `humaerror.From(err)` in handlers and `humaerror.Write(api, ctx, err)` in middleware; do not add local conversion wrappers.
- Register `humaerror.Transform` before schema transformers. It observes handler errors while keeping internal causes out of responses.
- Existing legacy Gin code may retain `internal/api/presenter.ErrorResponder`. Keep that package for legacy reuse; new Huma code and recovery must not depend on it.
- Do not manually build error JSON.
- Known failures must be returned as `*apperr.AppError` from services/usecases; avoid expose `err.Error()` in responses.

## Composition

- `routes.go` composes feature routes; `health.go` owns the public health endpoint; `openapi.go` configures Huma and exports the specification.
- `APIInfo` contains only the API name and version; environment configuration belongs to application startup.
- Global middleware order is RequestID, AccessLog, Recovery, CORS, then routes, so CORS early responses are observable.

### Access Log

- Exactly one access log entry per request.
- Includes:
  - request_id
  - method, path/route
  - status
  - latency
  - error_code (if present)

### Error Log Policy

- 4xx errors:
  - No detailed error logging in handlers/middleware.
  - AccessLog entry only.
- 5xx errors:
  - AccessLog entry
  - One detailed log with full error chain (Cause) emitted by the HTTP error writer.
- panic
  - Handled by Recovery middleware with one stacktrace log and a safe Huma response; never overwrite an already committed response.
