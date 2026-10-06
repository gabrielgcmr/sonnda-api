-- internal/infrastructure/database/postgres/sqlc/sql/queries/professional_queries.sql
-- Profissionais

-- name: CreateProfessional :one
-- Cria apenas a parte "profissional" (O ID vem do User já criado)
INSERT INTO professionals (
  user_id, kind, registration_number, registration_issuer, registration_state, status
) VALUES (
  $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetProfessionalByUserID :one
SELECT * 
FROM professionals
WHERE user_id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: ListProfessionalsByName :many
-- AQUI ESTÁ O TRUQUE: Fazemos JOIN para filtrar, mas retornamos dados do profissional
SELECT p.*
FROM professionals p
JOIN accounts u ON u.id = p.user_id
WHERE u.full_name ILIKE '%' || sqlc.arg(name) || '%'
  AND p.deleted_at IS NULL
LIMIT $1 OFFSET $2;

-- name: GetFullProfessionalDetails :one
-- Query especial para telas de perfil: Retorna TUDO junto
SELECT 
    p.*,
    u.full_name,
    u.phone
FROM professionals p
JOIN accounts u ON u.id = p.user_id
WHERE p.user_id = $1 AND p.deleted_at IS NULL
LIMIT 1;
