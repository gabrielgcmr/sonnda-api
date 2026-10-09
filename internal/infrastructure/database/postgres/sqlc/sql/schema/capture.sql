-- internal/infrastructure/database/postgres/sqlc/sql/schema/capture.sql
CREATE TABLE capture_sessions (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    pairing_code_hash bytea NOT NULL UNIQUE,
    pairing_expires_at timestamptz NOT NULL,
    upload_token_hash bytea UNIQUE,
    upload_token_expires_at timestamptz,
    claimed_at timestamptz,
    desktop_last_seen_at timestamptz NOT NULL,
    mobile_last_seen_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (id, account_id),
    CHECK (octet_length(pairing_code_hash) = 32),
    CHECK (upload_token_hash IS NULL OR octet_length(upload_token_hash) = 32),
    CHECK (pairing_expires_at > created_at AND pairing_expires_at <= created_at + interval '5 minutes'),
    CHECK (
        (claimed_at IS NULL AND upload_token_hash IS NULL AND upload_token_expires_at IS NULL)
        OR
        (claimed_at IS NOT NULL AND upload_token_hash IS NOT NULL
            AND upload_token_expires_at IS NOT NULL
            AND claimed_at >= created_at
            AND upload_token_expires_at > claimed_at
            AND upload_token_expires_at <= claimed_at + interval '12 hours')
    ),
    CHECK (
        desktop_last_seen_at >= created_at
        AND (mobile_last_seen_at IS NULL OR (claimed_at IS NOT NULL AND mobile_last_seen_at >= claimed_at))
    ),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at),
    CHECK (updated_at >= created_at)
);

CREATE UNIQUE INDEX capture_sessions_one_unrevoked_per_account_idx
    ON capture_sessions(account_id) WHERE revoked_at IS NULL;
CREATE INDEX capture_sessions_account_created_at_idx
    ON capture_sessions(account_id, created_at DESC);
CREATE INDEX capture_sessions_cleanup_idx
    ON capture_sessions(pairing_expires_at, upload_token_expires_at, revoked_at);

CREATE TABLE captures (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    capture_session_id uuid NOT NULL,
    storage_uri text,
    original_filename text NOT NULL,
    mime_type text NOT NULL,
    size_bytes bigint NOT NULL,
    status text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    FOREIGN KEY (capture_session_id, account_id)
        REFERENCES capture_sessions(id, account_id) ON DELETE RESTRICT,
    CHECK (btrim(original_filename) <> ''),
    CHECK (mime_type IN ('application/pdf', 'image/jpeg', 'image/png')),
    CHECK (size_bytes > 0 AND size_bytes <= 5242880),
    CHECK (status IN ('uploading', 'available', 'deleting')),
    CHECK (status <> 'available' OR (storage_uri IS NOT NULL AND btrim(storage_uri) <> '')),
    CHECK (expires_at = created_at + interval '24 hours'),
    CHECK (updated_at >= created_at)
);

CREATE INDEX captures_account_created_at_idx ON captures(account_id, created_at DESC);
CREATE INDEX captures_session_idx ON captures(capture_session_id);
CREATE INDEX captures_cleanup_idx ON captures(status, expires_at, updated_at);
