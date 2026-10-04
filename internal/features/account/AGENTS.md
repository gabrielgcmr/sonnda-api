<!-- internal/features/account/AGENTS.md -->
# Account

- Keep profile services, onboarding and application DTOs in this package. It must not import Gin or the HTTP layer.
- Keep HTTP binding, request identity and response presentation in `http/`.
- Return known failures through `internal/kernel/apperr`; Huma handlers call `humaerror.From(err)` directly and Huma middleware uses `humaerror.Write(api, ctx, err)`.
- Preserve exactly one access log per request. Do not log 4xx details in handlers; the shared Huma error adapter handles detailed 5xx logging with the original cause.
- Compose dependencies in `internal/application/bootstrap/account.go`.
- This is migration stage 2 for persistence: the repository interface and user persistence errors live in `repository.go`; the concrete SQLC/pgx adapter lives in `postgres/`.
- Application code depends on `Repository`, never on the Postgres adapter or the legacy concrete repositories. Generic persistence failures use `internal/kernel/persistence.ErrPersistenceFailure`.
- Keep `User`, `AccountType`, their validation rules and tests in `domain/` (package `accountdomain`), without an extra `entity/` directory.
- The domain package must not import account application services, HTTP, Gin, `apperr` or infrastructure. Other contexts may import it directly for account models.
- Patient access services, HTTP handlers, interfaces and adapters live in `internal/features/patient/access`; account must not depend on them.
- Onboarding registers accounts without a separate professional profile or professional service.
- Account types are account data, not permissions. Patient access is limited to ownership or active grants checked by `internal/features/patient/access`.
- Account endpoints are unversioned and registered through the application's single Huma API: `POST`, `GET`, `PUT` and `DELETE /me`, plus `POST /me/professional-activation`. The `/users` creation alias does not exist. The `/me/patients` route belongs to `patient/access`.
