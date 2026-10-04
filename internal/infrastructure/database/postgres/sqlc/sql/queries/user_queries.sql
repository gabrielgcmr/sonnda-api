-- name: CreateUser :exec
INSERT INTO
  users (
    id,
    auth_issuer,
    auth_subject,
    email,
    full_name,
    birth_date,
    cpf,
    phone,
    account_type,
    created_at,
    updated_at
  )
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: FindUserByAuthIdentity :one
SELECT
  *
FROM
  users
WHERE
  auth_issuer = $1
  AND auth_subject = $2
  AND deleted_at IS NULL;

-- name: FindUserByEmail :one
SELECT
  *
FROM
  users
WHERE
  email = $1
  AND deleted_at IS NULL
LIMIT
  1;

-- name: FindUserByID :one
SELECT
  *
FROM
  users
WHERE
  id = $1
  AND deleted_at IS NULL
LIMIT
  1;

-- name: FindUserByCPF :one
SELECT
  *
FROM
  users
WHERE
  cpf = $1
  AND deleted_at IS NULL
LIMIT
  1;

-- name: SoftDeleteUser :execrows
UPDATE
  users
SET
  deleted_at = now(),
  updated_at = now()
WHERE
  id = $1
  AND deleted_at IS NULL;

-- name: DeleteUser :execrows
DELETE FROM
  users
WHERE
  id = $1;

-- name: UpdateUser :one
UPDATE
  users
SET
  email = $2,
  full_name = $3,
  birth_date = $4,
  cpf = $5,
  phone = $6,
  updated_at = $7
WHERE
  id = $1
  AND deleted_at IS NULL RETURNING *;

-- name: ActivateUserAsProfessional :one
UPDATE
  users
SET
  account_type = 'professional',
  updated_at = now()
WHERE
  id = $1
  AND deleted_at IS NULL RETURNING *;
