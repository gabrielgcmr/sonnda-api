-- internal/infrastructure/database/postgres/sqlc/sql/schema/professional.sql
-- Professionals table: stores professional information
-- Reflete a entidade 'Professional' que criamos.
-- Relacionamento 1:1 com users.
CREATE TABLE professionals (
    user_id             UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL CHECK (kind IN ('doctor','nurse','nursing_tech','physiotherapist','psychologist','nutritionist','pharmacist','dentist')),
    registration_number TEXT NOT NULL,
    registration_issuer TEXT NOT NULL, -- Ex: CRM, COREN
    registration_state  TEXT,          -- Opcional (UF)
    status              TEXT NOT NULL CHECK (status IN ('pending', 'verified', 'rejected')),
    verified_at         TIMESTAMP WITH TIME ZONE,
    deleted_at          TIMESTAMP WITH TIME ZONE,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

-- Índices para profissionais
CREATE INDEX idx_professionals_status ON professionals(status);
CREATE INDEX idx_professionals_reg_number ON professionals(registration_number);
CREATE INDEX idx_professionals_deleted_at ON professionals (deleted_at);
