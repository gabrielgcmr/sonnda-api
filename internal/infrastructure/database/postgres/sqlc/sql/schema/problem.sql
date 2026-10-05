-- internal/infrastructure/database/postgres/sqlc/sql/schema/problem.sql
CREATE TABLE patient_problems (
    id uuid PRIMARY KEY,
    patient_id uuid NOT NULL REFERENCES patients(id) ON DELETE RESTRICT,
    name text NOT NULL,
    cid11_code text,
    cid11_system text,
    cid11_version text,
    classification text NOT NULL,
    clinical_status text NOT NULL,
    administrative_status text NOT NULL,
    merged_into_id uuid,
    created_by_account_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    version bigint NOT NULL,
    CONSTRAINT patient_problems_identity UNIQUE (id, patient_id),
    CONSTRAINT patient_problems_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT patient_problems_cid11_complete CHECK (
        (cid11_code IS NULL AND cid11_system IS NULL AND cid11_version IS NULL)
        OR (
            cid11_code IS NOT NULL AND cid11_system IS NOT NULL AND cid11_version IS NOT NULL
            AND btrim(cid11_code) <> '' AND btrim(cid11_system) <> '' AND btrim(cid11_version) <> ''
        )
    ),
    CONSTRAINT patient_problems_classification CHECK (classification IN ('acute', 'chronic')),
    CONSTRAINT patient_problems_clinical_status CHECK (clinical_status IN ('active', 'resolved')),
    CONSTRAINT patient_problems_administrative_status CHECK (
        administrative_status IN ('valid', 'merged', 'entered_in_error')
    ),
    CONSTRAINT patient_problems_chronic_not_resolved CHECK (
        NOT (classification = 'chronic' AND clinical_status = 'resolved')
    ),
    CONSTRAINT patient_problems_merge_state CHECK (
        (administrative_status = 'merged' AND merged_into_id IS NOT NULL AND merged_into_id <> id)
        OR (administrative_status <> 'merged' AND merged_into_id IS NULL)
    ),
    CONSTRAINT patient_problems_timestamps CHECK (updated_at >= created_at),
    CONSTRAINT patient_problems_version CHECK (version >= 1),
    CONSTRAINT patient_problems_merge_same_patient
        FOREIGN KEY (merged_into_id, patient_id)
        REFERENCES patient_problems(id, patient_id)
        ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE patient_problem_history (
    id uuid PRIMARY KEY,
    problem_id uuid NOT NULL,
    patient_id uuid NOT NULL,
    version bigint NOT NULL,
    action text NOT NULL,
    actor_account_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    occurred_at timestamptz NOT NULL,
    before_snapshot jsonb,
    after_snapshot jsonb NOT NULL,
    reason text,
    source_problem_ids uuid[] NOT NULL DEFAULT '{}',
    CONSTRAINT patient_problem_history_problem
        FOREIGN KEY (problem_id, patient_id)
        REFERENCES patient_problems(id, patient_id)
        ON DELETE RESTRICT,
    CONSTRAINT patient_problem_history_version UNIQUE (problem_id, version),
    CONSTRAINT patient_problem_history_version_positive CHECK (version >= 1),
    CONSTRAINT patient_problem_history_action CHECK (
        action IN (
            'created', 'edited', 'classified', 'resolved', 'reopened',
            'rectified', 'merged_source', 'merged_destination'
        )
    ),
    CONSTRAINT patient_problem_history_creation_shape CHECK (
        (action = 'created' AND version = 1 AND before_snapshot IS NULL)
        OR (action <> 'created' AND version > 1 AND before_snapshot IS NOT NULL)
    ),
    CONSTRAINT patient_problem_history_reason CHECK (
        (action = 'rectified' AND reason IS NOT NULL AND btrim(reason) <> '')
        OR (action <> 'rectified')
    ),
    CONSTRAINT patient_problem_history_merge_sources CHECK (
        (action = 'merged_destination' AND cardinality(source_problem_ids) > 0)
        OR (action <> 'merged_destination' AND cardinality(source_problem_ids) = 0)
    )
);
