-- internal/infrastructure/database/postgres/sqlc/sql/schema/lab.sql
-- Lab reports: optional extracted metadata, linked to patient and uploader.
CREATE TABLE lab_reports (
    id                 UUID PRIMARY KEY,
    patient_id         UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    uploaded_by_user_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    exam_document_id   UUID REFERENCES exam_documents(id) ON DELETE SET NULL,
    patient_name       TEXT,
    patient_dob        TIMESTAMP WITH TIME ZONE,
    lab_name           TEXT,
    lab_phone          TEXT,
    insurance_provider TEXT,
    requesting_doctor  TEXT,
    technical_manager  TEXT,
    report_date        TIMESTAMP WITH TIME ZONE,
    raw_text           TEXT,
    fingerprint        TEXT,
    created_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

-- Lab panel: one-to-many from lab_reports.
CREATE TABLE lab_panels (
    id            UUID PRIMARY KEY,
    lab_report_id UUID   NOT NULL REFERENCES lab_reports(id) ON DELETE CASCADE,
    test_name     TEXT NOT NULL,
    material      TEXT,
    method        TEXT,
    collected_at  TIMESTAMP WITH TIME ZONE,
    release_at    TIMESTAMP WITH TIME ZONE
);

-- Lab observations: one-to-many from lab_panels.
CREATE TABLE observations (
    id             UUID PRIMARY KEY,
    lab_panel_id   UUID NOT NULL REFERENCES lab_panels(id) ON DELETE CASCADE,
    parameter_name TEXT NOT NULL,
    result_value   TEXT,
    result_unit    TEXT,
    reference_text TEXT
);

-- Useful indexes/uniqueness for lookups and idempotency
CREATE UNIQUE INDEX idx_lab_reports_fingerprint ON lab_reports(fingerprint) WHERE fingerprint IS NOT NULL;
CREATE UNIQUE INDEX idx_lab_reports_exam_document ON lab_reports(exam_document_id) WHERE exam_document_id IS NOT NULL;
CREATE INDEX idx_lab_reports_patient ON lab_reports(patient_id);
CREATE INDEX idx_lab_reports_report_date ON lab_reports(report_date);
CREATE INDEX idx_lab_panels_report ON lab_panels(lab_report_id);
CREATE INDEX idx_observations_panel ON observations(lab_panel_id);