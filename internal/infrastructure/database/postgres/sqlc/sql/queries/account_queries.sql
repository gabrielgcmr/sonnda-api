-- internal/infrastructure/database/postgres/sqlc/sql/queries/account_queries.sql
-- name: CreateAccountWithIdentity :exec
WITH created_account AS (
    INSERT INTO accounts (id, account_type, full_name, birth_date, cpf, phone, created_at, updated_at)
    VALUES (sqlc.arg(id), sqlc.arg(account_type), sqlc.narg(full_name), sqlc.narg(birth_date),
            sqlc.narg(cpf), sqlc.narg(phone), sqlc.arg(created_at), sqlc.arg(updated_at))
    RETURNING id
)
INSERT INTO account_identities (account_id, issuer, subject, email, created_at, updated_at)
SELECT id, sqlc.arg(issuer), sqlc.arg(subject), sqlc.narg(email),
       sqlc.arg(identity_created_at), sqlc.arg(identity_updated_at)
FROM created_account;

-- name: CreateAccount :exec
INSERT INTO accounts (id, account_type, created_at, updated_at)
VALUES ($1, 'basic_care', now(), now());

-- name: CreateAccountIdentity :exec
INSERT INTO account_identities (account_id, issuer, subject, email)
VALUES ($1, $2, $3, $4);

-- name: FindAccountByAuthIdentity :one
SELECT a.* FROM accounts a
JOIN account_identities i ON i.account_id = a.id
WHERE i.issuer = $1 AND i.subject = $2;

-- name: FindAccountByID :one
SELECT * FROM accounts WHERE id = $1;

-- name: FindAccountByCPF :one
SELECT * FROM accounts WHERE cpf = $1 AND deleted_at IS NULL;

-- name: FindAccountIdentity :one
SELECT * FROM account_identities WHERE issuer = $1 AND subject = $2;

-- name: ListAccountIdentities :many
SELECT * FROM account_identities WHERE account_id = $1 ORDER BY created_at, issuer, subject;

-- name: UpdateAccountIdentityEmail :execrows
UPDATE account_identities SET email = $3, updated_at = now()
WHERE issuer = $1 AND subject = $2 AND email IS DISTINCT FROM $3;

-- name: UpdateAccountProfile :one
UPDATE accounts
SET full_name = $2, birth_date = $3, cpf = $4, phone = $5, updated_at = $6
WHERE id = $1 AND deleted_at IS NULL RETURNING *;

-- name: SoftDeleteAccount :execrows
UPDATE accounts SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ActivateAccountAsProfessional :one
UPDATE accounts SET account_type = 'professional', updated_at = now()
WHERE id = $1 AND deleted_at IS NULL RETURNING *;
