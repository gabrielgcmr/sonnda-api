-- internal/infrastructure/database/postgres/sqlc/sql/schema/patientaccess.sql
-- PatientAccess: N:N relationship linking app users to patients with specific relation types.
-- Mirrors the domain model: patient_id, grantee_id, relation_type, created_at, revoked_at, granted_by.
CREATE TABLE patient_access (
    patient_id UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    grantee_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    relation_type TEXT NOT NULL CHECK (relation_type IN ('caregiver', 'family', 'professional', 'self')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    revoked_at TIMESTAMP WITH TIME ZONE,
    granted_by UUID REFERENCES accounts(id),
    PRIMARY KEY (patient_id, grantee_id)
);

-- Indexes for common lookups
CREATE INDEX idx_patient_access_grantee ON patient_access(grantee_id);
CREATE INDEX idx_patient_access_patient ON patient_access(patient_id);
CREATE INDEX idx_patient_access_active ON patient_access(grantee_id, patient_id) WHERE revoked_at IS NULL;
