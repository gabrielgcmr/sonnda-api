-- internal/infrastructure/database/postgres/sqlc/sql/queries/problem_queries.sql
-- name: GetPatientProblem :one
SELECT * FROM patient_problems
WHERE patient_id = sqlc.arg(patient_id) AND id = sqlc.arg(id);

-- name: ListPatientProblems :many
SELECT * FROM patient_problems
WHERE patient_id = sqlc.arg(patient_id)
  AND (sqlc.arg(clinical_filter)::text = 'all' OR clinical_status = sqlc.arg(clinical_filter)::text)
  AND (sqlc.arg(administrative_filter)::text = 'all' OR administrative_status = sqlc.arg(administrative_filter)::text)
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: ListPatientProblemHistory :many
SELECT * FROM patient_problem_history
WHERE patient_id = sqlc.arg(patient_id) AND problem_id = sqlc.arg(problem_id)
ORDER BY version DESC
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: CreatePatientProblem :exec
INSERT INTO patient_problems (
    id, patient_id, name, cid11_code, cid11_system, cid11_version,
    classification, clinical_status, administrative_status, merged_into_id,
    created_by_account_id, created_at, updated_at, version
) VALUES (
    sqlc.arg(id), sqlc.arg(patient_id), sqlc.arg(name), sqlc.narg(cid11_code),
    sqlc.narg(cid11_system), sqlc.narg(cid11_version), sqlc.arg(classification),
    sqlc.arg(clinical_status), sqlc.arg(administrative_status), sqlc.narg(merged_into_id),
    sqlc.arg(created_by_account_id), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(version)
);

-- name: UpdatePatientProblemVersion :execrows
UPDATE patient_problems
SET name = sqlc.arg(name),
    cid11_code = sqlc.narg(cid11_code),
    cid11_system = sqlc.narg(cid11_system),
    cid11_version = sqlc.narg(cid11_version),
    classification = sqlc.arg(classification),
    clinical_status = sqlc.arg(clinical_status),
    administrative_status = sqlc.arg(administrative_status),
    merged_into_id = sqlc.narg(merged_into_id),
    updated_at = sqlc.arg(updated_at),
    version = sqlc.arg(version)
WHERE id = sqlc.arg(id)
  AND patient_id = sqlc.arg(patient_id)
  AND version = sqlc.arg(expected_version);

-- name: CreatePatientProblemHistory :exec
INSERT INTO patient_problem_history (
    id, problem_id, patient_id, version, action, actor_account_id,
    occurred_at, before_snapshot, after_snapshot, reason, source_problem_ids
) VALUES (
    sqlc.arg(id), sqlc.arg(problem_id), sqlc.arg(patient_id), sqlc.arg(version),
    sqlc.arg(action), sqlc.arg(actor_account_id), sqlc.arg(occurred_at),
    sqlc.narg(before_snapshot)::text::jsonb, sqlc.arg(after_snapshot)::text::jsonb, sqlc.narg(reason),
    sqlc.arg(source_problem_ids)::text::uuid[]
);
