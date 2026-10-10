<!-- internal/features/patient/profile/AGENTS.md -->
# Patient profile

- Keep patient registration flows, application DTOs, and error mapping in this package.
- Keep HTTP binding, request parsing, and response presentation in `http/`.
- Keep the patient repository interface and patient-specific persistence errors in `repository.go`.
- Keep the concrete pgx/sqlc adapter in `postgres/`; shared database clients and generated sqlc code remain in `internal/infrastructure`.
- Keep patient entities, validation rules, domain errors, and their tests in `domain/` (package `profiledomain`).
- The domain package must not import profile application services, HTTP, Gin, `apperr`, or infrastructure.
- `GET /patients` is intentionally not exposed: the application has no administrative role or authorization policy for a global patient listing. The dashboard listing is the paginated `GET /me/patients` route owned by `patient/access`. Keep `Repository.List` for explicit administrative tooling only; any future HTTP exposure requires dedicated admin authorization, pagination, and audit logging.
- The unversioned `GET /patients/{patientId}` route is registered through Huma and defines its OpenAPI schema in code.
- Keep relationship metadata out of profile DTOs and services. Initial access is coordinated by `internal/application/usecase/patientcreation`.
- Keep profile actions, policy, and authorizer in this feature. The authorizer combines the shared account/access facts with profile-specific permissions and returns the patient loaded during access resolution.
- Return known failures through `internal/kernel/apperr`; Huma handlers call `humaerror.From(err)` directly for standard Problem Details responses and shared observability.
- Compose dependencies in `internal/application/bootstrap/patient.go`.
