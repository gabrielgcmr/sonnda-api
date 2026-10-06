-- internal/infrastructure/database/postgres/sqlc/sql/schema/exam.sql
-- internal/infrastructure/persistence/postgres/sqlc/sql/schema/exam.sql
CREATE TABLE exam_documents (
    id                  UUID PRIMARY KEY,
    patient_id          UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    uploaded_by_user_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    review_status TEXT CHECK (review_status IN ('pending', 'confirmed', 'deleting')),
    lab_report_id UUID,
    confirmed_by_user_id UUID REFERENCES accounts(id),
    confirmed_at TIMESTAMPTZ,
    storage_uri         TEXT NOT NULL,
    original_filename   TEXT NOT NULL,
    mime_type           TEXT NOT NULL,
    status              TEXT NOT NULL,
    exam_type           TEXT,
    extraction_method   TEXT,
    confidence          DOUBLE PRECISION,
    extracted_text      TEXT,
    error_message       TEXT,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),

    CONSTRAINT chk_exam_documents_status CHECK (
        status IN ('uploaded', 'processing', 'processed', 'failed', 'needs_review')
    ),
    CONSTRAINT chk_exam_documents_exam_type CHECK (
        exam_type IS NULL OR exam_type IN ('laboratory', 'imaging', 'unknown')
    ),
    CONSTRAINT chk_exam_documents_confidence CHECK (
        confidence IS NULL OR (confidence >= 0 AND confidence <= 1)
    )
);

CREATE INDEX idx_exam_documents_patient ON exam_documents(patient_id);
CREATE INDEX idx_exam_documents_patient_created_at ON exam_documents(patient_id, created_at DESC);
CREATE INDEX idx_exam_documents_status ON exam_documents(status);
CREATE INDEX idx_exam_documents_exam_type ON exam_documents(exam_type);

CREATE TABLE exam_document_texts (
    id                  UUID PRIMARY KEY,
    exam_document_id    UUID UNIQUE REFERENCES exam_documents(id) ON DELETE SET NULL,
    patient_id          UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    uploaded_by_user_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    category            TEXT NOT NULL,
    title               TEXT,
    modality            TEXT,
    body_site           TEXT,
    performed_at        TIMESTAMP WITH TIME ZONE,
    facility_name       TEXT,
    interpreting_doctor TEXT,
    text                TEXT NOT NULL,
    conclusion          TEXT,
    extraction_method   TEXT,
    confidence          DOUBLE PRECISION,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),

    CONSTRAINT chk_exam_document_texts_category CHECK (
        category IN ('laboratory', 'imaging', 'unknown')
    ),
    CONSTRAINT chk_exam_document_texts_confidence CHECK (
        confidence IS NULL OR (confidence >= 0 AND confidence <= 1)
    )
);

CREATE INDEX idx_exam_document_texts_patient ON exam_document_texts(patient_id);
CREATE INDEX idx_exam_document_texts_patient_created_at ON exam_document_texts(patient_id, created_at DESC);
CREATE INDEX idx_exam_document_texts_category ON exam_document_texts(category);

CREATE TABLE exam_document_extractions (
    document_id UUID PRIMARY KEY REFERENCES exam_documents(id) ON DELETE CASCADE,
    snapshot JSONB NOT NULL
);