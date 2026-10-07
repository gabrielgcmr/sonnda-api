-- internal/infrastructure/database/postgres/sqlc/sql/schema/patient.sql
-- Patients table mirrors the domain model, allowing optional User link and phone/CNS.
CREATE TABLE patients (
    id          UUID PRIMARY KEY,
    owner_user_id     UUID UNIQUE,
    cpf         TEXT NOT NULL UNIQUE,
    cns         TEXT,
    full_name   TEXT NOT NULL,
    birth_date  DATE NOT NULL,
    gender      TEXT NOT NULL,
    race        TEXT NOT NULL,
    phone       TEXT,
    avatar_url  TEXT,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMP WITH TIME ZONE,
    CONSTRAINT fk_patients_user FOREIGN KEY (owner_user_id) REFERENCES accounts(id) ON DELETE SET NULL,
    CONSTRAINT chk_patients_gender CHECK (gender IN ('MALE','FEMALE','OTHER','UNKNOWN')),
    CONSTRAINT chk_patients_race CHECK (race IN ('WHITE','BLACK','ASIAN','MIXED','INDIGENOUS','UNKNOWN'))
);


-- Helpful indexes for lookups/searches
CREATE INDEX idx_patients_full_name_trgm
ON patients USING gin (full_name gin_trgm_ops);
CREATE INDEX idx_patients_cpf ON patients(cpf);
CREATE INDEX idx_patients_cns ON patients(cns);
CREATE UNIQUE INDEX IF NOT EXISTS ux_patients_owner_user_id_active
ON patients(owner_user_id)
WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_patients_cpf_active
ON patients(cpf)
WHERE deleted_at IS NULL;

